import Foundation

/// A remote desktop of a Windows computer: one WebSocket (`api/servers/{id}/desktop`), pinned like every other connection of the client, which
/// carries RDCleanPath to the RDP core. The core signs in (the password never leaves the phone in the clear: it is used inside the
/// connection) and sends the picture; the phone sends input.
public final class RdpConnection: @unchecked Sendable {
    /// The server closes with this code when a remote desktop of the computer is already open.
    public static let limitCode = 4030
    static let limitReason = "a remote desktop of this server is already open"

    public let events: AsyncStream<RdpEvent>

    private let task: URLSessionWebSocketTask
    private let engine: RdpEngine
    private let continuation: AsyncStream<RdpEvent>.Continuation
    private let outgoing: AsyncStream<Data>.Continuation
    private let outgoingStream: AsyncStream<Data>
    private let lock = NSLock()
    private var ended = false
    private var socketReason: String?
    private var socketCode = 0
    private var tasks: [Task<Void, Never>] = []
    /// What the connection belongs to (the client, whose session it runs in): the session is cancelled when the client goes.
    private let owner: AnyObject

    init(task: URLSessionWebSocketTask, engine: RdpEngine, owner: AnyObject) {
        self.task = task
        self.engine = engine
        self.owner = owner
        var c: AsyncStream<RdpEvent>.Continuation!
        events = AsyncStream(bufferingPolicy: .unbounded) { c = $0 }
        continuation = c
        var o: AsyncStream<Data>.Continuation!
        outgoingStream = AsyncStream(bufferingPolicy: .unbounded) { o = $0 }
        outgoing = o
    }

    /// Starts the core and the two pumps. False when the core cannot start.
    func start(_ params: RdpParams) -> Bool {
        let started = engine.start(params, send: { [weak self] data in self?.outgoing.yield(data) }, event: { [weak self] e in self?.engineEvent(e) })
        guard started else { return false }
        // messages go out one at a time, in the order the core gave them
        tasks.append(Task { [task, outgoingStream] in
            for await d in outgoingStream { try? await task.send(.data(d)) }
        })
        tasks.append(Task { [weak self] in await self?.readLoop() })
        return true
    }

    private func readLoop() async {
        let task = self.task
        do {
            while !Task.isCancelled {
                switch try await task.receive() {
                case .data(let d): engine.feed(d)
                default: break
                }
            }
        } catch {
            if Task.isCancelled { return }
            let reason = task.closeReason.flatMap { String(data: $0, encoding: .utf8) } ?? ""
            // the close code of the server is 4030, which the system does not know: its reason says what it was
            lock.withLock {
                socketReason = reason
                socketCode = reason == Self.limitReason ? Self.limitCode : 0
            }
            engine.feedEnd()
        }
    }

    private func engineEvent(_ e: RdpEngineEvent) {
        switch e {
        case .connected(let w, let h): continuation.yield(.connected(width: w, height: h))
        case .frame(let x, let y, let w, let h, let rgba): continuation.yield(.frame(x: x, y: y, w: w, h: h, rgba: rgba))
        case .ended(let text):
            let (reason, code) = lock.withLock { (socketReason, socketCode) }
            finish(reason: code != 0 ? (reason ?? text) : (text.isEmpty ? (reason ?? "") : text), code: code)
        }
    }

    private func finish(reason: String, code: Int) {
        let first: Bool = lock.withLock {
            defer { ended = true }
            return !ended
        }
        guard first else { return }
        continuation.yield(.closed(reason: reason, code: code))
        continuation.finish()
        outgoing.finish()
        task.cancel(with: .normalClosure, reason: nil)
        // the core joins its thread when it stops: that is not done on that thread
        let engine = self.engine
        DispatchQueue.global(qos: .utility).async { engine.stop() }
    }

    // MARK: input

    public func send(_ input: AssistInput) { engine.input(input) }

    /// Leaves the desktop: the core says goodbye to the computer and the socket is closed.
    public func close() {
        tasks.forEach { $0.cancel() }
        finish(reason: "", code: 0)
    }
}
