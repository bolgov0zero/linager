import CryptoKit
import Foundation

// The vault of the keys ("Keys" of the desktop application, app/frontend/src/lib/keys/vault.ts): a personal encrypted file of logins and
// passwords. The file stays where the person keeps it; it is opened with the passphrase, read into memory and forgotten when it is locked.
// This side only reads (the keys are made and edited in the desktop application).
//
// File layout (big endian): "LGRK", version 1, key derivation 1 (Argon2id), memory KiB (u32), passes (u32), lanes (u32), salt (16), nonce (12),
// then AES-256-GCM of the JSON `{"v":1,"entries":[...]}` with its 16-byte tag at the end; the first 46 bytes are the associated data.

public enum VaultType: String, Sendable, CaseIterable {
    case rdp, uac, any
}

/// A folder or a server a key is bound to: its id in the panel and its name when the key was saved (for the list, if it is gone).
public struct VaultRef: Hashable, Sendable {
    public let id: String
    public let name: String

    public init(id: String, name: String) {
        self.id = id
        self.name = name
    }
}

public struct VaultEntry: Hashable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let type: VaultType
    public let groups: [VaultRef]
    public let servers: [VaultRef]
    public let login: String
    public let domain: String
    public let password: String
    public let note: String

    public init(id: String, name: String, type: VaultType, groups: [VaultRef] = [], servers: [VaultRef] = [], login: String, domain: String = "", password: String = "", note: String = "") {
        self.id = id
        self.name = name
        self.type = type
        self.groups = groups
        self.servers = servers
        self.login = login
        self.domain = domain
        self.password = password
        self.note = note
    }

    /// "CORP\user", or just the login when there is no domain.
    public var account: String { domain.isEmpty ? login : domain + "\\" + login }
}

/// `format`: not a vault file (or damaged); `version`: a newer format; `auth`: a wrong passphrase or a changed file; `limit`: too big.
public enum VaultError: Error, Equatable, Sendable {
    case format, version, auth, limit
}

public struct Vault: Sendable, Equatable {
    public let entries: [VaultEntry]

    public init(entries: [VaultEntry]) { self.entries = entries }
}

public enum VaultFile {
    public static let maxFile = 1 << 20
    public static let maxEntries = 500
    public static let fileExtension = "lvault"

    private static let magic: [UInt8] = [0x4c, 0x47, 0x52, 0x4b]
    private static let headerSize = 46
    private static let tagSize = 16
    // What a file may ask for when it is opened: a file from elsewhere must not eat the memory of the phone.
    private static let minMemory = 8192, maxMemory = 262_144, maxPasses = 10, maxLanes = 4

    // The limits of the fields of a key (the same as LIMITS in vault.ts; the counts are of UTF-16 units, as JavaScript counts).
    private static let nameMax = 80, loginMax = 256, domainMax = 256, passwordMax = 512, noteMax = 1000, idMax = 32, refIdMax = 64, refsMax = 100

    /// Opens a file with the passphrase. The key derivation takes a moment (it is made slow on purpose): call it away from the main thread.
    public static func open(_ data: Data, passphrase: String) throws -> Vault {
        guard data.count <= maxFile else { throw VaultError.limit }
        let b = [UInt8](data)
        guard b.count >= headerSize + tagSize, Array(b[0..<4]) == magic else { throw VaultError.format }
        guard b[4] == 1, b[5] == 1 else { throw VaultError.version }
        func u32(_ o: Int) -> Int { Int(b[o]) << 24 | Int(b[o + 1]) << 16 | Int(b[o + 2]) << 8 | Int(b[o + 3]) }
        let m = u32(6), t = u32(10), p = u32(14)
        guard (minMemory...maxMemory).contains(m), (1...maxPasses).contains(t), (1...maxLanes).contains(p) else { throw VaultError.format }
        let head = Array(b[0..<headerSize])
        let salt = Array(b[18..<34]), nonce = Array(b[34..<46])

        // the passphrase is compared in one Unicode form, so that the same word typed on another keyboard opens the same file
        let pass = Array(passphrase.precomposedStringWithCompatibilityMapping.utf8)
        guard let key = Argon2id.hash(password: pass, salt: salt, iterations: t, memoryKiB: m, parallelism: p) else { throw VaultError.format }

        let plain: Data
        do {
            let sealed = Array(b[headerSize...])
            let box = try AES.GCM.SealedBox(nonce: AES.GCM.Nonce(data: nonce), ciphertext: sealed.dropLast(tagSize), tag: sealed.suffix(tagSize))
            plain = try AES.GCM.open(box, using: SymmetricKey(data: key), authenticating: head)
        } catch {
            throw VaultError.auth // a wrong passphrase and a changed byte look the same, on purpose
        }
        return try parse(plain)
    }

    // MARK: the content

    static func parse(_ plain: Data) throws -> Vault {
        guard let root = (try? JSONSerialization.jsonObject(with: plain)) as? [String: Any] else { throw VaultError.format }
        guard let v = root["v"] as? Int, v == 1 else { throw VaultError.version }
        guard let raw = root["entries"] as? [Any] else { throw VaultError.format }
        guard raw.count <= maxEntries else { throw VaultError.limit }
        var out: [VaultEntry] = []
        for r in raw {
            guard let e = entry(r) else { throw VaultError.format }
            out.append(e)
        }
        return Vault(entries: out)
    }

    private static func text(_ v: Any?, _ max: Int, optional: Bool = false) -> String? {
        if v == nil, optional { return "" }
        guard let s = v as? String, s.utf16.count <= max else { return nil }
        return s
    }

    private static func refs(_ v: Any?) -> [VaultRef]? {
        guard let v else { return [] }
        guard let list = v as? [Any], list.count <= refsMax else { return nil }
        var out: [VaultRef] = []
        for r in list {
            guard let o = r as? [String: Any], let id = text(o["id"], refIdMax), !id.isEmpty, let name = text(o["name"], nameMax, optional: true), !out.contains(where: { $0.id == id }) else { return nil }
            out.append(VaultRef(id: id, name: name))
        }
        return out
    }

    private static func entry(_ x: Any) -> VaultEntry? {
        guard let o = x as? [String: Any],
              let id = text(o["id"], idMax), !id.isEmpty,
              let name = text(o["name"], nameMax),
              let login = text(o["login"], loginMax),
              let domain = text(o["domain"], domainMax, optional: true),
              let password = text(o["password"], passwordMax, optional: true),
              let note = text(o["note"], noteMax, optional: true),
              let groups = refs(o["groups"]), let servers = refs(o["servers"]),
              let typeName = o["type"] as? String, let type = VaultType(rawValue: typeName) else { return nil }
        return VaultEntry(id: id, name: name, type: type, groups: groups, servers: servers, login: login, domain: domain, password: password, note: note)
    }
}

// MARK: the keys for a computer

/// The folders and the servers the panel knows now: a key bound only to things that are gone is "not found".
public struct VaultKnown: Sendable {
    public let servers: Set<String>
    public let groups: Set<String>

    public init(servers: Set<String>, groups: Set<String>) {
        self.servers = servers
        self.groups = groups
    }
}

public struct VaultSections: Equatable, Sendable {
    /// Bound to this server.
    public var here: [VaultEntry] = []
    /// Bound to the folder of this server (or a folder above it).
    public var group: [VaultEntry] = []
    /// For all servers; a binding that points only at things that are gone counts as this.
    public var all: [VaultEntry] = []
    /// Bound to other servers and folders.
    public var other: [VaultEntry] = []

    public init() {}

    /// What fits this computer: this server's keys first.
    public var fitting: [VaultEntry] { here + group + all }
}

public enum VaultKeys {
    public static func isForAll(_ e: VaultEntry) -> Bool { e.groups.isEmpty && e.servers.isEmpty }

    /// The key is bound, and everything it is bound to is gone from the panel.
    public static func isLost(_ e: VaultEntry, known: VaultKnown) -> Bool {
        if isForAll(e) { return false }
        return !e.groups.contains { known.groups.contains($0.id) } && !e.servers.contains { known.servers.contains($0.id) }
    }

    /// The keys of a login form of a server, in the order shown: this server, its folder, all, then the rest. `use` keeps the type that fits
    /// ("any" fits both). `groupChain` is the folder of the server and the folders above it.
    public static func sections(_ entries: [VaultEntry], serverId: String, groupChain: [Int64], known: VaultKnown, use: VaultType) -> VaultSections {
        var out = VaultSections()
        let chain = Set(groupChain.map(String.init))
        for e in entries where e.type == .any || e.type == use {
            if isForAll(e) || isLost(e, known: known) { out.all.append(e) }
            else if e.servers.contains(where: { $0.id == serverId }) { out.here.append(e) }
            else if e.groups.contains(where: { chain.contains($0.id) }) { out.group.append(e) }
            else { out.other.append(e) }
        }
        func byName(_ list: inout [VaultEntry]) { list.sort { $0.name.localizedStandardCompare($1.name) == .orderedAscending } }
        byName(&out.here); byName(&out.group); byName(&out.all); byName(&out.other)
        return out
    }

    /// The folder and the folders above it, the folder first (ids as the panel has them).
    public static func chain(of folderId: Int64?, in folders: [Folder]) -> [Int64] {
        var out: [Int64] = []
        let byId = Dictionary(uniqueKeysWithValues: folders.map { ($0.id, $0) })
        var cur = folderId.flatMap { byId[$0] }
        while let f = cur, !out.contains(f.id) {
            out.append(f.id)
            cur = f.parentId.flatMap { byId[$0] }
        }
        return out
    }
}

// MARK: signing in with a key

public enum SignInAction: Sendable {
    /// The login (with the domain), then Tab: the focus goes to the field of the password.
    case login
    /// The password, then Enter: for a screen that has the login already.
    case password
    /// The login, Tab, the password, Enter.
    case signIn
}

/// One thing to type on the sign-in or UAC screen, and what is pressed after it.
public struct SignInTyping: Equatable, Sendable {
    public let text: String
    public let after: String
    public let label: String
}

public extension VaultEntry {
    /// What to type for an action. The label is the name of the key (it goes to the audit journal, with no text).
    func typing(for action: SignInAction) -> [SignInTyping] {
        switch action {
        case .login: [SignInTyping(text: account, after: "tab", label: name)]
        case .password: [SignInTyping(text: password, after: "enter", label: name)]
        case .signIn: [SignInTyping(text: account, after: "tab", label: name), SignInTyping(text: password, after: "enter", label: name)]
        }
    }
}
