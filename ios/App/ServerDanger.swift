import LinagerKit
import LocalAuthentication
import SwiftUI

/// The two actions that cannot be taken back: to restart the server and to remove it from Linager.
enum DangerKind: Identifiable {
    case reboot, delete
    var id: Self { self }
}

private struct DangerKey: EnvironmentKey { nonisolated(unsafe) static var defaultValue: ((DangerKind) -> Void)? = nil }

extension EnvironmentValues {
    /// Asks for a dangerous action of the server of the page (nil: the account may not do it). The page asks the person to confirm it.
    var serverDanger: ((DangerKind) -> Void)? {
        get { self[DangerKey.self] }
        set { self[DangerKey.self] = newValue }
    }
}

/// Whether the person is there: Face ID or Touch ID when the phone has them, at once, with no question before. Without them (or when the
/// person chooses it on the prompt) the name of the server has to be typed, as in the desktop application.
@MainActor
enum Presence {
    enum Result { case confirmed, cancelled, needsName }

    static func check(reason: String) async -> Result {
        let context = LAContext()
        context.localizedFallbackTitle = L("danger.useName")
        var error: NSError?
        guard context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &error) else { return .needsName }
        do {
            return try await context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason) ? .confirmed : .cancelled
        } catch let e as LAError {
            switch e.code {
            case .userCancel, .systemCancel, .appCancel: return .cancelled
            default: return .needsName // the name instead of the face: the fallback button, no face enrolled, too many tries
            }
        } catch {
            return .needsName
        }
    }
}

/// "Type the name of the server": the confirmation for a phone that has no Face ID or Touch ID (or when the person did not want it).
struct NameConfirmSheet: View {
    let kind: DangerKind
    let name: String
    let confirm: () -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var typed = ""
    @FocusState private var focused: Bool

    private var matches: Bool { typed.trimmingCharacters(in: .whitespaces) == name }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(L(kind == .reboot ? "danger.reboot.text" : "danger.delete.text", name))
                    TextField(name, text: $typed)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .focused($focused)
                        .submitLabel(.go)
                        .onSubmit(go)
                } footer: {
                    Text(L("danger.typeName", name))
                }
                Section {
                    Button(role: .destructive, action: go) {
                        Text(LocalizedStringKey(kind == .reboot ? "srv.danger.reboot.do" : "srv.danger.delete.do")).frame(maxWidth: .infinity)
                    }
                    .disabled(!matches)
                }
            }
            .navigationTitle(LocalizedStringKey(kind == .reboot ? "srv.danger.reboot" : "srv.danger.delete"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("common.cancel") { dismiss() } } }
        }
        .presentationDetents([.medium])
        .onAppear { focused = true }
    }

    private func go() {
        guard matches else { return }
        dismiss()
        confirm()
    }
}
