import Charts
import LinagerKit
import SwiftUI

struct StatusDot: View {
    let level: Level

    var body: some View {
        Circle().fill(level.color).frame(width: 10, height: 10)
            .accessibilityHidden(true)
    }
}

/// A meter: a label, a value and a thin bar that turns orange from 75 and red from 90.
struct MetricBar: View {
    let label: LocalizedStringKey
    let value: Double

    var body: some View {
        VStack(spacing: 4) {
            HStack {
                Text(label).foregroundStyle(.secondary)
                Spacer(minLength: 2)
                Text("\(Int(value.rounded()))%").fontWeight(.semibold).monospacedDigit()
            }
            .font(.caption2)
            Capsule().fill(Color.secondary.opacity(0.2)).frame(height: 5)
                .overlay(alignment: .leading) {
                    GeometryReader { g in
                        Capsule().fill(meter(value).color).frame(width: g.size.width * min(1, max(0, value / 100)))
                    }
                }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityValue("\(Int(value.rounded()))%")
    }
}

private func subtitle(_ c: ServerCard, extra: String? = nil) -> String {
    var parts = [c.address, shortOs(c.os)].filter { !$0.isEmpty }
    if let extra { parts.append(extra) }
    return parts.joined(separator: " · ")
}

private func offlineFor(_ c: ServerCard) -> String? {
    guard !c.online, let seen = c.lastSeenAt else { return nil }
    return L("row.offlineFor", Fmt.ago(seen))
}

/// The actions of the long-press menu: copy, and what the badges and the tags say in words.
struct ServerMenu: View {
    let card: ServerCard
    let chips: [Chip]

    var body: some View {
        Button { UIPasteboard.general.string = card.name } label: { Label("ctx.copyName", systemImage: "doc.on.doc") }
        Button { UIPasteboard.general.string = card.address } label: { Label("ctx.copyAddress", systemImage: "network") }
            .disabled(card.address.isEmpty)
        if !chips.isEmpty {
            Section("ctx.state") {
                ForEach(chips) { chip in
                    Label(chip.title, systemImage: chip.id.symbol ?? "wifi.slash")
                }
            }
        }
        if !card.tags.isEmpty {
            Section("ctx.tags") {
                ForEach(card.tags) { Label($0.name, systemImage: "tag") }
            }
        }
    }
}

/// A server as a row of the list: the name with its badges, the address, three meters.
struct ServerRow: View {
    let card: ServerCard
    var agent: AgentFlag?

    var body: some View {
        let level = levelOf(card)
        let all = chips(for: card, agent: agent)
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                StatusDot(level: level)
                Text(card.name).font(.headline).foregroundStyle(level == .off ? .secondary : .primary).lineLimit(1)
                Spacer(minLength: 4)
                ChipsRow(chips: all)
                TagsRow(tags: card.tags)
            }
            Text(subtitle(card, extra: offlineFor(card))).font(.footnote).foregroundStyle(.secondary).lineLimit(1)
                .padding(.leading, 18)
            if card.online, let s = card.sample {
                HStack(spacing: 14) {
                    MetricBar(label: "m.cpu", value: s.cpu)
                    MetricBar(label: "m.ram", value: s.memPct)
                    MetricBar(label: "m.disk", value: s.diskPct)
                }
                .padding(.leading, 18)
                .padding(.top, 3)
            }
        }
        .padding(.vertical, 4)
        .contentShape(Rectangle())
        .contextMenu { ServerMenu(card: card, chips: all) }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(card.name + ", " + level.title)
    }
}

/// The CPU of the last minutes as a line with a light area under it.
struct Sparkline: View {
    let values: [Double]
    let hot: Bool

    var body: some View {
        let color: Color = hot ? .red : .accentColor
        let pts = values.count >= 2 ? values : [values.first ?? 0, values.first ?? 0]
        Chart {
            ForEach(Array(pts.enumerated()), id: \.offset) { i, v in
                AreaMark(x: .value("t", i), y: .value("cpu", min(100, max(0, v)))).foregroundStyle(color.opacity(0.16)).interpolationMethod(.monotone)
                LineMark(x: .value("t", i), y: .value("cpu", min(100, max(0, v)))).foregroundStyle(color).interpolationMethod(.monotone)
                    .lineStyle(StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
            }
        }
        .chartXAxis(.hidden)
        .chartYAxis(.hidden)
        .chartYScale(domain: 0...100)
        .chartLegend(.hidden)
        .accessibilityHidden(true)
    }
}

/// The second layout: a card per server with the CPU in large type and its chart.
struct ServerCardView: View {
    let card: ServerCard
    var agent: AgentFlag?

    var body: some View {
        let level = levelOf(card)
        let all = chips(for: card, agent: agent)
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                StatusDot(level: level)
                Text(card.name).font(.headline).foregroundStyle(level == .off ? .secondary : .primary).lineLimit(1)
                Spacer()
                Text(level.title).font(.footnote).foregroundStyle(.secondary)
            }
            Text(subtitle(card, extra: card.online ? card.sample.map { Fmt.duration(TimeInterval($0.uptime)) } : offlineFor(card)))
                .font(.footnote).foregroundStyle(.secondary).lineLimit(1).padding(.leading, 18)
            if card.online, let s = card.sample {
                HStack(alignment: .bottom, spacing: 12) {
                    VStack(alignment: .leading, spacing: 0) {
                        Text("\(Int(s.cpu.rounded()))%").font(.system(.largeTitle, design: .rounded).weight(.bold)).monospacedDigit()
                        Text("card.cpuLast").font(.caption2).foregroundStyle(.secondary)
                    }
                    Sparkline(values: card.spark, hot: s.cpu >= 90).frame(height: 44)
                }
                HStack(spacing: 16) {
                    MetricBar(label: "m.ram", value: s.memPct)
                    MetricBar(label: "m.disk", value: s.diskPct)
                }
                HStack(spacing: 14) {
                    Label(Fmt.rate(s.rxBps), systemImage: "arrow.down")
                    Label(Fmt.rate(s.txBps), systemImage: "arrow.up")
                }
                .font(.caption2).foregroundStyle(.secondary).monospacedDigit().labelStyle(.titleAndIcon)
            }
            HStack {
                ChipsRow(chips: all)
                Spacer(minLength: 0)
                TagsRow(tags: card.tags, max: 3)
            }
            .frame(minHeight: 20)
        }
        .padding(16)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 22, style: .continuous))
        .contentShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
        .contextMenu { ServerMenu(card: card, chips: all) }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(card.name + ", " + level.title)
    }
}
