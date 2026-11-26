import LinagerKit
import SwiftUI

/// The main screen: the console or the dashboard, switched by the system tab bar at the bottom (on iOS 26 a floating glass bar).
/// The application opens in the mode it was left in.
struct MainView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("main.mode") private var mode: AppMode = .console

    var body: some View {
        TabView(selection: $mode) {
            Tab("mode.console", systemImage: "apple.terminal", value: AppMode.console) { ConsoleView() }
            Tab("mode.dashboard", systemImage: "gauge.with.dots.needle.50percent", value: AppMode.dashboard) { DashboardView() }
        }
        .task { model.startLive() }
    }
}
