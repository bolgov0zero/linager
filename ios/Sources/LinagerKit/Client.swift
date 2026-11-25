import Foundation

/// A live event of the server: the dashboard has changed (`metrics`, `status` or `dashboard`). The data is not read: the screen
/// asks the dashboard again.
public struct LiveEvent: Sendable, Equatable {
    public let name: String
    public init(name: String) { self.name = name }
}

/// What the state of the application needs from the server. LinagerClient is the real one; the tests bring their own.
public protocol LinagerAPI: Sendable {
    func login(login: String, password: String, totp: String?) async throws -> SessionInfo
    func me() async throws -> SessionInfo
    func dashboard() async throws -> [ServerCard]
    func folders() async throws -> [Folder]
    /// Administrators only; anyone else (and any failure) gets an empty answer: the flags are optional.
    func agentFlags() async -> [String: AgentFlag]
    func logout() async
    func live() async -> AsyncThrowingStream<LiveEvent, Error>
    /// The session cookie and the CSRF token as they are now, to be kept.
    func credentials() async -> Credentials
}

public struct Credentials: Sendable, Equatable {
    public var token: String?
    public var csrf: String?
    public init(token: String? = nil, csrf: String? = nil) {
        self.token = token
        self.csrf = csrf
    }
}

/// The connection to one server: TLS pinned to a fingerprint, the session cookie kept by hand (so it can be stored in the
/// Keychain and never goes to a shared cookie jar).
public actor LinagerClient: LinagerAPI {
    public static let cookieName = "linager_session"

    private let endpoint: Endpoint
    private let session: URLSession
    private let delegate: PinDelegate
    private var token: String?
    private var csrf: String?

    public init(endpoint: Endpoint, fingerprint: String, credentials: Credentials = Credentials()) {
        self.endpoint = endpoint
        self.delegate = PinDelegate(mode: .pinned(fingerprint))
        let cfg = URLSessionConfiguration.ephemeral
        cfg.httpCookieStorage = nil
        cfg.httpShouldSetCookies = false
        cfg.waitsForConnectivity = false
        cfg.timeoutIntervalForRequest = 60 // between two bytes; the live stream pings every 15 seconds
        cfg.httpAdditionalHeaders = ["Accept": "application/json", "User-Agent": "Linager-iOS"]
        self.session = URLSession(configuration: cfg, delegate: delegate, delegateQueue: nil)
        self.token = credentials.token
        self.csrf = credentials.csrf
    }

    deinit { session.invalidateAndCancel() }

    public func credentials() -> Credentials { Credentials(token: token, csrf: csrf) }

    // MARK: calls

    public func login(login: String, password: String, totp: String?) async throws -> SessionInfo {
        struct Body: Encodable { let login: String; let password: String; let totp: String? }
        let info: SessionInfo = try await send("POST", "api/login", body: Body(login: login, password: password, totp: totp))
        csrf = info.csrf
        return info
    }

    public func me() async throws -> SessionInfo {
        let info: SessionInfo = try await send("GET", "api/me")
        csrf = info.csrf
        return info
    }

    public func dashboard() async throws -> [ServerCard] {
        struct Reply: Decodable { let items: [ServerCard] }
        let r: Reply = try await send("GET", "api/dashboard")
        return r.items
    }

    public func folders() async throws -> [Folder] {
        try await send("GET", "api/groups")
    }

    public func agentFlags() async -> [String: AgentFlag] {
        struct Agent: Decodable { let id: String; let status: String }
        struct Reply: Decodable { let agents: [Agent]? }
        guard let r: Reply = try? await send("GET", "api/updates") else { return [:] }
        var out: [String: AgentFlag] = [:]
        for a in r.agents ?? [] {
            if let f = AgentFlag(rawValue: a.status) { out[a.id] = f }
        }
        return out
    }

    public func logout() async {
        struct Empty: Decodable {}
        _ = try? await send("POST", "api/logout", body: [String: String]()) as Empty
        token = nil
        csrf = nil
    }

    public func live() -> AsyncThrowingStream<LiveEvent, Error> {
        let request: URLRequest
        do { request = try makeRequest("GET", "api/live", body: nil) } catch {
            return AsyncThrowingStream { $0.finish(throwing: error) }
        }
        let streamRequest: URLRequest = {
            var r = request
            r.setValue("text/event-stream", forHTTPHeaderField: "Accept")
            r.timeoutInterval = 60
            return r
        }()
        let session = self.session
        let delegate = self.delegate
        return AsyncThrowingStream { continuation in
            let task = Task {
                do {
                    let (bytes, response) = try await session.bytes(for: streamRequest, delegate: delegate)
                    guard let http = response as? HTTPURLResponse else { throw LinagerError.badResponse }
                    guard (200..<300).contains(http.statusCode) else { throw LinagerError.fromStatus(http.statusCode, message: "") }
                    // `lines` drops the empty lines that end an event, so an event is announced by its `event:` line
                    for try await line in bytes.lines {
                        if line.hasPrefix("event:") {
                            let name = line.dropFirst("event:".count).trimmingCharacters(in: .whitespaces)
                            continuation.yield(LiveEvent(name: name))
                        }
                    }
                    continuation.finish()
                } catch is CancellationError {
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: LinagerError.from(error, delegate: delegate))
                }
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    // MARK: transport

    private func makeRequest(_ method: String, _ path: String, body: (any Encodable)?) throws -> URLRequest {
        guard let url = endpoint.url(path: path) else { throw LinagerError.unreachable("bad address") }
        var r = URLRequest(url: url)
        r.httpMethod = method
        r.timeoutInterval = 20
        r.cachePolicy = .reloadIgnoringLocalCacheData
        if let token { r.setValue("\(Self.cookieName)=\(token)", forHTTPHeaderField: "Cookie") }
        if method != "GET" {
            if let csrf { r.setValue(csrf, forHTTPHeaderField: "X-CSRF-Token") }
            r.setValue("application/json", forHTTPHeaderField: "Content-Type")
            r.httpBody = try JSONEncoder().encode(AnyEncodable(body ?? [String: String]()))
        }
        return r
    }

    private func send<T: Decodable>(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> T {
        let request = try makeRequest(method, path, body: body)
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw LinagerError.from(error, delegate: delegate)
        }
        guard let http = response as? HTTPURLResponse else { throw LinagerError.badResponse }
        takeCookie(http)
        guard (200..<300).contains(http.statusCode) else {
            throw LinagerError.fromStatus(http.statusCode, message: Self.errorMessage(data))
        }
        do { return try JSONDecoder().decode(T.self, from: data) } catch { throw LinagerError.badResponse }
    }

    /// The session cookie of an answer: kept, or dropped when the server clears it.
    private func takeCookie(_ http: HTTPURLResponse) {
        guard let url = http.url, let fields = http.allHeaderFields as? [String: String] else { return }
        for c in HTTPCookie.cookies(withResponseHeaderFields: fields, for: url) where c.name == Self.cookieName {
            token = c.value.isEmpty ? nil : c.value
        }
    }

    static func errorMessage(_ data: Data) -> String {
        struct E: Decodable { let error: String? }
        if let e = try? JSONDecoder().decode(E.self, from: data), let m = e.error { return m }
        return String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

private struct AnyEncodable: Encodable {
    let value: any Encodable
    init(_ value: any Encodable) { self.value = value }
    func encode(to encoder: Encoder) throws { try value.encode(to: encoder) }
}
