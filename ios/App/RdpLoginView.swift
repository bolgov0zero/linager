import LinagerKit
import SwiftUI

/// The sign-in to a Windows computer for the remote desktop: the account (typed, or taken from a key of the vault) and the connect button.
/// The password is not kept: it goes into the connection and is gone with it. Only the login is remembered, per computer.
struct RdpLoginSheet: View {
    let card: ServerCard
    let onConnect: (RdpParams) -> Void
    @Environment(VaultModel.self) private var vault
    @Environment(LockModel.self) private var lock
    @Environment(\.dismiss) private var dismiss
    @State private var login = ""
    @State private var password = ""
    @State private var domain = ""
    @State private var fromKey: String?
    @State private var showKeys = false

    private var loginKey: String { "rdp.login.\(card.id)" }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("rdp.login", text: $login)
                        .textContentType(.username)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.next)
                    SecureField("rdp.password", text: $password)
                        .textContentType(.password)
                        .submitLabel(.next)
                    TextField("rdp.domain", text: $domain)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.go)
                        .onSubmit { if !login.isEmpty && !password.isEmpty { connect() } }
                } header: {
                    Text(verbatim: card.name)
                } footer: {
                    Text(fromKey.map { L("rdp.fromKey.used", $0) } ?? L("rdp.hint"))
                }
                Section {
                    Button { openKeys() } label: { Label("rdp.fromKey", systemImage: "key") }
                }
                Section {
                    Button(action: connect) { Text("rdp.connect").frame(maxWidth: .infinity) }
                        .buttonStyle(.borderedProminent)
                        .controlSize(.large)
                        .disabled(login.trimmingCharacters(in: .whitespaces).isEmpty || password.isEmpty)
                }
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
            }
            .navigationTitle("rdp.title")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("common.cancel") { dismiss() } } }
            .sheet(isPresented: $showKeys) {
                RdpKeysSheet(card: card) { e in
                    login = e.login
                    domain = e.domain
                    password = e.password
                    fromKey = e.name
                    showKeys = false
                }
            }
            .onAppear { login = UserDefaults.standard.string(forKey: loginKey) ?? "" }
        }
    }

    /// With the vault locked and Face ID on, the face is asked for at once, with no sheet in between.
    private func openKeys() {
        guard vault.state == .locked, vault.faceIDOn else {
            showKeys = true
            return
        }
        Task {
            await vault.unlockWithFaceID(context: lock.authenticated)
            showKeys = true
        }
    }

    private func connect() {
        let name = login.trimmingCharacters(in: .whitespaces)
        UserDefaults.standard.set(name, forKey: loginKey)
        // the computer is asked for the size of the screen of the phone in landscape (a wide one), so that the picture fills it
        let screen = UIScreen.main.bounds.size
        let long = max(screen.width, screen.height), short = min(screen.width, screen.height)
        let ratio = max(1.5, min(2.2, (long - 2 * 59) / short))
        let width = Int((720 * ratio / 4).rounded()) * 4
        let host = card.address.isEmpty ? card.name : card.address
        onConnect(RdpParams(username: name, password: password, domain: domain.trimmingCharacters(in: .whitespaces),
                            destination: "\(host):3389", width: width, height: 720))
    }
}

/// The keys of the vault that fit this computer for the remote desktop (the type RDP and the type "any"): a tap fills the form.
struct RdpKeysSheet: View {
    let card: ServerCard
    let pick: (VaultEntry) -> Void
    @Environment(AppModel.self) private var model
    @Environment(VaultModel.self) private var vault
    @Environment(\.dismiss) private var dismiss
    @State private var showAll = false

    private var sections: VaultSections {
        guard let v = vault.vault else { return VaultSections() }
        let known = VaultKnown(servers: Set(model.cards.map(\.id)), groups: Set(model.folders.map { String($0.id) }))
        return VaultKeys.sections(v.entries, serverId: card.id, groupChain: VaultKeys.chain(of: card.folderId, in: model.folders), known: known, use: .rdp)
    }

    var body: some View {
        NavigationStack {
            Group {
                if vault.state == .unlocked { keys } else { Form { Section { Text("rdp.keys.open").foregroundStyle(.secondary) }; VaultUnlockForm() } }
            }
            .navigationTitle("rdp.fromKey")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("common.cancel") { dismiss() } } }
        }
        .presentationSizing(.form)
    }

    private var keys: some View {
        let s = sections
        return List {
            group("remote.signin.keys.here", s.here)
            group("remote.signin.keys.group", s.group)
            group("remote.signin.keys.all", s.all)
            if !s.other.isEmpty {
                Section {
                    Button { withAnimation { showAll.toggle() } } label: { Label(LocalizedStringKey(showAll ? "keys.hide" : "remote.signin.showall"), systemImage: showAll ? "chevron.up" : "chevron.down") }
                }
                if showAll { group("remote.signin.keys.other", s.other) }
            }
            if s.fitting.isEmpty && s.other.isEmpty {
                Section { ContentUnavailableView("keys.empty", systemImage: "key") }.listRowBackground(Color.clear)
            }
        }
    }

    @ViewBuilder private func group(_ title: LocalizedStringKey, _ list: [VaultEntry]) -> some View {
        if !list.isEmpty {
            Section(title) {
                ForEach(list) { e in
                    Button { pick(e) } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(e.name).font(.headline).foregroundStyle(.primary).lineLimit(1)
                            Text(e.account).font(.system(.footnote, design: .monospaced)).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                        }
                    }
                }
            }
        }
    }
}
