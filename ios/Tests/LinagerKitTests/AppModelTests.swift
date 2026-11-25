import Foundation
import Testing
@testable import LinagerKit

/// A server that does what the test tells it to.
actor FakeAPI: LinagerAPI {
    var meResult: Result<SessionInfo, LinagerError> = .success(SessionInfo(csrf: "c1", login: "admin"))
    var needsCode = false
    var loginError: LinagerError?
    var dashboardResult: Result<[ServerCard], LinagerError> = .success([])
    var foldersResult: [Folder] = []
    var flags: [String: AgentFlag] = [:]
    private(set) var dashboardCalls = 0
    private(set) var loggedOut = false
    private(set) var tokenNow: String? = "tok"
    private var liveContinuation: AsyncThrowingStream<LiveEvent, Error>.Continuation?

    func set(me: Result<SessionInfo, LinagerError>? = nil, needsCode: Bool? = nil, loginError: LinagerError?? = nil,
             dashboard: Result<[ServerCard], LinagerError>? = nil, folders: [Folder]? = nil, flags: [String: AgentFlag]? = nil) {
        if let me { meResult = me }
        if let needsCode { self.needsCode = needsCode }
        if let loginError { self.loginError = loginError }
        if let dashboard { dashboardResult = dashboard }
        if let folders { foldersResult = folders }
        if let flags { self.flags = flags }
    }

    func login(login: String, password: String, totp: String?) async throws -> SessionInfo {
        if let loginError { throw loginError }
        if needsCode && (totp ?? "").isEmpty { throw LinagerError.totpRequired }
        if needsCode && totp != "123456" { throw LinagerError.invalidCredentials }
        tokenNow = "tok-new"
        return SessionInfo(csrf: "c2", login: login)
    }

    func me() async throws -> SessionInfo { try meResult.get() }

    func dashboard() async throws -> [ServerCard] {
        dashboardCalls += 1
        return try dashboardResult.get()
    }

    func folders() async throws -> [Folder] { foldersResult }
    func agentFlags() async -> [String: AgentFlag] { flags }

    func logout() async {
        loggedOut = true
        tokenNow = nil
    }

    func credentials() async -> Credentials { Credentials(token: tokenNow, csrf: tokenNow == nil ? nil : "c2") }

    func live() async -> AsyncThrowingStream<LiveEvent, Error> {
        AsyncThrowingStream { self.liveContinuation = $0 }
    }

    func emit(_ name: String) { liveContinuation?.yield(LiveEvent(name: name)) }
    func breakStream() { liveContinuation?.finish(throwing: LinagerError.unreachable("gone")); liveContinuation = nil }
}

private let ep = Endpoint(host: "srv.example.com", port: 8443)
private let pin = "SHA256:pinned"
private let cardA = ServerCard(id: "a", name: "web-01", address: "10.0.0.1", online: true, sample: Sample(cpu: 10), folderId: nil)
private let cardB = ServerCard(id: "b", name: "db-01", address: "10.0.0.2", online: true, sample: Sample(cpu: 95), folderId: nil)

@MainActor
private struct Rig {
    let model: AppModel
    let api: FakeAPI
    let store: MemoryProfileStore
    let snaps: MemorySnapshotStore

    init(profile: Profile? = nil, probe: @escaping @Sendable (Endpoint) async throws -> String = { _ in pin }) {
        api = FakeAPI()
        store = MemoryProfileStore(profile)
        snaps = MemorySnapshotStore()
        let fake = api
        model = AppModel(store: store, snapshots: snaps, makeClient: { _ in fake }, probe: probe,
                         now: { Date(timeIntervalSince1970: 1_000_000) }, liveDelay: .milliseconds(30), retryDelay: .milliseconds(30))
    }

    /// The persisting of the session runs in a task of its own.
    func settle() async { try? await Task.sleep(for: .milliseconds(80)) }
}

@MainActor
@Suite("AppModel")
struct AppModelTests {
    @Test func firstLaunchAsksForTheServer() async {
        let r = Rig()
        #expect(r.model.screen == .launching)
        await r.model.launch()
        #expect(r.model.screen == .connect)
    }

    @Test func connectingShowsTheFingerprintAndTrustPinsIt() async {
        let r = Rig()
        await r.model.launch()
        await r.model.connect(address: "srv.example.com", port: "8443")
        #expect(r.model.pending == PendingTrust(endpoint: ep, fingerprint: pin))
        #expect(r.store.load() == nil) // nothing is kept before the person confirms
        r.model.trust()
        #expect(r.model.screen == .login)
        #expect(r.store.load()?.fingerprint == pin)
        #expect(r.store.load()?.endpoint == ep)
        #expect(r.model.pending == nil)
    }

    @Test func cancellingTheTrustKeepsNothing() async {
        let r = Rig()
        await r.model.launch()
        await r.model.connect(address: "srv.example.com", port: "8443")
        r.model.cancelTrust()
        #expect(r.model.pending == nil)
        #expect(r.store.load() == nil)
        #expect(r.model.screen == .connect)
    }

    @Test func aBadAddressAndAnUnreachableServerAreReported() async {
        let r = Rig(probe: { _ in throw LinagerError.unreachable("refused") })
        await r.model.launch()
        await r.model.connect(address: "", port: "8443")
        #expect(r.model.connectError == .unreachable("bad address"))
        await r.model.connect(address: "srv.example.com", port: "8443")
        #expect(r.model.connectError == .unreachable("refused"))
        #expect(r.model.pending == nil)
        #expect(!r.model.probing)
    }

    @Test func signInShowsTheDashboardAndKeepsTheSession() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin))
        await r.api.set(dashboard: .success([cardA, cardB]), folders: [Folder(id: 1, name: "Офис")], flags: ["b": .available])
        await r.model.launch()
        #expect(r.model.screen == .login)
        await r.model.signIn(login: " admin ", password: "x", code: nil)
        #expect(r.model.screen == .dashboard)
        #expect(r.model.cards.map(\.id) == ["a", "b"])
        #expect(r.model.folders.count == 1)
        #expect(r.model.agentFlags == ["b": .available])
        #expect(r.model.session?.login == "admin")
        #expect(!r.model.offline)
        #expect(r.model.updatedAt == Date(timeIntervalSince1970: 1_000_000))
        await r.settle()
        #expect(r.store.load()?.token == "tok-new")
        #expect(r.store.load()?.login == "admin")
        #expect(r.snaps.load(for: ep)?.cards.count == 2)
    }

    @Test func theSecondStepAsksForTheCodeAndAWrongCodeIsAnError() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin))
        await r.api.set(needsCode: true)
        await r.model.launch()
        await r.model.signIn(login: "admin", password: "x", code: nil)
        #expect(r.model.needsCode)
        #expect(r.model.screen == .login)
        #expect(r.model.loginError == nil)
        await r.model.signIn(login: "admin", password: "x", code: "000000")
        #expect(r.model.loginError == .invalidCredentials)
        #expect(r.model.needsCode)
        await r.model.signIn(login: "admin", password: "x", code: "123456")
        #expect(r.model.screen == .dashboard)
        #expect(!r.model.needsCode)
        r.model.restartLogin()
        #expect(r.model.loginError == nil)
    }

    @Test func aWrongPasswordStaysOnTheLoginScreen() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin))
        await r.api.set(loginError: .invalidCredentials)
        await r.model.launch()
        await r.model.signIn(login: "admin", password: "bad", code: nil)
        #expect(r.model.screen == .login)
        #expect(r.model.loginError == .invalidCredentials)
        #expect(!r.model.signingIn)
    }

    @Test func aSavedSessionOpensTheDashboardAtOnce() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        #expect(r.model.screen == .dashboard)
        #expect(r.model.cards.map(\.id) == ["a"])
        #expect(r.model.rememberedLogin == "admin")
        #expect(!r.model.loading)
    }

    @Test func anEndedSessionAsksForTheSignInAgainOnTheSameServer() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "old", csrf: "c"))
        await r.api.set(me: .failure(.unauthorized))
        await r.model.launch()
        #expect(r.model.screen == .login)
        #expect(r.store.load()?.token == nil)
        #expect(r.store.load()?.fingerprint == pin) // the pin stays
        #expect(r.model.rememberedLogin == "admin")
    }

    @Test func whenTheServerIsDownTheLastDashboardIsShownAsOffline() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        r.snaps.save(DashboardSnapshot(endpoint: ep, savedAt: Date(timeIntervalSince1970: 500), cards: [cardA, cardB], folders: []))
        await r.api.set(me: .failure(.unreachable("down")))
        await r.model.launch()
        #expect(r.model.screen == .dashboard)
        #expect(r.model.offline)
        #expect(r.model.cards.count == 2)
        #expect(r.model.updatedAt == Date(timeIntervalSince1970: 500))
    }

    @Test func aFailedRefreshKeepsTheCardsAndASuccessfulOneClearsTheFlag() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        await r.api.set(dashboard: .failure(.unreachable("down")))
        await r.model.refresh()
        #expect(r.model.offline)
        #expect(r.model.cards.map(\.id) == ["a"])
        await r.api.set(dashboard: .success([cardA, cardB]))
        await r.model.refresh()
        #expect(!r.model.offline)
        #expect(r.model.cards.count == 2)
    }

    @Test func aSessionThatEndsDuringARefreshSendsThePersonToTheSignIn() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        await r.api.set(dashboard: .failure(.unauthorized))
        await r.model.refresh()
        #expect(r.model.screen == .login)
        #expect(r.store.load()?.token == nil)
    }

    @Test func aChangedCertificateIsReportedAndCanBeAccepted() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        await r.api.set(dashboard: .failure(.pinMismatch(seen: "SHA256:new")))
        await r.model.refresh()
        #expect(r.model.pinProblem == PinProblem(pinned: pin, seen: "SHA256:new"))
        #expect(r.model.screen == .dashboard)
        #expect(r.store.load()?.fingerprint == pin) // nothing changes by itself
        await r.api.set(dashboard: .success([cardA, cardB]))
        r.model.acceptNewCertificate()
        #expect(r.store.load()?.fingerprint == "SHA256:new")
        #expect(r.model.pinProblem == nil)
        #expect(r.store.load()?.token == "tok") // the session stays
    }

    @Test func signOutKeepsTheServerAndForgettingErasesIt() async {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        await r.model.signOut()
        #expect(await r.api.loggedOut)
        #expect(r.model.screen == .login)
        #expect(r.model.cards.isEmpty)
        #expect(r.store.load()?.token == nil)
        #expect(r.store.load()?.fingerprint == pin)
        #expect(r.snaps.load(for: ep) == nil)

        await r.model.forgetServer()
        #expect(r.model.screen == .connect)
        #expect(r.store.load() == nil)
        #expect(r.model.endpoint == nil)
    }

    @Test func manyLiveEventsMakeOneRefresh() async throws {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        let before = await r.api.dashboardCalls
        r.model.startLive()
        try await Task.sleep(for: .milliseconds(100)) // the first refresh and the stream are up
        let afterStart = await r.api.dashboardCalls
        #expect(afterStart == before + 1)
        for _ in 0..<20 { await r.api.emit("metrics") }
        try await Task.sleep(for: .milliseconds(200))
        #expect(await r.api.dashboardCalls == afterStart + 1)
        r.model.stopLive()
    }

    @Test func aBrokenStreamIsOpenedAgain() async throws {
        let r = Rig(profile: Profile(endpoint: ep, fingerprint: pin, login: "admin", token: "tok", csrf: "c"))
        await r.api.set(dashboard: .success([cardA]))
        await r.model.launch()
        r.model.startLive()
        try await Task.sleep(for: .milliseconds(100))
        let before = await r.api.dashboardCalls
        await r.api.breakStream()
        try await Task.sleep(for: .milliseconds(250))
        #expect(await r.api.dashboardCalls > before) // it came back and asked for the dashboard again
        r.model.stopLive()
    }
}
