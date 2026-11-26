import LinagerKit
import SwiftUI

/// Chooses a folder of the server (for copy and move): the same stack of folders as the files, but only folders are listed. The button at the
/// bottom takes the folder that is open.
struct FolderPickerSheet: View {
    let card: ServerCard
    /// What is being moved or copied: its name goes into the title.
    let subject: String
    let move: Bool
    let start: String
    let choose: (String) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var stack: [String] = []

    var body: some View {
        NavigationStack(path: $stack) {
            page("/")
                .navigationDestination(for: String.self) { page($0) }
        }
        .onAppear { if stack.isEmpty { stack = FilePath.chain(start) } }
    }

    private func page(_ folder: String) -> some View {
        PickerPage(card: card, folder: folder, subject: subject, move: move, choose: { choose($0); dismiss() }, cancel: { dismiss() })
    }
}

private struct PickerPage: View {
    let card: ServerCard
    let folder: String
    let subject: String
    let move: Bool
    let choose: (String) -> Void
    let cancel: () -> Void
    @Environment(AppModel.self) private var model
    @State private var list: FileList?
    @State private var failure: LinagerError?

    private var title: String { folder == "/" ? "/" : (folder as NSString).lastPathComponent }

    var body: some View {
        let folders = (list?.sorted ?? []).filter { $0.isFolder && !$0.name.hasPrefix(".") }
        List {
            if list != nil {
                if folders.isEmpty {
                    ContentUnavailableView("srv.pick.empty", systemImage: "folder").listRowBackground(Color.clear)
                } else {
                    ForEach(folders) { e in
                        NavigationLink(value: FilePath.child(folder, e.name)) {
                            Label(e.name, systemImage: "folder.fill").lineLimit(1)
                        }
                    }
                }
            } else if let failure {
                ContentUnavailableView { Label("srv.files.failed", systemImage: "exclamationmark.triangle") } description: { Text(failure.text()) }
                    .listRowBackground(Color.clear)
            } else {
                ProgressView().frame(maxWidth: .infinity, minHeight: 80).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar { ToolbarItem(placement: .cancellationAction) { Button("common.cancel", action: cancel) } }
        .safeAreaInset(edge: .bottom) {
            Button { choose(folder) } label: {
                Text(L(move ? "srv.pick.moveHere" : "srv.pick.copyHere", title)).lineLimit(1).frame(maxWidth: .infinity)
            }
            .buttonStyle(.glassProminent)
            .controlSize(.large)
            .padding(.horizontal, 16).padding(.bottom, 8)
        }
        .task(id: folder) {
            do {
                list = try await model.files(serverID: card.id, path: folder)
            } catch let e as LinagerError {
                failure = e
            } catch {
                failure = .unreachable(error.localizedDescription)
            }
        }
    }
}
