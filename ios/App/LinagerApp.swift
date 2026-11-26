import LinagerKit
import SwiftUI

@main
struct LinagerApp: App {
    @State private var model = AppModel()
    @State private var lock = LockModel()
    @State private var vault = VaultModel()
    @Environment(\.scenePhase) private var phase
    @AppStorage("app.theme") private var theme: AppTheme = .system
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .environment(lock)
                .environment(vault)
                .preferredColorScheme(theme.scheme)
                .task {
                    vault.appLockOn = lock.enabled
                    await model.launch()
                }
                .onChange(of: lock.enabled) { _, on in vault.appLockOn = on }
                // the person got into the application by the face: the keys open by themselves
                .onChange(of: lock.locked) { _, locked in
                    if !locked { Task { await vault.autoOpen(context: lock.authenticated) } }
                }
                .onChange(of: phase) { _, new in
                    lock.phaseChanged(new)
                    vault.scenePhaseChanged(new)
                    switch new {
                    case .active: model.startLive()
                    case .background: model.stopLive()
                    default: break
                    }
                }
        }
    }
}
