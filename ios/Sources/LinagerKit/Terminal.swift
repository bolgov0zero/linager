import Foundation

/// What a terminal says: the bytes the shell prints, and the end.
public enum TerminalEvent: Sendable, Equatable {
    case output(Data)
    /// The shell ended or the connection broke. `code` is 4029 when the server refused a new terminal (the limit per server), 0 otherwise.
    case closed(reason: String, code: Int)
}

/// A terminal of a server: one WebSocket (`api/servers/{id}/stream?kind=terminal`), pinned like every other connection of the client.
/// Binary frames carry the bytes of the shell both ways; the text frame `{"resize":{"cols":..,"rows":..}}` tells the size of the screen and the
/// server ends the stream with `{"closed":"<reason>"}`.
public final class TerminalConnection: @unchecked Sendable {
    /// The server sends this reason with the close code 4029 when a server already has the most terminals it allows.
    public static let tooManyCode = 4029
    static let tooManyReason = "too many open terminals"

    public let events: AsyncStream<TerminalEvent>

    private let task: URLSessionWebSocketTask
    private let continuation: AsyncStream<TerminalEvent>.Continuation
    private let lock = NSLock()
    private var ended = false
    private var reader: Task<Void, Never>?

    /// What the connection belongs to (the client, whose session it runs in): the session is cancelled when the client goes.
    private let owner: AnyObject

    init(task: URLSessionWebSocketTask, owner: AnyObject) {
        self.task = task
        self.owner = owner
        var c: AsyncStream<TerminalEvent>.Continuation!
        events = AsyncStream(bufferingPolicy: .unbounded) { c = $0 }
        continuation = c
    }

    func start() {
        reader = Task { [weak self] in await self?.readLoop() }
    }

    public func send(_ data: Data) async {
        guard !data.isEmpty else { return }
        try? await task.send(.data(data))
    }

    public func resize(cols: Int, rows: Int) async {
        let text = #"{"resize":{"cols":\#(max(1, cols)),"rows":\#(max(1, rows))}}"#
        try? await task.send(.string(text))
    }

    /// Closes the terminal from this side (the shell on the server ends with the connection).
    public func close() {
        reader?.cancel()
        task.cancel(with: .normalClosure, reason: nil)
        finish(reason: "", code: 0, announce: false)
    }

    private func readLoop() async {
        do {
            while !Task.isCancelled {
                switch try await task.receive() {
                case .data(let d):
                    continuation.yield(.output(d))
                case .string(let s):
                    if let reason = Self.closedReason(s) {
                        finish(reason: reason, code: 0, announce: true)
                        task.cancel(with: .normalClosure, reason: nil)
                        return
                    }
                @unknown default:
                    break
                }
            }
        } catch {
            if Task.isCancelled { return }
            let reason = task.closeReason.flatMap { String(data: $0, encoding: .utf8) } ?? ""
            // the close code of the server is 4029, which the system does not know: its reason says what it was
            let code = reason == Self.tooManyReason ? Self.tooManyCode : 0
            finish(reason: reason.isEmpty ? (error as NSError).localizedDescription : reason, code: code, announce: true)
        }
    }

    private func finish(reason: String, code: Int, announce: Bool) {
        let first: Bool = lock.withLock {
            defer { ended = true }
            return !ended
        }
        guard first else { return }
        if announce { continuation.yield(.closed(reason: reason, code: code)) }
        continuation.finish()
    }

    static func closedReason(_ text: String) -> String? {
        struct M: Decodable { let closed: String? }
        guard let d = text.data(using: .utf8), let m = try? JSONDecoder().decode(M.self, from: d), let r = m.closed else { return nil }
        return r
    }
}
