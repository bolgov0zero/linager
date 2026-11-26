import Foundation
import Testing
@testable import LinagerKit

private let session = Credentials(token: "tok123", csrf: nil)

/// Reads the events of a terminal in the background so that a test can ask for the next one and never hangs: after three seconds without
/// one it gets a marker (`TIMEOUT`) instead. `nil` means the stream is over.
private actor Events {
    private var queue: [TerminalEvent] = []
    private var over = false

    init(_ t: TerminalConnection) {
        let stream = t.events
        Task { [self] in
            for await e in stream { await push(e) }
            await finish()
        }
    }

    private func push(_ e: TerminalEvent) { queue.append(e) }
    private func finish() { over = true }

    func next() async -> TerminalEvent? {
        for _ in 0..<300 {
            if !queue.isEmpty { return queue.removeFirst() }
            if over { return nil }
            try? await Task.sleep(for: .milliseconds(10))
        }
        return .closed(reason: "TIMEOUT", code: -1)
    }
}

private func text(_ e: TerminalEvent?) -> String {
    if case .output(let d)? = e { return String(decoding: d, as: UTF8.self) }
    return "<\(String(describing: e))>"
}

@Suite("Terminal", .serialized)
struct TerminalTests {
    @Test func opensAShellAndTalksToIt() async throws {
        let server = try FakeServer()
        let t = try await server.client(credentials: session).openTerminal(serverID: "srv-a", cols: 100, rows: 30)
        let ev = Events(t)
        #expect(text(await ev.next()) == "opened 100x30 kind=terminal user=\n")
        await t.send(Data("ls\n".utf8))
        #expect(text(await ev.next()) == "echo:ls\n")
        await t.send(Data([0x03]))
        #expect(await ev.next() == .output(Data("echo:".utf8) + Data([0x03])))
        await t.resize(cols: 120, rows: 40)
        #expect(text(await ev.next()) == "resized 120x40\n")
        t.close()
        withExtendedLifetime(server) {} // the fake server must live as long as the terminal does
    }

    @Test func theEndOfTheShellIsAnnouncedOnce() async throws {
        let server = try FakeServer()
        let t = try await server.client(credentials: session).openTerminal(serverID: "srv-a", cols: 80, rows: 24)
        let ev = Events(t)
        _ = await ev.next() // the greeting
        await t.send(Data("exit\n".utf8))
        #expect(await ev.next() == .closed(reason: "exit", code: 0))
        #expect(await ev.next() == nil) // and the stream is over
        withExtendedLifetime(server) {}
    }

    @Test func theLimitOfTerminalsIsToldByItsCode() async throws {
        let server = try FakeServer()
        let t = try await server.client(credentials: session).openTerminal(serverID: "full", cols: 80, rows: 24)
        let ev = Events(t)
        let e = await ev.next()
        guard case .closed(_, let code)? = e else { Issue.record("expected a close, got \(String(describing: e))"); return }
        #expect(code == TerminalConnection.tooManyCode)
        withExtendedLifetime(server) {}
    }

    @Test func anOfflineAgentAndAMissingRightAreTold() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        do { _ = try await c.openTerminal(serverID: "off", cols: 80, rows: 24); Issue.record("opened a terminal of an offline server") } catch let e as LinagerError { #expect(e == .agentOffline) }
        do { _ = try await c.openTerminal(serverID: "denied", cols: 80, rows: 24); Issue.record("opened a terminal without the right") } catch let e as LinagerError { #expect(e == .noPermission) }
    }

    @Test func withoutASessionNothingOpens() async throws {
        let server = try FakeServer()
        do { _ = try await server.client().openTerminal(serverID: "srv-a", cols: 80, rows: 24); Issue.record("opened without a session") } catch let e as LinagerError { #expect(e == .unauthorized) }
    }

    @Test func aCertificateThatIsNotPinnedIsRefused() async throws {
        let server = try FakeServer()
        do {
            _ = try await server.client(pin: server.otherFingerprint, credentials: session).openTerminal(serverID: "srv-a", cols: 80, rows: 24)
            Issue.record("a terminal opened over a certificate that is not pinned")
        } catch let e as LinagerError { #expect(e == .pinMismatch(seen: server.fingerprint)) }
    }

    @Test func serverIdsAreEscapedInThePath() async throws {
        let server = try FakeServer()
        // "a/../off" must stay one path segment: unescaped it would read as the id "off" (an offline agent) and the open would fail
        let t = try await server.client(credentials: session).openTerminal(serverID: "a/../off", cols: 80, rows: 24)
        let ev = Events(t)
        #expect(text(await ev.next()) == "opened 80x24 kind=terminal user=\n")
        t.close()
        withExtendedLifetime(server) {}
    }

    @Test func aShellCanBeOpenedAsAnotherUser() async throws {
        let server = try FakeServer()
        let t = try await server.client(credentials: session).openTerminal(serverID: "srv-a", cols: 80, rows: 24, user: "anna")
        let ev = Events(t)
        #expect(text(await ev.next()) == "opened 80x24 kind=terminal user=anna\n")
        t.close()
        withExtendedLifetime(server) {}
    }

    @Test func oddUserNamesNeverReachTheAddress() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        for bad in ["a b", "x&kind=logs", "../root", "-x", String(repeating: "a", count: 65), "a/b"] {
            do { _ = try await c.openTerminal(serverID: "srv-a", cols: 80, rows: 24, user: bad); Issue.record("opened as \(bad)") } catch let e as LinagerError { #expect(e == .server(status: 400, message: "invalid user")) }
        }
        // an empty name is the default shell
        let t = try await c.openTerminal(serverID: "srv-a", cols: 80, rows: 24, user: "")
        #expect(text(await Events(t).next()) == "opened 80x24 kind=terminal user=\n")
        t.close()
    }

    @Test func snippetsAndTheirRunAreKnownToTheServer() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: Credentials(token: "tok123", csrf: "csrf-abc"))
        let list = try await c.snippets()
        #expect(list.map(\.name) == ["Disk usage", "Two lines"])
        #expect(list[1].command == "cd /var/log\nls")
        await c.snippetRan(id: list[0].id, serverID: "srv-a")
        // the stand-in keeps what it was told
        var req = URLRequest(url: try #require(server.endpoint.url(path: "/api/ran")))
        req.httpMethod = "POST"
        req.setValue("tok-ignored", forHTTPHeaderField: "X-Test")
        req.httpBody = Data("{}".utf8)
        let session = URLSession(configuration: .ephemeral, delegate: PinDelegate(mode: .pinned(server.fingerprint)), delegateQueue: nil)
        let (data, _) = try await session.data(for: req)
        let ran = try JSONDecoder().decode([[String]].self, from: data)
        #expect(ran == [["1", "srv-a"]])
        withExtendedLifetime(server) {}
    }

    @Test func snippetsNeedASession() async throws {
        let server = try FakeServer()
        do { _ = try await server.client().snippets(); Issue.record("snippets without a session") } catch let e as LinagerError { #expect(e == .unauthorized) }
    }

    @Test func onlyPeopleAndRootCanBeChosenAsTheUser() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        #expect(await c.terminalUsers(serverID: "srv-a") == ["root", "debian", "anna"]) // not www-data
        #expect(await c.terminalUsers(serverID: "denied").isEmpty) // an account without the right to see them gets none
        #expect(await server.client().terminalUsers(serverID: "srv-a").isEmpty) // nor one without a session
        withExtendedLifetime(server) {}
    }

    @Test func rightsToTheTerminal() throws {
        func info(_ json: String) throws -> SessionInfo { try JSONDecoder().decode(SessionInfo.self, from: Data(json.utf8)) }
        #expect(try info(#"{"csrf":"x","login":"a","role":"admin"}"#).canUseTerminal)
        #expect(try info(#"{"csrf":"x","login":"a","role":"operator","modules":["files","terminal"]}"#).canUseTerminal)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"operator","modules":["files"]}"#).canUseTerminal)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"auditor","modules":["terminal"]}"#).canUseTerminal)
        #expect(try !info(#"{"csrf":"x","login":"a","role":"operator"}"#).canUseTerminal) // an older server sends no modules
    }
}
