import LinagerKit
import SwiftUI

/// The look of the app: the one of the phone, or light or dark whatever the phone is set to. Remembered.
enum AppTheme: String, CaseIterable {
    case system, light, dark

    /// nil leaves the choice to the phone.
    var scheme: ColorScheme? {
        switch self {
        case .system: nil
        case .light: .light
        case .dark: .dark
        }
    }
}

/// Who is signed in on which server, the lock, the sign-out.
struct AccountView: View {
    @Environment(AppModel.self) private var model
    @Environment(LockModel.self) private var lock
    @Environment(VaultModel.self) private var vault
    @Environment(\.dismiss) private var dismiss
    @State private var confirm: Confirm?
    @State private var copied = false
    @State private var showKeys = false
    @AppStorage("app.theme") private var theme: AppTheme = .system

    private enum Confirm: Identifiable { case signOut, forget; var id: Int { self == .signOut ? 0 : 1 } }

    var body: some View {
        NavigationStack {
            Form {
                Section("acc.server") {
                    LabeledContent("acc.address", value: model.endpoint?.display ?? "—")
                    if let fp = model.fingerprint {
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Text("acc.fingerprint")
                                Spacer()
                                Button {
                                    UIPasteboard.general.string = fp
                                    copied = true
                                } label: {
                                    Image(systemName: copied ? "checkmark" : "doc.on.doc")
                                }
                                .buttonStyle(.borderless)
                                .accessibilityLabel(Text("common.copy"))
                            }
                            FingerprintBox(fingerprint: fp)
                        }
                    }
                }
                Section("acc.user") {
                    LabeledContent("acc.login", value: model.session?.login ?? model.rememberedLogin ?? "—")
                    if let role = model.session?.role { LabeledContent("acc.role", value: roleTitle(role)) }
                }
                Section {
                    Button { showKeys = true } label: {
                        HStack {
                            Label("keys.title", systemImage: "key").foregroundStyle(.primary)
                            Spacer()
                            Text(vault.state == .unlocked ? "keys.state.unlocked" : (vault.fileName == nil ? "keys.subtitle.none" : "keys.state.locked")).foregroundStyle(.secondary).font(.subheadline)
                            Image(systemName: "chevron.right").font(.caption.weight(.semibold)).foregroundStyle(.tertiary)
                        }
                    }
                }
                Section("acc.app") {
                    Picker(selection: $theme) {
                        Text("acc.theme.system").tag(AppTheme.system)
                        Text("acc.theme.light").tag(AppTheme.light)
                        Text("acc.theme.dark").tag(AppTheme.dark)
                    } label: {
                        Label("acc.theme", systemImage: "circle.lefthalf.filled")
                    }
                    Toggle("acc.lock", isOn: Binding(get: { lock.enabled }, set: { on in Task { _ = await lock.setEnabled(on) } }))
                }
                Section {
                    Button("acc.signout", role: .destructive) { confirm = .signOut }
                }
                Section {
                    Button("acc.forget", role: .destructive) { confirm = .forget }
                } footer: {
                    Text(version).frame(maxWidth: .infinity).padding(.top, 8)
                }
            }
            .navigationTitle("acc.title")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("common.done") { dismiss() } }
            }
            .sheet(isPresented: $showKeys) { KeysView() }
            .confirmationDialog("acc.signout.title", isPresented: Binding(get: { confirm == .signOut }, set: { if !$0 { confirm = nil } }), titleVisibility: .visible) {
                Button("acc.signout", role: .destructive) { dismiss(); Task { await model.signOut() } }
                Button("common.cancel", role: .cancel) {}
            }
            .confirmationDialog("acc.forget.title", isPresented: Binding(get: { confirm == .forget }, set: { if !$0 { confirm = nil } }), titleVisibility: .visible) {
                Button("acc.forget", role: .destructive) { dismiss(); Task { await model.forgetServer() } }
                Button("common.cancel", role: .cancel) {}
            } message: {
                Text("acc.forget.text")
            }
        }
        .presentationDetents([.large])
    }

    private func roleTitle(_ role: String) -> String {
        switch role {
        case "admin": L("role.admin")
        case "operator": L("role.operator")
        case "auditor": L("role.auditor")
        default: role
        }
    }

    private var version: String {
        let info = Bundle.main.infoDictionary
        return L("acc.version", info?["CFBundleShortVersionString"] as? String ?? "?", info?["CFBundleVersion"] as? String ?? "?")
    }
}

/// The server shows another certificate than the pinned one. The person compares the two fingerprints and either closes this (nothing
/// changes) or pins the new one, which is only right when the certificate of the server was renewed on purpose.
struct PinChangeSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var confirm = false

    var body: some View {
        NavigationStack {
            Form {
                if let p = model.pinProblem {
                    Section("pin.pinned") { FingerprintBox(fingerprint: p.pinned).listRowInsets(EdgeInsets()) }
                    if let seen = p.seen {
                        Section("pin.offered") { FingerprintBox(fingerprint: seen).listRowInsets(EdgeInsets()) }
                    }
                    Section {
                        Button("pin.accept", role: .destructive) { confirm = true }.disabled(p.seen == nil)
                    } footer: {
                        Text("pin.acceptHint")
                    }
                }
            }
            .navigationTitle("pin.title")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("common.close") { dismiss() } } }
            .confirmationDialog("pin.accept.title", isPresented: $confirm, titleVisibility: .visible) {
                Button("pin.accept", role: .destructive) { model.acceptNewCertificate(); dismiss() }
                Button("common.cancel", role: .cancel) {}
            } message: {
                Text("pin.accept.text")
            }
        }
    }
}
