import LinagerKit
import SwiftUI

struct RootView: View {
    @Environment(AppModel.self) private var model
    @Environment(LockModel.self) private var lock
    @Environment(VaultModel.self) private var vault
    @State private var showPinAlert = false
    @State private var showPinSheet = false

    var body: some View {
        ZStack {
            switch model.screen {
            case .launching: ProgressView()
            case .connect: ConnectView()
            case .login: LoginView()
            case .waiting: WaitingView()
            case .dashboard: MainView()
            }
            if lock.locked || lock.covering { LockOverlay() }
        }
        .animation(.default, value: model.screen)
        .animation(.default, value: lock.locked || lock.covering)
        .onChange(of: model.pinProblem) { _, new in showPinAlert = new != nil }
        // the keys are forgotten when the person leaves the server (a sign-out, a forgotten server)
        .onChange(of: model.screen) { _, new in
            if new != .dashboard { vault.lock() } else { Task { await vault.autoOpen(context: lock.authenticated) } }
        }
        .alert("pin.title", isPresented: $showPinAlert) {
            Button("pin.show") { showPinSheet = true }
            Button("common.cancel", role: .cancel) { model.dismissPinProblem() }
            Button("pin.forget", role: .destructive) { Task { await model.forgetServer() } }
        } message: {
            Text("pin.text")
        }
        .sheet(isPresented: $showPinSheet, onDismiss: { if model.pinProblem != nil { model.dismissPinProblem() } }) { PinChangeSheet() }
    }
}
