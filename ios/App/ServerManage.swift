import LinagerKit
import SwiftUI

// The management pages of a server, pushed from "More": services, processes, Docker, the journal, the disk and the updates.

// MARK: services

struct ServicesPage: View {
    let card: ServerCard
    /// A Windows computer: its services have the start type "static" (by hand), which can be turned to autostart too.
    var windows = false
    @Environment(AppModel.self) private var model
    @State private var units: [ServiceUnit]?
    @State private var failure: LinagerError?
    @State private var query = ""
    @State private var filter: ServiceList.Filter = .all
    @State private var asking: Ask?
    @State private var busy: String?
    @State private var notes = NoteBox()

    struct Ask: Identifiable {
        let unit: ServiceUnit
        let action: ServiceAction
        /// The server itself said the action is dangerous.
        let dangerous: Bool
        var id: String { unit.id + action.rawValue }
    }

    var body: some View {
        let rows = ServiceList.arranged(units ?? [], query: query, only: filter)
        List {
            if units != nil {
                if rows.isEmpty {
                    ContentUnavailableView.search(text: query).listRowBackground(Color.clear)
                } else {
                    Section {
                        ForEach(rows) { row($0) }
                    } footer: {
                        Text(L("srv.svc.count", rows.count, (units ?? []).filter(\.isFailed).count))
                    }
                }
            } else {
                PageStatus(failure: failure, retry: load).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("srv.sec.services")
        .navigationBarTitleDisplayMode(.inline)
        .searchable(text: $query, prompt: Text("srv.svc.search"))
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Picker("srv.svc.filter", selection: $filter) {
                        Label("srv.svc.f.all", systemImage: "list.bullet").tag(ServiceList.Filter.all)
                        Label("srv.svc.f.running", systemImage: "play.circle").tag(ServiceList.Filter.running)
                        Label("srv.svc.f.failed", systemImage: "exclamationmark.triangle").tag(ServiceList.Filter.failed)
                        Label("srv.svc.f.stopped", systemImage: "stop.circle").tag(ServiceList.Filter.stopped)
                    }
                } label: { Image(systemName: filter == .all ? "line.3.horizontal.decrease.circle" : "line.3.horizontal.decrease.circle.fill") }
                    .accessibilityLabel(Text("srv.svc.filter"))
            }
        }
        .refreshable { await load() }
        .refreshing { await load() }
        .noteOverlay(notes.text)
        .confirmationDialog(asking.map { L("srv.svc.confirm", actionTitle($0.action), $0.unit.shortName) } ?? "",
                            isPresented: Binding(get: { asking != nil }, set: { if !$0 { asking = nil } }), titleVisibility: .visible, presenting: asking) { a in
            Button(actionTitle(a.action), role: .destructive) { Task { await run(a.unit, a.action, confirm: true) } }
            Button("common.cancel", role: .cancel) {}
        } message: { a in
            if a.dangerous { Text("srv.svc.dangerous") }
        }
    }

    private func row(_ u: ServiceUnit) -> some View {
        HStack(spacing: 12) {
            Circle().fill(color(u)).frame(width: 9, height: 9).accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text(u.shortName).lineLimit(1)
                Text(subtitle(u)).font(.caption).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer(minLength: 0)
            if busy == u.id {
                ProgressView()
            } else {
                Menu { actions(u) } label: { Image(systemName: "ellipsis.circle").foregroundStyle(.secondary).frame(width: 44, height: 44).contentShape(Rectangle()) }
                    .buttonStyle(.plain)
                    .accessibilityLabel(Text(L("srv.svc.actionsFor", u.shortName)))
            }
        }
        .contextMenu { actions(u) }
    }

    @ViewBuilder private func actions(_ u: ServiceUnit) -> some View {
        if u.isRunning {
            Button { request(u, .restart) } label: { Label(actionTitle(.restart), systemImage: "arrow.clockwise") }
            Button(role: .destructive) { request(u, .stop) } label: { Label(actionTitle(.stop), systemImage: "stop.fill") }
        } else {
            Button { request(u, .start) } label: { Label(actionTitle(.start), systemImage: "play.fill") }
        }
        Section {
            if u.enabled == "enabled" {
                Button(role: .destructive) { request(u, .disable) } label: { Label(actionTitle(.disable), systemImage: "poweroff") }
            } else if u.enabled == "disabled" || (windows && u.enabled == "static") {
                Button { request(u, .enable) } label: { Label(actionTitle(.enable), systemImage: "power") }
            }
            Button { UIPasteboard.general.string = u.name; notes.show(L("srv.files.copied")) } label: { Label("ctx.copyName", systemImage: "textformat") }
        }
    }

    private func color(_ u: ServiceUnit) -> Color { u.isFailed ? .red : (u.isRunning ? .green : .secondary.opacity(0.5)) }

    private func subtitle(_ u: ServiceUnit) -> String {
        var parts = [stateTitle(u)]
        if u.enabled == "enabled" { parts.append(L("srv.svc.autostart")) }
        if !u.description.isEmpty { parts.append(u.description) }
        return parts.joined(separator: " · ")
    }

    private func stateTitle(_ u: ServiceUnit) -> String {
        if u.isFailed { return L("srv.svc.failed") }
        if u.isRunning { return L("srv.svc.running") }
        return u.active == "activating" ? L("srv.svc.starting") : L("srv.svc.stopped")
    }

    private func actionTitle(_ a: ServiceAction) -> String {
        switch a {
        case .start: L("srv.svc.act.start")
        case .stop: L("srv.svc.act.stop")
        case .restart: L("srv.svc.act.restart")
        case .enable: L("srv.svc.act.enable")
        case .disable: L("srv.svc.act.disable")
        }
    }

    /// Stopping and turning off are asked about; the rest goes at once (and a dangerous one is asked about when the server says so).
    private func request(_ u: ServiceUnit, _ a: ServiceAction) {
        if a == .stop || a == .disable {
            asking = Ask(unit: u, action: a, dangerous: false)
        } else {
            Task { await run(u, a, confirm: false) }
        }
    }

    private func run(_ u: ServiceUnit, _ a: ServiceAction, confirm: Bool) async {
        busy = u.id
        defer { busy = nil }
        do {
            try await model.serviceAction(serverID: card.id, unit: u.name, action: a, confirm: confirm)
            notes.show(L("srv.svc.done", actionTitle(a), u.shortName))
            await load()
        } catch LinagerError.confirmRequired {
            asking = Ask(unit: u, action: a, dangerous: true)
        } catch {
            notes.fail(error)
        }
    }

    private func load() async {
        do {
            units = try await model.services(serverID: card.id)
            failure = nil
        } catch let e as LinagerError {
            if units == nil { failure = e }
        } catch {
            if units == nil { failure = .unreachable(error.localizedDescription) }
        }
    }
}

// MARK: processes

struct ProcessesPage: View {
    let card: ServerCard
    /// A Windows computer: "End" ends that process, the other ends it with everything it started.
    var windows = false
    @Environment(AppModel.self) private var model
    @State private var list: ProcessList?
    @State private var failure: LinagerError?
    @AppStorage("srv.proc.sort") private var sortRaw = ProcessSort.cpu.rawValue
    @State private var asking: Ask?
    @State private var notes = NoteBox()

    struct Ask: Identifiable {
        let process: ProcessItem
        let signal: String
        var id: String { "\(process.pid)\(signal)" }
    }

    private var sort: ProcessSort { ProcessSort(rawValue: sortRaw) ?? .cpu }

    var body: some View {
        List {
            Section {
                GlassSegmented(items: [(tag: ProcessSort.cpu, title: LocalizedStringKey("srv.proc.cpu")), (tag: .mem, title: LocalizedStringKey("srv.proc.mem")), (tag: .name, title: LocalizedStringKey("srv.proc.name"))],
                               selection: Binding(get: { sort }, set: { sortRaw = $0.rawValue }))
                    .frame(height: 44)
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
            }
            if let list {
                Section {
                    ForEach(list.processes) { row($0) }
                } footer: {
                    Text(L("srv.proc.count", list.processes.count, list.total))
                }
            } else {
                PageStatus(failure: failure, retry: load).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("srv.sec.processes")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .refreshing { await load() }
        .onChange(of: sortRaw) { Task { await load() } }
        .noteOverlay(notes.text)
        .confirmationDialog(asking.map { L($0.signal == "KILL" ? "srv.proc.confirmKill" : "srv.proc.confirmTerm", $0.process.program, $0.process.pid) } ?? "",
                            isPresented: Binding(get: { asking != nil }, set: { if !$0 { asking = nil } }), titleVisibility: .visible, presenting: asking) { a in
            Button(L(a.signal == "KILL" ? (windows ? "srv.proc.winKill" : "srv.proc.kill") : "srv.proc.term"), role: .destructive) { Task { await signal(a) } }
            Button("common.cancel", role: .cancel) {}
        } message: { a in
            Text(L(a.signal == "KILL" ? (windows ? "srv.proc.winKillText" : "srv.proc.killText") : (windows ? "srv.proc.winTermText" : "srv.proc.termText")))
        }
    }

    private func row(_ p: ProcessItem) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline) {
                Text(p.program.isEmpty ? "?" : p.program).font(.body.weight(.medium)).lineLimit(1)
                Spacer(minLength: 8)
                Text(String(format: "%.1f%%", p.cpu)).font(.subheadline.monospacedDigit()).foregroundStyle(meter(p.cpu).color)
            }
            HStack(spacing: 10) {
                Text("PID \(p.pid)")
                Text(p.user)
                Text(L("srv.proc.mem2", Int(p.memPct.rounded()), sizeText(p.rss)))
                Spacer(minLength: 0)
            }
            .font(.caption).foregroundStyle(.secondary).lineLimit(1)
            if p.cmd != p.program {
                Text(p.cmd).font(.caption2.monospaced()).foregroundStyle(.tertiary).lineLimit(2).truncationMode(.middle)
            }
        }
        .padding(.vertical, 2)
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button(role: .destructive) { asking = Ask(process: p, signal: "KILL") } label: { Label(LocalizedStringKey(windows ? "srv.proc.winKill" : "srv.proc.kill"), systemImage: "xmark.octagon") }
            Button { asking = Ask(process: p, signal: "TERM") } label: { Label("srv.proc.term", systemImage: "stop.circle") }.tint(.orange)
        }
        .contextMenu {
            Button { asking = Ask(process: p, signal: "TERM") } label: { Label("srv.proc.term", systemImage: "stop.circle") }
            Button(role: .destructive) { asking = Ask(process: p, signal: "KILL") } label: { Label(LocalizedStringKey(windows ? "srv.proc.winKill" : "srv.proc.kill"), systemImage: "xmark.octagon") }
            Button { UIPasteboard.general.string = p.cmd; notes.show(L("srv.files.copied")) } label: { Label("srv.proc.copy", systemImage: "doc.on.doc") }
        }
    }

    private func signal(_ a: Ask) async {
        do {
            try await model.killProcess(serverID: card.id, pid: a.process.pid, signal: a.signal)
            notes.show(L("srv.proc.done", a.process.program))
            await load()
        } catch {
            notes.fail(error)
        }
    }

    private func load() async {
        do {
            list = try await model.processes(serverID: card.id, sort: sort)
            failure = nil
        } catch let e as LinagerError {
            if list == nil { failure = e }
        } catch {
            if list == nil { failure = .unreachable(error.localizedDescription) }
        }
    }
}

// MARK: Docker

struct DockerPage: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @State private var info: DockerInfo?
    @State private var failure: LinagerError?
    @State private var busy: String?
    @State private var removing: DockerContainer?
    @State private var notes = NoteBox()

    var body: some View {
        List {
            if let info {
                if !info.available {
                    ContentUnavailableView("srv.dock.none", systemImage: "shippingbox").listRowBackground(Color.clear)
                } else {
                    Section {
                        if info.containers.isEmpty {
                            Text("srv.dock.noContainers").foregroundStyle(.secondary)
                        }
                        ForEach(info.containers) { row($0) }
                    } header: {
                        Text("srv.dock.containers")
                    }
                    if !info.images.isEmpty {
                        Section {
                            ForEach(info.images) { image in
                                HStack {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(image.title).lineLimit(1).truncationMode(.middle)
                                        Text(image.used ? L("srv.dock.used") : L("srv.dock.unused")).font(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    Text(sizeText(image.size)).font(.subheadline.monospacedDigit()).foregroundStyle(.secondary)
                                }
                            }
                        } header: {
                            Text("srv.dock.images")
                        }
                    }
                }
            } else {
                PageStatus(failure: failure, retry: load).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("srv.sec.docker")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .refreshing { await load() }
        .noteOverlay(notes.text)
        .confirmationDialog(removing.map { L("srv.dock.confirmRemove", $0.name) } ?? "",
                            isPresented: Binding(get: { removing != nil }, set: { if !$0 { removing = nil } }), titleVisibility: .visible, presenting: removing) { c in
            Button("srv.dock.remove", role: .destructive) { Task { await run(c, .remove) } }
            Button("common.cancel", role: .cancel) {}
        } message: { _ in Text("srv.dock.removeText") }
    }

    private func row(_ c: DockerContainer) -> some View {
        HStack(spacing: 12) {
            Circle().fill(c.isRunning ? Color.green : Color.secondary.opacity(0.5)).frame(width: 9, height: 9).accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(c.name).lineLimit(1)
                    if c.hasNewImage {
                        Text("srv.dock.newImage").font(.caption2.weight(.semibold)).foregroundStyle(.orange)
                            .padding(.horizontal, 6).padding(.vertical, 1).background(Color.orange.opacity(0.15), in: Capsule())
                    }
                }
                Text(c.image).font(.caption).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                Text(detail(c)).font(.caption2).foregroundStyle(.tertiary).lineLimit(1)
            }
            Spacer(minLength: 0)
            if busy == c.id {
                ProgressView()
            } else {
                Menu { actions(c) } label: { Image(systemName: "ellipsis.circle").foregroundStyle(.secondary).frame(width: 44, height: 44).contentShape(Rectangle()) }
                    .buttonStyle(.plain)
                    .accessibilityLabel(Text(L("srv.svc.actionsFor", c.name)))
            }
        }
        .contextMenu { actions(c) }
    }

    @ViewBuilder private func actions(_ c: DockerContainer) -> some View {
        if c.isRunning {
            Button { Task { await run(c, .restart) } } label: { Label("srv.svc.act.restart", systemImage: "arrow.clockwise") }
            Button { Task { await run(c, .stop) } } label: { Label("srv.svc.act.stop", systemImage: "stop.fill") }
        } else {
            Button { Task { await run(c, .start) } } label: { Label("srv.svc.act.start", systemImage: "play.fill") }
        }
        Section {
            Button(role: .destructive) { removing = c } label: { Label("srv.dock.remove", systemImage: "trash") }
        }
    }

    private func detail(_ c: DockerContainer) -> String {
        var parts = [c.status]
        if c.isRunning {
            parts.append(String(format: "CPU %.1f%%", c.cpu))
            if c.memUsed > 0 { parts.append(Fmt.bytes(c.memUsed)) }
        }
        if !c.ports.isEmpty { parts.append(c.ports.prefix(2).joined(separator: ", ")) }
        return parts.filter { !$0.isEmpty }.joined(separator: " · ")
    }

    private func run(_ c: DockerContainer, _ a: DockerAction) async {
        busy = c.id
        defer { busy = nil }
        do {
            try await model.dockerAction(serverID: card.id, containerID: c.id, action: a)
            notes.show(L("srv.dock.done", c.name))
            await load()
        } catch {
            notes.fail(error)
        }
    }

    private func load() async {
        do {
            info = try await model.docker(serverID: card.id)
            failure = nil
        } catch let e as LinagerError {
            if info == nil { failure = e }
        } catch {
            if info == nil { failure = .unreachable(error.localizedDescription) }
        }
    }
}

// MARK: disk

struct DiskPage: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @State private var state: DiskState?
    @State private var failure: LinagerError?
    @State private var cleaning: DiskCleanItem?
    @State private var busy: String?
    @State private var notes = NoteBox()

    var body: some View {
        List {
            if let state {
                Section("srv.disk.fs") {
                    ForEach(state.filesystems) { fs in
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Text(fs.mount).font(.body.weight(.medium)).lineLimit(1)
                                Text(fs.fs).font(.caption).foregroundStyle(.secondary)
                                Spacer()
                                Text("\(Int(fs.pct.rounded()))%").font(.headline.monospacedDigit()).foregroundStyle(meter(fs.pct).color)
                            }
                            ProgressView(value: min(1, max(0, fs.pct / 100))).tint(meter(fs.pct).color)
                            HStack {
                                Text(L("srv.disk.usage", Fmt.bytes(fs.used), Fmt.bytes(fs.total)))
                                Spacer()
                                Text(L("srv.disk.free", Fmt.bytes(fs.avail)))
                            }
                            .font(.caption).foregroundStyle(.secondary)
                            if fs.daysLeft > 0 {
                                Label(L("srv.disk.forecast", Int(fs.daysLeft.rounded()), Fmt.bytes(Int64(fs.perDay))), systemImage: "chart.line.uptrend.xyaxis")
                                    .font(.caption).foregroundStyle(fs.daysLeft < 14 ? .orange : .secondary)
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
                if !state.cleanup.isEmpty {
                    Section {
                        ForEach(state.cleanup) { item in
                            HStack {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(title(item.kind))
                                    Text(L("srv.disk.frees", Fmt.bytes(item.frees))).font(.caption).foregroundStyle(.secondary)
                                }
                                Spacer()
                                if busy == item.kind {
                                    ProgressView()
                                } else {
                                    Button("srv.disk.clean") { cleaning = item }
                                        .buttonStyle(.bordered)
                                        .disabled(item.frees <= 0 || busy != nil)
                                }
                            }
                        }
                    } header: {
                        Text("srv.disk.cleanup")
                    } footer: {
                        Text("srv.disk.cleanupText")
                    }
                }
            } else {
                PageStatus(failure: failure, retry: load).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("srv.sec.disk")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .refreshing(every: 15) { await load() }
        .noteOverlay(notes.text)
        .confirmationDialog(cleaning.map { L("srv.disk.confirm", title($0.kind)) } ?? "",
                            isPresented: Binding(get: { cleaning != nil }, set: { if !$0 { cleaning = nil } }), titleVisibility: .visible, presenting: cleaning) { item in
            Button("srv.disk.clean", role: .destructive) { Task { await clean(item) } }
            Button("common.cancel", role: .cancel) {}
        } message: { item in
            Text(L("srv.disk.confirmText", Fmt.bytes(item.frees)))
        }
    }

    private func title(_ kind: String) -> String {
        switch kind {
        case "journal": L("srv.disk.k.journal")
        case "apt": L("srv.disk.k.apt")
        case "docker-images": L("srv.disk.k.dockerImages")
        case "docker-build": L("srv.disk.k.dockerBuild")
        case "kernels": L("srv.disk.k.kernels")
        case "win-temp": L("srv.disk.k.winTemp")
        case "win-update-cache": L("srv.disk.k.winUpdateCache")
        case "win-recycle": L("srv.disk.k.winRecycle")
        case "win-delivery": L("srv.disk.k.winDelivery")
        case "win-dumps": L("srv.disk.k.winDumps")
        case "win-old": L("srv.disk.k.winOld")
        default: kind
        }
    }

    private func clean(_ item: DiskCleanItem) async {
        busy = item.kind
        defer { busy = nil }
        do {
            let r = try await model.diskClean(serverID: card.id, kind: item.kind)
            notes.show(L("srv.disk.freed", Fmt.bytes(r.freed)))
            await load()
        } catch {
            notes.fail(error)
        }
    }

    private func load() async {
        do {
            state = try await model.disk(serverID: card.id)
            failure = nil
        } catch let e as LinagerError {
            if state == nil { failure = e }
        } catch {
            if state == nil { failure = .unreachable(error.localizedDescription) }
        }
    }
}
