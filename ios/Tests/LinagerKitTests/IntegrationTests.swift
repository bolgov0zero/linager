import Foundation
import Testing
@testable import LinagerKit

/// A real TLS server (the stand-in of Fixtures/fake_server.py) with a certificate made for the test: the pinning, the sign-in with the
/// second step, the session cookie, the CSRF token and the live stream are tried on a real connection.
final class FakeServer: @unchecked Sendable {
    let port: Int
    let fingerprint: String
    let otherFingerprint: String
    private let process: Process
    private let dir: URL

    init() throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("linager-test-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        fingerprint = try Self.makeCert("a", in: dir)
        otherFingerprint = try Self.makeCert("b", in: dir)
        let script = try #require(Bundle.module.url(forResource: "fake_server.py", withExtension: nil, subdirectory: "Fixtures"))
        let dash = try #require(Bundle.module.url(forResource: "dashboard.json", withExtension: nil, subdirectory: "Fixtures"))
        let groups = try #require(Bundle.module.url(forResource: "groups.json", withExtension: nil, subdirectory: "Fixtures"))
        process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/python3")
        process.arguments = ["-I", script.path, dir.appendingPathComponent("a.crt").path, dir.appendingPathComponent("a.key").path, dash.path, groups.path]
        let out = Pipe()
        process.standardOutput = out
        process.standardError = FileHandle.nullDevice
        try process.run()
        let line = out.fileHandleForReading.availableData
        guard let text = String(data: line, encoding: .utf8), let p = Int(text.trimmingCharacters(in: .whitespacesAndNewlines)) else {
            process.terminate()
            throw CocoaError(.executableLoad)
        }
        port = p
    }

    private static func openssl(_ args: [String]) throws {
            let p = Process()
            p.executableURL = URL(fileURLWithPath: "/usr/bin/openssl")
            p.arguments = args
            p.standardOutput = FileHandle.nullDevice
            p.standardError = FileHandle.nullDevice
            try p.run()
            p.waitUntilExit()
            guard p.terminationStatus == 0 else { throw CocoaError(.executableLoad) }
        }
    private static func makeCert(_ name: String, in dir: URL) throws -> String {
            let key = dir.appendingPathComponent("\(name).key").path
            let crt = dir.appendingPathComponent("\(name).crt").path
            let der = dir.appendingPathComponent("\(name).der").path
            try openssl(["ecparam", "-genkey", "-name", "prime256v1", "-noout", "-out", key])
            try openssl(["req", "-new", "-x509", "-key", key, "-out", crt, "-days", "30", "-subj", "/CN=linager-test-\(name)"])
            try openssl(["x509", "-in", crt, "-outform", "der", "-out", der])
            return Fingerprint.of(der: try Data(contentsOf: URL(fileURLWithPath: der)))
        }

    var endpoint: Endpoint { Endpoint(host: "127.0.0.1", port: port) }

    func client(pin: String? = nil, credentials: Credentials = Credentials(), device: DeviceIdentity? = nil) -> LinagerClient {
        LinagerClient(endpoint: endpoint, fingerprint: pin ?? fingerprint, credentials: credentials, device: device)
    }

    /// What the stand-in saw of the devices: the token and the description of the client of every request that came with a session.
    func seen() async throws -> [[String]] {
        let session = URLSession(configuration: .ephemeral, delegate: PinDelegate(mode: .pinned(fingerprint)), delegateQueue: nil)
        let (data, _) = try await session.data(from: try #require(endpoint.url(path: "/api/test/seen")))
        return try JSONDecoder().decode([[String]].self, from: data)
    }

    func approveDevices() async throws {
        let session = URLSession(configuration: .ephemeral, delegate: PinDelegate(mode: .pinned(fingerprint)), delegateQueue: nil)
        var req = URLRequest(url: try #require(endpoint.url(path: "/api/test/approve")))
        req.httpMethod = "POST"
        req.httpBody = Data("{}".utf8)
        _ = try await session.data(for: req)
    }

    deinit {
        process.terminate()
        try? FileManager.default.removeItem(at: dir)
    }
}

@Suite("Real TLS connection", .serialized)
struct IntegrationTests {
    @Test func probeReadsTheFingerprintWithoutSendingAnything() async throws {
        let server = try FakeServer()
        let fp = try await Probe.fingerprint(of: server.endpoint)
        #expect(fp == server.fingerprint)
    }

    @Test func probeOfNothingFails() async throws {
        let server = try FakeServer()
        let closed = Endpoint(host: "127.0.0.1", port: server.port == 1 ? 2 : 1)
        await #expect(throws: LinagerError.self) { _ = try await Probe.fingerprint(of: closed, timeout: 3) }
    }

    @Test func signInWithThePinnedCertificate() async throws {
        let server = try FakeServer()
        let c = server.client()
        let info = try await c.login(login: "admin", password: "secret", totp: nil)
        #expect(info.login == "admin")
        let creds = await c.credentials()
        #expect(creds.token == "tok123")
        #expect(creds.csrf == "csrf-abc")
        let cards = try await c.dashboard()
        #expect(cards.count == 4)
        #expect(cards[0].tags.count == 2)
        let folders = try await c.folders()
        #expect(folders.count == 4)
    }

    @Test func anotherCertificateIsRefusedBeforeAnythingIsSent() async throws {
        let server = try FakeServer()
        let c = server.client(pin: server.otherFingerprint)
        do {
            _ = try await c.login(login: "admin", password: "secret", totp: nil)
            Issue.record("the sign-in went through a certificate that is not pinned")
        } catch let e as LinagerError {
            #expect(e == .pinMismatch(seen: server.fingerprint))
        }
        #expect(await c.credentials().token == nil)
    }

    @Test func wrongPasswordAndTheSecondStep() async throws {
        let server = try FakeServer()
        let c = server.client()
        do {
            _ = try await c.login(login: "admin", password: "nope", totp: nil)
            Issue.record("a wrong password signed in")
        } catch let e as LinagerError { #expect(e == .invalidCredentials) }

        do {
            _ = try await c.login(login: "twofa", password: "secret", totp: nil)
            Issue.record("the second step was skipped")
        } catch let e as LinagerError { #expect(e == .totpRequired) }
        #expect(await c.credentials().token == nil)

        do {
            _ = try await c.login(login: "twofa", password: "secret", totp: "000000")
            Issue.record("a wrong code signed in")
        } catch let e as LinagerError { #expect(e == .invalidCredentials) }

        let info = try await c.login(login: "twofa", password: "secret", totp: "123456")
        #expect(info.totpEnabled)
        #expect(await c.credentials().token == "tok123")
    }

    @Test func withoutASessionTheDashboardIsRefused() async throws {
        let server = try FakeServer()
        do {
            _ = try await server.client().dashboard()
            Issue.record("the dashboard was served without a session")
        } catch let e as LinagerError { #expect(e == .unauthorized) }
    }

    @Test func aSavedSessionWorksAfterARestart() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: Credentials(token: "tok123", csrf: nil))
        let info = try await c.me()
        #expect(info.csrf == "csrf-abc")
        #expect(await c.credentials().csrf == "csrf-abc")
    }

    @Test func accessGuardRefusalIsToldApart() async throws {
        let server = try FakeServer()
        let c = server.client()
        // the stand-in answers a plain 403 on /deny, like the access guard of the application port
        var req = URLRequest(url: try #require(server.endpoint.url(path: "/deny")))
        req.httpMethod = "GET"
        let delegate = PinDelegate(mode: .pinned(server.fingerprint))
        let session = URLSession(configuration: .ephemeral, delegate: delegate, delegateQueue: nil)
        let (data, resp) = try await session.data(for: req)
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        #expect(LinagerError.fromStatus(status, message: LinagerClient.errorMessage(data)) == .accessDenied)
        _ = c
    }

    @Test func logoutEndsTheSession() async throws {
        let server = try FakeServer()
        let c = server.client()
        _ = try await c.login(login: "admin", password: "secret", totp: nil)
        await c.logout()
        let creds = await c.credentials()
        #expect(creds.token == nil)
        #expect(creds.csrf == nil)
    }

    @Test func administratorsGetAgentFlags() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: Credentials(token: "tok123", csrf: nil))
        let flags = await c.agentFlags()
        #expect(flags == ["srv-b": .available])
    }

    @Test func liveStreamAnnouncesEvents() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: Credentials(token: "tok123", csrf: nil))
        var names: [String] = []
        for try await e in await c.live() { names.append(e.name) }
        #expect(names == ["metrics", "status", "dashboard"])
    }

    @Test func liveStreamWithoutASessionFails() async throws {
        let server = try FakeServer()
        do {
            for try await _ in await server.client().live() {}
            Issue.record("the stream opened without a session")
        } catch let e as LinagerError { #expect(e == .unauthorized) }
    }
}
