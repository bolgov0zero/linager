import LinagerKit
import SwiftUI

/// The text editor of a file of the server. The file is read with the time of its version; the save sends that time back, so a file that was
/// changed meanwhile (by someone, by a program) is not wiped silently: the editor asks what to do. The server takes texts up to 1 MB.
struct FileEditorSheet: View {
    let card: ServerCard
    let path: String
    let saved: () -> Void
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var original = ""
    @State private var mtime: Int64 = 0
    @State private var loaded = false
    @State private var failure: LinagerError?
    @State private var saving = false
    @State private var discarding = false
    @State private var conflict = false
    @State private var note: String?
    @State private var selection: TextSelection?

    private var name: String { (path as NSString).lastPathComponent }
    private var dirty: Bool { loaded && text != original }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle(name)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(dirty ? "common.cancel" : "common.close") { if dirty { discarding = true } else { dismiss() } }
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("common.save") { Task { await save(over: false) } }
                            .disabled(!dirty || saving)
                    }
                }
        }
        .interactiveDismissDisabled(dirty)
        .task { await load() }
        .confirmationDialog("srv.edit.discard.title", isPresented: $discarding, titleVisibility: .visible) {
            Button("srv.edit.discard", role: .destructive) { dismiss() }
            Button("srv.edit.keep", role: .cancel) {}
        } message: { Text("srv.edit.discard.text") }
        .confirmationDialog("srv.edit.conflict.title", isPresented: $conflict, titleVisibility: .visible) {
            Button("srv.edit.overwrite", role: .destructive) { Task { await save(over: true) } }
            Button("srv.edit.reload", role: .destructive) { Task { await load() } }
            Button("common.cancel", role: .cancel) {}
        } message: { Text("srv.edit.conflict.text") }
    }

    @ViewBuilder private var content: some View {
        if loaded {
            TextEditor(text: $text, selection: $selection)
                .font(.system(.callout, design: .monospaced))
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .scrollDismissesKeyboard(.interactively)
                .padding(.horizontal, 10)
                .overlay(alignment: .bottom) {
                    if let note {
                        Text(note).font(.subheadline).padding(.horizontal, 16).padding(.vertical, 10)
                            .glassEffect(.regular, in: Capsule())
                            .padding(.bottom, 12)
                    }
                }
                .toolbar {
                    ToolbarItemGroup(placement: .keyboard) {
                        Button { insertTab() } label: { Image(systemName: "arrow.right.to.line") }.accessibilityLabel(Text("srv.edit.tab"))
                        Spacer()
                        Text(L("srv.edit.lines", text.split(separator: "\n", omittingEmptySubsequences: false).count)).font(.caption).foregroundStyle(.secondary)
                        Spacer()
                        Button { UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil) } label: { Image(systemName: "keyboard.chevron.compact.down") }
                            .accessibilityLabel(Text("srv.edit.hideKeyboard"))
                    }
                }
        } else if let failure {
            ContentUnavailableView {
                Label(title(failure), systemImage: failure == .notText || failure == .tooLarge ? "doc.badge.ellipsis" : "exclamationmark.triangle")
            } description: {
                Text(failure.text())
            } actions: {
                if failure != .notText && failure != .tooLarge { Button("common.retry") { Task { await load() } } }
            }
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func title(_ e: LinagerError) -> String {
        switch e {
        case .notText: L("srv.edit.notText")
        case .tooLarge: L("srv.edit.tooLarge")
        default: L("srv.files.failed")
        }
    }

    private func load() async {
        failure = nil
        do {
            let f = try await model.fileContent(serverID: card.id, path: path)
            text = f.content
            original = f.content
            mtime = f.mtime
            loaded = true
        } catch let e as LinagerError {
            failure = e
        } catch {
            failure = .unreachable(error.localizedDescription)
        }
    }

    /// Saves. `over`: the file changed on the server and the person chose to overwrite it: the newest version is read first, only for its time.
    private func save(over: Bool) async {
        guard !saving else { return }
        saving = true
        defer { saving = false }
        do {
            var version = mtime
            if over { version = try await model.fileContent(serverID: card.id, path: path).mtime }
            mtime = try await model.saveFile(serverID: card.id, path: path, content: text, mtime: version)
            original = text
            saved()
            flash(L("srv.edit.saved"))
        } catch LinagerError.fileChanged {
            conflict = true
        } catch let e as LinagerError {
            flash(e.text())
        } catch {
            flash(error.localizedDescription)
        }
    }

    /// A tab at the cursor (the keyboard has none): replaces the selection.
    private func insertTab() {
        guard let sel = selection, case .selection(let r) = sel.indices else { text += "\t"; return }
        let at = text.distance(from: text.startIndex, to: r.lowerBound)
        text.replaceSubrange(r, with: "\t")
        selection = TextSelection(insertionPoint: text.index(text.startIndex, offsetBy: at + 1))
    }

    private func flash(_ s: String) {
        note = s
        Task {
            try? await Task.sleep(for: .seconds(2))
            if note == s { note = nil }
        }
    }
}
