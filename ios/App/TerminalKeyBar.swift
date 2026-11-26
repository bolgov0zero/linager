import SwiftUI
import UIKit
import class SwiftTerm.TerminalView
import struct SwiftTerm.EscapeSequences

/// What a key of the bar does.
enum KeyAction {
    case ctrl
    case bytes([UInt8])
    case text(String)
    case up, down, left, right, home, end
}

/// The keys the keyboard of the phone does not have, above it. The state of Ctrl lives in the terminal itself (`controlModifier`): the next
/// letter that is typed goes out as Ctrl+letter, and the terminal tells when it has used the modifier.
@MainActor
@Observable
final class KeyBarModel {
    @ObservationIgnored private weak var terminal: TerminalView?
    @ObservationIgnored nonisolated(unsafe) private var observer: NSObjectProtocol?
    @ObservationIgnored private let haptic = UIImpactFeedbackGenerator(style: .light)
    private(set) var ctrl = false

    init(terminal: TerminalView) {
        self.terminal = terminal
        // the name of SwiftTerm's notification "controlModifierReset": posted when it used or dropped the modifier
        observer = NotificationCenter.default.addObserver(forName: Notification.Name("SwiftTerm.TerminalView.controlModifierReset"), object: terminal, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.ctrl = false }
        }
    }

    deinit {
        if let observer { NotificationCenter.default.removeObserver(observer) }
    }

    func perform(_ action: KeyAction) {
        guard let t = terminal else { return }
        haptic.impactOccurred()
        switch action {
        case .ctrl:
            ctrl.toggle()
            t.controlModifier = ctrl
        case .bytes(let b): t.send(b)
        case .text(let s): t.send(txt: s)
        case .up: t.sendKeyUp()
        case .down: t.sendKeyDown()
        case .left: t.sendKeyLeft()
        case .right: t.sendKeyRight()
        case .home: t.sendKeyHome()
        case .end: t.sendKeyEnd()
        }
    }

    func hideKeyboard() { _ = terminal?.resignFirstResponder() }
}

/// One key: just its sign on the glass; a pill shows under it while it is pressed, and while Ctrl is on. A tap presses it; the arrows go on repeating while held (the system's repeat of a button, which leaves the scrolling of the bar alone).
struct KeyCap: View {
    enum Face {
        case word(String)
        case mono(String)
        case symbol(String)
    }

    let face: Face
    var active = false
    var repeats = false
    let label: LocalizedStringKey
    let action: () -> Void

    @State private var pressed = false

    var body: some View {
        Button(action: action) {
            face_
                .frame(minWidth: 34, minHeight: 34)
                .padding(.horizontal, { if case .word = face { 5 } else { 0 } }())
                .background(Capsule().fill(active ? Color.accentColor : Color.primary.opacity(pressed ? 0.2 : 0)))
                .foregroundStyle(active ? Color.white : Color.primary)
                .contentShape(Capsule())
        }
        .buttonStyle(PressReporting(pressed: $pressed))
        // the system repeats a button that is held; no gesture of ours, so that the bar can still be scrolled from a key
        .buttonRepeatBehavior(repeats ? .enabled : .disabled)
        .accessibilityLabel(label)
        .accessibilityAddTraits(active ? .isSelected : [])
    }

    @ViewBuilder private var face_: some View {
        switch face {
        case .word(let s): Text(s).font(.system(size: 15, weight: .medium))
        case .mono(let s): Text(s).font(.system(size: 17, weight: .medium, design: .monospaced))
        case .symbol(let s): Image(systemName: s).font(.system(size: 15, weight: .semibold))
        }
    }
}

/// Tells the cap that a finger is on it (a plain `Button` shows nothing of its own here).
struct PressReporting: ButtonStyle {
    @Binding var pressed: Bool

    func makeBody(configuration: Configuration) -> some View {
        configuration.label.onChange(of: configuration.isPressed) { _, new in pressed = new }
    }
}

struct TerminalKeyBar: View {
    let model: KeyBarModel

    var body: some View {
        GlassEffectContainer(spacing: 8) {
            HStack(spacing: 8) {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 2) {
                        key(.word("esc"), "key.esc", .bytes([0x1b]))
                        KeyCap(face: .word("ctrl"), active: model.ctrl, label: "key.ctrl") { model.perform(.ctrl) }
                        key(.word("tab"), "key.tab", .bytes([0x09]))
                        divider
                        key(.symbol("arrow.left"), "key.left", .left, repeats: true)
                        key(.symbol("arrow.down"), "key.down", .down, repeats: true)
                        key(.symbol("arrow.up"), "key.up", .up, repeats: true)
                        key(.symbol("arrow.right"), "key.right", .right, repeats: true)
                        divider
                        ForEach(Array("~|/-\\`[]{}<>&*=+%$#@!^_:;\"'".enumerated()), id: \.offset) { _, c in
                            KeyCap(face: .mono(String(c)), label: LocalizedStringKey(String(c))) { model.perform(.text(String(c))) }
                        }
                        divider
                        key(.word("home"), "key.home", .home)
                        key(.word("end"), "key.end", .end)
                        key(.word("pgup"), "key.pgup", .bytes(EscapeSequences.cmdPageUp))
                        key(.word("pgdn"), "key.pgdn", .bytes(EscapeSequences.cmdPageDown))
                        key(.word("del"), "key.del", .bytes(EscapeSequences.cmdDelKey))
                        divider
                        ForEach(0..<10, id: \.self) { i in
                            KeyCap(face: .word("F\(i + 1)"), label: LocalizedStringKey("F\(i + 1)")) { model.perform(.bytes(EscapeSequences.cmdF[i])) }
                        }
                    }
                    .padding(.horizontal, 6)
                }
                .frame(height: 44)
                .clipShape(Capsule())
                .glassEffect(.regular, in: .capsule)

                Button { model.hideKeyboard() } label: {
                    Image(systemName: "keyboard.chevron.compact.down")
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundStyle(.primary)
                        .frame(width: 44, height: 44)
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .glassEffect(.regular.interactive(), in: .circle)
                .accessibilityLabel(Text("key.hide"))
            }
            .padding(.horizontal, 10)
            .padding(.top, 7)
            .padding(.bottom, 3) // close to the keyboard
        }
        .frame(height: KeyBarInputView.height)
    }

    /// A thin line between the groups of keys.
    private var divider: some View {
        Rectangle().fill(Color.primary.opacity(0.18)).frame(width: 0.5, height: 20).padding(.horizontal, 4)
    }

    private func key(_ face: KeyCap.Face, _ label: LocalizedStringKey, _ action: KeyAction, repeats: Bool = false) -> some View {
        KeyCap(face: face, repeats: repeats, label: label) { model.perform(action) }
    }
}

/// The bar as the accessory of the keyboard: a transparent input view, so that the glass capsules float over the terminal, with the SwiftUI bar inside.
final class KeyBarInputView: UIInputView {
    static let height: CGFloat = 54

    private let model: KeyBarModel
    private let host: UIHostingController<TerminalKeyBar>

    @MainActor init(terminal: TerminalView) {
        model = KeyBarModel(terminal: terminal)
        host = UIHostingController(rootView: TerminalKeyBar(model: model))
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: Self.height), inputViewStyle: .default)
        allowsSelfSizing = true
        // the bar stands right on the keyboard: the home indicator below is not its own room
        host.safeAreaRegions = []
        host.view.backgroundColor = .clear
        host.view.translatesAutoresizingMaskIntoConstraints = false
        addSubview(host.view)
        NSLayoutConstraint.activate([
            host.view.leadingAnchor.constraint(equalTo: leadingAnchor),
            host.view.trailingAnchor.constraint(equalTo: trailingAnchor),
            host.view.topAnchor.constraint(equalTo: topAnchor),
            host.view.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

    override var intrinsicContentSize: CGSize { CGSize(width: UIView.noIntrinsicMetric, height: Self.height) }
}
