import CoreGraphics
import Foundation
import Testing
@testable import LinagerKit

private let session = Credentials(token: "tok123", csrf: "csrf-abc")

private func bytes(_ parts: [UInt8]...) -> Data { Data(parts.flatMap { $0 }) }

/// Reads the events of an Assistant in the background, so that a test can ask for the next one and never hangs.
private actor AssistEvents {
    private var queue: [AssistantEvent] = []
    private var over = false

    init(_ c: AssistantConnection) {
        let stream = c.events
        Task { [self] in
            for await e in stream { await push(e) }
            await finish()
        }
    }

    private func push(_ e: AssistantEvent) { queue.append(e) }
    private func finish() { over = true }

    func next() async -> AssistantEvent? {
        for _ in 0..<300 {
            if !queue.isEmpty { return queue.removeFirst() }
            if over { return nil }
            try? await Task.sleep(for: .milliseconds(10))
        }
        return .closed(reason: "TIMEOUT", code: -1)
    }
}

@Suite("Assistant protocol")
struct AssistantProtocolTests {
    @Test func tilesAreParsed() {
        // two tiles: (1,2,3,4) with "ab" and (5,6,7,8) with "c"
        let payload = bytes([0, 2], [0, 1, 0, 2, 0, 3, 0, 4, 0, 0, 0, 2], [97, 98], [0, 5, 0, 6, 0, 7, 0, 8, 0, 0, 0, 1], [99])
        let t = AssistProtocol.parseTiles(payload)
        #expect(t?.count == 2)
        #expect(t?[0] == AssistTile(x: 1, y: 2, w: 3, h: 4, jpeg: Data("ab".utf8)))
        #expect(t?[1].jpeg == Data("c".utf8))
        #expect(AssistProtocol.parseTiles(Data([0, 0]))?.isEmpty == true)
    }

    @Test func cutAndLyingTilesAreRefused() {
        #expect(AssistProtocol.parseTiles(Data()) == nil)
        #expect(AssistProtocol.parseTiles(Data([0, 1])) == nil) // announces a tile that is not there
        #expect(AssistProtocol.parseTiles(bytes([0, 1], [0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 9], [1, 2])) == nil) // the length is more than the rest
    }

    @Test func framesBecomeEvents() {
        func ev(_ type: UInt8, _ json: String) -> AssistantEvent? { AssistProtocol.event(from: Data([type]) + Data(json.utf8)) }
        #expect(ev(1, #"{"w":1920,"h":1080,"session":3,"user":"anna","monitor":-1}"#) == .hello(AssistHello(w: 1920, h: 1080, user: "anna")))
        // a monitor that is not a rectangle is dropped, a missing field is neutral
        if case .hello(let h)? = ev(1, #"{"w":10,"h":10,"monitors":[{"w":10,"h":10,"primary":true},{"w":0,"h":5}],"platform":"linux"}"#) {
            #expect(h.monitors.count == 1)
            #expect(h.isLinux)
        } else { Issue.record("no hello") }
        #expect(ev(3, #"{"fps":14.5,"kbps":800,"q":80,"desktop":"winlogon"}"#) == .stats(AssistStats(fps: 14.5, kbps: 800, q: 80, desktop: "winlogon")))
        #expect(ev(3, "{}") == .stats(AssistStats()))
        #expect(ev(4, #"{"reason":"locked","reconnect":true}"#) == .end(reason: "locked", reconnect: true))
        #expect(ev(5, #"{"text":"привет"}"#) == .clip("привет"))
        #expect(ev(6, #"{"on":true}"#) == .waking(true))
        #expect(ev(7, #"{"ok":false,"reason":"desktop"}"#) == .typed(ok: false, reason: "desktop"))
        #expect(ev(99, "{}") == nil) // a frame that is not known is skipped
        #expect(ev(1, "not json") == nil)
        #expect(AssistProtocol.event(from: Data()) == nil)
    }

    @Test func theSignInDesktopIsTheOnlyOneToTypeOn() {
        #expect(AssistStats(desktop: "winlogon").isSignInDesktop)
        #expect(!AssistStats(desktop: "default").isSignInDesktop)
        #expect(!AssistStats(desktop: "screen-saver").isSignInDesktop)
    }

    @Test func inputIsEncodedAsTheServerParsesIt() throws {
        func json(_ i: AssistInput) throws -> [String: Any] {
            let f = i.frame
            #expect(f.first == AssistFrameType.input)
            return try #require(JSONSerialization.jsonObject(with: f.dropFirst()) as? [String: Any])
        }
        let m = try json(.move(x: 10, y: 20))
        #expect(m["t"] as? String == "m"); #expect(m["x"] as? Int == 10); #expect(m["y"] as? Int == 20)
        let b = try json(.button(2, down: true, x: 5, y: 6))
        #expect(b["t"] as? String == "b"); #expect(b["b"] as? Int == 2); #expect(b["d"] as? Bool == true)
        let w = try json(.wheel(dx: 0, dy: 3))
        #expect(w["t"] as? String == "w"); #expect(w["dy"] as? Int == 3)
        let k = try json(.key(sc: 0x48, ext: true, down: false))
        #expect(k["t"] as? String == "k"); #expect(k["sc"] as? Int == 0x48); #expect(k["ext"] as? Bool == true); #expect(k["d"] as? Bool == false)
        #expect(try json(.ctrlAltDel)["t"] as? String == "cad")
        // the numbers stay in the range the server accepts
        let far = try json(.move(x: -5, y: 99999))
        #expect(far["x"] as? Int == 0); #expect(far["y"] as? Int == 65535)
        #expect(try json(.wheel(dx: 500, dy: -500))["dx"] as? Int == 50)
        // and every frame is under the limit of the server (256 bytes of JSON)
        #expect(AssistInput.button(4, down: true, x: 65535, y: 65535).frame.count - 1 <= 256)
    }

    @Test func commandsAreEncoded() throws {
        #expect(AssistCommand.clipGet == Data([AssistFrameType.clipGet]))
        #expect(AssistCommand.pause(true).first == AssistFrameType.pause)
        #expect(String(decoding: AssistCommand.monitor(-1).dropFirst(), as: UTF8.self) == #"{"i":-1}"#)
        #expect(String(decoding: try #require(AssistCommand.clipSet("hi")).dropFirst(), as: UTF8.self) == #"{"text":"hi"}"#)
        #expect(AssistCommand.clipSet(String(repeating: "a", count: AssistCommand.clipMax + 1)) == nil)
    }

    @Test func aTypingIsCheckedLikeTheServerChecksIt() throws {
        let ok = try #require(AssistCommand.typing("p@ss", after: "enter", label: "office\npc"))
        let j = try #require(JSONSerialization.jsonObject(with: ok.dropFirst()) as? [String: Any])
        #expect(j["text"] as? String == "p@ss"); #expect(j["after"] as? String == "enter")
        #expect(j["label"] as? String == "office pc") // a control character of the label is a space
        #expect(AssistCommand.typing("") == nil)
        #expect(AssistCommand.typing("a\tb") == nil) // Tab has its own field
        #expect(AssistCommand.typing("a\u{7f}") == nil)
        #expect(AssistCommand.typing(String(repeating: "x", count: 513)) == nil)
        #expect(AssistCommand.typing(String(repeating: "x", count: 512)) != nil)
    }

    @Test func presetsAreThoseOfTheDesktopApplication() {
        #expect(AssistSettings.preset(.normal).preset == .normal)
        #expect(AssistSettings.preset(.eco).preset == .eco)
        #expect(AssistSettings.preset(.hq).preset == .hq)
        #expect(AssistSettings(colors: "24", fps: 15, mbit: 6).preset == nil) // custom
        var withPointer = AssistSettings.preset(.normal)
        withPointer.showCursor = true
        #expect(withPointer.preset == .normal) // the pointer does not count
        let data = try? JSONEncoder().encode(AssistSettings.preset(.hq))
        #expect(String(decoding: data ?? Data(), as: UTF8.self).contains(#""show_cursor":false"#))
    }
}

@Suite("Keyboard of the Assistant")
struct AssistantKeyboardTests {
    @Test func scanCodesOfTheUSLayout() {
        #expect(ScanCodes.stroke(for: "a") == KeyStroke(sc: 0x1e))
        #expect(ScanCodes.stroke(for: "A") == KeyStroke(sc: 0x1e, shift: true))
        #expect(ScanCodes.stroke(for: "1") == KeyStroke(sc: 0x02))
        #expect(ScanCodes.stroke(for: "!") == KeyStroke(sc: 0x02, shift: true))
        #expect(ScanCodes.stroke(for: "0") == KeyStroke(sc: 0x0b))
        #expect(ScanCodes.stroke(for: "z") == KeyStroke(sc: 0x2c))
        #expect(ScanCodes.stroke(for: "/") == KeyStroke(sc: 0x35))
        #expect(ScanCodes.stroke(for: "?") == KeyStroke(sc: 0x35, shift: true))
        #expect(ScanCodes.stroke(for: "\\") == KeyStroke(sc: 0x2b))
        #expect(ScanCodes.stroke(for: "|") == KeyStroke(sc: 0x2b, shift: true))
        #expect(ScanCodes.stroke(for: " ") == ScanCodes.space)
        #expect(ScanCodes.stroke(for: "ж") == nil)
        #expect(ScanCodes.stroke(for: "é") == nil)
        #expect(ScanCodes.stroke(for: "😀") == nil)
    }

    @Test func everyPrintableAsciiCharacterHasAKey() {
        for v in 0x20...0x7e {
            let c = Character(UnicodeScalar(UInt8(v)))
            #expect(ScanCodes.stroke(for: c) != nil, "no key for \(c)")
        }
    }

    @Test func extendedAndFunctionKeys() {
        #expect(ScanCodes.up == KeyStroke(sc: 0x48, ext: true))
        #expect(ScanCodes.win == KeyStroke(sc: 0x5b, ext: true))
        #expect(ScanCodes.function(1) == KeyStroke(sc: 0x3b))
        #expect(ScanCodes.function(10) == KeyStroke(sc: 0x44))
        #expect(ScanCodes.function(11) == KeyStroke(sc: 0x57))
        #expect(ScanCodes.function(13) == nil)
    }

    @Test func aLetterIsPressedAndReleased() {
        #expect(AssistTyping.plan("a") == [.input(.key(sc: 0x1e, ext: false, down: true)), .input(.key(sc: 0x1e, ext: false, down: false))])
    }

    @Test func aCapitalLetterHoldsShift() {
        let steps = AssistTyping.plan("A")
        #expect(steps.first == .input(.key(sc: 0x2a, ext: false, down: true)))
        #expect(steps.last == .input(.key(sc: 0x2a, ext: false, down: false)))
        #expect(steps.count == 4)
    }

    @Test func enterTabAndBackspaceAreKeys() {
        #expect(AssistTyping.plan("\n") == AssistTyping.press(ScanCodes.enter))
        #expect(AssistTyping.plan("\t") == AssistTyping.press(ScanCodes.tab))
        #expect(AssistTyping.plan("\u{7f}") == AssistTyping.press(ScanCodes.backspace))
    }

    @Test func otherAlphabetsGoThroughTheClipboard() {
        let steps = AssistTyping.plan("ab привет cd")
        // "ab ", then the run "привет" through the clipboard and Ctrl+V, then " cd"
        #expect(steps.contains(.clipboard("привет")))
        let at = steps.firstIndex(of: .clipboard("привет"))!
        #expect(steps[at + 1] == .input(.key(sc: 0x1d, ext: false, down: true))) // Ctrl
        #expect(steps[at + 2] == .input(.key(sc: 0x2f, ext: false, down: true))) // V
        #expect(steps.filter { if case .clipboard = $0 { true } else { false } }.count == 1)
        // the keys before and after are not lost
        #expect(steps.first == .input(.key(sc: 0x1e, ext: false, down: true)))
        #expect(steps.last == .input(.key(sc: 0x20, ext: false, down: false))) // d
    }

    @Test func modifiersHoldForTheFirstKeyOnly() {
        let steps = AssistTyping.plan("cv", modifiers: [.ctrl])
        // Ctrl down, c down, c up, Ctrl up, then v alone (down, up)
        #expect(steps.count == 6)
        #expect(steps[0] == .input(.key(sc: 0x1d, ext: false, down: true)))
        #expect(steps[3] == .input(.key(sc: 0x1d, ext: false, down: false)))
        #expect(steps[4] == .input(.key(sc: 0x2f, ext: false, down: true)))
    }

    @Test func aTextIsNeverLost() {
        // every character of a mixed text ends up either as a key or in a clipboard run
        let steps = AssistTyping.plan("Hi, мир! 123")
        let clip = steps.compactMap { s -> String? in if case .clipboard(let t) = s { t } else { nil } }.joined()
        #expect(clip == "мир")
    }
}

@Suite("Screen of the Assistant")
struct AssistantScreenTests {
    private func tile(x: Int, y: Int, w: Int = 16, h: Int = 16) throws -> AssistTile {
        AssistTile(x: x, y: y, w: w, h: h, jpeg: try fixture("tile.jpg"))
    }

    @Test func aTileIsDrawnWhereItBelongs() throws {
        let s = AssistantScreen()
        s.resize(width: 64, height: 48)
        #expect(s.apply([try tile(x: 4, y: 4)]) == 1)
        let inside = try #require(s.pixel(x: 10, y: 10))
        #expect(inside.r > 170 && inside.g < 70 && inside.b < 70) // the red of the tile
        #expect(s.pixel(x: 0, y: 0)! == (0, 0, 0)) // the rest is black
        #expect(s.pixel(x: 30, y: 30)! == (0, 0, 0))
        let edge = try #require(s.pixel(x: 3, y: 4))
        #expect(edge.r < 20)
    }

    @Test func theTopOfTheScreenIsTheTopOfTheTile() throws {
        let s = AssistantScreen()
        s.resize(width: 64, height: 48)
        // a tile at the very bottom row: it must not land at the top
        s.apply([try tile(x: 0, y: 32)])
        #expect(s.pixel(x: 5, y: 40)!.r > 170)
        #expect(s.pixel(x: 5, y: 5)! == (0, 0, 0))
    }

    @Test func badTilesAreSkipped() throws {
        let s = AssistantScreen()
        s.resize(width: 64, height: 48)
        #expect(s.apply([AssistTile(x: 0, y: 0, w: 16, h: 16, jpeg: Data("not a picture".utf8))]) == 0)
        #expect(s.apply([try tile(x: 60, y: 4)]) == 0) // sticks out on the right
        #expect(s.apply([try tile(x: 4, y: 40)]) == 0) // sticks out at the bottom
        #expect(s.apply([try tile(x: -1, y: 0)]) == 0)
        #expect(s.apply([try tile(x: 0, y: 0, w: 0, h: 16)]) == 0)
        // nothing was drawn
        #expect(s.pixel(x: 10, y: 10)! == (0, 0, 0))
    }

    @Test func aNewSizeStartsBlackAndAnImpossibleOneEmpties() throws {
        let s = AssistantScreen()
        s.resize(width: 64, height: 48)
        s.apply([try tile(x: 4, y: 4)])
        s.resize(width: 100, height: 80)
        #expect(s.width == 100 && s.height == 80)
        #expect(s.pixel(x: 10, y: 10)! == (0, 0, 0))
        s.resize(width: 0, height: 10)
        #expect(s.width == 0)
        #expect(s.apply([try tile(x: 0, y: 0)]) == 0)
        #expect(s.image() == nil)
        s.resize(width: 100000, height: 10) // above the limit
        #expect(s.width == 0)
    }

    @Test func theImageIsACopy() throws {
        let s = AssistantScreen()
        s.resize(width: 64, height: 48)
        let before = try #require(s.image())
        s.apply([try tile(x: 4, y: 4)])
        let after = try #require(s.image())
        #expect(before.width == 64 && after.height == 48)
    }
}

@Suite("Assistant over a real connection", .serialized)
struct AssistantConnectionTests {
    @Test func theComputerShowsItsScreen() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session).openAssistant(serverID: "srv-a", session: "console")
        let ev = AssistEvents(c)
        guard case .hello(let h)? = await ev.next() else { Issue.record("no hello"); return }
        #expect(h.w == 64 && h.h == 48 && h.user == "admin")
        #expect(h.monitors.count == 1) // the one that is not a rectangle is dropped
        guard case .tiles(let tiles)? = await ev.next() else { Issue.record("no tiles"); return }
        let screen = AssistantScreen()
        screen.resize(width: h.w, height: h.h)
        #expect(screen.apply(tiles) == 1)
        #expect(screen.pixel(x: 10, y: 10)!.r > 170)
        #expect(await ev.next() == .stats(AssistStats(fps: 14.5, kbps: 800, q: 80, desktop: "winlogon")))
        c.close()
        withExtendedLifetime(server) {}
    }

    @Test func inputReachesTheComputerAsTheServerParsesIt() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session).openAssistant(serverID: "srv-a", session: "console")
        let ev = AssistEvents(c)
        for _ in 0..<3 { _ = await ev.next() } // hello, tiles, stats
        await c.send(.button(0, down: true, x: 7, y: 9))
        guard case .clip(let echoed)? = await ev.next() else { Issue.record("no echo"); return }
        #expect(echoed.hasPrefix("16:"))
        let j = try #require(JSONSerialization.jsonObject(with: Data(echoed.dropFirst(3).utf8)) as? [String: Any])
        #expect(j["t"] as? String == "b"); #expect(j["x"] as? Int == 7)
        await c.selectMonitor(-1)
        #expect(await ev.next() == .clip(#"20:{"i":-1}"#))
        await c.pause(true)
        #expect(await ev.next() == .clip(#"21:{"on":true}"#))
        await c.getClipboard()
        #expect(await ev.next() == .clip("19:"))
        c.close()
        withExtendedLifetime(server) {}
    }

    @Test func aTypingGetsItsAnswer() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session).openAssistant(serverID: "srv-a", session: "console")
        let ev = AssistEvents(c)
        for _ in 0..<3 { _ = await ev.next() }
        #expect(await c.type("secret", after: "enter", label: "key") == true)
        _ = await ev.next() // the echo of the stand-in
        #expect(await ev.next() == .typed(ok: true, reason: nil))
        #expect(await c.type("a\tb") == false) // refused here, never sent
        c.close()
        withExtendedLifetime(server) {}
    }

    @Test func aTextWithOtherAlphabetsIsPlayedThroughTheClipboard() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session).openAssistant(serverID: "srv-a", session: "console")
        let ev = AssistEvents(c)
        for _ in 0..<3 { _ = await ev.next() }
        await c.play(AssistTyping.plan("мир"))
        // the clipboard comes first, then Ctrl down, V down, V up, Ctrl up
        #expect(await ev.next() == .clip(#"18:{"text":"мир"}"#))
        var keys = 0
        while case .clip(let t)? = await ev.next(), t.hasPrefix("16:") { keys += 1; if keys == 4 { break } }
        #expect(keys == 4)
        c.close()
        withExtendedLifetime(server) {}
    }

    @Test func theComputerEndsTheAssistant() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session).openAssistant(serverID: "srv-a", session: "console")
        let ev = AssistEvents(c)
        for _ in 0..<3 { _ = await ev.next() }
        await c.setClipboard("bye") // the stand-in ends on this
        #expect(await ev.next() == .end(reason: "done", reconnect: true))
        #expect(await ev.next() == nil) // nothing after the end, not even a close
        withExtendedLifetime(server) {}
    }

    @Test func aSessionThatIsOpenElsewhereIsToldByItsCode() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session).openAssistant(serverID: "full", session: "console")
        let ev = AssistEvents(c)
        guard case .closed(_, let code)? = await ev.next() else { Issue.record("expected a close"); return }
        #expect(code == AssistantConnection.busyCode)
        withExtendedLifetime(server) {}
    }

    @Test func refusalsAreToldInOurTerms() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        do { _ = try await c.openAssistant(serverID: "off", session: "console"); Issue.record("opened an offline computer") } catch let e as LinagerError { #expect(e == .agentOffline) }
        do { _ = try await c.openAssistant(serverID: "denied", session: "console"); Issue.record("opened without a right") } catch let e as LinagerError { #expect(e == .noPermission) }
        do { _ = try await server.client().openAssistant(serverID: "srv-a", session: "console"); Issue.record("opened without a session") } catch let e as LinagerError { #expect(e == .unauthorized) }
        do { _ = try await server.client(pin: server.otherFingerprint, credentials: session).openAssistant(serverID: "srv-a", session: "console"); Issue.record("opened on a certificate that is not pinned") } catch let e as LinagerError { #expect(e == .pinMismatch(seen: server.fingerprint)) }
    }

    @Test func oddSessionsNeverReachTheAddress() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        for bad in ["", "a b", "1&x=2", "../1", "console2", "1234567"] {
            do { _ = try await c.openAssistant(serverID: "srv-a", session: bad); Issue.record("opened session \(bad)") } catch let e as LinagerError { #expect(e == .server(status: 400, message: "invalid session")) }
        }
        let numbered = try await c.openAssistant(serverID: "srv-a", session: "3")
        numbered.close()
        withExtendedLifetime(server) {}
    }

    @Test func settingsAreReadAndChanged() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        #expect(try await c.assistantSettings(serverID: "srv-a").preset == .normal)
        try await c.setAssistantSettings(serverID: "srv-a", .preset(.eco))
        #expect(try await c.assistantSettings(serverID: "srv-a").preset == .eco)
        withExtendedLifetime(server) {}
    }

    @Test func rightsAndOfferingOfTheAssistant() throws {
        func info(_ json: String) throws -> SessionInfo { try JSONDecoder().decode(SessionInfo.self, from: Data(json.utf8)) }
        #expect(try info(#"{"csrf":"x","login":"a","role":"admin"}"#).canUseAssistant)
        #expect(try info(#"{"csrf":"x","login":"a","role":"operator","modules":["assistant"]}"#).canUseAssistant)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"operator","modules":["terminal"]}"#).canUseAssistant)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"auditor","modules":["assistant"]}"#).canUseAssistant)
        #expect(assistantOffered(ServerCard(id: "w", name: "w", os: "Windows 11 Pro")))
        #expect(assistantOffered(ServerCard(id: "l", name: "l", os: "Ubuntu 24.04", sample: Sample(gui: true))))
        #expect(!assistantOffered(ServerCard(id: "d", name: "d", os: "Debian 12", sample: Sample())))
        #expect(!assistantOffered(ServerCard(id: "n", name: "n", os: "Debian 12")))
        let s = try JSONDecoder().decode(Sample.self, from: Data(#"{"cpu":1,"gui":true,"desktop_off":true}"#.utf8))
        #expect(s.gui && s.desktopOff)
    }
}

@Suite("Edge scroll")
struct EdgeScrollTests {
    @Test func standsStillInTheMiddleAndGrowsToTheEdges() {
        #expect(EdgeScroll.speed(pos: 400, size: 800) == 0)
        #expect(EdgeScroll.speed(pos: 0, size: 800) == -EdgeScroll.maxSpeed)
        #expect(EdgeScroll.speed(pos: 800, size: 800) == EdgeScroll.maxSpeed)
        let a = EdgeScroll.speed(pos: 40, size: 800), b = EdgeScroll.speed(pos: 10, size: 800)
        #expect(a < 0 && b < a) // nearer the edge is faster
        // a pointer outside the box counts as at the edge
        #expect(EdgeScroll.speed(pos: -30, size: 800) == -EdgeScroll.maxSpeed)
        #expect(EdgeScroll.speed(pos: 1000, size: 800) == EdgeScroll.maxSpeed)
    }

    @Test func aSmallBoxStillHasAMiddle() {
        // the area above the keyboard in landscape is about 100 points high: the zones take 30% each, the middle stays
        #expect(EdgeScroll.zone(of: 100) == 30)
        #expect(EdgeScroll.speed(pos: 50, size: 100) == 0)
        #expect(EdgeScroll.speed(pos: 95, size: 100) > 0)
        #expect(EdgeScroll.speed(pos: 10, size: 0) == 0)
    }
}
