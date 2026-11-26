import LinagerKit
import PhotosUI
import QuickLook
import SwiftUI
import UniformTypeIdentifiers

/// The files of a server as a page of the server: a stack of folders. At the top of every folder is the way to it as chips (a tap jumps to any
/// folder on it), then the entries. The sort, the hidden files and the search are in the menu and the search field; the last folder is
/// remembered for the server. A tap on a file downloads it and shows it (Quick Look). The menu of a row (and a long press) edits a text, renames, copies, moves, changes the
/// permissions and the owner, shares and deletes; the plus makes a file or a folder and sends files and photos of the phone to the folder.
struct ServerFilesTab: View {
    let card: ServerCard
    let close: () -> Void
    @State private var stack: [String]
    @AppStorage private var last: String

    init(card: ServerCard, close: @escaping () -> Void) {
        self.card = card
        self.close = close
        let key = "srv.files.last.\(card.id)"
        _last = AppStorage(wrappedValue: "/", key)
        _stack = State(initialValue: FilePath.chain(UserDefaults.standard.string(forKey: key) ?? "/"))
    }

    var body: some View {
        NavigationStack(path: $stack) {
            FolderPage(card: card, folder: "/", isRoot: true, goTo: goTo, close: close)
                .navigationDestination(for: String.self) { FolderPage(card: card, folder: $0, isRoot: false, goTo: goTo, close: close) }
        }
        .onChange(of: stack) { _, now in last = now.last ?? "/" }
    }

    /// Jumps to a folder: the stack becomes the way to it.
    private func goTo(_ path: String) {
        withAnimation { stack = FilePath.chain(path) }
    }
}

struct FolderPage: View {
    let card: ServerCard
    let folder: String
    let isRoot: Bool
    let goTo: (String) -> Void
    let close: () -> Void
    @Environment(AppModel.self) private var model
    @AppStorage("srv.files.sort") private var sort: FileSort = .name
    @AppStorage("srv.files.hidden") private var showHidden = false
    @State private var query = ""
    @State private var list: FileList?
    @State private var failure: LinagerError?
    @State private var loading = true
    @State private var busy: String?
    @State private var preview: URL?
    @State private var sharing: ShareItem?
    @State private var note: String?
    @State private var editing: PathItem?
    @State private var permissions: PathItem?
    @State private var picking: Pick?
    @State private var prompt: Prompt?
    @State private var nameText = ""
    @State private var deleting: FileEntry?
    @State private var clash: Clash?
    @State private var importing = false
    @State private var choosingPhotos = false
    @State private var photos: [PhotosPickerItem] = []
    @State private var transfer = Transfer()

    struct ShareItem: Identifiable { let url: URL; var id: URL { url } }
    struct PathItem: Identifiable { let path: String; var id: String { path } }
    struct Pick: Identifiable { let entry: FileEntry; let move: Bool; var id: String { entry.id + (move ? "m" : "c") } }
    struct Clash: Identifiable { let entry: FileEntry; let dir: String; let move: Bool; var id: String { entry.id + dir } }
    enum Prompt { case file, folder, rename(FileEntry) }

    /// The progress of the uploads in flight: written from the callback of the session, so it lives on the main actor.
    @MainActor @Observable final class Transfer {
        var name: String?
        var fraction = 0.0
        var left = 0
    }

    private var title: String { isRoot ? L("srv.files") : (folder as NSString).lastPathComponent }

    /// Places that are worth a jump.
    private static let places: [(String, String)] = [("/", "house"), ("/home", "person"), ("/etc", "gearshape"), ("/var/log", "doc.text"), ("/var/www", "globe"), ("/opt", "shippingbox"), ("/tmp", "clock")]

    var body: some View {
        let entries = list?.arranged(sort: sort, showHidden: showHidden, query: query) ?? []
        List {
            Section { crumbs }
                .listRowInsets(EdgeInsets())
                .listRowBackground(Color.clear)
            if list != nil {
                if entries.isEmpty {
                    Section {
                        if query.isEmpty {
                            ContentUnavailableView("srv.files.empty", systemImage: "folder")
                        } else {
                            ContentUnavailableView.search(text: query)
                        }
                    }
                    .listRowBackground(Color.clear)
                } else {
                    Section {
                        ForEach(entries) { entry in row(entry) }
                    } footer: {
                        footer(entries.count)
                    }
                }
            } else if let failure {
                Section {
                    ContentUnavailableView {
                        Label(failure == .accessDenied || failure == .noPermission ? "srv.files.denied" : "srv.files.failed", systemImage: "exclamationmark.triangle")
                    } description: {
                        Text(failure.text())
                    } actions: {
                        Button("common.retry") { Task { await load() } }
                    }
                }
                .listRowBackground(Color.clear)
            } else if loading {
                Section { ProgressView().frame(maxWidth: .infinity, minHeight: 80) }.listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(isRoot ? .large : .inline)
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: Text("srv.files.search"))
        .toolbar {
            if isRoot {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: close) { Image(systemName: "chevron.backward") }.accessibilityLabel(Text("srv.back"))
                }
            }
            ToolbarItemGroup(placement: .topBarTrailing) {
                addMenu
                menu
            }
        }
        .refreshable { await load() }
        .task(id: folder) {
            // the folder is read again every 5 seconds while it is on the screen (a file that appears, one that goes)
            while !Task.isCancelled {
                await load()
                try? await Task.sleep(for: .seconds(5))
            }
        }
        .overlay(alignment: .bottom) {
            if let name = transfer.name {
                HStack(spacing: 10) {
                    ProgressView(value: transfer.fraction).progressViewStyle(.circular)
                    Text(L(transfer.left > 1 ? "srv.files.uploadingMore" : "srv.files.uploading", name, transfer.left)).lineLimit(1)
                }
                .padding(.horizontal, 16).padding(.vertical, 10)
                .glassEffect(.regular, in: Capsule())
                .padding(.bottom, 12)
            } else if let busy {
                HStack(spacing: 10) { ProgressView(); Text(L("srv.files.loading", busy)).lineLimit(1) }
                    .padding(.horizontal, 16).padding(.vertical, 10)
                    .glassEffect(.regular, in: Capsule())
                    .padding(.bottom, 12)
            } else if let note {
                Text(note).font(.subheadline).padding(.horizontal, 16).padding(.vertical, 10)
                    .glassEffect(.regular, in: Capsule())
                    .padding(.bottom, 12)
            }
        }
        .quickLookPreview($preview)
        .sheet(item: $sharing) { ShareSheet(url: $0.url) }
        .fullScreenCover(item: $editing) { item in
            FileEditorSheet(card: card, path: item.path) { Task { await load() } }
        }
        .sheet(item: $permissions) { item in
            FilePermissionsSheet(card: card, path: item.path) { Task { await load() } }
        }
        .sheet(item: $picking) { pick in
            FolderPickerSheet(card: card, subject: pick.entry.name, move: pick.move, start: folder) { dir in
                Task { await transferEntry(pick.entry, to: dir, move: pick.move, mode: .fail) }
            }
        }
        .fileImporter(isPresented: $importing, allowedContentTypes: [.item], allowsMultipleSelection: true) { result in
            if case .success(let urls) = result { Task { await upload(urls, scoped: true) } }
        }
        .photosPicker(isPresented: $choosingPhotos, selection: $photos, matching: .any(of: [.images, .videos]))
        .onChange(of: photos) { _, items in
            guard !items.isEmpty else { return }
            photos = []
            Task { await uploadPhotos(items) }
        }
        .alert(promptTitle, isPresented: Binding(get: { prompt != nil }, set: { if !$0 { prompt = nil } })) {
            TextField("srv.files.name", text: $nameText)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            Button("common.cancel", role: .cancel) { prompt = nil }
            Button(promptButton) { if let p = prompt { prompt = nil; Task { await submit(p) } } }
        }
        .confirmationDialog(deleting.map { L($0.isFolder ? "srv.files.deleteFolder.title" : "srv.files.delete.title", $0.name) } ?? "",
                            isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } }), titleVisibility: .visible, presenting: deleting) { e in
            Button("srv.files.delete", role: .destructive) { Task { await remove(e) } }
            Button("common.cancel", role: .cancel) {}
        } message: { e in
            Text(e.isFolder ? "srv.files.deleteFolder.text" : "srv.files.delete.text")
        }
        .confirmationDialog(clash.map { L("srv.files.clash.title", $0.entry.name) } ?? "",
                            isPresented: Binding(get: { clash != nil }, set: { if !$0 { clash = nil } }), titleVisibility: .visible, presenting: clash) { c in
            Button("srv.files.clash.keep") { Task { await transferEntry(c.entry, to: c.dir, move: c.move, mode: .keep) } }
            Button("srv.files.clash.replace", role: .destructive) { Task { await transferEntry(c.entry, to: c.dir, move: c.move, mode: .overwrite) } }
            Button("common.cancel", role: .cancel) {}
        } message: { _ in Text("srv.files.clash.text") }
    }

    private var promptTitle: String {
        switch prompt {
        case .file: L("srv.files.newFile")
        case .folder: L("srv.files.newFolder")
        case .rename(let e): L("srv.files.rename.title", e.name)
        case nil: ""
        }
    }

    private var promptButton: String {
        if case .rename = prompt { return L("srv.files.rename") }
        return L("srv.files.create")
    }

    private func ask(_ p: Prompt) {
        if case .rename(let e) = p { nameText = e.name } else { nameText = "" }
        prompt = p
    }

    // MARK: the way to the folder

    private var crumbs: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    let chain = FilePath.chain(folder)
                    crumb(label: "/", symbol: "externaldrive", path: "/", current: chain.isEmpty)
                    ForEach(Array(chain.enumerated()), id: \.offset) { i, p in
                        Image(systemName: "chevron.right").font(.caption2.weight(.semibold)).foregroundStyle(.tertiary)
                        crumb(label: (p as NSString).lastPathComponent, symbol: nil, path: p, current: i == chain.count - 1)
                    }
                    Color.clear.frame(width: 1, height: 1).id("end")
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 4)
            }
            .onAppear { proxy.scrollTo("end", anchor: .trailing) }
        }
    }

    private func crumb(label: String, symbol: String?, path: String, current: Bool) -> some View {
        Button { if !current { goTo(path) } } label: {
            HStack(spacing: 4) {
                if let symbol { Image(systemName: symbol) }
                Text(label).lineLimit(1)
            }
            .font(.subheadline.weight(current ? .semibold : .regular))
            .foregroundStyle(current ? Color.primary : Color.accentColor)
            .padding(.horizontal, 12)
            .frame(minHeight: 34)
            .glassEffect(.regular, in: Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(current ? .isSelected : [])
    }

    // MARK: menu

    private var addMenu: some View {
        Menu {
            Button { ask(.file) } label: { Label("srv.files.newFile", systemImage: "doc.badge.plus") }
            Button { ask(.folder) } label: { Label("srv.files.newFolder", systemImage: "folder.badge.plus") }
            Section {
                Button { importing = true } label: { Label("srv.files.uploadFiles", systemImage: "square.and.arrow.up") }
                Button { choosingPhotos = true } label: { Label("srv.files.uploadPhotos", systemImage: "photo.on.rectangle") }
            }
        } label: {
            Image(systemName: "plus")
        }
        .accessibilityLabel(Text("srv.files.add"))
    }

    private var menu: some View {
        Menu {
            Picker("srv.files.sort", selection: $sort) {
                Label("srv.files.sort.name", systemImage: "textformat").tag(FileSort.name)
                Label("srv.files.sort.date", systemImage: "calendar").tag(FileSort.date)
                Label("srv.files.sort.size", systemImage: "arrow.up.arrow.down").tag(FileSort.size)
            }
            Toggle(isOn: $showHidden) { Label("srv.files.hidden", systemImage: "eye") }
            Menu {
                ForEach(Self.places, id: \.0) { p in
                    Button { goTo(p.0) } label: { Label(p.0, systemImage: p.1) }
                }
            } label: { Label("srv.files.places", systemImage: "star") }
            Section {
                Button { UIPasteboard.general.string = folder; flash(L("srv.files.copied")) } label: { Label("srv.files.copyPath", systemImage: "doc.on.doc") }
                Button { Task { await load() } } label: { Label("dash.menu.refresh", systemImage: "arrow.clockwise") }
            }
        } label: {
            Image(systemName: "ellipsis.circle")
        }
        .accessibilityLabel(Text("dash.menu"))
    }

    // MARK: rows

    @ViewBuilder private func footer(_ shown: Int) -> some View {
        let hidden = list?.hiddenCount(showHidden: showHidden) ?? 0
        Text(hidden > 0 ? L("srv.files.countHidden", shown, hidden) : L("srv.files.count", shown))
    }

    @ViewBuilder private func row(_ e: FileEntry) -> some View {
        HStack(spacing: 0) {
            if e.isFolder {
                NavigationLink(value: FilePath.child(folder, e.name)) { label(e) }
            } else {
                Button { Task { await open(e, share: false) } } label: { label(e) }
                    .buttonStyle(.plain)
                    .disabled(busy != nil)
            }
            Menu {
                actions(e)
            } label: {
                Image(systemName: "ellipsis.circle").foregroundStyle(.secondary).frame(width: 44, height: 44).contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(Text(L("srv.files.actions", e.name)))
        }
        .contextMenu { actions(e) }
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button(role: .destructive) { deleting = e } label: { Label("srv.files.delete", systemImage: "trash") }
            if !e.isFolder {
                Button { Task { await open(e, share: true) } } label: { Label("srv.files.share", systemImage: "square.and.arrow.up") }.tint(.accentColor)
            }
            Button { UIPasteboard.general.string = FilePath.child(folder, e.name); flash(L("srv.files.copied")) } label: { Label("srv.files.copyPath", systemImage: "doc.on.doc") }.tint(.gray)
        }
    }

    private func label(_ e: FileEntry) -> some View {
        HStack(spacing: 12) {
            Image(systemName: icon(e)).font(.title3).foregroundStyle(e.isFolder ? Color.accentColor : .secondary).frame(width: 28)
            VStack(alignment: .leading, spacing: 1) {
                Text(e.name).lineLimit(1).truncationMode(.middle)
                Text(meta(e)).font(.caption).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
    }

    /// The actions of a row: not the tap of the row again (it opens), but what the tap does not do.
    @ViewBuilder private func actions(_ e: FileEntry) -> some View {
        let path = FilePath.child(folder, e.name)
        Section {
            if e.kind == .file {
                Button { editing = PathItem(path: path) } label: { Label("srv.files.edit", systemImage: "square.and.pencil") }
            }
            if !e.isFolder {
                Button { Task { await open(e, share: true) } } label: { Label("srv.files.share", systemImage: "square.and.arrow.up") }
            }
        }
        Section {
            Button { ask(.rename(e)) } label: { Label("srv.files.rename", systemImage: "pencil") }
            Button { picking = Pick(entry: e, move: false) } label: { Label("srv.files.copyTo", systemImage: "doc.on.doc") }
            Button { picking = Pick(entry: e, move: true) } label: { Label("srv.files.moveTo", systemImage: "folder") }
            Button { permissions = PathItem(path: path) } label: { Label("srv.files.permissions", systemImage: "lock") }
        }
        Section {
            Button { UIPasteboard.general.string = path; flash(L("srv.files.copied")) } label: { Label("srv.files.copyPath", systemImage: "link") }
            Button { UIPasteboard.general.string = e.name; flash(L("srv.files.copied")) } label: { Label("ctx.copyName", systemImage: "textformat") }
        }
        Section {
            Button(role: .destructive) { deleting = e } label: { Label("srv.files.delete", systemImage: "trash") }
        }
    }

    private func icon(_ e: FileEntry) -> String {
        switch e.kind {
        case .dir: return "folder.fill"
        case .link: return "link"
        case .other: return "questionmark.square.dashed"
        case .file:
            switch (e.name as NSString).pathExtension.lowercased() {
            case "png", "jpg", "jpeg", "gif", "webp", "heic", "svg": return "photo"
            case "zip", "gz", "tgz", "tar", "xz", "bz2", "7z", "rar": return "archivebox"
            case "sh", "bash", "py", "js", "ts", "go", "rb", "pl", "php": return "chevron.left.forwardslash.chevron.right"
            case "conf", "cfg", "ini", "yml", "yaml", "json", "toml", "env", "service", "timer": return "gearshape"
            case "log", "txt", "md": return "doc.text"
            case "pdf": return "doc.richtext"
            case "sql", "db", "sqlite": return "cylinder"
            default: return "doc"
            }
        }
    }

    private func meta(_ e: FileEntry) -> String {
        var parts: [String] = []
        if e.kind == .file { parts.append(Fmt.bytes(e.size)) }
        if e.kind == .link, let t = e.linkTarget { parts.append("→ " + t) }
        if e.mtime > 0 { parts.append(Fmt.shortDate(Date(timeIntervalSince1970: TimeInterval(e.mtime)))) }
        return parts.joined(separator: " · ")
    }

    // MARK: actions

    private func load() async {
        if list == nil { loading = true }
        defer { loading = false }
        do {
            list = try await model.files(serverID: card.id, path: folder)
            failure = nil
        } catch let e as LinagerError {
            if list == nil { failure = e }
        } catch {
            if list == nil { failure = .unreachable(error.localizedDescription) }
        }
    }

    /// Downloads a file, then shows it or hands it to the share sheet.
    private func open(_ e: FileEntry, share: Bool) async {
        guard busy == nil else { return }
        busy = e.name
        defer { busy = nil }
        do {
            let url = try await model.downloadFile(serverID: card.id, path: FilePath.child(folder, e.name))
            if share { sharing = ShareItem(url: url) } else { preview = url }
        } catch let e as LinagerError {
            flash(e.text())
        } catch {
            flash(error.localizedDescription)
        }
    }

    /// Runs a change on the server, then reads the folder again. Says why it failed; true when it went through.
    @discardableResult private func perform(_ work: () async throws -> Void) async -> Bool {
        do {
            try await work()
            await load()
            return true
        } catch let e as LinagerError {
            flash(e.text())
        } catch {
            flash(error.localizedDescription)
        }
        return false
    }

    private func submit(_ p: Prompt) async {
        let name = nameText.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty, !name.contains("/"), name != ".", name != ".." else { flash(L("srv.files.badName")); return }
        let sid = card.id
        switch p {
        case .file:
            let path = FilePath.child(folder, name)
            if await perform({ try await model.createFile(serverID: sid, path: path) }) { editing = PathItem(path: path) }
        case .folder:
            await perform { try await model.createFolder(serverID: sid, path: FilePath.child(folder, name)) }
        case .rename(let e):
            guard name != e.name else { return }
            await perform { try await model.renameEntry(serverID: sid, path: FilePath.child(folder, e.name), name: name) }
        }
    }

    private func remove(_ e: FileEntry) async {
        let sid = card.id
        await perform { try await model.deleteEntry(serverID: sid, path: FilePath.child(folder, e.name)) }
    }

    /// Copies or moves into a folder. A name that is taken there is asked about: both stay, or the old one is replaced.
    private func transferEntry(_ e: FileEntry, to dir: String, move: Bool, mode: TransferMode) async {
        let sid = card.id
        do {
            try await model.transferEntry(serverID: sid, path: FilePath.child(folder, e.name), to: dir, move: move, mode: mode)
            await load()
            flash(L(move ? "srv.files.moved" : "srv.files.copiedTo", dir))
        } catch LinagerError.exists {
            clash = Clash(entry: e, dir: dir, move: move)
        } catch let err as LinagerError {
            flash(err.text())
        } catch {
            flash(error.localizedDescription)
        }
    }

    /// Sends files of the phone one after another into this folder. `scoped`: they come from the Files app and need access to be opened.
    private func upload(_ urls: [URL], scoped: Bool) async {
        let sid = card.id
        let dir = folder
        let box = transfer
        var failed: String?
        for (i, url) in urls.enumerated() {
            let access = scoped && url.startAccessingSecurityScopedResource()
            defer { if access { url.stopAccessingSecurityScopedResource() } }
            box.name = url.lastPathComponent
            box.fraction = 0
            box.left = urls.count - i
            do {
                try await model.uploadFile(serverID: sid, to: FilePath.child(dir, url.lastPathComponent), from: url) { p in
                    Task { @MainActor in box.fraction = p }
                }
            } catch let e as LinagerError {
                failed = e == .exists ? L("srv.files.uploadExists", url.lastPathComponent) : e.text()
            } catch {
                failed = error.localizedDescription
            }
        }
        box.name = nil
        await load()
        flash(failed ?? L("srv.files.uploaded", urls.count))
    }

    private func uploadPhotos(_ items: [PhotosPickerItem]) async {
        var files: [URL] = []
        for item in items {
            if let f = try? await item.loadTransferable(type: PickedFile.self) { files.append(f.url) }
        }
        guard !files.isEmpty else { flash(L("srv.files.photoFailed")); return }
        await upload(files, scoped: false)
        for f in files { try? FileManager.default.removeItem(at: f.deletingLastPathComponent()) }
    }

    private func flash(_ text: String) {
        note = text
        Task {
            try? await Task.sleep(for: .seconds(2))
            if note == text { note = nil }
        }
    }
}

/// The system share sheet for a file.
struct ShareSheet: UIViewControllerRepresentable {
    let url: URL

    func makeUIViewController(context: Context) -> UIActivityViewController { UIActivityViewController(activityItems: [url], applicationActivities: nil) }
    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}

/// A photo or a video chosen in the library, copied to a file of its own (the name keeps the extension: the server shows the kind by it).
struct PickedFile: Transferable {
    let url: URL

    static var transferRepresentation: some TransferRepresentation {
        FileRepresentation(importedContentType: .item) { received in
            let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            let dest = dir.appendingPathComponent(received.file.lastPathComponent)
            try FileManager.default.copyItem(at: received.file, to: dest)
            return PickedFile(url: dest)
        }
    }
}
