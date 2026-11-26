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
    /// Opens a shell on a server (the terminal). It fails when the agent is offline, when the account has no right, or when there is no session.
    func openTerminal(serverID: String, cols: Int, rows: Int, user: String?) async throws -> TerminalConnection
    /// Opens the Assistant of a computer: its screen as tiles, the input to it. `session` is "console" or the number of a Windows session.
    func openAssistant(serverID: String, session: String) async throws -> AssistantConnection
    /// Opens the remote desktop (RDP) of a Windows computer through the RDP core `engine`; the core signs in with `params` inside the connection.
    func openDesktop(serverID: String, params: RdpParams, engine: RdpEngine) async throws -> RdpConnection
    /// The page of a server: the host, the last measurement, the modules.
    func serverDetail(serverID: String) async throws -> ServerDetail
    /// The history of the metrics of a server.
    func history(serverID: String, period: HistoryPeriod) async throws -> History
    /// The entries of a folder of a server.
    func files(serverID: String, path: String) async throws -> FileList
    /// Downloads a file of a server into a temporary file of the phone and returns where it is.
    func downloadFile(serverID: String, path: String) async throws -> URL
    /// The text of a file (up to a megabyte) and the time it was changed, which the save checks.
    func fileContent(serverID: String, path: String) async throws -> FileContent
    /// Saves a text. `mtime` is the time of the version that was read: if the file has been changed since, the save is refused (`.fileChanged`).
    func saveFile(serverID: String, path: String, content: String, mtime: Int64) async throws -> Int64
    func createFile(serverID: String, path: String) async throws
    func createFolder(serverID: String, path: String) async throws
    func deleteEntry(serverID: String, path: String) async throws
    func renameEntry(serverID: String, path: String, name: String) async throws
    /// Copies or moves an entry into a folder. `mode`: "fail" (an existing one stops it), "keep" (both stay, the new one gets another name), "overwrite" (confirmed).
    func transferEntry(serverID: String, path: String, to dir: String, move: Bool, mode: TransferMode) async throws
    func stat(serverID: String, path: String) async throws -> FileStat
    func setPermissions(serverID: String, path: String, mode: String, recursive: Bool) async throws
    func setOwner(serverID: String, path: String, owner: String, group: String, recursive: Bool) async throws
    /// Sends a file of the phone to a path of the server (the whole path, with the name).
    func uploadFile(serverID: String, to path: String, from file: URL, progress: @escaping @Sendable (Double) -> Void) async throws
    /// The services of the server (systemd) and what can be done with one. `confirm` answers the server that wants a dangerous action confirmed (`.confirmRequired`).
    func services(serverID: String) async throws -> [ServiceUnit]
    func serviceAction(serverID: String, unit: String, action: ServiceAction, confirm: Bool) async throws
    func processes(serverID: String, sort: ProcessSort, count: Int) async throws -> ProcessList
    /// `signal`: "TERM" (asks the program to end) or "KILL" (ends it at once).
    func killProcess(serverID: String, pid: Int, signal: String) async throws
    func apt(serverID: String) async throws -> AptList
    func docker(serverID: String) async throws -> DockerInfo
    func dockerAction(serverID: String, containerID: String, action: DockerAction) async throws
    func disk(serverID: String) async throws -> DiskState
    func diskClean(serverID: String, kind: String) async throws -> DiskCleanResult
    /// Restarts the server itself. The server asks for it with a confirmation; the application confirms with Face ID or the name of the server first.
    func reboot(serverID: String) async throws
    /// Removes the server from Linager. `name` is the name of the server (the server compares it), `uninstall` also removes the agent from the computer.
    func deleteServer(serverID: String, name: String, uninstall: Bool) async throws
    /// The updates Windows Update has for a Windows computer. The search can take minutes.
    func winUpdates(serverID: String) async throws -> WinUpdateList
    /// Opens a stream of text of a server: the journal, or the output of a job of the updates.
    func openStream(serverID: String, stream: ServerStream) async throws -> TerminalConnection
    /// The picture settings of the computer, kept on the server (one set per computer).
    func assistantSettings(serverID: String) async throws -> AssistSettings
    func setAssistantSettings(serverID: String, _ settings: AssistSettings) async throws
    /// The snippets of the server (commands to type into a terminal).
    func snippets() async throws -> [Snippet]
    /// Tells the server (for the audit journal) that a snippet was typed into the terminal of a server.
    func snippetRan(id: Int64, serverID: String) async
    /// The accounts a shell can be opened as: the people of the server and root. Empty when the account may not see them (or the agent is offline).
    func terminalUsers(serverID: String) async -> [String]
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
    private let device: DeviceIdentity?

    public init(endpoint: Endpoint, fingerprint: String, credentials: Credentials = Credentials(), device: DeviceIdentity? = nil) {
        self.endpoint = endpoint
        self.device = device
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
            // the stream keeps the client alive: its session is cancelled when the client goes
            let task = Task { [self] in
                defer { withExtendedLifetime(self) {} }
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

    public func openTerminal(serverID: String, cols: Int, rows: Int, user: String? = nil) async throws -> TerminalConnection {
        var items = [URLQueryItem(name: "kind", value: "terminal"), URLQueryItem(name: "cols", value: String(max(1, cols))), URLQueryItem(name: "rows", value: String(max(1, rows)))]
        if let user, !user.isEmpty {
            // the server accepts only plain account names and so do we: nothing odd goes into the address
            guard user.range(of: #"^[A-Za-z0-9._][A-Za-z0-9._-]{0,63}$"#, options: .regularExpression) != nil else { throw LinagerError.server(status: 400, message: "invalid user") }
            items.append(URLQueryItem(name: "user", value: user))
        }
        let task = try await openSocket(path: "api/servers/\(try Self.escaped(serverID))/stream", query: items)
        let connection = TerminalConnection(task: task, owner: self)
        connection.start()
        return connection
    }

    public func openAssistant(serverID: String, session: String = "console") async throws -> AssistantConnection {
        // "console" or the number of a Windows session, as the server accepts
        guard session.range(of: #"^(console|[0-9]{1,6})$"#, options: .regularExpression) != nil else { throw LinagerError.server(status: 400, message: "invalid session") }
        let task = try await openSocket(path: "api/servers/\(try Self.escaped(serverID))/assistant", query: [URLQueryItem(name: "session", value: session)])
        let connection = AssistantConnection(task: task, owner: self)
        connection.start()
        return connection
    }

    public func openDesktop(serverID: String, params: RdpParams, engine: RdpEngine) async throws -> RdpConnection {
        // the name goes only to the journal of the server, which takes up to 64 characters without control characters
        let login = String(params.username.unicodeScalars.filter { $0.value >= 0x20 }.prefix(64).map(Character.init))
        let task = try await openSocket(path: "api/servers/\(try Self.escaped(serverID))/desktop", query: [URLQueryItem(name: "login", value: login)])
        let connection = RdpConnection(task: task, engine: engine, owner: self)
        guard connection.start(params) else {
            task.cancel(with: .goingAway, reason: nil)
            throw LinagerError.unreachable("the RDP core did not start")
        }
        return connection
    }

    public func serverDetail(serverID: String) async throws -> ServerDetail {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))")
    }

    public func history(serverID: String, period: HistoryPeriod) async throws -> History {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))/metrics?period=\(period.rawValue)")
    }

    public func files(serverID: String, path: String) async throws -> FileList {
        guard let q = Self.queryValue(path) else { throw LinagerError.badResponse }
        return try await send("GET", "api/servers/\(try Self.escaped(serverID))/files?path=\(q)")
    }

    public func downloadFile(serverID: String, path: String) async throws -> URL {
        guard let q = Self.queryValue(path) else { throw LinagerError.badResponse }
        var request = try makeRequest("GET", "api/servers/\(try Self.escaped(serverID))/files/download?path=\(q)", body: nil)
        request.timeoutInterval = 600
        let location: URL
        let response: URLResponse
        do {
            (location, response) = try await session.download(for: request)
        } catch {
            throw LinagerError.from(error, delegate: delegate)
        }
        guard let http = response as? HTTPURLResponse else { throw LinagerError.badResponse }
        guard (200..<300).contains(http.statusCode) else {
            throw LinagerError.fromStatus(http.statusCode, message: "")
        }
        // the system deletes its temporary file when this returns: the file is moved to a name of its own (the name of the file on the server)
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("linager-download-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let name = (path as NSString).lastPathComponent.isEmpty ? "file" : (path as NSString).lastPathComponent
        let target = dir.appendingPathComponent(name)
        try FileManager.default.moveItem(at: location, to: target)
        return target
    }

    // MARK: file operations

    private struct Done: Decodable {}
    private struct PathBody: Encodable { let path: String }

    public func fileContent(serverID: String, path: String) async throws -> FileContent {
        guard let q = Self.queryValue(path) else { throw LinagerError.badResponse }
        return try await send("GET", "api/servers/\(try Self.escaped(serverID))/files/content?path=\(q)")
    }

    public func saveFile(serverID: String, path: String, content: String, mtime: Int64) async throws -> Int64 {
        struct Body: Encodable { let path: String; let content: String; let mtime: Int64 }
        struct Reply: Decodable { let mtime: Int64? }
        let r: Reply = try await send("PUT", "api/servers/\(try Self.escaped(serverID))/files/content", body: Body(path: path, content: content, mtime: mtime))
        return r.mtime ?? 0
    }

    public func createFile(serverID: String, path: String) async throws {
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/create", body: PathBody(path: path)) as Done
    }

    public func createFolder(serverID: String, path: String) async throws {
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/mkdir", body: PathBody(path: path)) as Done
    }

    public func deleteEntry(serverID: String, path: String) async throws {
        struct Body: Encodable { let path: String; let confirm = true }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/delete", body: Body(path: path)) as Done
    }

    public func renameEntry(serverID: String, path: String, name: String) async throws {
        struct Body: Encodable { let path: String; let name: String }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/rename", body: Body(path: path, name: name)) as Done
    }

    public func transferEntry(serverID: String, path: String, to dir: String, move: Bool, mode: TransferMode) async throws {
        struct Body: Encodable { let path: String; let dir: String; let mode: String; let confirm: Bool }
        let body = Body(path: path, dir: dir, mode: mode.rawValue, confirm: mode == .overwrite)
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/\(move ? "move" : "copy")", body: body) as Done
    }

    public func stat(serverID: String, path: String) async throws -> FileStat {
        guard let q = Self.queryValue(path) else { throw LinagerError.badResponse }
        return try await send("GET", "api/servers/\(try Self.escaped(serverID))/files/stat?path=\(q)")
    }

    public func setPermissions(serverID: String, path: String, mode: String, recursive: Bool) async throws {
        struct Body: Encodable { let path: String; let mode: String; let recursive: Bool; let confirm = true }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/chmod", body: Body(path: path, mode: mode, recursive: recursive)) as Done
    }

    public func setOwner(serverID: String, path: String, owner: String, group: String, recursive: Bool) async throws {
        struct Body: Encodable { let path: String; let owner: String; let group: String; let recursive: Bool; let confirm = true }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/files/chown", body: Body(path: path, owner: owner, group: group, recursive: recursive)) as Done
    }

    public func uploadFile(serverID: String, to path: String, from file: URL, progress: @escaping @Sendable (Double) -> Void) async throws {
        guard let q = Self.queryValue(path) else { throw LinagerError.badResponse }
        var request = try makeRequest("POST", "api/servers/\(try Self.escaped(serverID))/files/upload?path=\(q)", body: nil)
        request.httpBody = nil
        request.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 3600
        let watcher = UploadProgress(progress)
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.upload(for: request, fromFile: file, delegate: watcher)
        } catch {
            throw LinagerError.from(error, delegate: delegate)
        }
        guard let http = response as? HTTPURLResponse else { throw LinagerError.badResponse }
        guard (200..<300).contains(http.statusCode) else {
            throw LinagerError.fromStatus(http.statusCode, message: Self.errorMessage(data))
        }
    }

    // MARK: management of a server

    public func services(serverID: String) async throws -> [ServiceUnit] {
        struct Reply: Decodable { let services: [ServiceUnit]? }
        let r: Reply = try await send("GET", "api/servers/\(try Self.escaped(serverID))/services")
        return r.services ?? []
    }

    public func serviceAction(serverID: String, unit: String, action: ServiceAction, confirm: Bool) async throws {
        struct Body: Encodable { let confirm: Bool }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/services/\(try Self.escaped(unit))/\(action.rawValue)", body: Body(confirm: confirm)) as Done
    }

    public func processes(serverID: String, sort: ProcessSort, count: Int) async throws -> ProcessList {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))/processes?sort=\(sort.rawValue)&n=\(min(max(count, 1), 200))")
    }

    public func killProcess(serverID: String, pid: Int, signal: String) async throws {
        struct Body: Encodable { let signal: String; let confirm = true }
        guard pid > 1 else { throw LinagerError.server(status: 400, message: "invalid pid") }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/processes/\(pid)/kill", body: Body(signal: signal == "KILL" ? "KILL" : "TERM")) as Done
    }

    public func apt(serverID: String) async throws -> AptList {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))/apt")
    }

    public func docker(serverID: String) async throws -> DockerInfo {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))/docker")
    }

    public func dockerAction(serverID: String, containerID: String, action: DockerAction) async throws {
        struct Body: Encodable { let confirm: Bool }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/docker/containers/\(try Self.escaped(containerID))/\(action.rawValue)", body: Body(confirm: action == .remove)) as Done
    }

    public func disk(serverID: String) async throws -> DiskState {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))/disk")
    }

    public func diskClean(serverID: String, kind: String) async throws -> DiskCleanResult {
        struct Body: Encodable { let kind: String; let confirm = true }
        return try await send("POST", "api/servers/\(try Self.escaped(serverID))/disk/clean", body: Body(kind: kind))
    }

    public func reboot(serverID: String) async throws {
        struct Body: Encodable { let confirm = true }
        _ = try await send("POST", "api/servers/\(try Self.escaped(serverID))/reboot", body: Body()) as Done
    }

    public func deleteServer(serverID: String, name: String, uninstall: Bool) async throws {
        struct Body: Encodable { let name: String; let uninstall: Bool }
        _ = try await send("DELETE", "api/servers/\(try Self.escaped(serverID))", body: Body(name: name, uninstall: uninstall)) as Done
    }

    public func winUpdates(serverID: String) async throws -> WinUpdateList {
        struct Empty: Encodable {}
        // Windows Update searches for a long time (the server waits up to six minutes), so the phone waits as long
        return try await send("POST", "api/servers/\(try Self.escaped(serverID))/win/winupdate/list", body: Empty(), timeout: 400)
    }

    public func openStream(serverID: String, stream: ServerStream) async throws -> TerminalConnection {
        guard let query = stream.query else { throw LinagerError.server(status: 400, message: "invalid stream") }
        let task = try await openSocket(path: "api/servers/\(try Self.escaped(serverID))/stream", query: query)
        let connection = TerminalConnection(task: task, owner: self)
        connection.start()
        return connection
    }

    public func assistantSettings(serverID: String) async throws -> AssistSettings {
        try await send("GET", "api/servers/\(try Self.escaped(serverID))/assistant/settings")
    }

    public func setAssistantSettings(serverID: String, _ settings: AssistSettings) async throws {
        struct Empty: Decodable {}
        _ = try await send("PUT", "api/servers/\(try Self.escaped(serverID))/assistant/settings", body: settings) as Empty
    }

    /// A value of a query: everything but letters, digits and a few signs is percent-encoded (so that "&", "?", "=", "+" and "#" in a name cannot be taken for the structure).
    static func queryValue(_ s: String) -> String? {
        s.addingPercentEncoding(withAllowedCharacters: CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~/"))
    }

    private static func escaped(_ id: String) throws -> String {
        guard let e = id.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) else { throw LinagerError.unreachable("bad address") }
        return e
    }

    /// Opens a WebSocket of the server and returns once the upgrade is done. A refusal of the server (the handshake gets an HTTP status) is told
    /// in our terms: no session, no right, an agent that is offline.
    private func openSocket(path: String, query: [URLQueryItem]) async throws -> URLSessionWebSocketTask {
        guard var comps = URLComponents(url: endpoint.url(path: path) ?? endpoint.url(path: "/")!, resolvingAgainstBaseURL: false) else { throw LinagerError.unreachable("bad address") }
        comps.scheme = "wss"
        comps.queryItems = query
        guard let url = comps.url else { throw LinagerError.unreachable("bad address") }
        var request = URLRequest(url: url)
        request.timeoutInterval = 20
        if let token { request.setValue("\(Self.cookieName)=\(token)", forHTTPHeaderField: "Cookie") }
        device?.apply(to: &request)
        let task = session.webSocketTask(with: request)
        task.delegate = delegate
        task.resume()
        do {
            try await delegate.waitUntilOpen(task)
        } catch {
            let status = (task.response as? HTTPURLResponse)?.statusCode
            task.cancel(with: .goingAway, reason: nil)
            switch status {
            case 401?: throw LinagerError.unauthorized
            case 403?: throw LinagerError.noPermission
            case 503?: throw LinagerError.agentOffline
            case let s? where s != 101: throw LinagerError.server(status: s, message: "")
            default: throw LinagerError.from(error, delegate: delegate)
            }
        }
        return task
    }

    public func snippets() async throws -> [Snippet] {
        struct Reply: Decodable { let snippets: [Snippet]? }
        let r: Reply = try await send("GET", "api/snippets")
        return r.snippets ?? []
    }

    public func snippetRan(id: Int64, serverID: String) async {
        struct Body: Encodable { let server_id: String }
        struct Empty: Decodable {}
        _ = try? await send("POST", "api/snippets/\(id)/ran", body: Body(server_id: serverID)) as Empty
    }

    public func terminalUsers(serverID: String) async -> [String] {
        struct U: Decodable { let name: String; let human: Bool? }
        struct Reply: Decodable { let users: [U]? }
        guard let id = serverID.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed),
              let r: Reply = try? await send("GET", "api/servers/\(id)/users") else { return [] }
        return (r.users ?? []).filter { $0.human == true || $0.name == "root" }.map(\.name)
    }

    // MARK: transport

    private func makeRequest(_ method: String, _ path: String, body: (any Encodable)?, timeout: TimeInterval = 20) throws -> URLRequest {
        guard let url = endpoint.url(path: path) else { throw LinagerError.unreachable("bad address") }
        var r = URLRequest(url: url)
        r.httpMethod = method
        r.timeoutInterval = timeout
        r.cachePolicy = .reloadIgnoringLocalCacheData
        if let token { r.setValue("\(Self.cookieName)=\(token)", forHTTPHeaderField: "Cookie") }
        device?.apply(to: &r)
        if method != "GET" {
            if let csrf { r.setValue(csrf, forHTTPHeaderField: "X-CSRF-Token") }
            r.setValue("application/json", forHTTPHeaderField: "Content-Type")
            r.httpBody = try JSONEncoder().encode(AnyEncodable(body ?? [String: String]()))
        }
        return r
    }

    private func send<T: Decodable>(_ method: String, _ path: String, body: (any Encodable)? = nil, timeout: TimeInterval = 20) async throws -> T {
        let request = try makeRequest(method, path, body: body, timeout: timeout)
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


/// Tells how much of an upload has gone (a delegate of one task).
final class UploadProgress: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    private let report: @Sendable (Double) -> Void

    init(_ report: @escaping @Sendable (Double) -> Void) { self.report = report }

    func urlSession(_ session: URLSession, task: URLSessionTask, didSendBodyData bytesSent: Int64, totalBytesSent: Int64, totalBytesExpectedToSend: Int64) {
        guard totalBytesExpectedToSend > 0 else { return }
        report(min(1, Double(totalBytesSent) / Double(totalBytesExpectedToSend)))
    }
}
