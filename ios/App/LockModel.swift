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
            if enabled { locked = true }
        @unknown default:
            break
        }
    }

    func unlock() async {
        guard locked, !busy else { return }
        busy = true
        defer { busy = false }
        let ctx = LAContext()
        var err: NSError?
        // a phone without a passcode cannot be asked: there is nothing to unlock with, and the lock cannot have been turned on
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthentication, error: &err) else {
            locked = false
            return
        }
        if (try? await ctx.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: L("lock.reason"))) == true { locked = false }
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
