import LinagerKit
import SwiftUI

// The pages of a server that read a stream of text: the journal, and the jobs of the updates.

/// The lines of a stream as a view shows them. The text comes in many small pieces, so it is gathered and put on the screen a few times a second.
@MainActor @Observable
final class LogModel {
    enum Phase: Equatable { case connecting, open, closed(String), failed(LinagerError) }

    private(set) var lines: [String] = [""]
    /// How many lines went out at the top: the number of a line stays what it was, so the view moves nothing.
    private(set) var firstID = 0
    private(set) var phase: Phase = .connecting
    private var pending = ""
    private var scheduled = false
    private let limit: Int

    init(limit: Int = 2000) { self.limit = limit }

    func reset() {
        lines = [""]
        firstID = 0
        pending = ""
        phase = .connecting
    }

    func opened() { phase = .open }

    func finished(_ reason: String = "") { flush(); phase = .closed(reason) }

    func failed(_ e: LinagerError) { flush(); phase = .failed(e) }

    func feed(_ text: String) {
        guard !text.isEmpty else { return }
        pending += text
        guard !scheduled else { return }
        scheduled = true
        Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(150))
            self?.flush()
        }
    }

    private func flush() {
        scheduled = false
        guard !pending.isEmpty else { return }
        LogText.append(pending, to: &lines, limit: Int.max)
        pending = ""
        if lines.count > limit {
            let cut = lines.count - limit
            lines.removeFirst(cut)
            firstID += cut
        }
    }

    /// The lines with their numbers.
    var rows: [Row] { lines.enumerated().map { Row(id: firstID + $0.offset, text: $0.element) } }

    struct Row: Identifiable { let id: Int; let text: String }
}

/// The text of a stream in a scrolling box; it follows the end until the person scrolls up.
struct LogBox: View {
    let log: LogModel
    @Binding var follow: Bool

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(log.rows) { row in
                        Text(row.text.isEmpty ? " " : row.text)
                            .font(.system(size: 11.5, design: .monospaced))
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .textSelection(.enabled)
                    }
                    Color.clear.frame(height: 1).id("end")
                }
                .padding(.horizontal, 12).padding(.vertical, 8)
            }
            .onScrollPhaseChange { _, new in if new == .interacting { follow = false } }
            .onChange(of: log.lines.count) { if follow { proxy.scrollTo("end", anchor: .bottom) } }
            .onChange(of: follow) { _, on in if on { proxy.scrollTo("end", anchor: .bottom) } }
            .defaultScrollAnchor(.bottom)
        }
    }
}

// MARK: the journal

struct LogsPage: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @State private var log = LogModel()
    @State private var field = ""
    @State private var unit = ""
    @AppStorage("srv.log.priority") private var priority = 6
    @State private var kernel = false
    @State private var follow = true

    private struct Key: Equatable { let unit: String; let priority: Int; let kernel: Bool }

    var body: some View {
        VStack(spacing: 0) {
            controls
            Divider()
            LogBox(log: log, follow: $follow)
                .background(Color(.secondarySystemGroupedBackground))
                .overlay { overlay }
        }
        .navigationTitle("srv.sec.logs")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { follow = true } label: { Image(systemName: follow ? "arrow.down.circle.fill" : "arrow.down.circle") }
                    .accessibilityLabel(Text("srv.log.follow"))
            }
        }
        .task(id: Key(unit: unit, priority: priority, kernel: kernel)) { await stream() }
    }

    private var controls: some View {
        VStack(spacing: 10) {
            HStack(spacing: 10) {
                Image(systemName: "magnifyingglass").foregroundStyle(.secondary)
                TextField("srv.log.unit", text: $field)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.search)
                    .onSubmit { unit = field.trimmingCharacters(in: .whitespaces) }
                if !field.isEmpty {
                    Button { field = ""; unit = "" } label: { Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary) }.buttonStyle(.plain)
                        .accessibilityLabel(Text("common.close"))
                }
            }
            .padding(.horizontal, 12).frame(minHeight: 40)
            .background(Color(.secondarySystemGroupedBackground), in: Capsule())
            HStack(spacing: 10) {
                Menu {
                    Picker("srv.log.level", selection: $priority) {
                        Text("srv.log.p7").tag(7)
                        Text("srv.log.p6").tag(6)
                        Text("srv.log.p4").tag(4)
                        Text("srv.log.p3").tag(3)
                    }
                } label: {
                    Label(levelTitle, systemImage: "slider.horizontal.3").font(.subheadline)
                }
                .buttonStyle(.bordered)
                Toggle(isOn: $kernel) { Label("srv.log.kernel", systemImage: "cpu").font(.subheadline) }
                    .toggleStyle(.button)
                Spacer(minLength: 0)
                Text(statusText).font(.caption).foregroundStyle(.secondary)
            }
        }
        .padding(.horizontal, 16).padding(.vertical, 10)
    }

    @ViewBuilder private var overlay: some View {
        switch log.phase {
        case .failed(let e): ContentUnavailableView { Label("srv.files.failed", systemImage: "exclamationmark.triangle") } description: { Text(e.text()) }.background(.background)
        case .connecting: ProgressView()
        default: EmptyView()
        }
    }

    private var levelTitle: String {
        switch priority {
        case 7: L("srv.log.p7")
        case 4: L("srv.log.p4")
        case 3: L("srv.log.p3")
        default: L("srv.log.p6")
        }
    }

    private var statusText: String {
        switch log.phase {
        case .connecting: L("srv.log.connecting")
        case .open: "journalctl"
        case .closed: L("srv.log.closed")
        case .failed: ""
        }
    }

    private func stream() async {
        log.reset()
        follow = true
        do {
            let connection = try await model.openStream(serverID: card.id, stream: .logs(unit: unit, priority: priority, kernel: kernel, lines: 200))
            await withTaskCancellationHandler {
                log.opened()
                var decoder = Utf8Stream()
                for await event in connection.events {
                    switch event {
                    case .output(let data): log.feed(decoder.append(data))
                    case .closed(let reason, _): log.finished(reason)
                    }
                }
                if log.phase == .open { log.finished() }
            } onCancel: {
                connection.close()
            }
        } catch let e as LinagerError {
            log.failed(e)
        } catch {
            log.failed(.unreachable(error.localizedDescription))
        }
    }
}

// MARK: updates

/// A job of the updates as the page shows it: no console, each package tells its own state (waiting, working, done, failed) as the agent reports it.
@MainActor @Observable
final class UpdateRun {
    enum Phase: Equatable { case running, finished, failed(String) }

    let title: String
    /// The text of a Windows computer is read by its own parser.
    let windows: Bool
    private(set) var run = AptRun()
    private(set) var phase: Phase = .running

    init(title: String, windows: Bool = false) {
        self.title = title
        self.windows = windows
    }

    /// Reads the stream until it ends, then settles the job: a package that is still open is closed by the agent itself.
    func consume(_ connection: TerminalConnection) async {
        var bytes = Utf8Stream()
        var parser = AptStreamParser()
        var winParser = WinUpdateStreamParser()
        var closedReason = ""
        for await event in connection.events {
            switch event {
            case .output(let data):
                let piece = bytes.append(data)
                if windows {
                    for e in winParser.feed(piece) { run.apply(e) }
                    if let why = WinUpdateStreamParser.failure(in: piece) { run.fail(why) }
                } else {
                    let (text, events) = parser.feed(piece)
                    for e in events { run.apply(e) }
                    run.note(text: text)
                }
            case .closed(let reason, _):
                closedReason = reason
            }
        }
        if Task.isCancelled { return }
        finish(closedReason)
    }

    func fail(_ error: LinagerError) { phase = .failed(error.text()) }

    private func finish(_ reason: String) {
        if run.hasFailure {
            let named = run.order.first { run.state($0) == .error }.map { run.messages[$0] ?? $0 }
            phase = .failed(run.lastError ?? named ?? L("srv.upd.failed"))
        } else {
            phase = .finished
        }
    }

    var running: Bool { phase == .running }
}

/// The line over the packages of a job: what it is doing now and how far it is, then how it ended.
struct UpdateRunHeader: View {
    let run: UpdateRun
    let dismiss: () -> Void

    var body: some View {
        switch run.phase {
        case .running:
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 10) {
                    ProgressView().controlSize(.small)
                    Text(stageText).font(.subheadline.weight(.medium))
                    Spacer()
                    if run.run.total > 0 { Text(L("srv.upd.count", run.run.doneCount, run.run.total)).font(.subheadline.monospacedDigit()).foregroundStyle(.secondary) }
                }
                if run.run.stage != nil { progressBar }
            }
        case .finished:
            HStack(spacing: 10) {
                Image(systemName: "checkmark.circle.fill").foregroundStyle(.green).font(.title3)
                Text(run.run.total > 0 ? L("srv.upd.updated", run.run.doneCount) : L("srv.upd.checkedNow")).font(.subheadline.weight(.medium))
                Spacer()
                Button("srv.upd.dismiss", action: dismiss).buttonStyle(.bordered).controlSize(.small)
            }
        case .failed(let message):
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 10) {
                    Image(systemName: "xmark.circle.fill").foregroundStyle(.red).font(.title3)
                    Text(run.run.doneCount > 0 ? L("srv.upd.partly", run.run.doneCount, run.run.total) : L("srv.upd.failed")).font(.subheadline.weight(.medium))
                    Spacer()
                    Button("srv.upd.dismiss", action: dismiss).buttonStyle(.bordered).controlSize(.small)
                }
                Text(message).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            }
        }
    }

    /// The bar of the phase: as far as the percent says, or going back and forth when the phase has none (Windows Update).
    @ViewBuilder private var progressBar: some View {
        if let o = run.run.overall { ProgressView(value: o).animation(.default, value: run.run.pct) } else { ProgressView().progressViewStyle(.linear) }
    }

    private var stageText: String {
        let r = run.run
        switch r.stage {
        case "download": return r.pct >= 0 ? L("srv.upd.stage.download", r.pct) : L("srv.upd.stage.downloadNoPct")
        case "install": return r.pct >= 0 ? L("srv.upd.stage.install", r.pct) : L("srv.upd.stage.installNoPct")
        default: return run.title
        }
    }
}

struct UpdatesPage: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @State private var list: AptList?
    @State private var failure: LinagerError?
    @State private var current: UpdateRun?
    @State private var task: Task<Void, Never>?
    @State private var asking: Job?
    @State private var notes = NoteBox()

    struct Job: Identifiable {
        let title: String
        let stream: ServerStream
        /// Changes the system: asked about first.
        let confirm: Bool
        var id: String { title }
    }

    private var busy: Bool { (current?.running ?? false) || (list?.busy ?? false) }

    var body: some View {
        List {
            if let list {
                Section { header(list) }
                if let run = current, !run.run.order.isEmpty {
                    Section {
                        ForEach(run.run.order, id: \.self) { name in jobRow(name, run: run.run, from: list) }
                    } header: {
                        Text(L("srv.upd.thisRun", run.run.total))
                    }
                }
                let rest = list.packages.filter { p in !(current?.run.order.contains(p.name) ?? false) }
                if rest.isEmpty && (current?.run.order.isEmpty ?? true) {
                    Section { Label("srv.upd.uptodate", systemImage: "checkmark.circle.fill").foregroundStyle(.green) }
                } else if !rest.isEmpty {
                    Section {
                        ForEach(rest) { row($0) }
                    } header: {
                        Text(L("srv.upd.packages", rest.filter { !$0.ignored }.count))
                    }
                }
            } else {
                PageStatus(failure: failure, retry: load).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("srv.sec.updates")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button { start(Job(title: L("srv.upd.checking"), stream: .apt(.update), confirm: false)) } label: { Label("srv.upd.check", systemImage: "arrow.triangle.2.circlepath") }
                    Section {
                        Button { start(Job(title: L("srv.upd.installing"), stream: .apt(.upgrade), confirm: true)) } label: { Label("srv.upd.installAll", systemImage: "arrow.down.circle") }
                        Button { start(Job(title: L("srv.upd.installing"), stream: .apt(.security), confirm: true)) } label: { Label("srv.upd.installSecurity", systemImage: "lock.shield") }
                    }
                    .disabled((list?.counted.isEmpty ?? true))
                } label: { Image(systemName: "ellipsis.circle") }
                    .disabled(!card.online || busy)
                    .accessibilityLabel(Text("dash.menu"))
            }
        }
        .refreshable { await load() }
        .refreshing(every: 10) { await load() }
        .noteOverlay(notes.text)
        .onDisappear { task?.cancel() } // the job goes on at the agent; the page picks it up again when it is opened
        .confirmationDialog(asking?.title ?? "", isPresented: Binding(get: { asking != nil }, set: { if !$0 { asking = nil } }), titleVisibility: .visible, presenting: asking) { a in
            Button(L("srv.upd.install"), role: .destructive) { run(a) }
            Button("common.cancel", role: .cancel) {}
        } message: { _ in Text("srv.upd.confirmText") }
    }

    // MARK: pieces

    @ViewBuilder private func header(_ list: AptList) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            if let run = current {
                UpdateRunHeader(run: run) { current = nil }
            } else {
                HStack {
                    Text(list.lastCheck > 0 ? L("srv.upd.checked", Fmt.shortDate(Date(timeIntervalSince1970: TimeInterval(list.lastCheck)))) : L("srv.upd.neverChecked"))
                        .font(.subheadline).foregroundStyle(.secondary)
                    Spacer()
                    if list.busy { ProgressView().controlSize(.small) }
                }
            }
            if list.securityCount > 0 && current == nil {
                Label(L("srv.upd.security", list.securityCount), systemImage: "exclamationmark.shield.fill").foregroundStyle(.orange).font(.subheadline)
            }
            if list.rebootRequired {
                Label("srv.upd.rebootNeeded", systemImage: "arrow.clockwise.circle.fill").foregroundStyle(.orange).font(.subheadline)
            }
        }
        .animation(.default, value: current?.phase)
    }

    /// A package of the job: its versions and its own state, as the App Store shows an app that is being updated.
    private func jobRow(_ name: String, run: AptRun, from list: AptList) -> some View {
        let p = list.packages.first { $0.name == name } ?? run.info[name] ?? AptPackage(name: name)
        let state = run.state(name)
        return HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(p.name).lineLimit(1)
                    if p.security { Image(systemName: "lock.shield.fill").font(.caption).foregroundStyle(.orange) }
                }
                if state == .error, let m = run.messages[name] {
                    Text(m).font(.caption).foregroundStyle(.red).lineLimit(2)
                } else if !p.candidate.isEmpty {
                    Text("\(p.current) → \(p.candidate)").font(.caption.monospaced()).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                } else {
                    Text(stateText(state)).font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 0)
            StateMark(state: state)
        }
        .animation(.default, value: state)
    }

    private func stateText(_ s: PackageState) -> String {
        switch s {
        case .queued: L("srv.upd.state.queued")
        case .running: L("srv.upd.state.running")
        case .done: L("srv.upd.state.done")
        case .error: L("srv.upd.state.error")
        case .skipped: L("srv.upd.state.skipped")
        }
    }

    private func row(_ p: AptPackage) -> some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(p.name).lineLimit(1)
                    if p.security { Image(systemName: "lock.shield.fill").font(.caption).foregroundStyle(.orange).accessibilityLabel(Text("srv.upd.securityBadge")) }
                    if p.reboot { Image(systemName: "arrow.clockwise").font(.caption).foregroundStyle(.secondary).accessibilityLabel(Text("srv.upd.rebootBadge")) }
                }
                Text("\(p.current) → \(p.candidate)").font(.caption.monospaced()).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
            }
            Spacer(minLength: 0)
            if p.ignored { Text("srv.upd.ignored").font(.caption).foregroundStyle(.tertiary) }
        }
        .opacity(p.ignored ? 0.55 : 1)
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button { start(Job(title: L("srv.upd.installOne", p.name), stream: .apt(.packages([p.name])), confirm: true)) } label: { Label("srv.upd.install", systemImage: "arrow.down.circle") }
                .tint(.accentColor)
                .disabled(busy || !card.online)
        }
    }

    // MARK: running

    private func start(_ j: Job) {
        guard !busy else { return }
        if j.confirm { asking = j } else { run(j) }
    }

    /// Opens the stream of the job and reads it; the packages of the page change as the events come.
    private func run(_ j: Job) {
        let r = UpdateRun(title: j.title)
        current = r
        task?.cancel()
        task = Task {
            do {
                let connection = try await model.openStream(serverID: card.id, stream: j.stream)
                await withTaskCancellationHandler { await r.consume(connection) } onCancel: { connection.close() }
            } catch let e as LinagerError {
                r.fail(e)
            } catch {
                r.fail(.unreachable(error.localizedDescription))
            }
            if !Task.isCancelled { await load() }
            // a check that found nothing to tell goes away by itself; an installation stays until the person says "Done"
            if r.phase == .finished && r.run.total == 0 && !Task.isCancelled {
                try? await Task.sleep(for: .seconds(2))
                if current === r { current = nil }
            }
        }
    }

    private func load() async {
        do {
            let fresh = try await model.apt(serverID: card.id)
            list = fresh
            failure = nil
            // a job of the agent that was started before this page was opened (or by the desktop application): the stream of "check" does not
            // start anything when no job runs, and attaches to the running one with its events from the start
            if fresh.busy && current == nil && task == nil { attach() }
        } catch let e as LinagerError {
            if list == nil { failure = e }
        } catch {
            if list == nil { failure = .unreachable(error.localizedDescription) }
        }
    }

    private func attach() { run(Job(title: L("srv.upd.working"), stream: .apt(.update), confirm: false)) }
}

/// The mark at the right of a package of a job: waiting, working, done, failed.
struct StateMark: View {
    let state: PackageState

    var body: some View {
        Group {
            switch state {
            case .queued:
                Image(systemName: "clock").foregroundStyle(.tertiary)
            case .running:
                ProgressView()
            case .done:
                Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                    .symbolEffect(.bounce, value: state)
            case .error:
                Image(systemName: "xmark.circle.fill").foregroundStyle(.red)
                    .symbolEffect(.bounce, value: state)
            case .skipped:
                Image(systemName: "minus.circle").foregroundStyle(.secondary)
            }
        }
        .font(.title3)
        .frame(width: 28, height: 28)
        .transition(.scale.combined(with: .opacity))
        .accessibilityLabel(Text(label))
    }

    private var label: String {
        switch state {
        case .queued: L("srv.upd.state.queued")
        case .running: L("srv.upd.state.running")
        case .done: L("srv.upd.state.done")
        case .error: L("srv.upd.state.error")
        case .skipped: L("srv.upd.state.skipped")
        }
    }
}


// MARK: Windows Update

struct WinUpdatesPage: View {
    let card: ServerCard
    @Environment(AppModel.self) private var model
    @State private var list: WinUpdateList?
    @State private var failure: LinagerError?
    @State private var searching = false
    @State private var current: UpdateRun?
    @State private var task: Task<Void, Never>?
    @State private var asking: Ask?
    @State private var notes = NoteBox()

    struct Ask: Identifiable {
        let title: String
        let ids: [String]
        var id: String { title }
    }

    private var busy: Bool { searching || (current?.running ?? false) }

    var body: some View {
        List {
            if let list {
                Section { header(list) }
                if let run = current, !run.run.order.isEmpty {
                    Section {
                        ForEach(run.run.order, id: \.self) { title in jobRow(title, run: run.run, from: list) }
                    } header: {
                        Text(L("srv.upd.thisRun", run.run.total))
                    }
                }
                let rest = list.updates.filter { u in !(current?.run.order.contains(u.title) ?? false) }
                if rest.isEmpty && (current?.run.order.isEmpty ?? true) {
                    Section { Label("srv.upd.uptodate", systemImage: "checkmark.circle.fill").foregroundStyle(.green) }
                } else if !rest.isEmpty {
                    Section {
                        ForEach(rest) { row($0) }
                    } header: {
                        Text(L("srv.upd.packages", rest.count))
                    }
                }
            } else if searching {
                Section { HStack(spacing: 12) { ProgressView(); Text("srv.win.searching").foregroundStyle(.secondary) } }.listRowBackground(Color.clear)
            } else {
                PageStatus(failure: failure, retry: search).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("srv.sec.updates")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button { Task { await search() } } label: { Label("srv.upd.check", systemImage: "arrow.triangle.2.circlepath") }
                    Section {
                        Button { ask(L("srv.upd.installAll"), (list?.updates ?? []).map(\.id)) } label: { Label("srv.upd.installAll", systemImage: "arrow.down.circle") }
                        Button { ask(L("srv.upd.installSecurity"), (list?.updates ?? []).filter(\.security).map(\.id)) } label: { Label("srv.upd.installSecurity", systemImage: "lock.shield") }
                            .disabled((list?.securityCount ?? 0) == 0)
                    }
                    .disabled((list?.updates.isEmpty ?? true))
                } label: { Image(systemName: "ellipsis.circle") }
                    .disabled(!card.online || busy)
                    .accessibilityLabel(Text("dash.menu"))
            }
        }
        .refreshable { await search() }
        .task { if list == nil { await search() } }
        .noteOverlay(notes.text)
        .onDisappear { task?.cancel() }
        .confirmationDialog(asking?.title ?? "", isPresented: Binding(get: { asking != nil }, set: { if !$0 { asking = nil } }), titleVisibility: .visible, presenting: asking) { a in
            Button(L("srv.upd.install"), role: .destructive) { install(a) }
            Button("common.cancel", role: .cancel) {}
        } message: { a in Text(L("srv.win.confirmText", a.ids.count)) }
    }

    @ViewBuilder private func header(_ list: WinUpdateList) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            if let run = current {
                UpdateRunHeader(run: run) { current = nil }
            } else {
                HStack {
                    Text(searching ? L("srv.win.searching") : L("srv.win.found", list.updates.count, list.securityCount)).font(.subheadline).foregroundStyle(.secondary)
                    Spacer()
                    if searching { ProgressView().controlSize(.small) }
                }
            }
            if list.rebootPending {
                Label("srv.upd.rebootNeeded", systemImage: "arrow.clockwise.circle.fill").foregroundStyle(.orange).font(.subheadline)
            }
        }
        .animation(.default, value: current?.phase)
    }

    private func jobRow(_ title: String, run: AptRun, from list: WinUpdateList) -> some View {
        let state = run.state(title)
        let u = list.updates.first { $0.title == title }
        return HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(title).lineLimit(2)
                    if u?.security == true { Image(systemName: "lock.shield.fill").font(.caption).foregroundStyle(.orange) }
                }
                if state == .error, let m = run.messages[title] {
                    Text(L("srv.win.result", m)).font(.caption).foregroundStyle(.red).lineLimit(2)
                } else {
                    Text(state == .queued || state == .running ? stateText(state) : (u?.kb.isEmpty == false ? u?.kb ?? "" : stateText(state)))
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 0)
            StateMark(state: state)
        }
        .animation(.default, value: state)
    }

    private func stateText(_ s: PackageState) -> String {
        switch s {
        case .queued: L("srv.upd.state.queued")
        case .running: L("srv.upd.state.running")
        case .done: L("srv.upd.state.done")
        case .error: L("srv.upd.state.error")
        case .skipped: L("srv.upd.state.skipped")
        }
    }

    private func row(_ u: WinUpdate) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Text(u.title).lineLimit(2)
                if u.security { Image(systemName: "lock.shield.fill").font(.caption).foregroundStyle(.orange).accessibilityLabel(Text("srv.upd.securityBadge")) }
                if u.needReboot { Image(systemName: "arrow.clockwise").font(.caption).foregroundStyle(.secondary).accessibilityLabel(Text("srv.upd.rebootBadge")) }
            }
            Text([u.kb, sizeText(u.size), u.downloaded ? L("srv.win.downloaded") : ""].filter { !$0.isEmpty && $0 != "—" }.joined(separator: " · "))
                .font(.caption).foregroundStyle(.secondary)
        }
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button { ask(L("srv.upd.installOne", u.kb.isEmpty ? u.title : u.kb), [u.id]) } label: { Label("srv.upd.install", systemImage: "arrow.down.circle") }
                .tint(.accentColor)
                .disabled(busy || !card.online)
        }
    }

    private func ask(_ title: String, _ ids: [String]) {
        guard !busy, !ids.isEmpty else { return }
        asking = Ask(title: title, ids: ids)
    }

    /// Opens the stream of the installation; the updates change their marks as the lines of the log come.
    private func install(_ a: Ask) {
        let r = UpdateRun(title: L("srv.upd.installing"), windows: true)
        current = r
        task?.cancel()
        task = Task {
            do {
                // all of them is the empty list: the agent takes everything it found
                let all = a.ids.count == (list?.updates.count ?? -1)
                let connection = try await model.openStream(serverID: card.id, stream: .winUpdate(ids: all ? [] : a.ids))
                await withTaskCancellationHandler { await r.consume(connection) } onCancel: { connection.close() }
            } catch let e as LinagerError {
                r.fail(e)
            } catch {
                r.fail(.unreachable(error.localizedDescription))
            }
            if !Task.isCancelled { await search() }
        }
    }

    private func search() async {
        guard !searching else { return }
        searching = true
        defer { searching = false }
        do {
            list = try await model.winUpdates(serverID: card.id)
            failure = nil
        } catch let e as LinagerError {
            if list == nil { failure = e } else { notes.fail(e) }
        } catch {
            if list == nil { failure = .unreachable(error.localizedDescription) } else { notes.fail(error) }
        }
    }
}
