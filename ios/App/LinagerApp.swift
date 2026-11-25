import LinagerKit
import SwiftUI

@main
struct LinagerApp: App {
    @State private var model = AppModel()
    @State private var lock = LockModel()
    @Environment(\.scenePhase) private var phase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .environment(lock)
                .task { await model.launch() }
                .onChange(of: phase) { _, new in
                    lock.phaseChanged(new)
                    switch new {
                    case .active: model.startLive()
                    case .background: model.stopLive()
                    default: break
                    }
                }
        }
    }
}
