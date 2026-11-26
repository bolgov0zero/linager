import LinagerKit
import SwiftUI
import UIKit

/// The keys of the remote keyboard that the keyboard of the phone does not have: the same glass capsule as the key bar of the terminal. Ctrl, Alt
/// and Win are sticky (they hold for the next key).
struct RemoteKeyBar: View {
    let model: RemoteModel
    let onHide: () -> Void

    var body: some View {
        GeometryReader { g in bar(side: sideMargin(width: g.size.width)) }
            .frame(height: GlassAccessoryView<RemoteKeyBar>.height)
    }

    /// The room at the sides. In portrait a small one. In landscape the keyboard keeps its letters between the notch (and the home bar) and
    /// the side keys (the globe, the microphone): the bar stands over the letters only, not under the Dynamic Island.
    private func sideMargin(width: CGFloat) -> CGFloat {
        width > 600 ? width * 0.13 : 10
    }

    private func bar(side: CGFloat) -> some View {
        GlassEffectContainer(spacing: 8) {
            HStack(spacing: 8) {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 2) {
                        press(.word("esc"), "key.esc", ScanCodes.esc)
                        sticky("ctrl", "key.ctrl", .ctrl)
                        sticky("alt", "key.alt", .alt)
                        sticky("win", "key.win", .win)
                        press(.word("tab"), "key.tab", ScanCodes.tab)
                        divider
                        press(.symbol("arrow.left"), "key.left", ScanCodes.left, repeats: true)
                        press(.symbol("arrow.down"), "key.down", ScanCodes.down, repeats: true)
                        press(.symbol("arrow.up"), "key.up", ScanCodes.up, repeats: true)
                        press(.symbol("arrow.right"), "key.right", ScanCodes.right, repeats: true)
                        divider
                        ForEach(Array("~|/-\\`[]{}<>&*=+%$#@!^_:;\"'".enumerated()), id: \.offset) { _, c in
                            KeyCap(face: .mono(String(c)), label: LocalizedStringKey(String(c))) { model.typeText(String(c)) }
                        }
                        divider
                        press(.word("home"), "key.home", ScanCodes.home)
                        press(.word("end"), "key.end", ScanCodes.end)
                        press(.word("pgup"), "key.pgup", ScanCodes.pageUp)
                        press(.word("pgdn"), "key.pgdn", ScanCodes.pageDown)
                        press(.word("ins"), "key.ins", ScanCodes.insert)
                        press(.word("del"), "key.del", ScanCodes.delete)
                        divider
                        ForEach(1...12, id: \.self) { n in
                            KeyCap(face: .word("F\(n)"), label: LocalizedStringKey("F\(n)")) {
                                if let k = ScanCodes.function(n) { model.press(k) }
                            }
                        }
                    }
                    .padding(.horizontal, 6)
                }
                .frame(height: 44)
                .clipShape(Capsule())
                .glassEffect(.regular, in: .capsule)

                Button(action: onHide) {
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
            .padding(.horizontal, side)
            .padding(.top, 7)
            .padding(.bottom, 9) // a little room above the keyboard
        }
    }

    private var divider: some View {
        Rectangle().fill(Color.primary.opacity(0.18)).frame(width: 0.5, height: 20).padding(.horizontal, 4)
    }

    private func press(_ face: KeyCap.Face, _ label: LocalizedStringKey, _ key: KeyStroke, repeats: Bool = false) -> some View {
        KeyCap(face: face, repeats: repeats, label: label) { model.press(key) }
    }

    private func sticky(_ word: String, _ label: LocalizedStringKey, _ m: AssistModifiers) -> some View {
        KeyCap(face: .word(word), active: model.modifiers.contains(m), label: label) { model.toggle(m) }
    }
}

/// A SwiftUI bar as the accessory of a keyboard: a transparent input view, so that the glass floats over the picture.
final class GlassAccessoryView<Content: View>: UIInputView {
    static var height: CGFloat { 60 }

    private let host: UIHostingController<Content>

    @MainActor init(content: Content) {
        host = UIHostingController(rootView: content)
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: Self.height), inputViewStyle: .default)
        allowsSelfSizing = true
        // the bar stands right on the keyboard: the home indicator below is not its own room (it would lift the glass by that much)
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
