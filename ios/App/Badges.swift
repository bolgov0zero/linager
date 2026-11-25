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
