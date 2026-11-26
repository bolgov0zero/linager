import Foundation

/// The Assistant of a computer: one WebSocket (`api/servers/{id}/assistant`), pinned like every other connection of the client. The computer
/// sends the picture as tiles; the phone sends input, the clipboard, the choice of a monitor and a pause.
public final class AssistantConnection: @unchecked Sendable {
    /// The server closes with this code (and this reason) when the session is already open in another Assistant.
    public static let busyCode = 4031
    static let busyReason = "this session is already open in the Assistant"

    public let events: AsyncStream<AssistantEvent>

    private let task: URLSessionWebSocketTask
    private let continuation: AsyncStream<AssistantEvent>.Continuation
    private let lock = NSLock()
    private var ended = false
    private var reader: Task<Void, Never>?
    /// What the connection belongs to (the client, whose session it runs in): the session is cancelled when the client goes.
    private let owner: AnyObject

    init(task: URLSessionWebSocketTask, owner: AnyObject) {
        self.task = task
        self.owner = owner
        var c: AsyncStream<AssistantEvent>.Continuation!
        events = AsyncStream(bufferingPolicy: .unbounded) { c = $0 }
        continuation = c
    }

    func start() {
        reader = Task { [weak self] in await self?.readLoop() }
    }

    // MARK: sending

    private func sendFrame(_ data: Data) async { try? await task.send(.data(data)) }

    public func send(_ input: AssistInput) async { await sendFrame(input.frame) }

    /// Puts a text on the clipboard of the computer; false when it is too long.
    @discardableResult
    public func setClipboard(_ text: String) async -> Bool {
        guard let f = AssistCommand.clipSet(text) else { return false }
        await sendFrame(f)
        return true
    }

    /// Asks for the clipboard of the computer; the text comes as `.clip`.
    public func getClipboard() async { await sendFrame(AssistCommand.clipGet) }

    public func selectMonitor(_ index: Int) async { await sendFrame(AssistCommand.monitor(index)) }

    public func pause(_ on: Bool) async { await sendFrame(AssistCommand.pause(on)) }

    /// Types a text on the sign-in or the UAC screen; the answer comes as `.typed`. False when the text is not acceptable.
    @discardableResult
    public func type(_ text: String, after: String = "", label: String = "") async -> Bool {
        guard let f = AssistCommand.typing(text, after: after, label: label) else { return false }
        await sendFrame(f)
        return true
    }

    /// Plays the steps of a typing: keys, and the clipboard with the pause that lets the computer receive it before Ctrl+V.
    public func play(_ steps: [AssistTyping.Step]) async {
        for step in steps {
            switch step {
            case .input(let i): await send(i)
            case .clipboard(let text):
                await setClipboard(text)
                try? await Task.sleep(for: .milliseconds(150))
            }
        }
    }

    /// Closes the Assistant from this side.
    public func close() {
        reader?.cancel()
        task.cancel(with: .normalClosure, reason: nil)
        finish(nil)
    }

    // MARK: receiving

    private func readLoop() async {
        do {
            while !Task.isCancelled {
                switch try await task.receive() {
                case .data(let d):
                    guard let e = AssistProtocol.event(from: d) else { continue }
                    continuation.yield(e)
                    if case .end = e {
                        // the computer stops the Assistant: nothing more comes, the close of the socket is not a news
                        finish(nil)
                        task.cancel(with: .normalClosure, reason: nil)
                        return
                    }
                case .string:
                    break
                @unknown default:
                    break
                }
            }
        } catch {
            if Task.isCancelled { return }
            let reason = task.closeReason.flatMap { String(data: $0, encoding: .utf8) } ?? ""
            // the close code of the server is 4031, which the system does not know: its reason says what it was
            let code = reason == Self.busyReason ? Self.busyCode : 0
            finish(.closed(reason: reason.isEmpty ? (error as NSError).localizedDescription : reason, code: code))
        }
    }

    private func finish(_ last: AssistantEvent?) {
        let first: Bool = lock.withLock {
            defer { ended = true }
            return !ended
        }
        guard first else { return }
        if let last { continuation.yield(last) }
        continuation.finish()
    }
}
