import Foundation
import Observation

/// What is certain about a certificate that changed: the pinned one and the one the server shows now.
public struct PinProblem: Equatable, Sendable {
    public let pinned: String
    public let seen: String?
}

/// A server whose certificate has been read and waits for the person to confirm it.
public struct PendingTrust: Equatable, Sendable, Identifiable {
    public let endpoint: Endpoint
    public let fingerprint: String
    public var id: String { endpoint.display + fingerprint }
}

/// The state of the whole application: which screen, what the server said last, what to show when it does not answer. The
/// screens only read it and call its methods.
@MainActor
@Observable
public final class AppModel {
    public enum Screen: Equatable, Sendable {
        case launching, connect, login, waiting, dashboard
    }

    // MARK: what the screens read

    public private(set) var screen: Screen = .launching
    public private(set) var endpoint: Endpoint?
    public private(set) var fingerprint: String?
    public private(set) var session: SessionInfo?

    // connect
    public private(set) var probing = false
    public private(set) var connectError: LinagerError?
    public private(set) var pending: PendingTrust?

    // login
    public private(set) var signingIn = false
    public private(set) var needsCode = false
    public private(set) var loginError: LinagerError?
    public private(set) var rememberedLogin: String?

    // waiting for the approval of this device
    /// The server cannot be asked while the device waits (the screen says so; it asks again by itself).
    public private(set) var waitingOffline = false

    // dashboard
    public private(set) var cards: [ServerCard] = []
    public private(set) var folders: [Folder] = []
    public private(set) var agentFlags: [String: AgentFlag] = [:]
    /// True until the first answer (or the first failure) of the dashboard.
    public private(set) var loading = false
    /// The server cannot be reached: what is shown comes from the last answer.
    public private(set) var offline = false
    /// When the data on the screen was received.
    public private(set) var updatedAt: Date?
    public private(set) var pinProblem: PinProblem?

    // MARK: dependencies

    private let store: ProfileStore
    private let snapshots: SnapshotStore
    private let makeClient: @Sendable (Profile) -> any LinagerAPI
    private let probe: @Sendable (Endpoint) async throws -> String
    private let now: @Sendable () -> Date
    /// How long to wait after a live event before the dashboard is asked again (a server sends one per server every few seconds).
    private let liveDelay: Duration
    private let retryDelay: Duration
    /// How often a device that waits for the approval asks whether it was approved.
    private let waitDelay: Duration
    private let deviceTokens: any DeviceTokenStore

    private var client: (any LinagerAPI)?
    private var liveTask: Task<Void, Never>?
    private var refreshTask: Task<Void, Never>?
    private var waitTask: Task<Void, Never>?

    public init(store: ProfileStore = KeychainProfileStore(),
                snapshots: SnapshotStore = FileSnapshotStore(),
                deviceTokens: any DeviceTokenStore = KeychainDeviceTokenStore(),
                clientInfo: ClientInfo = .current,
                makeClient: (@Sendable (Profile) -> any LinagerAPI)? = nil,
                probe: @escaping @Sendable (Endpoint) async throws -> String = { try await Probe.fingerprint(of: $0) },
                now: @escaping @Sendable () -> Date = Date.init,
                liveDelay: Duration = .seconds(1),
                retryDelay: Duration = .seconds(3),
                waitDelay: Duration = .seconds(3)) {
        self.store = store
        self.snapshots = snapshots
        self.deviceTokens = deviceTokens
        self.waitDelay = waitDelay
        // every request of the client carries the device: this phone's secret token for this server, and what it says about itself
        self.makeClient = makeClient ?? { profile in
            LinagerClient(endpoint: profile.endpoint, fingerprint: profile.fingerprint, credentials: profile.credentials,
                          device: DeviceIdentity(token: deviceTokens.token(for: profile.endpoint), info: clientInfo))
        }
        self.probe = probe
        self.now = now
        self.liveDelay = liveDelay
        self.retryDelay = retryDelay
    }

    // MARK: start

    /// The first thing at the launch: the saved server and session, or the connect screen.
    public func launch() async {
        guard let profile = store.load() else {
            screen = .connect
            return
        }
        adopt(profile)
        guard profile.token != nil else {
            screen = .login
            return
        }
        if let snap = snapshots.load(for: profile.endpoint) {
            cards = snap.cards
            folders = snap.folders
            updatedAt = snap.savedAt
        }
        screen = .dashboard
        loading = cards.isEmpty
        guard let client else { return }
        do {
            let info = try await client.me()
            session = info
            persist(from: client)
            if info.isWaiting {
                loading = false
                becomeWaiting()
                return
            }
            await refresh()
        } catch let e as LinagerError {
            loading = false
            switch e {
            case .unauthorized: endSession()
            default: handle(e)
            }
        } catch {
            loading = false
        }
    }

    private func adopt(_ profile: Profile) {
        endpoint = profile.endpoint
        fingerprint = profile.fingerprint
        rememberedLogin = profile.login
        client = makeClient(profile)
    }

    // MARK: connect

    /// Reads the certificate of the server and asks the person to confirm it.
    public func connect(address: String, port: String) async {
        connectError = nil
        guard let ep = Endpoint.parse(address: address, port: port) else {
            connectError = .unreachable("bad address")
            return
        }
        probing = true
        defer { probing = false }
        do {
            let fp = try await probe(ep)
            pending = PendingTrust(endpoint: ep, fingerprint: fp)
        } catch let e as LinagerError {
            connectError = e
        } catch {
            connectError = .unreachable(error.localizedDescription)
        }
    }

    public func cancelTrust() { pending = nil }

    /// The person compared the fingerprint and trusts it: it is pinned and the sign-in follows.
    public func trust() {
        guard let p = pending else { return }
        pending = nil
        let profile = Profile(endpoint: p.endpoint, fingerprint: p.fingerprint)
        store.save(profile)
        adopt(profile)
        resetState()
        screen = .login
    }

    // MARK: sign-in

    public func signIn(login: String, password: String, code: String?) async {
        guard let client else { return }
        loginError = nil
        signingIn = true
        defer { signingIn = false }
        let typed = login.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanCode = code?.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            let info = try await client.login(login: typed, password: password, totp: (cleanCode?.isEmpty ?? true) ? nil : cleanCode)
            session = info
            needsCode = false
            rememberedLogin = info.login
            persist(from: client, login: info.login)
            if info.isWaiting {
                // signed in, but an administrator has not approved this device: it may only wait and sign out
                becomeWaiting()
                return
            }
            screen = .dashboard
            loading = true
            await refresh()
        } catch LinagerError.totpRequired {
            needsCode = true
        } catch let e as LinagerError {
            if case .pinMismatch(let seen) = e { pinProblem = PinProblem(pinned: fingerprint ?? "", seen: seen) }
            loginError = e
        } catch {
            loginError = .unreachable(error.localizedDescription)
        }
    }

    /// Back from the second step to the password.
    public func restartLogin() {
        needsCode = false
        loginError = nil
    }

    // MARK: dashboard

    /// Asks the server for the dashboard. Several callers at once share one request.
    public func refresh() async {
        if let t = refreshTask {
            await t.value
            return
        }
        let t = Task { [weak self] in
            guard let self else { return }
            await self.load()
        }
        refreshTask = t
        await t.value
        refreshTask = nil
    }

    private func load() async {
        guard let client, let endpoint else { return }
        do {
            async let c = client.dashboard()
            async let f = client.folders()
            async let a = client.agentFlags()
            let (newCards, newFolders, flags) = try await (c, f, a)
            cards = newCards
            folders = newFolders
            agentFlags = flags
            offline = false
            pinProblem = nil
            loading = false
            let stamp = now()
            updatedAt = stamp
            snapshots.save(DashboardSnapshot(endpoint: endpoint, savedAt: stamp, cards: newCards, folders: newFolders))
            persist(from: client)
        } catch let e as LinagerError {
            loading = false
            handle(e)
        } catch {
            loading = false
            offline = true
        }
    }

    private func handle(_ e: LinagerError) {
        switch e {
        case .unreachable, .server, .badResponse, .rateLimited: offline = true
        case .pinMismatch(let seen):
            // the stream and the refreshes would only try again and again against a certificate that is not trusted
            offline = true
            stopLive()
            let problem = PinProblem(pinned: fingerprint ?? "", seen: seen)
            if problem != dismissedPin { pinProblem = problem }
        case .unauthorized: endSession()
        case .approvalPending: becomeWaiting()
        case .deviceDenied: endSession(.deviceDenied)
        case .deviceRequired: endSession(.deviceRequired)
        case .accessDenied: offline = true
        case .invalidCredentials, .totpRequired, .twoFactorSetupRequired, .noPermission, .agentOffline,
             .fileChanged, .exists, .confirmRequired, .tooLarge, .notText, .notFound: break
        }
    }

    /// The server no longer knows the session: the sign-in again, on the same server. `notice` is why, when the sign-in screen should say it
    /// (an administrator refused or removed this device).
    private func endSession(_ notice: LinagerError? = nil) {
        stopLive()
        stopWaiting()
        if var p = store.load() {
            p.token = nil
            p.csrf = nil
            store.save(p)
            adopt(p)
        }
        session = nil
        screen = .login
        loginError = notice
    }

    // MARK: waiting for the approval of the device

    private func becomeWaiting() {
        guard screen != .waiting else { return }
        stopLive()
        screen = .waiting
        startWaiting()
    }

    /// Asks "who am I" every few seconds (the one thing a device that waits may ask) until the answer says the device is approved.
    private func startWaiting() {
        waitTask?.cancel()
        waitTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self, let client = self.client else { return }
                do {
                    let info = try await client.me()
                    if Task.isCancelled { return }
                    self.waitingOffline = false
                    if !info.isWaiting {
                        self.session = info
                        self.persist(from: client)
                        self.waitTask = nil
                        self.screen = .dashboard
                        self.loading = true
                        await self.refresh()
                        return
                    }
                } catch LinagerError.deviceDenied {
                    if !Task.isCancelled { self.endSession(.deviceDenied) }
                    return
                } catch LinagerError.unauthorized {
                    // an administrator removed the device (and with it the session)
                    if !Task.isCancelled { self.endSession(.unauthorized) }
                    return
                } catch {
                    if Task.isCancelled { return }
                    self.waitingOffline = true
                }
                try? await Task.sleep(for: self.waitDelay)
            }
        }
    }

    private func stopWaiting() {
        waitTask?.cancel()
        waitTask = nil
        waitingOffline = false
    }

    // MARK: live

    /// The stream of live events, while the dashboard is on the screen: every event asks for the dashboard again, after a short
    /// pause so that the many events of a busy server make one request. A broken stream is opened again after a pause.
    public func startLive() {
        guard liveTask == nil, screen == .dashboard else { return }
        liveTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self, let client = self.client else { return }
                await self.refresh() // catches up after a pause or a broken stream
                do {
                    for try await _ in await client.live() { self.liveEvent() }
                } catch let e as LinagerError {
                    if !Task.isCancelled { self.handle(e) }
                } catch {
                    if !Task.isCancelled { self.offline = true }
                }
                if Task.isCancelled { return }
                try? await Task.sleep(for: self.retryDelay)
            }
        }
    }

    private var liveScheduled = false

    private func liveEvent() {
        guard !liveScheduled else { return }
        liveScheduled = true
        let delay = liveDelay
        Task { [weak self] in
            try? await Task.sleep(for: delay)
            guard let self else { return }
            self.liveScheduled = false
            await self.refresh()
        }
    }

    public func stopLive() {
        liveTask?.cancel()
        liveTask = nil
    }

    // MARK: terminal

    /// Whether the account may open a terminal (an administrator, or an operator who was given that module).
    public var canUseTerminal: Bool { session?.canUseTerminal ?? false }

    /// Opens a shell on a server. A session that has ended or a certificate that changed is dealt with here, as for every other call.
    public func openTerminal(serverID: String, cols: Int, rows: Int, user: String? = nil) async throws -> TerminalConnection {
        guard let client else { throw LinagerError.unauthorized }
        do {
            return try await client.openTerminal(serverID: serverID, cols: cols, rows: rows, user: user)
        } catch let e as LinagerError {
            switch e {
            case .unauthorized, .pinMismatch, .approvalPending, .deviceDenied: handle(e)
            default: break
            }
            throw e
        }
    }

    /// Whether the account may open the Assistant (an administrator, or an operator who was given that module).
    public var canUseAssistant: Bool { session?.canUseAssistant ?? false }

    public func openAssistant(serverID: String, session: String = "console") async throws -> AssistantConnection {
        guard let client else { throw LinagerError.unauthorized }
        do {
            return try await client.openAssistant(serverID: serverID, session: session)
        } catch let e as LinagerError {
            switch e {
            case .unauthorized, .pinMismatch, .approvalPending, .deviceDenied: handle(e)
            default: break
            }
            throw e
        }
    }

    /// Whether the account may open the remote desktop (an administrator, or an operator who was given that module).
    public var canUseDesktop: Bool { session?.canUseDesktop ?? false }

    public func openDesktop(serverID: String, params: RdpParams, engine: RdpEngine) async throws -> RdpConnection {
        guard let client else { throw LinagerError.unauthorized }
        do {
            return try await client.openDesktop(serverID: serverID, params: params, engine: engine)
        } catch let e as LinagerError {
            switch e {
            case .unauthorized, .pinMismatch, .approvalPending, .deviceDenied: handle(e)
            default: break
            }
            throw e
        }
    }

    // MARK: the page of a server

    /// Runs a call of the page of a server: a session that has ended or a certificate that changed is handled like anywhere else.
    private func page<T>(_ call: (any LinagerAPI) async throws -> T) async throws -> T {
        guard let client else { throw LinagerError.unauthorized }
        do {
            return try await call(client)
        } catch let e as LinagerError {
            switch e {
            case .unauthorized, .pinMismatch, .approvalPending, .deviceDenied: handle(e)
            default: break
            }
            throw e
        }
    }

    public func serverDetail(serverID: String) async throws -> ServerDetail { try await page { try await $0.serverDetail(serverID: serverID) } }
    public func history(serverID: String, period: HistoryPeriod) async throws -> History { try await page { try await $0.history(serverID: serverID, period: period) } }
    public func files(serverID: String, path: String) async throws -> FileList { try await page { try await $0.files(serverID: serverID, path: path) } }
    public func downloadFile(serverID: String, path: String) async throws -> URL { try await page { try await $0.downloadFile(serverID: serverID, path: path) } }
    public func fileContent(serverID: String, path: String) async throws -> FileContent { try await page { try await $0.fileContent(serverID: serverID, path: path) } }
    public func saveFile(serverID: String, path: String, content: String, mtime: Int64) async throws -> Int64 { try await page { try await $0.saveFile(serverID: serverID, path: path, content: content, mtime: mtime) } }
    public func createFile(serverID: String, path: String) async throws { try await page { try await $0.createFile(serverID: serverID, path: path) } }
    public func createFolder(serverID: String, path: String) async throws { try await page { try await $0.createFolder(serverID: serverID, path: path) } }
    public func deleteEntry(serverID: String, path: String) async throws { try await page { try await $0.deleteEntry(serverID: serverID, path: path) } }
    public func renameEntry(serverID: String, path: String, name: String) async throws { try await page { try await $0.renameEntry(serverID: serverID, path: path, name: name) } }
    public func transferEntry(serverID: String, path: String, to dir: String, move: Bool, mode: TransferMode) async throws { try await page { try await $0.transferEntry(serverID: serverID, path: path, to: dir, move: move, mode: mode) } }
    public func stat(serverID: String, path: String) async throws -> FileStat { try await page { try await $0.stat(serverID: serverID, path: path) } }
    public func setPermissions(serverID: String, path: String, mode: String, recursive: Bool) async throws { try await page { try await $0.setPermissions(serverID: serverID, path: path, mode: mode, recursive: recursive) } }
    public func setOwner(serverID: String, path: String, owner: String, group: String, recursive: Bool) async throws { try await page { try await $0.setOwner(serverID: serverID, path: path, owner: owner, group: group, recursive: recursive) } }
    public func uploadFile(serverID: String, to path: String, from file: URL, progress: @escaping @Sendable (Double) -> Void) async throws { try await page { try await $0.uploadFile(serverID: serverID, to: path, from: file, progress: progress) } }

    public func services(serverID: String) async throws -> [ServiceUnit] { try await page { try await $0.services(serverID: serverID) } }
    public func serviceAction(serverID: String, unit: String, action: ServiceAction, confirm: Bool) async throws { try await page { try await $0.serviceAction(serverID: serverID, unit: unit, action: action, confirm: confirm) } }
    public func processes(serverID: String, sort: ProcessSort, count: Int = 40) async throws -> ProcessList { try await page { try await $0.processes(serverID: serverID, sort: sort, count: count) } }
    public func killProcess(serverID: String, pid: Int, signal: String) async throws { try await page { try await $0.killProcess(serverID: serverID, pid: pid, signal: signal) } }
    public func apt(serverID: String) async throws -> AptList { try await page { try await $0.apt(serverID: serverID) } }
    public func docker(serverID: String) async throws -> DockerInfo { try await page { try await $0.docker(serverID: serverID) } }
    public func dockerAction(serverID: String, containerID: String, action: DockerAction) async throws { try await page { try await $0.dockerAction(serverID: serverID, containerID: containerID, action: action) } }
    public func disk(serverID: String) async throws -> DiskState { try await page { try await $0.disk(serverID: serverID) } }
    public func diskClean(serverID: String, kind: String) async throws -> DiskCleanResult { try await page { try await $0.diskClean(serverID: serverID, kind: kind) } }
    public func reboot(serverID: String) async throws { try await page { try await $0.reboot(serverID: serverID) } }
    /// Removes the server; the dashboard is asked again at once (the server is gone from it).
    public func deleteServer(serverID: String, name: String, uninstall: Bool) async throws {
        try await page { try await $0.deleteServer(serverID: serverID, name: name, uninstall: uninstall) }
        await refresh()
    }
    public func winUpdates(serverID: String) async throws -> WinUpdateList { try await page { try await $0.winUpdates(serverID: serverID) } }
    public func openStream(serverID: String, stream: ServerStream) async throws -> TerminalConnection { try await page { try await $0.openStream(serverID: serverID, stream: stream) } }

    public func assistantSettings(serverID: String) async throws -> AssistSettings {
        guard let client else { throw LinagerError.unauthorized }
        return try await client.assistantSettings(serverID: serverID)
    }

    public func setAssistantSettings(serverID: String, _ settings: AssistSettings) async throws {
        guard let client else { throw LinagerError.unauthorized }
        try await client.setAssistantSettings(serverID: serverID, settings)
    }

    public func snippets() async throws -> [Snippet] {
        guard let client else { throw LinagerError.unauthorized }
        return try await client.snippets()
    }

    public func snippetRan(id: Int64, serverID: String) async { await client?.snippetRan(id: id, serverID: serverID) }

    public func terminalUsers(serverID: String) async -> [String] { await client?.terminalUsers(serverID: serverID) ?? [] }

    // MARK: leaving

    /// Ends the session on the server and on the phone, and keeps the server: the sign-in follows.
    public func signOut() async {
        stopLive()
        stopWaiting()
        await client?.logout()
        endSession()
        cards = []
        folders = []
        snapshots.clear()
    }

    /// Forgets the server: the pin, the session and the cached dashboard are erased, the connect screen follows.
    public func forgetServer() async {
        stopLive()
        stopWaiting()
        await client?.logout()
        // the device is forgotten with the server: if it is added again the server sees a new device
        if let endpoint { deviceTokens.forget(endpoint) }
        store.clear()
        snapshots.clear()
        client = nil
        endpoint = nil
        fingerprint = nil
        rememberedLogin = nil
        resetState()
        screen = .connect
    }

    /// The server shows a new certificate and the person confirmed it (after comparing the fingerprints): it replaces the pinned one.
    public func acceptNewCertificate() {
        guard let p = pinProblem, let seen = p.seen, var profile = store.load() else { return }
        profile.fingerprint = seen
        store.save(profile)
        adopt(profile)
        pinProblem = nil
        dismissedPin = nil
        connectError = nil
        loginError = nil
        offline = false
        if screen == .dashboard {
            Task {
                await refresh()
                startLive()
            }
        }
    }

    /// "Not now": the same certificate is not announced again until the person acts or the server shows another one.
    public func dismissPinProblem() {
        dismissedPin = pinProblem
        pinProblem = nil
    }

    private var dismissedPin: PinProblem?

    // MARK: helpers

    private func persist(from client: any LinagerAPI, login: String? = nil) {
        Task {
            let c = await client.credentials()
            guard var p = store.load(), p.endpoint == endpoint else { return }
            p.token = c.token
            p.csrf = c.csrf
            if let login { p.login = login }
            store.save(p)
        }
    }

    private func resetState() {
        cards = []
        folders = []
        agentFlags = [:]
        session = nil
        offline = false
        updatedAt = nil
        pinProblem = nil
        dismissedPin = nil
        loginError = nil
        needsCode = false
        connectError = nil
    }
}
