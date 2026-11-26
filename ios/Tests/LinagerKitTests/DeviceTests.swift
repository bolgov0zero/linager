import Foundation
import Testing
@testable import LinagerKit

private let session = Credentials(token: "tok123", csrf: "csrf-abc")
private let ep = Endpoint(host: "srv.example.com", port: 8443)

@Suite("Device")
struct DeviceModelTests {
    @Test func theClientIsDescribedAsAQueryString() throws {
        let info = ClientInfo(platform: "ios", app: "0.1.0 (55)", os: "27.1", model: "iPhone18,1")
        let items = try #require(URLComponents(string: "x://h/?" + info.header)?.queryItems)
        #expect(items.first { $0.name == "platform" }?.value == "ios")
        #expect(items.first { $0.name == "app" }?.value == "0.1.0 (55)")
        #expect(items.first { $0.name == "os" }?.value == "27.1")
        // the comma of a model is escaped: the server reads the header as a query string
        #expect(info.header.contains("model=iPhone18%2C1"))
        // what is empty is left out
        #expect(!ClientInfo(platform: "ios", app: "", os: "", model: "").header.contains("app="))
    }

    @Test func thisInstallationDescribesItself() {
        let c = ClientInfo.current
        #expect(["ios", "macos"].contains(c.platform))
        #expect(!c.os.isEmpty && !c.model.isEmpty)
    }

    @Test func aTokenIsRandomAndFitsTheServer() {
        let a = DeviceIdentity.makeToken(), b = DeviceIdentity.makeToken()
        #expect(a != b)
        // 32 bytes as base64url without padding: 43 characters the server accepts (32 to 128 of that alphabet)
        #expect(a.count == 43)
        #expect(a.allSatisfy { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" })
    }

    @Test func eachServerHasItsOwnDeviceAndForgettingMakesANewOne() {
        let store = MemoryDeviceTokenStore()
        let other = Endpoint(host: "other.example.com", port: 8443)
        let a = store.token(for: ep)
        #expect(store.token(for: ep) == a)
        #expect(store.token(for: other) != a)
        store.forget(ep)
        #expect(store.token(for: ep) != a)
    }

    @Test func theServerAnswersAreToldApart() {
        #expect(LinagerError.fromStatus(403, message: "approval_pending") == .approvalPending)
        #expect(LinagerError.fromStatus(403, message: "device_denied") == .deviceDenied)
        #expect(LinagerError.fromStatus(403, message: "device_required") == .deviceRequired)
        #expect(LinagerError.fromStatus(403, message: "forbidden") == .accessDenied)
    }

    @Test func theSessionTellsWhetherTheDeviceWaits() throws {
        let waiting = try JSONDecoder().decode(SessionInfo.self, from: Data(#"{"csrf":"c","login":"a","device":{"confirm":true,"id":3,"status":"pending"}}"#.utf8))
        #expect(waiting.isWaiting && waiting.device?.id == 3)
        let approved = try JSONDecoder().decode(SessionInfo.self, from: Data(#"{"csrf":"c","login":"a","device":{"confirm":true,"status":"approved"}}"#.utf8))
        #expect(!approved.isWaiting)
        // with the confirmation off nothing waits, whatever the status says
        let off = try JSONDecoder().decode(SessionInfo.self, from: Data(#"{"csrf":"c","login":"a","device":{"confirm":false,"status":"pending"}}"#.utf8))
        #expect(!off.isWaiting)
        // an older server does not know devices
        let old = try JSONDecoder().decode(SessionInfo.self, from: Data(#"{"csrf":"c","login":"a"}"#.utf8))
        #expect(old.device == nil && !old.isWaiting)
    }
}

@Suite("Device over a real connection", .serialized)
struct DeviceConnectionTests {
    private func identity(_ first: Character) -> DeviceIdentity {
        DeviceIdentity(token: String(first) + String(repeating: "x", count: 42), info: ClientInfo(platform: "ios", app: "9.9", os: "27.1", model: "iPhone18,1"))
    }

    @Test func everyRequestAndEverySocketCarriesTheDevice() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session, device: identity("A"))
        _ = try await c.me()
        _ = try await c.dashboard()
        let t = try await c.openTerminal(serverID: "srv-a", cols: 80, rows: 24)
        t.close()
        let seen = try await server.seen()
        #expect(seen.count >= 3)
        for row in seen {
            #expect(row[0] == "A" + String(repeating: "x", count: 42))
            #expect(row[1].contains("platform=ios") && row[1].contains("app=9.9") && row[1].contains("model=iPhone18%2C1"))
        }
        withExtendedLifetime(server) {}
    }

    @Test func aClientWithNoDeviceSendsNoHeaders() async throws {
        let server = try FakeServer()
        _ = try await server.client(credentials: session).me()
        #expect(try await server.seen().allSatisfy { $0[0].isEmpty && $0[1].isEmpty })
        withExtendedLifetime(server) {}
    }

    @Test func aDeviceThatWaitsMayOnlyAskWhoItIsUntilItIsApproved() async throws {
        let server = try FakeServer()
        let c = server.client(device: identity("P"))
        let info = try await c.login(login: "admin", password: "secret", totp: nil)
        #expect(info.isWaiting)
        #expect(try await c.me().isWaiting)
        await #expect(throws: LinagerError.approvalPending) { _ = try await c.dashboard() }
        try await server.approveDevices()
        #expect(try await c.me().isWaiting == false)
        #expect(try await c.dashboard().count == 4)
        withExtendedLifetime(server) {}
    }

    @Test func aDeviceThatWasRefusedCannotSignInAndIsToldSo() async throws {
        let server = try FakeServer()
        let c = server.client(device: identity("D"))
        await #expect(throws: LinagerError.deviceDenied) { _ = try await c.login(login: "admin", password: "secret", totp: nil) }
        let signedIn = server.client(credentials: session, device: identity("D"))
        await #expect(throws: LinagerError.deviceDenied) { _ = try await signedIn.dashboard() }
        withExtendedLifetime(server) {}
    }
}
