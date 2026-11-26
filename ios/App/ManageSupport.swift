import LinagerKit
import SwiftUI

/// A short message at the bottom of a page (what an action did, or why it failed). It goes away by itself.
@MainActor @Observable
final class NoteBox {
    var text: String?
    private var token = 0

    func show(_ s: String) {
        text = s
        token += 1
        let mine = token
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(2.5))
            if let self, self.token == mine { self.text = nil }
        }
    }

    /// Says why a call failed, in the words of the person.
    func fail(_ error: Error) {
        show((error as? LinagerError)?.text() ?? error.localizedDescription)
    }
}

extension View {
    /// The note of a page: a glass capsule at the bottom.
    func noteOverlay(_ text: String?, bottom: CGFloat = 12) -> some View {
        overlay(alignment: .bottom) {
            if let text {
                Text(text)
                    .font(.subheadline)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 16).padding(.vertical, 10)
                    .glassEffect(.regular, in: Capsule())
                    .padding(.horizontal, 20)
                    .padding(.bottom, bottom)
                    .transition(.opacity)
            }
        }
        .animation(.default, value: text)
    }

    /// Runs `work` now and then every `seconds` while the page is on the screen (the data of a server page stays fresh).
    func refreshing(every seconds: Double = 5, _ work: @escaping () async -> Void) -> some View {
        task {
            while !Task.isCancelled {
                await work()
                try? await Task.sleep(for: .seconds(seconds))
            }
        }
    }
}

/// What a page shows while it has nothing: the wait, or the reason with a way to try again.
struct PageStatus: View {
    let failure: LinagerError?
    let retry: () async -> Void

    var body: some View {
        if let failure {
            ContentUnavailableView {
                Label("srv.files.failed", systemImage: "exclamationmark.triangle")
            } description: {
                Text(failure.text())
            } actions: {
                Button("common.retry") { Task { await retry() } }
            }
        } else {
            ProgressView().frame(maxWidth: .infinity, minHeight: 120)
        }
    }
}

/// A bytes size for a page; empty for what is not known.
func sizeText(_ n: Int64) -> String { n > 0 ? Fmt.bytes(n) : "—" }
