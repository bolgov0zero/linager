import LinagerKit
import Observation
import SwiftUI
import UIKit

/// What a terminal screen needs from the application (the real one is the AppModel, the preview and the tests bring their own).
struct TerminalBackend {
    var open: @MainActor (_ cols: Int, _ rows: Int, _ user: String) async throws -> TerminalConnection
    var users: @MainActor () async -> [String]
    var snippets: @MainActor () async throws -> [Snippet]
    var ran: @MainActor (Snippet) async -> Void
}

/// One terminal on the screen: opens the shell once the size of the screen is known, carries the bytes both ways, tells how it ended.
@MainActor
@Observable
final class TerminalModel {
    enum State: Equatable {
        case connecting
        case open
        /// The shell ended or the connection broke; the text says why.
        case closed(String)
        /// The server already has the most terminals it allows.
        case tooMany
        case failed(LinagerError)
    }

    static let defaultSize: Double = 11
    static let sizeRange: ClosedRange<Double> = 6...22
    private static let sizeKey = "term.fontSize"
    /// How long a terminal lives in the background: iOS lets an application run a little after it was left.
    static let backgroundGrace: Duration = .seconds(30)

    private(set) var state: State = .connecting
    /// The size of the font while it is being changed (shown for a moment in the middle of the screen).
    private(set) var hud: Double?
    private(set) var fontSize: Double
    /// The account the shell runs as; empty is the default one of the server.
    private(set) var user = ""
    /// The accounts a shell can be opened as (empty when the account may not list them: the button is then not shown).
    private(set) var users: [String] = []

    @ObservationIgnored private var backend: TerminalBackend?
    @ObservationIgnored private var feed: (([UInt8]) -> Void)?
    @ObservationIgnored private var cols = 0
    @ObservationIgnored private var rows = 0
    @ObservationIgnored private var started = false
    @ObservationIgnored private var connection: TerminalConnection?
    @ObservationIgnored private var input: AsyncStream<Data>.Continuation?
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    @ObservationIgnored private var hudTask: Task<Void, Never>?
    @ObservationIgnored private var graceTask: Task<Void, Never>?
    @ObservationIgnored private var background: UIBackgroundTaskIdentifier = .invalid

    init() {
        let saved = UserDefaults.standard.double(forKey: Self.sizeKey)
        fontSize = Self.sizeRange.contains(saved) ? saved : Self.defaultSize
    }

    // MARK: wiring

    func configure(_ backend: TerminalBackend) {
        self.backend = backend
        startWhenReady()
        Task { users = await backend.users() }
    }

    func attach(feed: @escaping ([UInt8]) -> Void) { self.feed = feed }

    /// The screen has (another) size in characters.
    func sizeChanged(cols: Int, rows: Int) {
        guard cols > 0, rows > 0 else { return }
        self.cols = cols
        self.rows = rows
        if started {
            let c = connection
            Task { await c?.resize(cols: cols, rows: rows) }
        } else {
            startWhenReady()
        }
    }

    /// The person typed something (or pasted it): to the shell, in order.
    func send(_ data: Data) { input?.yield(data) }

    // MARK: connecting

    private func startWhenReady() {
        guard !started, backend != nil, cols > 0, rows > 0 else { return }
        started = true
        connect()
    }

    private func connect() {
        state = .connecting
        guard let backend else { return }
        let (c, r, u) = (cols, rows, user)
        tasks.append(Task { [weak self] in
            do {
                let connection = try await backend.open(c, r, u)
                self?.opened(connection)
            } catch let e as LinagerError {
                self?.state = .failed(e)
            } catch {
                self?.state = .failed(.unreachable(error.localizedDescription))
            }
        })
    }

    private func opened(_ connection: TerminalConnection) {
        self.connection = connection
        state = .open
        let (stream, continuation) = AsyncStream<Data>.makeStream()
        input = continuation
        tasks.append(Task {
            for await d in stream { await connection.send(d) }
        })
        tasks.append(Task { [weak self] in
            for await e in connection.events {
                switch e {
                case .output(let d): self?.feed?(Array(d))
                case .closed(let reason, let code): self?.ended(reason: reason, code: code)
                }
            }
        })
    }

    private func ended(reason: String, code: Int) {
        input?.finish()
        input = nil
        connection = nil
        state = code == TerminalConnection.tooManyCode ? .tooMany : .closed(reason)
    }

    /// A new shell on the same screen: what was printed stays, for copying.
    func reconnect() {
        guard case .closed = state else { return }
        feed?(Array("\r\n".utf8))
        connect()
    }

    /// A shell as another account: the one that runs is closed and a new one opens on a clean screen, so that its prompt is the first line.
    func switchUser(_ name: String) {
        guard name != user else { return }
        dropConnection()
        user = name
        // a full reset of the emulator (ESC c: screen, modes, colours), then the scrollback (CSI 3 J) that the reset may leave
        feed?(Array("\u{1B}c\u{1B}[3J".utf8))
        connect()
    }

    /// Types a snippet into the shell, with the Enter that runs it unless `paste` (then it is only typed, to be changed first).
    func run(_ snippet: Snippet, paste: Bool) {
        guard case .open = state else { return }
        // a line break of a snippet is an Enter of the keyboard
        var text = snippet.command.replacingOccurrences(of: "\r\n", with: "\r").replacingOccurrences(of: "\n", with: "\r")
        while text.hasSuffix("\r") { text.removeLast() }
        send(Data((text + (paste ? "" : "\r")).utf8))
        if let ran = backend?.ran { Task { await ran(snippet) } }
    }

    func loadSnippets() async throws -> [Snippet] {
        guard let backend else { return [] }
        return try await backend.snippets()
    }

    private func dropConnection() {
        tasks.forEach { $0.cancel() }
        tasks = []
        input?.finish()
        input = nil
        connection?.close()
        connection = nil
    }

    /// Leaves the screen: the shell on the server ends with the connection.
    func close() {
        graceTask?.cancel()
        endBackgroundTask()
        dropConnection()
    }

    // MARK: background

    func enteredBackground() {
        guard case .open = state else { return }
        background = UIApplication.shared.beginBackgroundTask(withName: "terminal") { [weak self] in
            MainActor.assumeIsolated { self?.closeForBackground() }
        }
        graceTask?.cancel()
        graceTask = Task { [weak self] in
            try? await Task.sleep(for: Self.backgroundGrace)
            guard !Task.isCancelled else { return }
            self?.closeForBackground()
        }
    }

    func becameActive() {
        graceTask?.cancel()
        graceTask = nil
        endBackgroundTask()
    }

    private func closeForBackground() {
        guard case .open = state else { endBackgroundTask(); return }
        connection?.close()
        ended(reason: L("term.backgrounded"), code: 0)
        endBackgroundTask()
    }

    private func endBackgroundTask() {
        guard background != .invalid else { return }
        UIApplication.shared.endBackgroundTask(background)
        background = .invalid
    }

    // MARK: font

    func setFontSize(_ size: Double) {
        let s = min(Self.sizeRange.upperBound, max(Self.sizeRange.lowerBound, (size * 2).rounded() / 2))
        guard s != fontSize else { return }
        fontSize = s
        UserDefaults.standard.set(s, forKey: Self.sizeKey)
        hud = s
        hudTask?.cancel()
        hudTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(1))
            if !Task.isCancelled { self?.hud = nil }
        }
    }

    func resetFontSize() { setFontSize(Self.defaultSize) }
}
