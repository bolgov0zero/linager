import Foundation
import Testing
@testable import LinagerKit

@Suite("Connection")
struct ConnectionTests {
    @Test func addressParsing() {
        #expect(Endpoint.parse(address: "linager.example.com", port: "8443") == Endpoint(host: "linager.example.com", port: 8443))
        #expect(Endpoint.parse(address: "  10.0.0.5 ", port: " 9000 ") == Endpoint(host: "10.0.0.5", port: 9000))
        #expect(Endpoint.parse(address: "https://srv.example.com/admin/", port: "8443") == Endpoint(host: "srv.example.com", port: 8443))
        // a port typed with the address wins over the port field
        #expect(Endpoint.parse(address: "srv.example.com:7443", port: "8443") == Endpoint(host: "srv.example.com", port: 7443))
        #expect(Endpoint.parse(address: "[::1]:7443", port: "8443") == Endpoint(host: "::1", port: 7443))
        #expect(Endpoint.parse(address: "[fe80::1]", port: "8443") == Endpoint(host: "fe80::1", port: 8443))
        #expect(Endpoint.parse(address: "fe80::1", port: "8443") == Endpoint(host: "fe80::1", port: 8443))
        #expect(Endpoint.parse(address: "", port: "8443") == nil)
        #expect(Endpoint.parse(address: "host", port: "") == nil)
        #expect(Endpoint.parse(address: "host", port: "0") == nil)
        #expect(Endpoint.parse(address: "host", port: "70000") == nil)
        #expect(Endpoint.parse(address: "ho st", port: "1") == nil)
    }

    @Test func urls() {
        #expect(Endpoint(host: "a.b", port: 8443).url(path: "api/me")?.absoluteString == "https://a.b:8443/api/me")
        #expect(Endpoint(host: "::1", port: 1).url(path: "/api/me")?.absoluteString == "https://[::1]:1/api/me")
        #expect(Endpoint(host: "::1", port: 1).display == "[::1]:1")
    }

    @Test func fingerprintFormat() {
        // the SHA-256 of "abc", base64 without the padding: the form the server shows
        #expect(Fingerprint.of(der: Data("abc".utf8)) == "SHA256:ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0")
        let fp = Fingerprint.of(der: Data("abc".utf8))
        #expect(Fingerprint.lines(fp, width: 24).joined() == fp)
        #expect(Fingerprint.lines(fp, width: 24).count == 3)
    }

    @Test func onlyThePinnedCertificateIsAccepted() {
        #expect(PinPolicy.accepts(.pinned("SHA256:aaa"), seen: "SHA256:aaa"))
        #expect(!PinPolicy.accepts(.pinned("SHA256:aaa"), seen: "SHA256:aab"))
        #expect(!PinPolicy.accepts(.pinned("SHA256:aaa"), seen: "sha256:aaa"))
        #expect(!PinPolicy.accepts(.probe, seen: "SHA256:aaa")) // the probe never lets a request through
    }

    @Test func statusesBecomeErrors() {
        #expect(LinagerError.fromStatus(401, message: "totp_required") == .totpRequired)
        #expect(LinagerError.fromStatus(401, message: "invalid credentials") == .invalidCredentials)
        #expect(LinagerError.fromStatus(401, message: "unauthorized") == .unauthorized)
        #expect(LinagerError.fromStatus(403, message: "forbidden") == .accessDenied)
        #expect(LinagerError.fromStatus(403, message: "bad csrf token") == .unauthorized)
        #expect(LinagerError.fromStatus(403, message: "2fa_required") == .twoFactorSetupRequired)
        #expect(LinagerError.fromStatus(429, message: "too many attempts") == .rateLimited("too many attempts"))
        #expect(LinagerError.fromStatus(500, message: "internal error") == .server(status: 500, message: "internal error"))
    }

    @Test func errorTextOfAnAnswer() {
        #expect(LinagerClient.errorMessage(Data(#"{"error":"bad csrf token"}"#.utf8)) == "bad csrf token")
        #expect(LinagerClient.errorMessage(Data("forbidden\n".utf8)) == "forbidden")
        #expect(LinagerClient.errorMessage(Data()) == "")
    }
}
