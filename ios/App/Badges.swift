import LinagerKit
import SwiftUI

extension Level {
    var color: Color {
        switch self {
        case .ok: .green
        case .warn: .orange
        case .crit: .red
        case .off: Color(.systemGray)
        }
    }
}

extension MeterLevel {
    var color: Color {
        switch self {
        case .ok: .accentColor
        case .warn: .orange
        case .crit: .red
        }
    }
}

extension ChipTone {
    var color: Color {
        switch self {
        case .mute: .secondary
        case .info: .accentColor
        case .warn: .orange
        case .crit: .red
        }
    }
}

extension ChipID {
    /// An SF Symbol per badge; the offline badge has a word instead.
    var symbol: String? {
        switch self {
        case .offline: nil
        case .failed, .agentOld: "exclamationmark.triangle"
        case .reboot: "power"
        case .updates: "arrow.up.circle"
        case .images: "shippingbox"
        case .agentUpdate: "sparkles"
        }
    }
}

extension Chip {
    /// What the badge says in words (VoiceOver, and the list in the long-press menu).
    var title: String {
        switch id {
        case .offline: L("b.offline")
        case .failed: L("b.failed", n ?? 0)
        case .reboot: L("b.reboot")
        case .updates: L("b.updates", n ?? 0)
        case .images: L("b.images", n ?? 0)
        case .agentUpdate: L("b.agentUpdate")
        case .agentOld: L("b.agentOld")
        }
    }
}

/// The compact badge of a state: a symbol and a number, no words (only "no connection" has one). The colour says how much.
struct ChipView: View {
    let chip: Chip
    @ScaledMetric(relativeTo: .caption2) private var height: CGFloat = 18

    var body: some View {
        HStack(spacing: 3) {
            if let symbol = chip.id.symbol {
                Image(systemName: symbol).font(.system(size: 10, weight: .semibold))
            }
            if chip.id == .offline {
                Text("b.offline").font(.caption2.weight(.medium))
            } else if let n = chip.n {
                Text("\(n)").font(.system(.caption2, design: .monospaced).weight(.medium))
            }
        }
        .padding(.horizontal, chip.id == .offline ? 6 : 5)
        .frame(height: height)
        .foregroundStyle(chip.tone.color)
        .background(chip.tone.color.opacity(chip.tone == .mute ? 0.14 : 0.15), in: RoundedRectangle(cornerRadius: 5, style: .continuous))
        .fixedSize()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(chip.title)
    }
}

/// "+N": the badges that did not fit.
struct MoreView: View {
    let count: Int
    let tone: ChipTone
    @ScaledMetric(relativeTo: .caption2) private var height: CGFloat = 18

    var body: some View {
        Text("+\(count)")
            .font(.system(.caption2, design: .monospaced).weight(.medium))
            .padding(.horizontal, 5)
            .frame(height: height)
            .foregroundStyle(.secondary)
            .background(Color.secondary.opacity(0.14), in: RoundedRectangle(cornerRadius: 5, style: .continuous))
            .accessibilityLabel(L("b.more", count))
    }
}

struct ChipsRow: View {
    let chips: [Chip]
    var max = 4

    var body: some View {
        let v = visibleChips(chips, max: max)
        HStack(spacing: 3) {
            ForEach(v.shown) { ChipView(chip: $0) }
            if !v.rest.isEmpty { MoreView(count: v.rest.count, tone: v.rest.map(\.tone).max() ?? .mute) }
        }
    }
}

extension Color {
    init(rgb: UInt32) {
        self.init(red: Double((rgb >> 16) & 0xFF) / 255, green: Double((rgb >> 8) & 0xFF) / 255, blue: Double(rgb & 0xFF) / 255)
    }
}

/// A tag as a small letter badge: the first letter on the colour of the tag (the full name is in the long-press menu).
struct TagBadge: View {
    let tag: TagItem
    @ScaledMetric(relativeTo: .caption2) private var side: CGFloat = 20

    var body: some View {
        let c = Color(rgb: TagPalette.rgb(tag.color))
        Text(tagLetter(tag.name))
            .font(.system(size: 11, weight: .semibold))
            .foregroundStyle(c)
            .frame(width: side, height: side)
            .background(c.opacity(0.18), in: RoundedRectangle(cornerRadius: 5, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 5, style: .continuous).strokeBorder(c.opacity(0.38), lineWidth: 1))
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(L("b.tag", tag.name))
    }
}

struct TagsRow: View {
    let tags: [TagItem]
    var max = 2

    var body: some View {
        let shown = Array(tags.prefix(max))
        let rest = tags.count - shown.count
        HStack(spacing: 3) {
            ForEach(shown) { TagBadge(tag: $0) }
            if rest > 0 { MoreView(count: rest, tone: .mute) }
        }
    }
}

// MARK: system badge

/// The four panes of the Windows mark. SF Symbols has no logo of Windows or Linux, so both are drawn here as shapes (they take the
/// colour of the badge like a template symbol does).
struct WindowsMark: Shape {
    func path(in rect: CGRect) -> Path {
        let s = min(rect.width, rect.height) / 24
        let dx = rect.midX - 12 * s, dy = rect.midY - 12 * s
        func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: dx + x * s, y: dy + y * s) }
        var path = Path()
        path.move(to: p(3, 5.5)); path.addLine(to: p(10.5, 4.5)); path.addLine(to: p(10.5, 11.5)); path.addLine(to: p(3, 11.5)); path.closeSubpath()
        path.move(to: p(12, 4.3)); path.addLine(to: p(21, 3)); path.addLine(to: p(21, 11.5)); path.addLine(to: p(12, 11.5)); path.closeSubpath()
        path.move(to: p(3, 13)); path.addLine(to: p(10.5, 13)); path.addLine(to: p(10.5, 20)); path.addLine(to: p(3, 18.8)); path.closeSubpath()
        path.move(to: p(12, 13)); path.addLine(to: p(21, 13)); path.addLine(to: p(21, 21)); path.addLine(to: p(12, 19.7)); path.closeSubpath()
        return path
    }
}

/// A penguin: the body with the belly, the eyes and the beak cut out (fill it with the even-odd rule).
struct LinuxMark: Shape {
    func path(in rect: CGRect) -> Path {
        let s = min(rect.width, rect.height) / 24
        let dx = rect.midX - 12 * s, dy = rect.midY - 12 * s
        func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: dx + x * s, y: dy + y * s) }
        var path = Path()
        path.move(to: p(12, 2.5))
        path.addCurve(to: p(8.4, 6.8), control1: p(9.4, 2.5), control2: p(8.4, 4.6))
        path.addCurve(to: p(7.3, 10.2), control1: p(8.4, 8.2), control2: p(8, 9))
        path.addCurve(to: p(4.8, 16.8), control1: p(6, 12.3), control2: p(4.8, 14.2))
        path.addCurve(to: p(7.6, 20.3), control1: p(4.8, 19), control2: p(6.2, 20))
        path.addCurve(to: p(12, 21.5), control1: p(8.3, 21.2), control2: p(9.8, 21.5))
        path.addCurve(to: p(16.4, 20.3), control1: p(14.2, 21.5), control2: p(15.7, 21.2))
        path.addCurve(to: p(19.2, 16.8), control1: p(17.8, 20), control2: p(19.2, 19))
        path.addCurve(to: p(16.7, 10.2), control1: p(19.2, 14.2), control2: p(18, 12.3))
        path.addCurve(to: p(15.6, 6.8), control1: p(16, 9), control2: p(15.6, 8.2))
        path.addCurve(to: p(12, 2.5), control1: p(15.6, 4.6), control2: p(14.6, 2.5))
        path.closeSubpath()
        // the belly
        path.move(to: p(12, 9.6))
        path.addCurve(to: p(8.2, 15.2), control1: p(9.9, 9.6), control2: p(8.2, 12))
        path.addCurve(to: p(12, 19.6), control1: p(8.2, 18), control2: p(9.8, 19.6))
        path.addCurve(to: p(15.8, 15.2), control1: p(14.2, 19.6), control2: p(15.8, 18))
        path.addCurve(to: p(12, 9.6), control1: p(15.8, 12), control2: p(14.1, 9.6))
        path.closeSubpath()
        // the beak
        path.move(to: p(10.5, 7.7))
        path.addCurve(to: p(13.5, 7.7), control1: p(10.9, 8.6), control2: p(13.1, 8.6))
        path.addCurve(to: p(10.5, 7.7), control1: p(12.7, 7.2), control2: p(11.3, 7.2))
        path.closeSubpath()
        // the eyes
        path.addEllipse(in: CGRect(x: dx + 9.5 * s, y: dy + 5.1 * s, width: 1.8 * s, height: 1.8 * s))
        path.addEllipse(in: CGRect(x: dx + 12.7 * s, y: dy + 5.1 * s, width: 1.8 * s, height: 1.8 * s))
        return path
    }
}

/// The system of a server as a small coloured badge with a mark and no text, as the desktop app does with "WIN" and "LIN":
/// Windows in the accent colour, Linux in green. VoiceOver reads the name.
struct OSBadge: View {
    let card: ServerCard

    var body: some View {
        let win = isWindows(card.os)
        let tint: Color = win ? .accentColor : .green
        ZStack {
            RoundedRectangle(cornerRadius: 6, style: .continuous).fill(tint.opacity(0.15))
            RoundedRectangle(cornerRadius: 6, style: .continuous).strokeBorder(tint.opacity(0.4), lineWidth: 1)
            if win {
                WindowsMark().fill(tint).frame(width: 13, height: 13)
            } else {
                LinuxMark().fill(tint, style: FillStyle(eoFill: true)).frame(width: 17, height: 17)
            }
        }
        .frame(width: 28, height: 20)
        .accessibilityElement()
        .accessibilityLabel(Text(win ? "os.windows" : "os.linux"))
    }
}
