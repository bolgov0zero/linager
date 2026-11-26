import Foundation
import Testing
@testable import LinagerKit

/// The RDP core stands in for itself: it records what it is given, and says what a test makes it say.
final class FakeEngine: RdpEngine, @unchecked Sendable {
    private let lock = NSLock()
    private var send: (@Sendable (Data) -> Void)?
    private var event: (@Sendable (RdpEngineEvent) -> Void)?
    private(set) var params: RdpParams?
    private(set) var fed: [Data] = []
    private(set) var feedEnded = false
    private(set) var inputs: [AssistInput] = []
    private(set) var stopped = false
    var canStart = true

    func start(_ params: RdpParams, send: @escaping @Sendable (Data) -> Void, event: @escaping @Sendable (RdpEngineEvent) -> Void) -> Bool {
        guard canStart else { return false }
        lock.withLock { self.params = params; self.send = send; self.event = event }
        return true
    }
    func feed(_ data: Data) { lock.withLock { fed.append(data) } }
    func feedEnd() {
        let e: (@Sendable (RdpEngineEvent) -> Void)? = lock.withLock { feedEnded = true; return event }
        // the real core ends its session when the socket ends, and says so
        e?(.ended("the connection is lost"))
    }
    func input(_ i: AssistInput) { lock.withLock { inputs.append(i) } }
    func stop() { lock.withLock { stopped = true } }

    func emit(_ e: RdpEngineEvent) { let f = lock.withLock { event }; f?(e) }
    func transmit(_ d: Data) { let f = lock.withLock { send }; f?(d) }
    func wait(_ what: () -> Bool) async -> Bool {
        for _ in 0..<200 { if lock.withLock({ what() }) { return true }; try? await Task.sleep(for: .milliseconds(25)) }
        return false
    }
}

private let session = Credentials(token: "tok123", csrf: "csrf-abc")

@Suite("RDP typing")
struct RdpTypingTests {
    @Test func theLayoutOfTheUSIsKeysAndTheRestIsTheCharacter() {
        let a = RdpTyping.steps("a")
        #expect(a == [.key(sc: 0x1e, ext: false, down: true), .key(sc: 0x1e, ext: false, down: false)])
        // Cyrillic needs no clipboard: the characters go as they are
        let ya = RdpTyping.steps("я")
        #expect(ya == [.unicode(scalar: 0x44f, down: true), .unicode(scalar: 0x44f, down: false)])
        // beyond the 16 bits it is still one character
        #expect(RdpTyping.steps("😀") == [.unicode(scalar: 0x1F600, down: true), .unicode(scalar: 0x1F600, down: false)])
        #expect(RdpTyping.steps("\n").first == .key(sc: 0x1c, ext: false, down: true))
    }

    @Test func modifiersHoldForTheFirstKeyOnly() {
        let s = RdpTyping.steps("ab", modifiers: [.ctrl])
        #expect(s.first == .key(sc: ScanCodes.ctrl.sc, ext: false, down: true))
        let downs = s.filter { if case .key(let sc, _, true) = $0 { sc == ScanCodes.ctrl.sc } else { false } }
        #expect(downs.count == 1)
        #expect(RdpTyping.press(ScanCodes.esc).count == 2)
    }

    @Test func theUnicodeFrameIsNotForTheAssistant() {
        #expect(AssistInput.unicode(scalar: 65, down: true).frame.isEmpty)
    }

    @Test func rightsToTheRemoteDesktop() throws {
        func info(_ json: String) throws -> SessionInfo { try JSONDecoder().decode(SessionInfo.self, from: Data(json.utf8)) }
        #expect(try info(#"{"csrf":"x","login":"a","role":"admin","modules":[]}"#).canUseDesktop)
        #expect(try info(#"{"csrf":"x","login":"a","role":"operator","modules":["desktop"]}"#).canUseDesktop)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"operator","modules":["terminal"]}"#).canUseDesktop)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"auditor","modules":["desktop"]}"#).canUseDesktop)
    }
}

@Suite("RDP picture")
struct RdpPictureTests {
    @Test func rectanglesOfRGBAAreDrawnWhereTheyBelong() {
        let screen = AssistantScreen()
        screen.resize(width: 8, height: 6)
        // a 2x2 rectangle at (3, 2): red, green, blue, white
        let px: [UInt8] = [255, 0, 0, 255,  0, 255, 0, 255,  0, 0, 255, 255,  255, 255, 255, 255]
        #expect(screen.applyRGBA(x: 3, y: 2, w: 2, h: 2, rgba: Data(px)))
        #expect(screen.pixel(x: 3, y: 2)! == (r: 255, g: 0, b: 0))
        #expect(screen.pixel(x: 4, y: 2)! == (r: 0, g: 255, b: 0))
        #expect(screen.pixel(x: 3, y: 3)! == (r: 0, g: 0, b: 255))
        #expect(screen.pixel(x: 4, y: 3)! == (r: 255, g: 255, b: 255))
        #expect(screen.pixel(x: 2, y: 2)! == (r: 0, g: 0, b: 0)) // around it nothing changed
        #expect(screen.image() != nil)
        // what lies outside, or is too short, is refused and draws nothing
        #expect(!screen.applyRGBA(x: 7, y: 5, w: 2, h: 2, rgba: Data(px)))
        #expect(!screen.applyRGBA(x: 0, y: 0, w: 2, h: 2, rgba: Data([1, 2, 3])))
        #expect(!screen.applyRGBA(x: -1, y: 0, w: 1, h: 1, rgba: Data(px)))
    }
}

@Suite("RDP over a real connection", .serialized)
struct RdpConnectionTests {
    @Test func messagesGoBothWaysThroughTheSocket() async throws {
        let server = try FakeServer()
        let engine = FakeEngine()
        let params = RdpParams(username: "bolgov", password: "secret", destination: "10.0.0.5:3389")
        let c = try await server.client(credentials: session).openDesktop(serverID: "srv-a", params: params, engine: engine)
        #expect(engine.params == params)
        // what the server sends reaches the core, what the core sends reaches the server (and comes back)
        #expect(await engine.wait { engine.fed.first == Data("hello:bolgov".utf8) }) // the name is in the address, the password is not
        engine.transmit(Data([1, 2, 3]))
        #expect(await engine.wait { engine.fed.last == Data("echo:".utf8) + Data([1, 2, 3]) })
        // the core's events are the connection's events
        engine.emit(.connected(width: 640, height: 480))
        var it = c.events.makeAsyncIterator()
        #expect(await it.next() == .connected(width: 640, height: 480))
        engine.emit(.frame(x: 1, y: 2, w: 3, h: 4, rgba: Data(count: 48)))
        #expect(await it.next() == .frame(x: 1, y: 2, w: 3, h: 4, rgba: Data(count: 48)))
        c.send(.key(sc: 0x1e, ext: false, down: true))
        #expect(engine.inputs == [.key(sc: 0x1e, ext: false, down: true)])
        c.close()
        #expect(await it.next() == .closed(reason: "", code: 0))
        #expect(await engine.wait { engine.stopped })
        withExtendedLifetime(server) {}
    }

    @Test func theLimitOfOneDesktopIsTold() async throws {
        let server = try FakeServer()
        let engine = FakeEngine()
        let c = try await server.client(credentials: session).openDesktop(serverID: "full", params: RdpParams(username: "u", password: "p", destination: "x:1"), engine: engine)
        var it = c.events.makeAsyncIterator()
        guard case .closed(let reason, let code)? = await it.next() else { Issue.record("not closed"); return }
        #expect(code == RdpConnection.limitCode)
        #expect(reason.contains("already open"))
        #expect(await engine.wait { engine.feedEnded && engine.stopped })
        withExtendedLifetime(server) {}
    }

    @Test func aRefusalOfTheServerIsTold() async throws {
        let server = try FakeServer()
        let client = try await server.client(credentials: session)
        await #expect(throws: LinagerError.agentOffline) {
            _ = try await client.openDesktop(serverID: "off", params: RdpParams(username: "u", password: "p", destination: "x:1"), engine: FakeEngine())
        }
        let engine = FakeEngine()
        engine.canStart = false
        await #expect(throws: LinagerError.self) {
            _ = try await client.openDesktop(serverID: "srv-a", params: RdpParams(username: "u", password: "p", destination: "x:1"), engine: engine)
        }
        withExtendedLifetime(server) {}
    }

    @Test func theReasonOfTheServerWinsOverTheEndOfTheCore() async throws {
        let server = try FakeServer()
        let engine = FakeEngine()
        let c = try await server.client(credentials: session).openDesktop(serverID: "srv-a", params: RdpParams(username: "u", password: "p", destination: "x:1"), engine: engine)
        _ = await engine.wait { !engine.fed.isEmpty }
        engine.transmit(Data("drop".utf8)) // the server closes with an error and a reason
        var it = c.events.makeAsyncIterator()
        guard case .closed(let reason, _)? = await it.next() else { Issue.record("not closed"); return }
        #expect(reason == "the connection is lost" || reason == "cannot open the remote desktop")
        withExtendedLifetime(server) {}
    }
}
