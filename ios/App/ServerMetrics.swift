import Charts
import LinagerKit
import SwiftUI

/// The history of the metrics of a server: the period, the charts of the CPU, the memory, the network and the disk, what happened.
struct ServerMetricsTab: View {
    let card: ServerCard
    let close: () -> Void
    @Environment(AppModel.self) private var model
    @AppStorage("srv.period") private var period: HistoryPeriod = .day
    @State private var history: History?
    @State private var failure: LinagerError?
    @State private var loading = true
    /// The moment a person touched: shared by the charts while "Sync tooltips" is on.
    @State private var shared: Date?
    @AppStorage("srv.metrics.sync") private var sync = true

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(spacing: 14) {
                    GlassSegmented(items: HistoryPeriod.allCases.map { (tag: $0, title: title($0)) }, selection: $period)
                        .padding(.horizontal, groupMargin)
                    if let h = history {
                        if h.points.count < 2 {
                            ContentUnavailableView("srv.metrics.none", systemImage: "chart.xyaxis.line").padding(.top, 30)
                        } else {
                            MetricCard(title: "m.cpu", color: .accentColor, unit: .percent, series: [(h.points.map { ($0.ts, $0.cpuAvg) }, .accentColor)], stats: h.stats["cpu"], period: period, sync: sync, shared: $shared)
                            MetricCard(title: "m.ram", color: .green, unit: .percent, series: [(h.points.map { ($0.ts, $0.memAvg) }, .green)], stats: h.stats["mem"], period: period, sync: sync, shared: $shared)
                            MetricCard(title: "srv.net", color: .accentColor, unit: .rate, series: [(h.points.map { ($0.ts, $0.rxAvg) }, .accentColor), (h.points.map { ($0.ts, $0.txAvg) }, .orange)], stats: h.stats["rx"], period: period,
                                       legend: [(LocalizedStringKey("srv.net.rx"), .accentColor), (LocalizedStringKey("srv.net.tx"), .orange)], sync: sync, shared: $shared)
                            MetricCard(title: "m.disk", color: .orange, unit: .percent, series: [(h.points.map { ($0.ts, $0.diskAvg) }, .orange)], stats: h.stats["disk"], period: period, sync: sync, shared: $shared)
                        }
                        if !h.events.isEmpty { events(h.events) }
                    } else if let failure {
                        ContentUnavailableView {
                            Label("srv.files.failed", systemImage: "exclamationmark.triangle")
                        } description: {
                            Text(failure.text())
                        } actions: {
                            Button("common.retry") { Task { await load() } }
                        }
                    } else if loading {
                        ProgressView().padding(.top, 60)
                    }
                }
                .padding(.vertical, 8)
                .padding(.bottom, 12)
            }
            .background(Color(.systemGroupedBackground))
            .refreshable { await load() }
            .navigationTitle("srv.metrics")
            .navigationBarTitleDisplayMode(.large)
            .toolbar {
                ServerToolbar(card: card, close: close, refresh: load) {
                    Section {
                        Toggle(isOn: $sync) { Label("srv.sync", systemImage: "link") }
                    }
                }
            }
            // the charts follow the page: a new load every 5 seconds, at once when the period changes
            .task(id: period) {
                while !Task.isCancelled {
                    await load()
                    try? await Task.sleep(for: .seconds(5))
                }
            }
        }
    }

    private func title(_ p: HistoryPeriod) -> LocalizedStringKey {
        switch p {
        case .hour: "srv.p.1h"
        case .day: "srv.p.24h"
        case .week: "srv.p.7d"
        case .month: "srv.p.30d"
        }
    }

    private func load() async {
        loading = true
        defer { loading = false }
        do {
            history = try await model.history(serverID: card.id, period: period)
            failure = nil
        } catch let e as LinagerError {
            if history == nil { failure = e }
        } catch {
            if history == nil { failure = .unreachable(error.localizedDescription) }
        }
    }

    private func events(_ list: [HistoryEvent]) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("srv.events").font(.footnote).foregroundStyle(.secondary).padding(.horizontal, groupMargin + 16)
            GroupCard {
                ForEach(Array(list.suffix(12).reversed().enumerated()), id: \.element.id) { i, e in
                    GroupRow(first: i == 0) {
                        HStack(spacing: 12) {
                            Image(systemName: symbol(e.kind)).foregroundStyle(tint(e.kind)).frame(width: 24)
                            VStack(alignment: .leading, spacing: 0) {
                                Text(eventText(e)).lineLimit(2)
                                Text(Date(timeIntervalSince1970: TimeInterval(e.ts)).formatted(date: .abbreviated, time: .shortened)).font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer(minLength: 0)
                        }
                        .padding(.vertical, 6)
                    }
                }
            }
        }
    }

    /// What happened, in the language of the phone (the server writes the events in English).
    private func eventText(_ e: HistoryEvent) -> String {
        switch e.kind {
        case "online": return L("srv.ev.online")
        case "offline": return L("srv.ev.offline")
        case "reboot": return L("srv.ev.reboot")
        case "service_failed":
            if let n = Int(e.message.filter(\.isNumber)) { return L("srv.ev.failed", n) }
            return L("srv.ev.failedAny")
        default: return e.message.isEmpty ? e.kind : e.message
        }
    }

    private func symbol(_ kind: String) -> String {
        switch kind {
        case "reboot": "power"
        case "service_failed": "exclamationmark.triangle"
        case "offline": "wifi.slash"
        default: "checkmark.circle"
        }
    }

    private func tint(_ kind: String) -> Color {
        switch kind {
        case "reboot": .orange
        case "service_failed": .red
        case "offline": .secondary
        default: .green
        }
    }
}

enum MetricUnit { case percent, rate }

/// One chart of a card: a title, the values of the metric, a line with a light area, the least, the average and the most of the period.
struct MetricCard: View {
    let title: LocalizedStringKey
    let color: Color
    let unit: MetricUnit
    let series: [(points: [(Int64, Double)], color: Color)]
    let stats: MetricStats?
    let period: HistoryPeriod
    var legend: [(LocalizedStringKey, Color)] = []
    /// Whether the tooltip is shared with the other charts, and the moment shared.
    var sync = true
    @Binding var shared: Date?
    @State private var own: Date?

    /// The moment under the finger: the shared one or the chart's own.
    private var selection: Binding<Date?> { sync ? $shared : $own }

    private var last: Double { series.first?.points.last?.1 ?? 0 }

    private func text(_ v: Double) -> String {
        switch unit {
        case .percent: "\(Int(v.rounded()))%"
        case .rate: Fmt.rate(v)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                Text(title).font(.footnote.weight(.semibold)).foregroundStyle(.secondary)
                ForEach(Array(legend.enumerated()), id: \.offset) { _, l in
                    HStack(spacing: 4) { Circle().fill(l.1).frame(width: 7, height: 7); Text(l.0).font(.caption).foregroundStyle(.secondary) }
                }
                Spacer()
                if let s = stats {
                    Text(L("srv.stats", text(s.avg), text(s.max))).font(.caption).foregroundStyle(.secondary).monospacedDigit()
                }
            }
            Text(text(last)).font(.system(.title, design: .rounded).weight(.bold)).monospacedDigit()
            Chart {
                if let when = selection.wrappedValue, let nearest = nearestTime(to: when) {
                    RuleMark(x: .value("t", nearest))
                        .foregroundStyle(Color.secondary.opacity(0.6))
                        .lineStyle(StrokeStyle(lineWidth: 1, dash: [3, 3]))
                        .annotation(position: .top, spacing: 4, overflowResolution: .init(x: .fit(to: .chart), y: .disabled)) { tooltip(at: nearest) }
                }
                ForEach(Array(series.enumerated()), id: \.offset) { i, s in
                    ForEach(s.points, id: \.0) { p in
                        if i == 0 {
                            AreaMark(x: .value("t", Date(timeIntervalSince1970: TimeInterval(p.0))), y: .value("v", p.1), series: .value("s", i))
                                .foregroundStyle(s.color.opacity(0.16)).interpolationMethod(.monotone)
                        }
                        LineMark(x: .value("t", Date(timeIntervalSince1970: TimeInterval(p.0))), y: .value("v", p.1), series: .value("l", i))
                            .foregroundStyle(s.color).interpolationMethod(.monotone)
                            .lineStyle(StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
                    }
                }
            }
            .chartXSelection(value: selection)
            .chartYScale(domain: unit == .percent ? 0...100 : 0...yMax)
            .chartXAxis {
                AxisMarks(values: .automatic(desiredCount: 4)) { _ in
                    AxisGridLine(stroke: StrokeStyle(lineWidth: 0.5)).foregroundStyle(Color.secondary.opacity(0.3))
                    AxisValueLabel(format: period == .hour || period == .day ? .dateTime.hour().minute() : .dateTime.day().month(.abbreviated))
                }
            }
            .chartYAxis {
                AxisMarks(position: .trailing, values: .automatic(desiredCount: 3)) { v in
                    AxisGridLine(stroke: StrokeStyle(lineWidth: 0.5)).foregroundStyle(Color.secondary.opacity(0.3))
                    AxisValueLabel { if let d = v.as(Double.self) { Text(unit == .percent ? "\(Int(d))%" : Fmt.rate(d)).font(.caption2) } }
                }
            }
            .frame(height: 130)
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: cardRadius, style: .continuous))
        .padding(.horizontal, groupMargin)
    }

    /// The time of the point nearest to a moment (the first series gives the times).
    private func nearestTime(to date: Date) -> Date? {
        let t = Int64(date.timeIntervalSince1970)
        guard let ts = series.first?.points.map(\.0), let best = ts.min(by: { abs($0 - t) < abs($1 - t) }) else { return nil }
        return Date(timeIntervalSince1970: TimeInterval(best))
    }

    /// The time and the value of every line at that point.
    private func tooltip(at date: Date) -> some View {
        let t = Int64(date.timeIntervalSince1970)
        return VStack(alignment: .leading, spacing: 2) {
            Text(date.formatted(date: period == .hour || period == .day ? .omitted : .abbreviated, time: .shortened)).font(.caption2).foregroundStyle(.secondary)
            ForEach(Array(series.enumerated()), id: \.offset) { _, s in
                if let p = s.points.min(by: { abs($0.0 - t) < abs($1.0 - t) }) {
                    HStack(spacing: 4) {
                        Circle().fill(s.color).frame(width: 6, height: 6)
                        Text(text(p.1)).font(.caption.weight(.semibold)).monospacedDigit()
                    }
                }
            }
        }
        .padding(.horizontal, 8).padding(.vertical, 5)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
    }

    private var yMax: Double {
        let m = series.flatMap { $0.points.map(\.1) }.max() ?? 0
        return max(1024, m * 1.15)
    }
}
