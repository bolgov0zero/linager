import LinagerKit
import class SwiftTerm.TerminalView
import protocol SwiftTerm.TerminalViewDelegate
import SwiftUI

/// The screen of a terminal over the dashboard: the emulator, the state of the connection on the top bar, what to do when it is closed.
struct TerminalScreen: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var phase
    @Environment(\.colorScheme) private var scheme
    @State private var term = TerminalModel()
    @State private var showSnippets = false

    var body: some View {
        NavigationStack {
            ZStack(alignment: .bottom) {
                TerminalRepresentable(model: term, dark: scheme == .dark)
                if let size = term.hud {
                    Text(L("term.size", Int(size)))
                        .font(.headline)
                        .padding(.horizontal, 20).padding(.vertical, 10)
                        .background(.regularMaterial, in: Capsule())
                        .frame(maxHeight: .infinity)
                        .allowsHitTesting(false)
                        .transition(.opacity)
                }
                if case .closed(let reason) = term.state { ClosedBar(reason: reason) { term.reconnect() } }
            }
            .background(Color(uiColor: TerminalRepresentable.background(dark: scheme == .dark)).ignoresSafeArea())
            .animation(.default, value: term.hud)
            .navigationTitle(card.name)
            .navigationSubtitle(term.user.isEmpty ? card.address : card.address + " · " + term.user)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button { dismiss() } label: { Image(systemName: "xmark") }.accessibilityLabel(Text("common.close"))
                }
                ToolbarItemGroup(placement: .topBarTrailing) {
                    if !term.users.isEmpty { userMenu }
                    Button { showSnippets = true } label: { Image(systemName: "chevron.left.forwardslash.chevron.right") }
                        .disabled(term.state != .open)
                        .accessibilityLabel(Text("term.snippets"))
                }
            }
        }
        .task {
            let id = card.id
            term.configure(TerminalBackend(
                open: { cols, rows, user in try await model.openTerminal(serverID: id, cols: cols, rows: rows, user: user) },
                users: { await model.terminalUsers(serverID: id) },
                snippets: { try await model.snippets() },
                ran: { await model.snippetRan(id: $0.id, serverID: id) }))
        }
        .onChange(of: phase) { _, new in
            switch new {
            case .background: term.enteredBackground()
            case .active: term.becameActive()
            default: break
            }
        }
        .onDisappear { term.close() }
        .sheet(isPresented: $showSnippets) { SnippetsSheet(term: term) }
        .alert(failureTitle, isPresented: Binding(get: { failureText != nil }, set: { if !$0 { dismiss() } })) {
            Button("common.ok") { dismiss() }
        } message: {
            Text(failureText ?? "")
        }
    }

    /// The account the shell runs as: choosing another closes this shell and opens a new one.
    private var userMenu: some View {
        Menu {
            Picker("term.user", selection: Binding(get: { term.user }, set: { term.switchUser($0) })) {
                Text("term.user.default").tag("")
                ForEach(term.users, id: \.self) { Text($0).tag($0) }
            }
            .pickerStyle(.inline)
        } label: {
            Image(systemName: "person.crop.circle")
        }
        .accessibilityLabel(Text("term.user"))
    }

    private var failureTitle: LocalizedStringKey { "term.failed.title" }

    /// Why the terminal could not be opened (nil while it works or after it was closed in the ordinary way).
    private var failureText: String? {
        switch term.state {
        case .failed(let e): e.text()
        case .tooMany: L("term.tooMany", card.name)
        default: nil
        }
    }
}

private struct ClosedBar: View {
    let reason: String
    let reconnect: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("term.closed").font(.headline)
            Text(reason == "exit" ? L("term.closed.exit") : reason).font(.footnote).foregroundStyle(.secondary)
            Button(action: reconnect) { Text("term.reconnect").frame(maxWidth: .infinity) }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .padding(.top, 8)
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: cardRadius, style: .continuous))
        .padding(12)
        .transition(.move(edge: .bottom).combined(with: .opacity))
    }
}

/// The emulator (SwiftTerm) in SwiftUI. The font follows the pinch, the colours follow the theme of the system.
struct TerminalRepresentable: UIViewRepresentable {
    let model: TerminalModel
    let dark: Bool

    static func background(dark: Bool) -> UIColor { dark ? .black : .white }
    static func foreground(dark: Bool) -> UIColor { dark ? UIColor(white: 0.9, alpha: 1) : UIColor(white: 0.11, alpha: 1) }

    func makeCoordinator() -> Coordinator { Coordinator(model: model) }

    func makeUIView(context: Context) -> TerminalView {
        let v = TerminalView(frame: .zero)
        v.terminalDelegate = context.coordinator
        // the terminal starts at the top of its own frame (below the bar); the system adds no inset of its own
        v.contentInsetAdjustmentBehavior = .never
        v.font = UIFont.monospacedSystemFont(ofSize: model.fontSize, weight: .regular)
        v.nativeBackgroundColor = Self.background(dark: dark)
        v.nativeForegroundColor = Self.foreground(dark: dark)
        // our own key bar instead of the stock one of the emulator: the look of the rest of the application
        v.inputAccessoryView = KeyBarInputView(terminal: v)
        context.coordinator.terminal = v
        model.attach { [weak v] bytes in v?.feed(byteArray: bytes[...]) }

        let pinch = UIPinchGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.pinch(_:)))
        v.addGestureRecognizer(pinch)
        let reset = UITapGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.resetSize(_:)))
        reset.numberOfTouchesRequired = 2
        reset.numberOfTapsRequired = 2
        v.addGestureRecognizer(reset)
        return v
    }

    func updateUIView(_ v: TerminalView, context: Context) {
        v.nativeBackgroundColor = Self.background(dark: dark)
        v.nativeForegroundColor = Self.foreground(dark: dark)
        // the keyboard comes up as soon as the view is on the screen
        if v.window != nil, !context.coordinator.focused {
            context.coordinator.focused = true
            DispatchQueue.main.async { _ = v.becomeFirstResponder() }
        }
        let size = model.fontSize
        if abs(v.font.pointSize - size) > 0.01 { v.font = UIFont.monospacedSystemFont(ofSize: size, weight: .regular) }
    }

    @MainActor
    final class Coordinator: NSObject, TerminalViewDelegate {
        let model: TerminalModel
        weak var terminal: TerminalView?
        var focused = false
        private var pinchStart: Double = TerminalModel.defaultSize

        init(model: TerminalModel) { self.model = model }

        @objc func pinch(_ g: UIPinchGestureRecognizer) {
            switch g.state {
            case .began: pinchStart = model.fontSize
            case .changed: model.setFontSize(pinchStart * Double(g.scale))
            default: break
            }
        }

        @objc func resetSize(_ g: UITapGestureRecognizer) { model.resetFontSize() }

        // MARK: TerminalViewDelegate

        func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) { model.sizeChanged(cols: newCols, rows: newRows) }
        func send(source: TerminalView, data: ArraySlice<UInt8>) { model.send(Data(data)) }
        func clipboardCopy(source: TerminalView, content: Data) { UIPasteboard.general.string = String(decoding: content, as: UTF8.self) }
        func setTerminalTitle(source: TerminalView, title: String) {}
        func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
        func scrolled(source: TerminalView, position: Double) {}
        /// A link in the output is not opened: what a server prints is not to be trusted with the phone.
        func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {}
        func bell(source: TerminalView) {}
        func iTermContent(source: TerminalView, content: ArraySlice<UInt8>) {}
        func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
    }
}
