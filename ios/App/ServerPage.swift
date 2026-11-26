import Charts
import LinagerKit
import SwiftUI

/// The page of a server: the system tab bar with Overview, Metrics, Files and More. The ways to connect (terminal, Assistant, RDP) are in More.
struct ServerPageScreen: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var phase

    enum PageTab: Hashable { case overview, metrics, files, more }

    @State private var tab: PageTab = .overview
    @State private var detail: ServerDetail?
    /// The last hour of the metrics: the chart of the network on the overview.
    @State private var recent: [HistoryPoint] = []
    @State private var failure: LinagerError?
    @State private var terminalCard: ServerCard?
    @State private var remoteCard: ServerCard?
    @State private var rdpLoginCard: ServerCard?
    @State private var rdpPending: RdpTarget?
    @State private var rdpTarget: RdpTarget?

    struct RdpTarget: Identifiable {
        let card: ServerCard
        let params: RdpParams
        var id: String { card.id }
    }

    /// The card as the dashboard has it now (it stays alive while the page is open).
    private var live: ServerCard { model.cards.first { $0.id == card.id } ?? card }

    private var isAdmin: Bool { model.session?.role == "admin" }
    /// An administrator may open the files of every server; another account only where the module is on for this server.
    private var canFiles: Bool { isAdmin || detail == nil || (detail?.modules.contains("files") ?? false) }

    var body: some View {
        TabView(selection: $tab) {
            Tab("srv.overview", systemImage: "gauge.with.dots.needle.50percent", value: PageTab.overview) {
                ServerOverviewTab(card: live, detail: detail, recent: recent, failure: failure, refresh: load, actions: actions)
            }
            Tab("srv.metrics", systemImage: "chart.xyaxis.line", value: PageTab.metrics) {
                ServerMetricsTab(card: live, close: { dismiss() })
            }
            if canFiles {
                Tab("srv.files", systemImage: "folder", value: PageTab.files) {
                    ServerFilesTab(card: live, close: { dismiss() })
                }
            }
            Tab("srv.more", systemImage: "ellipsis", value: PageTab.more) {
                ServerMoreTab(card: live, actions: actions, close: { dismiss() })
            }
        }
        .fullScreenCover(item: $terminalCard) { TerminalScreen(card: $0) }
        .fullScreenCover(item: $remoteCard) { RemoteScreen(card: $0) }
        .sheet(item: $rdpLoginCard, onDismiss: {
            if let p = rdpPending { rdpPending = nil; rdpTarget = p }
        }) { c in
            RdpLoginSheet(card: c) { params in
                rdpPending = RdpTarget(card: c, params: params)
                rdpLoginCard = nil
            }
        }
        .fullScreenCover(item: $rdpTarget) { RemoteScreen(card: $0.card, rdp: $0.params) }
        // the page keeps itself fresh: every few seconds, at once when the dashboard has had news (the live stream of the application), and on
        // the return of the application from the background
        .task {
            while !Task.isCancelled {
                await load()
                try? await Task.sleep(for: .seconds(5))
            }
        }
        .onChange(of: model.updatedAt) { Task { await load() } }
        .onChange(of: phase) { _, new in if new == .active { Task { await load() } } }
    }

    private func load() async {
        do {
            detail = try await model.serverDetail(serverID: card.id)
            failure = nil
            // the chart is a gentle extra: when it cannot be had the overview stays as it is
            if let h = try? await model.history(serverID: card.id, period: .hour) { recent = h.points }
        } catch let e as LinagerError {
            failure = e
        } catch {
            failure = .unreachable(error.localizedDescription)
        }
    }

    /// What the page can open from its buttons.
    private var actions: ServerActions {
        ServerActions(
            terminal: model.canUseTerminal ? { terminalCard = live } : nil,
            assistant: model.canUseAssistant && assistantOffered(live) ? { remoteCard = live } : nil,
            rdp: model.canUseDesktop && ConnectKind.rdp.offers(live) ? { rdpLoginCard = live } : nil
        )
    }
}

/// The ways to connect that the page offers: nil where the account has no right to it or the server does not have it.
struct ServerActions {
    var terminal: (() -> Void)?
    var assistant: (() -> Void)?
    var rdp: (() -> Void)?
}

// MARK: overview

struct ServerOverviewTab: View {
    let card: ServerCard
    let detail: ServerDetail?
    let recent: [HistoryPoint]
    let failure: LinagerError?
    let refresh: () async -> Void
    let actions: ServerActions
    @Environment(\.dismiss) private var dismiss

    private var sample: SampleDetail? { detail?.sample }

    var body: some View {
        NavigationStack {
            List {
                Section { header }
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
                if let s = sample {
                    Section { gauges(s) }
                    Section {
                        networkCard(s)
                    }
                    attention(s)
                    system(s)
                } else if let failure, detail == nil {
                    Section { Label(failure.text(), systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red).font(.subheadline) }
                } else if detail == nil {
                    Section { ProgressView().frame(maxWidth: .infinity) }.listRowBackground(Color.clear)
                }
                if let note = detail?.note, !note.isEmpty {
                    Section("srv.note") { MarkdownView(text: note) }
                }
            }
            .listStyle(.insetGrouped)
            .refreshable { await refresh() }
            .navigationTitle(card.name)
            .navigationBarTitleDisplayMode(.large)
            .toolbar { ServerToolbar(card: card, close: { dismiss() }, refresh: refresh) }
        }
    }

    // MARK: pieces

    private var header: some View {
        let level = levelOf(card)
        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                StatusDot(level: level)
                Text(level == .off ? L("srv.offline") : L("srv.online") + (sample.map { " · " + Fmt.duration(TimeInterval($0.uptime)) } ?? ""))
                    .font(.subheadline).foregroundStyle(.secondary)
                    .lineLimit(1).minimumScaleFactor(0.8)
                Spacer(minLength: 8)
                OSBadge(card: card)
            }
            let all = chips(for: card)
            if !all.isEmpty || !card.tags.isEmpty {
                HStack(spacing: 6) {
                    ChipsRow(chips: all, max: 4)
                    Spacer(minLength: 8)
                    TagsRow(tags: card.tags)
                }
            }
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 2)
    }

    private func gauges(_ s: SampleDetail) -> some View {
        HStack(alignment: .top, spacing: 6) {
            Ring(title: "m.cpu", value: s.cpu, subtitle: detail?.system.map { L("srv.cores", $0.cpuCores) } ?? "")
            Ring(title: "m.ram", value: s.memPct, subtitle: s.memTotal > 0 ? "\(Fmt.bytes(s.memUsed)) / \(Fmt.bytes(s.memTotal))" : "")
            Ring(title: "m.disk", value: s.diskPct, subtitle: s.disks.first.map { "\(Fmt.bytes($0.used)) / \(Fmt.bytes($0.total))" } ?? "")
        }
        .padding(.vertical, 12)
    }

    /// The network: the rates now and the last hour as a chart (in blue what comes in, in orange what goes out).
    private func networkCard(_ s: SampleDetail) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                Text("srv.netLast").font(.footnote.weight(.semibold)).foregroundStyle(.secondary)
                Spacer()
            }
            HStack(spacing: 22) {
                rate("arrow.down", s.rxBps, .accentColor)
                rate("arrow.up", s.txBps, .orange)
                Spacer(minLength: 0)
            }
            NetworkChart(points: recent)
                .frame(height: 70)
        }
        .padding(.vertical, 6)
    }

    private func rate(_ symbol: String, _ value: Double, _ color: Color) -> some View {
        HStack(spacing: 6) {
            Image(systemName: symbol).font(.headline).foregroundStyle(color)
            Text(Fmt.rate(value)).font(.system(.title3, design: .rounded).weight(.bold)).monospacedDigit().lineLimit(1).minimumScaleFactor(0.7)
        }
    }

    @ViewBuilder private func attention(_ s: SampleDetail) -> some View {
        let rows = attentionRows(s)
        if !rows.isEmpty {
            Section("srv.attention") {
                ForEach(rows, id: \.title) { r in
                    HStack(spacing: 12) {
                        Image(systemName: r.symbol).foregroundStyle(.white).frame(width: 30, height: 30)
                            .background(r.tint, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
                        VStack(alignment: .leading, spacing: 0) {
                            Text(r.title)
                            if let sub = r.subtitle { Text(sub).font(.footnote).foregroundStyle(.secondary) }
                        }
                        Spacer()
                        if let v = r.value { Text(v).foregroundStyle(.secondary) }
                    }
                }
            }
        }
    }

    private struct AttentionRow { let symbol: String; let tint: Color; let title: String; let subtitle: String?; let value: String? }

    private func attentionRows(_ s: SampleDetail) -> [AttentionRow] {
        var r: [AttentionRow] = []
        if s.failedServices > 0 { r.append(AttentionRow(symbol: "exclamationmark.triangle", tint: .red, title: L("srv.att.failed"), subtitle: nil, value: "\(s.failedServices)")) }
        if s.rebootRequired { r.append(AttentionRow(symbol: "power", tint: .orange, title: L("srv.att.reboot"), subtitle: nil, value: nil)) }
        if s.updates > 0 {
            r.append(AttentionRow(symbol: "arrow.up.circle", tint: .accentColor, title: L("srv.att.updates"), subtitle: s.securityUpdates > 0 ? L("srv.att.security", s.securityUpdates) : nil, value: "\(s.updates)"))
        }
        if s.imageUpdates > 0 { r.append(AttentionRow(symbol: "shippingbox", tint: .orange, title: L("srv.att.images"), subtitle: nil, value: "\(s.imageUpdates)")) }
        return r
    }

    private func system(_ s: SampleDetail) -> some View {
        Section("srv.system") {
            if let sys = detail?.system {
                SpecRow("srv.os", shortOs(sys.os.isEmpty ? card.os : sys.os))
                if !sys.kernel.isEmpty { SpecRow("srv.kernel", sys.kernel) }
                if !sys.arch.isEmpty { SpecRow("srv.arch", sys.arch) }
                if !sys.cpuModel.isEmpty { SpecRow("srv.cpu", sys.cpuModel) }
            }
            SpecRow("srv.address", card.address, mono: true)
            if let v = detail?.agentVersion, !v.isEmpty { SpecRow("srv.agent", v) }
            SpecRow("srv.uptime", Fmt.duration(TimeInterval(s.uptime)))
            if s.load.count >= 3 { SpecRow("srv.loadAvg", String(format: "%.2f · %.2f · %.2f", s.load[0], s.load[1], s.load[2])) }
            if s.procs > 0 { SpecRow("srv.procs", "\(s.procs)") }
            if s.swapTotal > 0 { SpecRow("srv.swap", "\(Fmt.bytes(s.swapUsed)) / \(Fmt.bytes(s.swapTotal))") }
            ForEach(s.disks.dropFirst()) { d in
                SpecRow(LocalizedStringKey(d.mount), "\(Fmt.bytes(d.used)) / \(Fmt.bytes(d.total))")
            }
        }
    }
}

/// A fact of the system: the name at the left and the value at the right, always on one line (a long value shrinks a little and is cut in the middle).
struct SpecRow: View {
    let title: LocalizedStringKey
    let value: String
    var mono = false

    init(_ title: LocalizedStringKey, _ value: String, mono: Bool = false) {
        self.title = title
        self.value = value
        self.mono = mono
    }

    var body: some View {
        HStack(spacing: 12) {
            Text(title).lineLimit(1).fixedSize()
            Spacer(minLength: 8)
            Text(value)
                .font(mono ? .body.monospaced() : .body)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
                .truncationMode(.middle)
                .textSelection(.enabled)
        }
        .accessibilityElement(children: .combine)
    }
}

/// The two lines of the network over the last hour, with no axes (the page of the metrics has them).
struct NetworkChart: View {
    let points: [HistoryPoint]

    var body: some View {
        let top = max(1024, (points.map { max($0.rxAvg, $0.txAvg) }.max() ?? 0) * 1.15)
        Chart {
            ForEach(points) { p in
                AreaMark(x: .value("t", Date(timeIntervalSince1970: TimeInterval(p.ts))), y: .value("rx", p.rxAvg), series: .value("s", "rxArea"))
                    .foregroundStyle(Color.accentColor.opacity(0.16)).interpolationMethod(.monotone)
                LineMark(x: .value("t", Date(timeIntervalSince1970: TimeInterval(p.ts))), y: .value("rx", p.rxAvg), series: .value("s", "rx"))
                    .foregroundStyle(Color.accentColor).interpolationMethod(.monotone).lineStyle(StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
                LineMark(x: .value("t", Date(timeIntervalSince1970: TimeInterval(p.ts))), y: .value("tx", p.txAvg), series: .value("s", "tx"))
                    .foregroundStyle(Color.orange).interpolationMethod(.monotone).lineStyle(StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
            }
        }
        .chartYScale(domain: 0...top)
        .chartXAxis(.hidden)
        .chartYAxis(.hidden)
        .chartLegend(.hidden)
        .accessibilityHidden(true)
    }
}

/// A ring with the value in it and the name and the detail under it.
struct Ring: View {
    let title: LocalizedStringKey
    let value: Double
    let subtitle: String

    var body: some View {
        VStack(spacing: 8) {
            ZStack {
                Circle().stroke(Color.secondary.opacity(0.22), lineWidth: 9)
                Circle().trim(from: 0, to: min(1, max(0, value / 100)))
                    .stroke(meter(value).color, style: StrokeStyle(lineWidth: 9, lineCap: .round))
                    .rotationEffect(.degrees(-90))
                Text("\(Int(value.rounded()))%").font(.system(.title3, design: .rounded).weight(.bold)).monospacedDigit()
            }
            .frame(width: 84, height: 84)
            VStack(spacing: 1) {
                Text(title).font(.subheadline.weight(.semibold))
                if !subtitle.isEmpty { Text(subtitle).font(.caption).foregroundStyle(.secondary).lineLimit(1).minimumScaleFactor(0.7) }
            }
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(title))
        .accessibilityValue("\(Int(value.rounded()))%")
    }
}

/// The back button and the menu of a server page.
struct ServerToolbar<Extra: View>: ToolbarContent {
    let card: ServerCard
    let close: () -> Void
    let refresh: () async -> Void
    let extra: Extra

    init(card: ServerCard, close: @escaping () -> Void, refresh: @escaping () async -> Void, @ViewBuilder extra: () -> Extra = { EmptyView() }) {
        self.card = card
        self.close = close
        self.refresh = refresh
        self.extra = extra()
    }

    var body: some ToolbarContent {
        ToolbarItem(placement: .topBarLeading) {
            Button(action: close) { Image(systemName: "chevron.backward") }
                .accessibilityLabel(Text("srv.back"))
        }
        ToolbarItem(placement: .topBarTrailing) {
            Menu {
                Button { Task { await refresh() } } label: { Label("dash.menu.refresh", systemImage: "arrow.clockwise") }
                Button { UIPasteboard.general.string = card.name } label: { Label("ctx.copyName", systemImage: "doc.on.doc") }
                Button { UIPasteboard.general.string = card.address } label: { Label("ctx.copyAddress", systemImage: "network") }
                    .disabled(card.address.isEmpty)
                extra
            } label: { Image(systemName: "ellipsis.circle") }
                .accessibilityLabel(Text("dash.menu"))
        }
    }
}

// MARK: more

private struct MoreSection: Identifiable {
    let id: String
    let symbol: String
    let title: LocalizedStringKey
}

struct ServerMoreTab: View {
    let card: ServerCard
    let actions: ServerActions
    let close: () -> Void

    private static let soon: [[MoreSection]] = [
        [MoreSection(id: "services", symbol: "gearshape.2", title: "srv.sec.services"), MoreSection(id: "processes", symbol: "cpu", title: "srv.sec.processes"), MoreSection(id: "docker", symbol: "shippingbox", title: "srv.sec.docker")],
        [MoreSection(id: "logs", symbol: "doc.text", title: "srv.sec.logs"), MoreSection(id: "disk", symbol: "internaldrive", title: "srv.sec.disk"), MoreSection(id: "updates", symbol: "arrow.up.circle", title: "srv.sec.updates")],
        [MoreSection(id: "network", symbol: "network", title: "srv.sec.network"), MoreSection(id: "firewall", symbol: "flame", title: "srv.sec.firewall"), MoreSection(id: "security", symbol: "lock.shield", title: "srv.sec.security"), MoreSection(id: "users", symbol: "person.2", title: "srv.sec.users")],
    ]

    var body: some View {
        NavigationStack {
            List {
                if actions.terminal != nil || actions.assistant != nil || actions.rdp != nil {
                    Section("srv.connect") {
                        if let a = actions.terminal { Button(action: a) { Label("kind.terminal", systemImage: "apple.terminal") }.disabled(!card.online) }
                        if let a = actions.assistant { Button(action: a) { Label("kind.assistant", systemImage: "cursorarrow.rays") }.disabled(!card.online) }
                        if let a = actions.rdp { Button(action: a) { Label("kind.rdp", systemImage: "display") }.disabled(!card.online) }
                    }
                }
                ForEach(Array(Self.soon.enumerated()), id: \.offset) { i, group in
                    Section(i == 0 ? "srv.sections" : "") {
                        ForEach(group) { s in
                            HStack {
                                Label(s.title, systemImage: s.symbol)
                                Spacer()
                                Text("srv.soon").font(.footnote).foregroundStyle(.tertiary)
                            }
                            .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .listStyle(.insetGrouped)
            .navigationTitle("srv.more")
            .navigationBarTitleDisplayMode(.large)
            .toolbar { ServerToolbar(card: card, close: close, refresh: {}) }
        }
    }
}
