import LinagerKit
import SwiftUI

/// The console: a bare tree of folders and servers (a status dot, the name, the system) to connect from. What the list shows depends on
/// the kind of connection chosen at the top: a server that cannot be opened that way is not shown, nor a folder with no such server.
/// The kind, the filters, the folded folders (and the mode itself) are remembered.
struct ConsoleView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("console.kind") private var kind: ConnectKind = .terminal
    @AppStorage("console.system") private var system: SystemFilter = .all
    @AppStorage("console.status") private var status: StatusFilter = .all
    @AppStorage("console.folded") private var foldedRaw = ""
    @State private var query = ""
    @State private var showAccount = false
    @State private var terminalCard: ServerCard?
    @State private var remoteCard: ServerCard?
    /// The remote desktop: the computer whose sign-in form is open, then (after the form has closed) the one that is connected.
    @State private var rdpLoginCard: ServerCard?
    @State private var rdpPending: RdpTarget?
    @State private var rdpTarget: RdpTarget?
    @State private var notice: Notice?
    @State private var pageCard: ServerCard?

    struct RdpTarget: Identifiable {
        let card: ServerCard
        let params: RdpParams
        var id: String { card.id }
    }

    struct Notice: Equatable {
        let title: String
        let text: String
    }

    private var folded: Set<Int64> { Console.decode(foldedRaw) }

    /// The kinds this account may use, in the order of the switch. The remote desktop is always listed, grey for now.
    private var kinds: [ConnectKind] {
        ConnectKind.allCases.filter { k in
            switch k {
            // before the session is known (the first picture comes from the saved data) the rights are not known: nothing is hidden
            case .terminal: model.session == nil || model.canUseTerminal
            case .rdp: model.session == nil || model.canUseDesktop
            case .assistant: model.session == nil || model.canUseAssistant
            }
        }
    }

    /// The chosen kind, or the first usable one when the stored one is not (not ready, or no right to it any more).
    private var activeKind: ConnectKind {
        kinds.contains(kind) && kind.isReady ? kind : (kinds.first { $0.isReady } ?? .terminal)
    }

    private var filter: ConsoleFilter { ConsoleFilter(kind: activeKind, system: system, status: status, query: query) }

    private var rows: [ConsoleRow] {
        Console.rows(cards: model.cards, folders: model.folders, filter: filter, collapsed: folded)
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("console.title")
                .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: Text("console.search"))
                .toolbar {
                    // two buttons in one glass capsule, as the system apps do
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        filterMenu
                        Button { showAccount = true } label: { Image(systemName: "person.crop.circle") }
                            .accessibilityLabel(Text("acc.title"))
                    }
                }
        }
        .sheet(isPresented: $showAccount) { AccountView() }
        .fullScreenCover(item: $terminalCard) { TerminalScreen(card: $0) }
        .fullScreenCover(item: $remoteCard) { RemoteScreen(card: $0) }
        .sheet(item: $rdpLoginCard, onDismiss: {
            // the cover comes after the form is gone (two presentations at once do not work)
            if let p = rdpPending { rdpPending = nil; rdpTarget = p }
        }) { card in
            RdpLoginSheet(card: card) { params in
                rdpPending = RdpTarget(card: card, params: params)
                rdpLoginCard = nil
            }
        }
        .fullScreenCover(item: $rdpTarget) { RemoteScreen(card: $0.card, rdp: $0.params) }
        .fullScreenCover(item: $pageCard) { ServerPageScreen(card: $0) }
        .alert(notice?.title ?? "", isPresented: Binding(get: { notice != nil }, set: { if !$0 { notice = nil } })) {
            Button("common.ok", role: .cancel) {}
        } message: {
            Text(notice?.text ?? "")
        }
    }

    // MARK: content

    /// The kind of connection: the system segmented control. It is the header of the one section of the screen: it scrolls with the list and,
    /// reaching the bar, stays under it.
    private var kindBar: some View {
        GlassSegmented(items: kinds.map { k in (tag: k, title: kindTitle(k)) }, selection: Binding(get: { activeKind }, set: { kind = $0 }))
    }

    private func kindTitle(_ k: ConnectKind) -> LocalizedStringKey {
        switch k {
        case .terminal: "kind.terminal"
        case .rdp: "kind.rdp"
        case .assistant: "kind.assistant"
        }
    }

    private var content: some View {
        let list = rows
        return PinnedSwitchScroll(above: {
            if model.offline && !model.cards.isEmpty {
                OfflineBanner(since: model.updatedAt).padding(.horizontal, groupMargin).padding(.bottom, 8)
            }
        }, bar: {
            kindBar
        }, below: {
            if model.cards.isEmpty && model.loading {
                ProgressView().frame(maxWidth: .infinity).padding(.top, 40)
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
            } else if list.isEmpty {
                Group {
                    if filter.searching {
                        ContentUnavailableView.search(text: query)
                    } else {
                        ContentUnavailableView(filter.narrowed ? "console.nothing.filtered" : "console.nothing.kind", systemImage: "line.3.horizontal.decrease.circle")
                    }
                }
                .padding(.top, 24)
            } else {
                GroupCard {
                    ForEach(Array(list.enumerated()), id: \.element.id) { i, row in
                        switch row.content {
                        case .folder(let id, let name, let count, let open):
                            folderRow(id: id, name: name, count: count, open: open, depth: row.depth, first: i == 0)
                        case .server(let card, let path):
                            serverRow(card, path: path, depth: row.depth, first: i == 0)
                        }
                    }
                }
                .padding(.bottom, 16)
            }
        }, refresh: { await model.refresh() })
    }

    /// How far a row is moved in by its depth, and the line above it with it.
    private func indent(_ depth: Int, extra: CGFloat = 0) -> CGFloat { CGFloat(depth) * 22 + extra }

    private func folderRow(id: Int64, name: String, count: Int, open: Bool, depth: Int, first: Bool) -> some View {
        Button {
            withAnimation { toggle(id) }
        } label: {
            HStack(spacing: 10) {
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold)).foregroundStyle(.tertiary)
                    .rotationEffect(.degrees(open ? 90 : 0))
                    .frame(width: 14)
                Image(systemName: "folder").foregroundStyle(.tint)
                Text(name).fontWeight(.semibold).lineLimit(1)
                Spacer(minLength: 8)
                Text("\(count)").foregroundStyle(.secondary).monospacedDigit()
            }
            .padding(.leading, indent(depth))
            .frame(minHeight: rowHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .modifier(RowFrame(first: first, lineInset: 16 + indent(depth) + 24))
        .accessibilityLabel(Text(verbatim: name + ", \(count)"))
        .accessibilityValue(Text(LocalizedStringKey(open ? "dash.expanded" : "dash.collapsed")))
    }

    private func serverRow(_ card: ServerCard, path: [String], depth: Int, first: Bool) -> some View {
        let level = levelOf(card)
        let extra: CGFloat = path.isEmpty && depth > 0 ? 24 : 0
        return Button {
            open(card, activeKind)
        } label: {
            HStack(spacing: 10) {
                StatusDot(level: level)
                VStack(alignment: .leading, spacing: 1) {
                    Text(card.name).foregroundStyle(level == .off ? .secondary : .primary).lineLimit(1)
                    if !path.isEmpty {
                        Text(path.joined(separator: " › ")).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                    }
                }
                Spacer(minLength: 8)
                OSBadge(card: card)
            }
            .padding(.leading, indent(depth, extra: extra))
            .frame(minHeight: rowHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .modifier(RowFrame(first: first, lineInset: 16 + indent(depth, extra: extra) + 20))
        .contextMenu {
            Button { pageCard = card } label: { Label("ctx.page", systemImage: "info.circle") }
            ForEach(kinds.filter { $0.isReady && $0.offers(card) }, id: \.self) { k in
                Button { open(card, k) } label: {
                    Label(k == .terminal ? "ctx.terminal" : (k == .rdp ? "ctx.rdp" : "ctx.assistant"), systemImage: k == .terminal ? "apple.terminal" : (k == .rdp ? "display" : "cursorarrow.rays"))
                }
                .disabled(!card.online)
            }
            Button { UIPasteboard.general.string = card.name } label: { Label("ctx.copyName", systemImage: "doc.on.doc") }
        }
        .accessibilityLabel(Text(verbatim: card.name + ", " + level.title))
    }

    // MARK: actions

    private func toggle(_ id: Int64) {
        var s = folded
        if s.contains(id) { s.remove(id) } else { s.insert(id) }
        foldedRaw = Console.encode(s)
    }

    private func open(_ card: ServerCard, _ k: ConnectKind) {
        guard card.online else {
            notice = Notice(title: L("console.offline.title"), text: L("console.offline.text", card.name))
            return
        }
        switch k {
        case .terminal: terminalCard = card
        case .assistant: remoteCard = card
        case .rdp: rdpLoginCard = card
        }
    }

    // MARK: menu

    private var filterMenu: some View {
        Menu {
            Picker("console.system", selection: $system) {
                Text("console.system.all").tag(SystemFilter.all)
                Text("console.system.windows").tag(SystemFilter.windows)
                Text("console.system.linux").tag(SystemFilter.linux)
            }
            .pickerStyle(.inline)
            Picker("console.status", selection: $status) {
                Text("console.status.all").tag(StatusFilter.all)
                Text("console.status.online").tag(StatusFilter.online)
                Text("console.status.offline").tag(StatusFilter.offline)
            }
            .pickerStyle(.inline)
            if !model.folders.isEmpty {
                Section {
                    Button { withAnimation { foldedRaw = "" } } label: { Label("console.expandAll", systemImage: "chevron.down") }
                    Button { withAnimation { foldedRaw = Console.encode(Console.allFolderIds(model.folders)) } } label: { Label("console.collapseAll", systemImage: "chevron.right") }
                }
            }
        } label: {
            // a filled symbol says that a filter narrows the list
            Image(systemName: "ellipsis.circle").symbolVariant(filter.narrowed ? .fill : .none)
        }
        .accessibilityLabel(Text("dash.menu"))
        .accessibilityValue(Text(filter.narrowed ? "console.filtered" : ""))
    }
}
