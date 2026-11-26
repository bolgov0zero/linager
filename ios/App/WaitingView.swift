import LinagerKit
import SwiftUI

/// The phone signed in, but an administrator has not approved it yet (Settings → Devices in the desktop application). The server lets it ask
/// who it is and sign out, nothing else: the model asks again every few seconds, and this screen goes on by itself when the answer changes.
struct WaitingView: View {
    @Environment(AppModel.self) private var model
    @State private var since = Date()

    var body: some View {
        NavigationStack {
            ContentUnavailableView {
                Label("wait.title", systemImage: "lock.shield")
            } description: {
                VStack(spacing: 12) {
                    Text(L("wait.text", model.endpoint?.display ?? ""))
                    TimelineView(.periodic(from: since, by: 1)) { context in
                        Text(clock(context.date))
                            .font(.title3.monospacedDigit())
                            .foregroundStyle(.primary)
                            .accessibilityHidden(true)
                    }
                    Text("wait.hint").font(.footnote)
                    if model.waitingOffline {
                        Label("wait.offline", systemImage: "wifi.slash").font(.footnote).foregroundStyle(.orange)
                    }
                }
            } actions: {
                Button("wait.signOut", role: .destructive) { Task { await model.signOut() } }
                    .buttonStyle(.bordered)
                    .controlSize(.large)
            }
            .navigationBarTitleDisplayMode(.inline)
        }
        .onAppear { since = Date() }
    }

    private func clock(_ now: Date) -> String {
        let s = max(0, Int(now.timeIntervalSince(since)))
        return String(format: "%d:%02d", s / 60, s % 60)
    }
}
