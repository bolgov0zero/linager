import CryptoKit
import Foundation
import Security

/// The address of a server: host and the port of the application (not the agent port).
public struct Endpoint: Codable, Hashable, Sendable {
    public var host: String
    public var port: Int

    public init(host: String, port: Int) {
        self.host = host
        self.port = port
    }

    /// What the connect screen typed: an address (a scheme, a port and a path are tolerated and cut) and a port.
    public static func parse(address: String, port: String) -> Endpoint? {
        var a = address.trimmingCharacters(in: .whitespacesAndNewlines)
        if let r = a.range(of: "://") { a = String(a[r.upperBound...]) }
        if let slash = a.firstIndex(of: "/") { a = String(a[..<slash]) }
        var typedPort: Int?
        if a.hasPrefix("[") {
            // an IPv6 literal: [::1] or [::1]:8443
            guard let close = a.firstIndex(of: "]") else { return nil }
            let rest = a[a.index(after: close)...]
            if rest.hasPrefix(":") { typedPort = Int(rest.dropFirst()) }
            a = String(a[a.index(after: a.startIndex)..<close])
        } else if a.filter({ $0 == ":" }).count == 1, let colon = a.firstIndex(of: ":") {
            typedPort = Int(a[a.index(after: colon)...])
            a = String(a[..<colon])
        }
        guard !a.isEmpty, !a.contains(" ") else { return nil }
        let p = typedPort ?? Int(port.trimmingCharacters(in: .whitespaces))
        guard let p, (1...65535).contains(p) else { return nil }
        return Endpoint(host: a, port: p)
    }

    func url(path: String) -> URL? {
        var c = URLComponents()
        c.scheme = "https"
        c.host = host.contains(":") ? "[\(host)]" : host
        c.port = port
        c.path = path.hasPrefix("/") ? path : "/" + path
        return c.url
    }

    /// For the screen: "host:port" (an IPv6 literal in brackets).
    public var display: String { host.contains(":") ? "[\(host)]:\(port)" : "\(host):\(port)" }
}

/// "SHA256:<base64>" of the DER bytes of a certificate: the form the server, the installer and the desktop application show.
public enum Fingerprint {
    public static func of(der: Data) -> String {
        let digest = Data(SHA256.hash(data: der))
        return "SHA256:" + digest.base64EncodedString().replacingOccurrences(of: "=", with: "")
    }

    /// The fingerprint in lines of at most `width` characters, for a screen that shows it to be compared by eye.
    public static func lines(_ fp: String, width: Int = 24) -> [String] {
        var out: [String] = []
        var cur = ""
        for ch in fp {
            cur.append(ch)
            if cur.count == width { out.append(cur); cur = "" }
        }
        if !cur.isEmpty { out.append(cur) }
        return out
    }
}

/// How a connection is judged: one certificate is trusted, the one with the pinned fingerprint. There is no chain to verify; the
/// pin is the trust (as in the desktop application and in SSH). The probe mode accepts nothing: it only reads the fingerprint.
public enum PinMode: Sendable, Equatable {
    case pinned(String)
    case probe
}

enum PinPolicy {
    static func accepts(_ mode: PinMode, seen: String) -> Bool {
        if case .pinned(let want) = mode { return want == seen }
        return false
    }
}

/// Judges the certificate of the server in the TLS handshake of every connection of a session, before any byte of the request
/// is sent. A refused certificate leaves a trace here, so the error can say "the certificate changed" instead of "failed".
final class PinDelegate: NSObject, URLSessionDelegate, URLSessionTaskDelegate, @unchecked Sendable {
    let mode: PinMode
    private let lock = NSLock()
    private var _seen: String?
    private var _mismatch = false

    init(mode: PinMode) { self.mode = mode }

    var seenFingerprint: String? { lock.withLock { _seen } }
    var mismatched: Bool { lock.withLock { _mismatch } }

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping @Sendable (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        judge(challenge, completionHandler)
    }

    /// The same judgement for a challenge that comes to the task (the streaming calls ask for it there).
    func urlSession(_ session: URLSession, task: URLSessionTask, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping @Sendable (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        judge(challenge, completionHandler)
    }

    private func judge(_ challenge: URLAuthenticationChallenge, _ completionHandler: @escaping @Sendable (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust, let trust = challenge.protectionSpace.serverTrust else {
            completionHandler(.performDefaultHandling, nil)
            return
        }
        guard let chain = SecTrustCopyCertificateChain(trust) as? [SecCertificate], let leaf = chain.first else {
            completionHandler(.cancelAuthenticationChallenge, nil)
            return
        }
        let fp = Fingerprint.of(der: SecCertificateCopyData(leaf) as Data)
        lock.withLock { _seen = fp }
        if PinPolicy.accepts(mode, seen: fp) {
            completionHandler(.useCredential, URLCredential(trust: trust))
        } else {
            if case .pinned = mode { lock.withLock { _mismatch = true } }
            completionHandler(.cancelAuthenticationChallenge, nil)
        }
    }
}

/// The error of a request as a line of numbers a person can read out: the domain and the code, the error under it, the codes of the
/// connection and of the TLS layer (what tells a refused connection from a failed handshake), then the text.
func describe(_ error: Error) -> String {
    let ns = error as NSError
    var parts = ["\(ns.domain) \(ns.code)"]
    if let u = ns.userInfo[NSUnderlyingErrorKey] as? NSError { parts.append("under \(u.domain) \(u.code)") }
    if let d = ns.userInfo["_kCFStreamErrorDomainKey"], let c = ns.userInfo["_kCFStreamErrorCodeKey"] { parts.append("stream \(d)/\(c)") }
    if let c = ns.userInfo["_NSURLErrorNWPathKey"] { parts.append("path \(c)") }
    parts.append(ns.localizedDescription)
    return parts.joined(separator: "; ")
}

extension LinagerError {
    /// A failed request, told in our terms. `delegate` knows whether the certificate was refused.
    static func from(_ error: Error, delegate: PinDelegate?) -> LinagerError {
        if let e = error as? LinagerError { return e }
        if let d = delegate, d.mismatched { return .pinMismatch(seen: d.seenFingerprint) }
        return .unreachable(describe(error))
    }
}

/// "Trust on first use": reads the fingerprint of the certificate a server shows, without sending anything. The person compares
/// it with the one the server shows (the installer, Settings, Connection) and only then it is pinned.
public enum Probe {
    public static func fingerprint(of endpoint: Endpoint, timeout: TimeInterval = 8) async throws -> String {
        guard let url = endpoint.url(path: "/api/me") else { throw LinagerError.unreachable("bad address") }
        let delegate = PinDelegate(mode: .probe)
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = timeout
        cfg.timeoutIntervalForResource = timeout + 2
        cfg.waitsForConnectivity = false
        cfg.httpCookieStorage = nil
        let session = URLSession(configuration: cfg, delegate: delegate, delegateQueue: nil)
        defer { session.invalidateAndCancel() }
        do {
            _ = try await session.data(from: url)
        } catch {
            if let fp = delegate.seenFingerprint { return fp }
            throw LinagerError.from(error, delegate: delegate)
        }
        if let fp = delegate.seenFingerprint { return fp }
        throw LinagerError.unreachable("no certificate")
    }
}
