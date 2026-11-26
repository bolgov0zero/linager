import LocalAuthentication
import Observation
import SwiftUI

/// The lock of the application: with it on, the screen is covered while the application is in the background and opens with
/// Face ID (or the passcode of the phone).
@MainActor
@Observable
final class LockModel {
    private static let key = "lock.enabled"

    private(set) var enabled: Bool
    private(set) var locked: Bool
    /// The application is not in front (the app switcher takes its picture now): hide the content.
    private(set) var covering = false
    private var busy = false
    /// The context that passed the face (or the passcode) of the lock: the keys behind Face ID open with it, with no second prompt.
    private(set) var authenticated: LAContext?

    init() {
        let on = UserDefaults.standard.bool(forKey: Self.key)
        enabled = on
        locked = on
    }

    func phaseChanged(_ phase: ScenePhase) {
        switch phase {
        case .active:
            covering = false
            if locked { Task { await unlock() } }
        case .inactive:
            covering = enabled
        case .background:
            covering = enabled
            if enabled {
                locked = true
                authenticated = nil
            }
        @unknown default:
            break
        }
    }

    func unlock() async {
        guard locked, !busy else { return }
        busy = true
        defer { busy = false }
        // the face first: a context that passed it is good for the Keychain item of the keys too. When the face does not do (no match, locked out),
        // the passcode of the phone.
        let face = LAContext()
        var err: NSError?
        if face.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) {
            do {
                if try await face.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: L("lock.reason")) {
                    authenticated = face
                    locked = false
                    return
                }
            } catch let e as LAError where e.code == .userFallback || e.code == .biometryLockout || e.code == .biometryNotAvailable || e.code == .biometryNotEnrolled {
                // on to the passcode
            } catch {
                return // cancelled: the person is asked again at the button
            }
        }
        let code = LAContext()
        // a phone without a passcode cannot be asked: there is nothing to unlock with, and the lock cannot have been turned on
        guard code.canEvaluatePolicy(.deviceOwnerAuthentication, error: &err) else {
            locked = false
            return
        }
        if (try? await code.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: L("lock.reason"))) == true {
            authenticated = code
            locked = false
        }
    }

    /// Turning it on asks once, so that a phone that cannot unlock is never locked out.
    func setEnabled(_ on: Bool) async -> Bool {
        if on {
            let ctx = LAContext()
            var err: NSError?
            guard ctx.canEvaluatePolicy(.deviceOwnerAuthentication, error: &err),
                  (try? await ctx.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: L("lock.reason"))) == true else { return false }
        }
        enabled = on
        UserDefaults.standard.set(on, forKey: Self.key)
        return true
    }
}

struct LockOverlay: View {
    @Environment(LockModel.self) private var lock

    var body: some View {
        ZStack {
            Rectangle().fill(.regularMaterial).ignoresSafeArea()
            Color(.systemBackground).opacity(0.6).ignoresSafeArea()
            VStack(spacing: 14) {
                Image(systemName: "lock.fill").font(.system(size: 44)).foregroundStyle(.secondary)
                Text("lock.title").font(.title3.weight(.semibold))
                if lock.locked {
                    Button { Task { await lock.unlock() } } label: { Label("lock.unlock", systemImage: "faceid") }
                        .buttonStyle(.borderedProminent).controlSize(.large)
                }
            }
        }
        .transition(.opacity)
    }
}
