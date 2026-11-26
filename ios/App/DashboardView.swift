import LinagerKit
import SwiftUI

enum DashboardLayout: String {
    case rows, cards
}

struct DashboardView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("dash.layout") private var layout: DashboardLayout = .rows
    @AppStorage("dash.order") private var order: DashboardOrder = .dashboard
    @State private var filter: DashboardFilter = .all
    @State private var query = ""
    @State private var folder: Int64?
    @State private var collapsed: Set<Int64> = []
    @State private var showAccount = false
    @State private var terminalCard: ServerCard?
    @State private var remoteCard: ServerCard?
    /// The page of a server (opened by a tap on its row).
    @State private var pageCard: ServerCard?

    private var searching: Bool { !query.trimmingCharacters(in: .whitespaces).isEmpty }

    private var shown: [ServerCard] {
        Dashboard.visible(model.cards, filter: filter, query: query, folder: folder, folders: model.folders, order: order)
    }

    private var sections: [DashboardSection] {
        searching ? [DashboardSection(id: -1, path: [], servers: shown)] : Dashboard.sections(shown, folders: model.folders)
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("dash.title")
                .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: Text("dash.search"))
                .toolbar {
                    // two buttons in one glass capsule, as the system apps do
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        viewMenu
                        Button { showAccount = true } label: { Image(systemName: "person.crop.circle") }
                            .accessibilityLabel(Text("acc.title"))
                    }
                }
        }
        .sheet(isPresented: $showAccount) { AccountView() }
        .fullScreenCover(item: $terminalCard) { TerminalScreen(card: $0) }
        .fullScreenCover(item: $remoteCard) { RemoteScreen(card: $0) }
        .fullScreenCover(item: $pageCard) { ServerPageScreen(card: $0) }
    }

    // MARK: content

    @ViewBuilder private var content: some View {
        if model.cards.isEmpty && model.loading {
            ScrollView {
                GroupCard {
                    ForEach(Array(Self.placeholders.enumerated()), id: \.element.id) { i, c in
                        GroupRow(first: i == 0, minHeight: 60) { ServerRow(card: c).padding(.vertical, 6) }
                    }
                }
                .padding(.top, 8)
            }
            .background(Color(.systemGroupedBackground))
            .redacted(reason: .placeholder)
            .disabled(true)
        } else if model.cards.isEmpty && model.offline {
            ContentUnavailableView {
                Label("dash.offline.title", systemImage: "wifi.slash")
            } description: {
                Text("dash.offline.empty")
            } actions: {
                Button("common.retry") { Task { await model.refresh() } }
            }
        } else if model.cards.isEmpty {
            ContentUnavailableView {
                Label("dash.empty.title", systemImage: "server.rack")
            } description: {
                Text("dash.empty.text")
            }
        } else {
            list
        }
    }

    /// All, problems, offline: the system segmented control. It scrolls with the tiles above it and, reaching the bar, stays under it.
    private var filterBar: some View {
        GlassSegmented(items: [(tag: DashboardFilter.all, title: "dash.filter.all"), (tag: .problems, title: "dash.filter.bad"), (tag: .offline, title: "dash.filter.off")], selection: $filter)
    }

    private var list: some View {
        PinnedSwitchScroll(above: {
            if model.offline {
                OfflineBanner(since: model.updatedAt).padding(.horizontal, groupMargin).padding(.bottom, 12)
            }
            if searching {
                Text(L("dash.found", shown.count)).font(.footnote).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, groupMargin + 4).padding(.bottom, 4)
            } else {
                SummaryTiles(totals: summarize(model.cards)).padding(.horizontal, groupMargin)
            }
        }, bar: {
            filterBar
        }, below: {
            if shown.isEmpty {
                Group {
                    if searching {
                        ContentUnavailableView.search(text: query)
                    } else {
                        ContentUnavailableView("dash.nothing", systemImage: "line.3.horizontal.decrease.circle")
                    }
                }
                .padding(.top, 24)
            } else {
                let flat = sections.count == 1 && sections[0].id <= 0
                ForEach(sections) { sec in
                    VStack(alignment: .leading, spacing: 0) {
                        if !flat { sectionHeader(sec).padding(.horizontal, groupMargin + 16).padding(.top, 12).padding(.bottom, 6) }
                        if flat || !collapsed.contains(sec.id) { rows(sec) }
                    }
                    .padding(.bottom, 8)
                }
            }
        }, refresh: { await model.refresh() })
    }

    @ViewBuilder private func rows(_ sec: DashboardSection) -> some View {
        if layout == .rows {
            GroupCard {
                ForEach(Array(sec.servers.enumerated()), id: \.element.id) { i, card in
                    GroupRow(first: i == 0, minHeight: 60) {
                        Button { pageCard = card } label: {
                            ServerRow(card: card, agent: model.agentFlags[card.id], onTerminal: openTerminal, onAssistant: openAssistant)
                                .padding(.vertical, 8)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        } else {
            VStack(spacing: 12) {
                ForEach(sec.servers) { card in
                    Button { pageCard = card } label: {
                        ServerCardView(card: card, agent: model.agentFlags[card.id], onTerminal: openTerminal, onAssistant: openAssistant)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, groupMargin)
        }
    }

    /// The action of the menu "Terminal", or nil when this account may not open one.
    private var openTerminal: ((ServerCard) -> Void)? {
        model.canUseTerminal ? { terminalCard = $0 } : nil
    }

    /// The action of the menu "Assistant", or nil when this account may not open one.
    private var openAssistant: ((ServerCard) -> Void)? {
        model.canUseAssistant ? { remoteCard = $0 } : nil
    }

    private func sectionHeader(_ sec: DashboardSection) -> some View {
        let closed = collapsed.contains(sec.id)
        return Button {
            withAnimation { if closed { collapsed.remove(sec.id) } else { collapsed.insert(sec.id) } }
        } label: {
            HStack(spacing: 6) {
                Image(systemName: "folder")
                Text(sec.id == 0 ? L("dash.nofolder") : sec.title)
                Text("\(sec.servers.count)").foregroundStyle(.tertiary)
                Spacer()
                Image(systemName: closed ? "chevron.right" : "chevron.down").font(.caption.weight(.semibold))
            }
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(.secondary)
            .textCase(nil)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(verbatim: (sec.id == 0 ? L("dash.nofolder") : sec.title) + ", \(sec.servers.count)"))
        .accessibilityValue(Text(LocalizedStringKey(closed ? "dash.collapsed" : "dash.expanded")))
    }

    // MARK: menu

    private var viewMenu: some View {
        Menu {
            Picker("dash.menu.view", selection: $layout) {
                Label("dash.layout.rows", systemImage: "list.bullet").tag(DashboardLayout.rows)
                Label("dash.layout.cards", systemImage: "square.grid.2x2").tag(DashboardLayout.cards)
            }
            .pickerStyle(.inline)
            Picker("dash.menu.order", selection: $order) {
                Label("dash.order.dashboard", systemImage: "arrow.up.arrow.down").tag(DashboardOrder.dashboard)
                Label("dash.order.state", systemImage: "exclamationmark.triangle").tag(DashboardOrder.state)
                Label("dash.order.name", systemImage: "textformat").tag(DashboardOrder.name)
            }
            .pickerStyle(.inline)
            if !model.folders.isEmpty {
                Menu {
                    Picker("dash.folder", selection: $folder) {
                        Text("dash.folder.all").tag(Int64?.none)
                        ForEach(Dashboard.flatten(model.folders), id: \.folder.id) { item in
                            Text(item.path.joined(separator: " › ")).tag(Int64?.some(item.folder.id))
                        }
                    }
                    .pickerStyle(.inline)
                } label: {
                    Label(L("dash.menu.folder", folderName), systemImage: "folder")
                }
            }
            Button { Task { await model.refresh() } } label: { Label("dash.menu.refresh", systemImage: "arrow.clockwise") }
        } label: {
            Image(systemName: "ellipsis.circle")
        }
        .accessibilityLabel(Text("dash.menu"))
    }

    private var folderName: String {
        guard let folder, let item = Dashboard.flatten(model.folders).first(where: { $0.folder.id == folder }) else { return L("dash.folder.all") }
        return item.path.joined(separator: " › ")
    }

    /// Rows for the first picture, before the first answer of the server.
    private static let placeholders: [ServerCard] = (1...4).map {
        ServerCard(id: "p\($0)", name: "server-0\($0)", address: "10.0.0.\($0)", os: "Debian 12", online: true, sample: Sample(cpu: 30, memPct: 40, diskPct: 50), tags: [])
    }
}

struct OfflineBanner: View {
    let since: Date?

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "wifi.slash").font(.title3).foregroundStyle(.orange)
            VStack(alignment: .leading, spacing: 2) {
                Text("dash.offline.title").font(.subheadline.weight(.semibold))
                Text(since.map { L("dash.offline.since", Fmt.time($0)) } ?? L("dash.offline.retry"))
                    .font(.footnote).foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(Color.orange.opacity(0.16), in: RoundedRectangle(cornerRadius: boxRadius, style: .continuous))
        .accessibilityElement(children: .combine)
    }
}

/// Four tiles of the whole picture: how many answer, what needs a look, the updates, the CPU.
struct SummaryTiles: View {
    let totals: Totals

    var body: some View {
        LazyVGrid(columns: [GridItem(.flexible(), spacing: 10), GridItem(.flexible())], spacing: 10) {
            Tile(symbol: "checkmark.shield", tint: .green, title: "dash.tile.online", sub: L("dash.tile.offline", totals.offline)) {
                Text("\(totals.online) \(Text(L("dash.tile.of", totals.total)).font(.headline).foregroundStyle(.secondary))")
            }
            Tile(symbol: "exclamationmark.triangle", tint: .orange, title: "dash.tile.attention", sub: L("dash.tile.critical", totals.critical)) {
                Text("\(totals.problems)")
            }
            Tile(symbol: "arrow.up.circle", tint: .accentColor, title: "dash.tile.updates", sub: L("dash.tile.reboot", totals.rebootPending)) {
                Text("\(totals.updates)")
            }
            Tile(symbol: "cpu", tint: .accentColor, title: "dash.tile.cpu", sub: L("dash.tile.cpuSub")) {
                Text("\(totals.cpuAvg.map { "\(Int($0.rounded()))" } ?? "—")\(Text("%").font(.headline).foregroundStyle(.secondary))")
            }
        }
    }
}

private struct Tile: View {
    let symbol: String
    let tint: Color
    let title: LocalizedStringKey
    let sub: String
    @ViewBuilder let value: () -> Text

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 6) {
                Image(systemName: symbol).foregroundStyle(tint).font(.footnote.weight(.semibold))
                Text(title).font(.footnote.weight(.semibold)).foregroundStyle(.secondary)
            }
            value().font(.system(.title, design: .rounded).weight(.bold)).monospacedDigit()
            Text(sub).font(.caption).foregroundStyle(.secondary).lineLimit(1).minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: cardRadius, style: .continuous))
        .accessibilityElement(children: .combine)
    }
}
