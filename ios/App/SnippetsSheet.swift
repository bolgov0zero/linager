import LinagerKit
import SwiftUI

/// The snippets of the server: a tap types the command and runs it, a swipe only types it (to be changed before it runs).
struct SnippetsSheet: View {
    let term: TerminalModel
    @Environment(\.dismiss) private var dismiss
    @State private var phase: Phase = .loading

    private enum Phase { case loading, loaded([Snippet]), failed }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("term.snippets")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar { ToolbarItem(placement: .confirmationAction) { Button("common.done") { dismiss() } } }
        }
        .presentationDetents([.medium, .large])
        .task { await load() }
    }

    @ViewBuilder private var content: some View {
        switch phase {
        case .loading:
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        case .failed:
            ContentUnavailableView {
                Label("term.snippets.failed", systemImage: "exclamationmark.triangle")
            } actions: {
                Button("common.retry") { Task { await load() } }
            }
        case .loaded(let list) where list.isEmpty:
            ContentUnavailableView {
                Label("term.snippets.none", systemImage: "chevron.left.forwardslash.chevron.right")
            } description: {
                Text("term.snippets.noneHint")
            }
        case .loaded(let list):
            List {
                Section {
                    ForEach(list) { snippet in
                        Button {
                            term.run(snippet, paste: false)
                            dismiss()
                        } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(snippet.name).font(.headline).foregroundStyle(.primary)
                                Text(snippet.command).font(.system(.footnote, design: .monospaced)).foregroundStyle(.secondary).lineLimit(2)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .contentShape(Rectangle())
                        }
                        .swipeActions(edge: .trailing) {
                            Button {
                                term.run(snippet, paste: true)
                                dismiss()
                            } label: {
                                Label("term.snippets.paste", systemImage: "text.insert")
                            }
                            .tint(.blue)
                        }
                    }
                } footer: {
                    Text("term.snippets.hint")
                }
            }
        }
    }

    private func load() async {
        phase = .loading
        do { phase = .loaded(try await term.loadSnippets()) } catch { phase = .failed }
    }
}
