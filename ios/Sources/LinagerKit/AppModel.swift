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
        case launching, connect, login, dashboard
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

    private var client: (any LinagerAPI)?
    private var liveTask: Task<Void, Never>?
    private var refreshTask: Task<Void, Never>?

    public init(store: ProfileStore = KeychainProfileStore(),
                snapshots: SnapshotStore = FileSnapshotStore(),
                makeClient: @escaping @Sendable (Profile) -> any LinagerAPI = { LinagerClient(endpoint: $0.endpoint, fingerprint: $0.fingerprint, credentials: $0.credentials) },
                probe: @escaping @Sendable (Endpoint) async throws -> String = { try await Probe.fingerprint(of: $0) },
                now: @escaping @Sendable () -> Date = Date.init,
                liveDelay: Duration = .seconds(1),
                retryDelay: Duration = .seconds(3)) {
        self.store = store
        self.snapshots = snapshots
        self.makeClient = makeClient
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
            session = try await client.me()
            persist(from: client)
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
        case .accessDenied: offline = true
        case .invalidCredentials, .totpRequired, .twoFactorSetupRequired: break
        }
    }

    /// The server no longer knows the session: the sign-in again, on the same server.
    private func endSession() {
        stopLive()
        if var p = store.load() {
            p.token = nil
            p.csrf = nil
            store.save(p)
            adopt(p)
        }
        session = nil
        screen = .login
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

    // MARK: leaving

    /// Ends the session on the server and on the phone, and keeps the server: the sign-in follows.
    public func signOut() async {
        stopLive()
        await client?.logout()
        endSession()
        cards = []
        folders = []
        snapshots.clear()
    }

    /// Forgets the server: the pin, the session and the cached dashboard are erased, the connect screen follows.
    public func forgetServer() async {
        stopLive()
        await client?.logout()
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
