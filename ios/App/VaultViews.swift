import LinagerKit
import SwiftUI
import UniformTypeIdentifiers

/// The file and the passphrase: what is needed to open the vault. Sections of a Form.
struct VaultUnlockForm: View {
    @Environment(VaultModel.self) private var vault
    @State private var passphrase = ""
    @State private var remember = false
    @State private var picking = false

    private static let fileType = UTType(filenameExtension: VaultFile.fileExtension) ?? .data

    var body: some View {
        Section {
            // the picker hangs on the button itself: on an empty view (as it was) there is nothing for the system to present from
            if let name = vault.fileName {
                LabeledContent("keys.file", value: name)
                Button("keys.file.change") { picking = true }
                    .fileImporter(isPresented: $picking, allowedContentTypes: [Self.fileType, .data], onCompletion: picked)
            } else {
                Button { picking = true } label: { Label("keys.file.choose", systemImage: "folder") }
                    .fileImporter(isPresented: $picking, allowedContentTypes: [Self.fileType, .data], onCompletion: picked)
            }
        } footer: {
            if vault.fileName == nil { Text("keys.intro") }
        }
        if vault.fileName != nil {
            Section {
                SecureField("keys.passphrase", text: $passphrase)
                    .textContentType(.password)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.go)
                    .onSubmit(unlock)
                    .onAppear { if vault.appLockOn { remember = true } } // the application is locked by the face: the keys follow it
                if vault.faceIDAvailable && !vault.faceIDOn {
                    Toggle("keys.faceid.remember", isOn: $remember)
                }
            } footer: {
                if let p = vault.problem { Label(p, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red).font(.footnote) }
            }
            Section {
                // one row for both buttons, with room between them (as two rows without insets they stuck together)
                VStack(spacing: 12) {
                    Button(action: unlock) {
                        HStack(spacing: 8) {
                            if vault.state == .unlocking { ProgressView() }
                            Text("keys.unlock").frame(maxWidth: .infinity)
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .disabled(passphrase.isEmpty || vault.state == .unlocking)
                    if vault.faceIDOn {
                        Button { Task { await vault.unlockWithFaceID() } } label: { Label("keys.unlock.faceid", systemImage: "faceid").frame(maxWidth: .infinity) }
                            .buttonStyle(.bordered)
                            .controlSize(.large)
                            .disabled(vault.state == .unlocking)
                    }
                }
                .padding(.vertical, 8)
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
            }
        }
    }

    private func picked(_ result: Result<URL, Error>) {
        if case .success(let url) = result { vault.chooseFile(url) }
    }

    private func unlock() {
        guard !passphrase.isEmpty else { return }
        let p = passphrase
        Task {
            await vault.unlock(passphrase: p, remember: remember)
            if vault.state == .unlocked { passphrase = "" }
        }
    }
}

/// "Keys" of the account: the file, and when it is open the list of the keys (read only: they are made and edited in the desktop application).
struct KeysView: View {
    @Environment(VaultModel.self) private var vault
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var query = ""
    @State private var type: VaultType?
    @State private var confirmForget = false

    var body: some View {
        NavigationStack {
            Group {
                if vault.state == .unlocked, let v = vault.vault { list(v) } else { Form { VaultUnlockForm() }.scrollDismissesKeyboard(.interactively) }
            }
            .navigationTitle("keys.title")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("common.done") { dismiss() } }
                if vault.state == .unlocked { ToolbarItem(placement: .topBarLeading) { menu } }
            }
            .confirmationDialog("keys.file.forget.title", isPresented: $confirmForget, titleVisibility: .visible) {
                Button("keys.file.forget", role: .destructive) { vault.forgetFile() }
                Button("common.cancel", role: .cancel) {}
            } message: {
                Text("keys.file.forget.text")
            }
        }
    }

    private var menu: some View {
        Menu {
            Button { vault.lock() } label: { Label("keys.lock", systemImage: "lock") }
            if vault.appLockOn {
                Label("keys.autolock.app", systemImage: "faceid")
            } else {
            Picker("keys.autolock", selection: Binding(get: { vault.autolock }, set: { vault.autolock = $0 })) {
                Text("keys.autolock.0").tag(0)
                ForEach([1, 5, 15, 30], id: \.self) { Text(L("keys.autolock.n", $0)).tag($0) }
                Text("keys.autolock.never").tag(-1)
            }
            }
            if vault.faceIDOn {
                Button { vault.turnFaceIDOff() } label: { Label("keys.faceid.off", systemImage: "faceid") }
            }
            Button(role: .destructive) { confirmForget = true } label: { Label("keys.file.forget", systemImage: "trash") }
        } label: { Image(systemName: "ellipsis.circle") }
    }

    private func list(_ v: Vault) -> some View {
        let shown = v.entries.filter { e in
            (type == nil || e.type == type) && (query.isEmpty || e.name.localizedCaseInsensitiveContains(query) || e.login.localizedCaseInsensitiveContains(query))
        }
        .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        return List {
            Section {
                // the filter by the use of a key: the same switch as in the Console
                GlassSegmented(items: [(tag: VaultType?.none, title: LocalizedStringKey("keys.type.all"))]
                                + VaultType.allCases.map { (tag: VaultType?.some($0), title: LocalizedStringKey("keys.type.\($0.rawValue)")) },
                               selection: $type)
                    .listRowInsets(EdgeInsets(top: 4, leading: 0, bottom: 4, trailing: 0))
                    .listRowBackground(Color.clear)
            }
            if shown.isEmpty {
                ContentUnavailableView("keys.empty", systemImage: "key").listRowBackground(Color.clear)
            } else {
                Section {
                    ForEach(shown) { e in
                        NavigationLink { KeyDetailView(entry: e) } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                HStack {
                                    Text(e.name).font(.headline)
                                    Spacer()
                                    Text(LocalizedStringKey("keys.type.\(e.type.rawValue)")).font(.caption2.weight(.medium)).padding(.horizontal, 6).padding(.vertical, 2)
                                        .background(Color.secondary.opacity(0.15), in: Capsule())
                                }
                                Text(e.account).font(.system(.footnote, design: .monospaced)).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
        }
        .searchable(text: $query, prompt: Text("keys.search"))
    }
}

/// One key: the fields, the password hidden until it is asked for. What is copied leaves the clipboard in a minute.
struct KeyDetailView: View {
    let entry: VaultEntry
    @Environment(AppModel.self) private var model
    @State private var revealed = false
    @State private var copied = false

    var body: some View {
        List {
            Section {
                LabeledContent("keys.use", value: L("keys.type.\(entry.type.rawValue)"))
            }
            Section {
                field("keys.login", entry.login, secret: false)
                if !entry.domain.isEmpty { field("keys.domain", entry.domain, secret: false) }
                field("keys.password", entry.password, secret: true)
            } footer: {
                if copied { Text("keys.copied") }
            }
            if !entry.note.isEmpty {
                Section("keys.note") { Text(entry.note).textSelection(.enabled) }
            }
            Section("keys.bound") {
                if entry.groups.isEmpty && entry.servers.isEmpty {
                    Text("keys.bound.all").foregroundStyle(.secondary)
                }
                ForEach(entry.servers, id: \.id) { r in boundRow("server.rack", r.id, r.name, exists: model.cards.contains { $0.id == r.id }, current: model.cards.first { $0.id == r.id }?.name) }
                ForEach(entry.groups, id: \.id) { r in boundRow("folder", r.id, r.name, exists: model.folders.contains { String($0.id) == r.id }, current: model.folders.first { String($0.id) == r.id }?.name) }
            }
        }
        .navigationTitle(entry.name)
        .navigationBarTitleDisplayMode(.inline)
    }

    private func boundRow(_ symbol: String, _ id: String, _ stored: String, exists: Bool, current: String?) -> some View {
        HStack {
            Label(current ?? stored, systemImage: symbol)
            Spacer()
            if !exists { Text("keys.bound.lost").font(.footnote).foregroundStyle(.orange) }
        }
    }

    private func field(_ title: LocalizedStringKey, _ value: String, secret: Bool) -> some View {
        HStack {
            Text(title)
            Spacer()
            Text(secret && !revealed ? String(repeating: "•", count: 8) : value)
                .font(.system(.body, design: .monospaced))
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
            if secret {
                Button { reveal() } label: { Image(systemName: revealed ? "eye.slash" : "eye") }.buttonStyle(.borderless)
                    .accessibilityLabel(Text(revealed ? "keys.hide" : "keys.show"))
            }
            Button { copy(value, secret: secret) } label: { Image(systemName: "doc.on.doc") }.buttonStyle(.borderless)
                .accessibilityLabel(Text("common.copy"))
        }
    }

    private func reveal() {
        revealed.toggle()
        guard revealed else { return }
        Task {
            try? await Task.sleep(for: .seconds(10))
            revealed = false
        }
    }

    private func copy(_ value: String, secret: Bool) {
        // a password does not stay on the clipboard (and does not go to other devices)
        UIPasteboard.general.setItems([[UTType.plainText.identifier: value]], options: secret ? [.expirationDate: Date().addingTimeInterval(60), .localOnly: true] : [:])
        copied = secret
    }
}

/// The keys for the sign-in or the UAC screen of a computer: this server's first. Each has the three things to type.
struct SignInKeysSheet: View {
    let remote: RemoteModel
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @Environment(VaultModel.self) private var vault
    @Environment(\.dismiss) private var dismiss
    @State private var showAll = false
    @State private var manual = false

    private var sections: VaultSections {
        guard let v = vault.vault else { return VaultSections() }
        let known = VaultKnown(servers: Set(model.cards.map(\.id)), groups: Set(model.folders.map { String($0.id) }))
        return VaultKeys.sections(v.entries, serverId: card.id, groupChain: VaultKeys.chain(of: card.folderId, in: model.folders), known: known, use: .uac)
    }

    var body: some View {
        NavigationStack {
            Group {
                if vault.state == .unlocked { keys } else { Form { Section { Text("remote.signin.open").foregroundStyle(.secondary) }; VaultUnlockForm() } }
            }
            .navigationTitle("remote.signin.title")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("common.done") { dismiss() } } }
            .sheet(isPresented: $manual) { SignInTypingSheet(remote: remote) }
        }
        .presentationSizing(.form) // a card, not a sheet over the whole width of the landscape screen
    }

    private var keys: some View {
        let s = sections
        return List {
            if !remote.stats.isSignInDesktop {
                Section { Label("remote.signin.notnow", systemImage: "info.circle").font(.footnote).foregroundStyle(.secondary) }
            }
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
            Section {
                Button { manual = true } label: { Label("remote.signin.manual", systemImage: "keyboard") }
            }
        }
    }

    @ViewBuilder private func group(_ title: LocalizedStringKey, _ list: [VaultEntry]) -> some View {
        if !list.isEmpty {
            Section(title) {
                ForEach(list) { e in
                    HStack(spacing: 12) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(e.name).font(.headline).lineLimit(1)
                            Text(e.account).font(.system(.footnote, design: .monospaced)).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                        }
                        Spacer(minLength: 8)
                        // the whole sign-in is the main thing; the login alone and the password alone are behind the dots
                        Menu {
                            Button { run(e, .login) } label: { Label("remote.signin.act.login", systemImage: "person") }
                            Button { run(e, .password) } label: { Label("remote.signin.act.password", systemImage: "key") }
                        } label: {
                            Image(systemName: "ellipsis.circle").font(.title3).frame(width: 34, height: 34)
                        }
                        .buttonStyle(.borderless)
                        Button { run(e, .signIn) } label: { Text("remote.signin.act.signin").fontWeight(.semibold) }
                            .buttonStyle(.borderedProminent)
                            .controlSize(.regular)
                    }
                    .padding(.vertical, 2)
                }
            }
        }
    }

    private func run(_ e: VaultEntry, _ a: SignInAction) {
        if !remote.signIn(e.typing(for: a)) { remote.show(L("remote.type.desktop")) }
        dismiss()
    }
}
