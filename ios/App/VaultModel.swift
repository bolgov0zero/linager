import LinagerKit
import LocalAuthentication
import Observation
import Security
import SwiftUI
import UniformTypeIdentifiers

/// The passphrase of the vault behind Face ID: an item of the Keychain that only the face of this phone opens, and that is gone when a face
/// is added or the passcode is removed. It is optional and off until the person asks for it.
enum VaultPassphraseStore {
    private static let service = "app.linager.ios.vault"
    private static let account = "passphrase"

    private static var base: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
    }

    static var biometryAvailable: Bool {
        var err: NSError?
        return LAContext().canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err)
    }

    @discardableResult
    static func save(_ passphrase: String) -> Bool {
        remove()
        var err: Unmanaged<CFError>?
        guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .biometryCurrentSet, &err) else { return false }
        var item = base
        item[kSecValueData as String] = Data(passphrase.utf8)
        item[kSecAttrAccessControl as String] = access
        return SecItemAdd(item as CFDictionary, nil) == errSecSuccess
    }

    /// Reads the passphrase: the system asks for the face, unless `context` has passed it already. Blocking, so it is called away from the main thread.
    static func read(reason: String, context: LAContext? = nil) -> String? {
        let ctx = context ?? LAContext()
        ctx.localizedReason = reason
        var q = base
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        q[kSecUseAuthenticationContext as String] = ctx
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let d = out as? Data else { return nil }
        return String(data: d, encoding: .utf8)
    }

    /// Whether the item is there (without asking for the face).
    static var exists: Bool {
        let ctx = LAContext()
        ctx.interactionNotAllowed = true
        var q = base
        q[kSecUseAuthenticationContext as String] = ctx
        let status = SecItemCopyMatching(q as CFDictionary, nil)
        return status == errSecSuccess || status == errSecInteractionNotAllowed
    }

    static func remove() { SecItemDelete(base as CFDictionary) }
}

/// A Face ID context that goes to a background task (it is only handed on, never used from two places at once).
struct ContextBox: @unchecked Sendable {
    let context: LAContext?
    init(_ c: LAContext?) { context = c }
}

/// The vault of the keys on this phone: where the file is, whether it is open, what is in it. The file is never copied: its place is
/// remembered (a bookmark) and it is read again at every opening, so what is changed on the computer is what the phone sees.
@MainActor
@Observable
final class VaultModel {
    enum State: Equatable { case noFile, locked, unlocking, unlocked }

    private static let bookmarkKey = "vault.bookmark"
    private static let nameKey = "vault.fileName"
    private static let autolockKey = "vault.autolock"

    private(set) var state: State
    private(set) var vault: Vault?
    private(set) var fileName: String?
    /// Why the last opening failed (in words of the screen).
    private(set) var problem: String?
    private(set) var faceIDOn: Bool

    /// Minutes in the background after which the vault locks itself: 0 at once on leaving, -1 never (it is forgotten when the application closes).
    var autolock: Int {
        didSet { UserDefaults.standard.set(autolock, forKey: Self.autolockKey) }
    }

    @ObservationIgnored private var leftAt: Date?
    /// The application itself is locked with the face: the person is known, so the keys are not locked apart (they open by themselves and stay open).
    var appLockOn = false {
        didSet { if appLockOn { leftAt = nil } }
    }

    init() {
        let d = UserDefaults.standard
        fileName = d.string(forKey: Self.nameKey)
        state = d.data(forKey: Self.bookmarkKey) == nil ? .noFile : .locked
        autolock = d.object(forKey: Self.autolockKey) as? Int ?? 5
        faceIDOn = VaultPassphraseStore.exists
    }

    var faceIDAvailable: Bool { VaultPassphraseStore.biometryAvailable }

    // MARK: the file

    /// The person picked a file in Files: its place is remembered.
    func chooseFile(_ url: URL) {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        do {
            let bookmark = try url.bookmarkData(options: .minimalBookmark, includingResourceValuesForKeys: nil, relativeTo: nil)
            UserDefaults.standard.set(bookmark, forKey: Self.bookmarkKey)
            UserDefaults.standard.set(url.lastPathComponent, forKey: Self.nameKey)
            // another file is another passphrase
            VaultPassphraseStore.remove()
            faceIDOn = false
            fileName = url.lastPathComponent
            vault = nil
            problem = nil
            state = .locked
        } catch {
            problem = L("keys.err.read")
        }
    }

    /// Forgets the file, the passphrase behind Face ID and what was read.
    func forgetFile() {
        lock()
        UserDefaults.standard.removeObject(forKey: Self.bookmarkKey)
        UserDefaults.standard.removeObject(forKey: Self.nameKey)
        VaultPassphraseStore.remove()
        faceIDOn = false
        fileName = nil
        problem = nil
        state = .noFile
    }

    nonisolated private static func readFile(bookmark: Data) throws -> (data: Data, fresh: Data?) {
        var stale = false
        let url = try URL(resolvingBookmarkData: bookmark, options: [], relativeTo: nil, bookmarkDataIsStale: &stale)
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        let fresh = stale ? try? url.bookmarkData(options: .minimalBookmark, includingResourceValuesForKeys: nil, relativeTo: nil) : nil
        // a coordinated read: a file of iCloud Drive that is not on the phone yet is fetched first
        var result: Result<Data, Error> = .failure(CocoaError(.fileReadUnknown))
        var coordination: NSError?
        NSFileCoordinator().coordinate(readingItemAt: url, options: [], error: &coordination) { readable in
            result = Result { try Data(contentsOf: readable) }
        }
        if let coordination { throw coordination }
        return (try result.get(), fresh)
    }

    // MARK: opening

    /// Opens the file with the passphrase. With `remember` the passphrase is kept behind Face ID after it was right.
    func unlock(passphrase: String, remember: Bool = false) async {
        guard let bookmark = UserDefaults.standard.data(forKey: Self.bookmarkKey), state != .unlocking else { return }
        state = .unlocking
        problem = nil
        // reading and the key derivation are the slow part: away from the main thread
        let result: Result<(Vault, Data?), Error> = await Task.detached {
            Result {
                let file = try Self.readFile(bookmark: bookmark)
                return (try VaultFile.open(file.data, passphrase: passphrase), file.fresh)
            }
        }.value
        switch result {
        case .success(let (opened, fresh)):
            if let fresh { UserDefaults.standard.set(fresh, forKey: Self.bookmarkKey) }
            vault = opened
            state = .unlocked
            if remember { faceIDOn = VaultPassphraseStore.save(passphrase) }
        case .failure(let error):
            state = .locked
            problem = Self.words(error)
        }
    }

    /// Opens with the passphrase kept behind Face ID. `context` is one that has passed the face already (the lock of the application): no second prompt.
    func unlockWithFaceID(context: LAContext? = nil) async {
        let reason = L("keys.faceid.reason")
        let box = ContextBox(context)
        let pass = await Task.detached { VaultPassphraseStore.read(reason: reason, context: box.context) }.value
        guard let pass else {
            // cancelled, or the faces of the phone changed since: the passphrase is typed
            faceIDOn = VaultPassphraseStore.exists
            return
        }
        await unlock(passphrase: pass)
    }

    /// When the application is locked with the face and the passphrase is kept behind it, the vault opens by itself: the person is known already.
    func autoOpen(context: LAContext?) async {
        guard appLockOn, faceIDOn, state == .locked else { return }
        await unlockWithFaceID(context: context)
    }

    func turnFaceIDOff() {
        VaultPassphraseStore.remove()
        faceIDOn = false
    }

    private static func words(_ error: Error) -> String {
        switch error as? VaultError {
        case .auth?: L("keys.err.auth")
        case .format?: L("keys.err.format")
        case .version?: L("keys.err.version")
        case .limit?: L("keys.err.limit")
        case nil: L("keys.err.read")
        }
    }

    /// Forgets what was read.
    func lock() {
        vault = nil
        if state == .unlocked || state == .unlocking { state = fileName == nil ? .noFile : .locked }
    }

    func scenePhaseChanged(_ phase: ScenePhase) {
        switch phase {
        case .background:
            leftAt = Date()
            if autolock == 0, !appLockOn { lock() }
        case .active:
            if let left = leftAt, autolock > 0, !appLockOn, Date().timeIntervalSince(left) > Double(autolock) * 60 { lock() }
            leftAt = nil
        default: break
        }
    }
}
