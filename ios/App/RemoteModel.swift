import CoreGraphics
import LinagerKit
import Observation
import SwiftUI
import UIKit

/// What a remote screen needs from the application.
struct AssistBackend {
    var open: @MainActor () async throws -> AssistantConnection
    var settings: @MainActor () async throws -> AssistSettings
    var setSettings: @MainActor (AssistSettings) async throws -> Void
}

/// What a remote screen of the remote desktop (RDP) needs from the application: a connection that is already signed in.
struct RdpBackend {
    var open: @MainActor () async throws -> RdpConnection
}

/// The remote screen of the Assistant, and of the remote desktop (RDP), which shares the picture, the touches and the keyboard with it:
/// connects, keeps the picture of the computer, carries the input, tells how it ended.
@MainActor
@Observable
final class RemoteModel {
    enum State: Equatable {
        case connecting
        case open
        /// The screen of the computer is off and is being woken.
        case waking
        /// The Assistant ended or the connection broke; the text says why.
        case closed(String)
        /// The session is already open in another Assistant.
        case busy
        case failed(LinagerError)
    }

    private(set) var state: State = .connecting
    private(set) var hello: AssistHello?
    private(set) var stats = AssistStats()
    private(set) var settings: AssistSettings?
    /// A short message for the middle of the screen ("Ctrl+Alt+Del sent").
    private(set) var notice: String?
    /// The modifiers that hold for the next key (the sticky Ctrl, Alt and Win of the key bar).
    private(set) var modifiers: AssistModifiers = []
    /// The picture has had its first tiles.
    private(set) var hasPicture = false
    private(set) var keyboardShown = false
    /// The Assistant is connecting again by itself after a sign-in (the picture stays, nothing is asked).
    private(set) var reconnecting = false

    /// The picture of the computer; the surface draws it.
    let screen = AssistantScreen()
    /// Called when the picture changed (at most about thirty times a second) or its size did.
    @ObservationIgnored var onFrame: ((CGImage?, CGSize) -> Void)?

    @ObservationIgnored private var backend: AssistBackend?
    @ObservationIgnored private var rdpBackend: RdpBackend?
    @ObservationIgnored private var rdp: RdpConnection?
    /// The size of the picture of the computer (the Assistant says it in its hello, the remote desktop when it has connected).
    @ObservationIgnored private var pictureSize = CGSize.zero
    var isRdp: Bool { rdpBackend != nil }
    @ObservationIgnored private var connection: AssistantConnection?
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    @ObservationIgnored private var output: AsyncStream<Out>.Continuation?
    @ObservationIgnored private var dirty = false
    @ObservationIgnored private var noticeTask: Task<Void, Never>?
    @ObservationIgnored private var awaitingClip = false
    @ObservationIgnored private var paused = false
    /// When the sign-in screen was on the picture last, and when a key was typed: a connection that ends soon after either is the sign-in changing the session.
    @ObservationIgnored private var signInSeen: Date?
    @ObservationIgnored private var typedAt: Date?
    @ObservationIgnored private var quietAttempts: [Date] = []

    private enum Out {
        case input(AssistInput)
        case clipboard(String)
    }

    func configure(_ backend: AssistBackend) {
        guard self.backend == nil else { return }
        self.backend = backend
        connect()
        Task { settings = try? await backend.settings() }
    }

    /// The remote desktop: the same screen, another connection.
    func configureRdp(_ backend: RdpBackend) {
        guard rdpBackend == nil, self.backend == nil else { return }
        rdpBackend = backend
        connectRdp()
    }

    private func connectRdp() {
        guard let rdpBackend else { return }
        state = .connecting
        hasPicture = false
        tasks.append(Task { [weak self] in
            do {
                let c = try await rdpBackend.open()
                self?.openedRdp(c)
            } catch let e as LinagerError {
                self?.state = .failed(e)
            } catch {
                self?.state = .failed(.unreachable(error.localizedDescription))
            }
        })
    }

    private func openedRdp(_ c: RdpConnection) {
        rdp = c
        let (stream, continuation) = AsyncStream<Out>.makeStream()
        output = continuation
        tasks.append(Task {
            for await o in stream {
                if case .input(let i) = o { c.send(i) }
            }
        })
        tasks.append(Task { [weak self] in
            for await e in c.events { self?.handleRdp(e) }
        })
    }

    private func handleRdp(_ e: RdpEvent) {
        switch e {
        case .connected(let w, let h):
            pictureSize = CGSize(width: w, height: h)
            screen.resize(width: w, height: h)
            onFrame?(nil, pictureSize)
            state = .open
        case .frame(let x, let y, let w, let h, let rgba):
            let screen = screen
            Task { [weak self] in
                await Task.detached { screen.applyRGBA(x: x, y: y, w: w, h: h, rgba: rgba) }.value
                self?.pictureChanged()
            }
        case .closed(let reason, let code):
            output?.finish()
            output = nil
            rdp = nil
            state = .closed(code == RdpConnection.limitCode ? L("rdp.limit") : (reason.isEmpty ? L("remote.ended") : reason))
        }
    }

    // MARK: connecting

    private func connect(quiet: Bool = false) {
        guard let backend else { return }
        if !quiet {
            state = .connecting
            hasPicture = false
        }
        tasks.append(Task { [weak self] in
            do {
                let connection = try await backend.open()
                self?.opened(connection)
            } catch let e as LinagerError {
                self?.connectFailed(e, quiet: quiet)
            } catch {
                self?.connectFailed(.unreachable(error.localizedDescription), quiet: quiet)
            }
        })
    }

    private func connectFailed(_ e: LinagerError, quiet: Bool) {
        // a quiet reconnect that does not get through is tried again (within the limit); anything else is told
        if quiet, case .unreachable = e, quietReconnectAllowed() {
            scheduleQuietReconnect()
        } else if quiet, case .agentOffline = e, quietReconnectAllowed() {
            scheduleQuietReconnect()
        } else {
            reconnecting = false
            state = .failed(e)
        }
    }

    /// A sign-in changes the session of the computer, and the connection that was open ends: if that happens soon after the sign-in screen or a
    /// typed key, the Assistant connects again by itself (not more than twelve times a minute, two seconds apart).
    private func signInIsRecent() -> Bool {
        let last = [signInSeen, typedAt].compactMap { $0 }.max()
        return last.map { Date().timeIntervalSince($0) < 30 } ?? false
    }

    private func quietReconnectAllowed() -> Bool {
        quietAttempts.removeAll { Date().timeIntervalSince($0) > 60 }
        return quietAttempts.count < 12
    }

    private func scheduleQuietReconnect() {
        quietAttempts.append(Date())
        reconnecting = true
        tasks.append(Task { [weak self] in
            try? await Task.sleep(for: .seconds(2))
            guard !Task.isCancelled else { return }
            self?.connect(quiet: true)
        })
    }

    private func opened(_ connection: AssistantConnection) {
        self.connection = connection
        reconnecting = false
        let (stream, continuation) = AsyncStream<Out>.makeStream()
        output = continuation
        tasks.append(Task {
            for await o in stream {
                switch o {
                case .input(let i): await connection.send(i)
                case .clipboard(let t):
                    await connection.setClipboard(t)
                    try? await Task.sleep(for: .milliseconds(150)) // the computer has the text before Ctrl+V comes
                }
            }
        })
        tasks.append(Task { [weak self] in
            for await e in connection.events { self?.handle(e) }
        })
        if paused { Task { await connection.pause(true) } }
    }

    private func handle(_ e: AssistantEvent) {
        switch e {
        case .hello(let h):
            hello = h
            pictureSize = CGSize(width: h.w, height: h.h)
            screen.resize(width: h.w, height: h.h)
            onFrame?(nil, pictureSize)
            if state == .connecting { state = .open }
        case .tiles(let tiles):
            let screen = screen
            Task { [weak self] in
                await Task.detached { screen.apply(tiles) }.value
                self?.pictureChanged()
            }
        case .stats(let s):
            stats = s
            if s.isSignInDesktop { signInSeen = Date() }
        case .waking(let on):
            state = on ? .waking : .open
        case .clip(let text):
            if awaitingClip {
                awaitingClip = false
                UIPasteboard.general.string = text
                show(L("remote.clip.got"))
            }
        case .typed(let ok, let reason):
            show(ok ? L("remote.type.ok") : (reason == "desktop" ? L("remote.type.desktop") : L("remote.type.failed")))
        case .end(let reason, let reconnect):
            endConnection()
            if (reconnect || signInIsRecent()) && quietReconnectAllowed() {
                // the helper asks to connect again (the screen changed: the desktop opened after the sign-in, the screen was locked), or the
                // sign-in just ended the session: no question, the picture stays
                scheduleQuietReconnect()
            } else {
                reconnecting = false
                state = .closed(reason.isEmpty ? L("remote.ended") : reason)
            }
        case .closed(let reason, let code):
            endConnection()
            if code != AssistantConnection.busyCode, signInIsRecent(), quietReconnectAllowed() {
                scheduleQuietReconnect()
            } else {
                reconnecting = false
                state = code == AssistantConnection.busyCode ? .busy : .closed(reason.isEmpty ? L("remote.lost") : reason)
            }
        }
    }

    private func pictureChanged() {
        hasPicture = true
        guard !dirty else { return }
        dirty = true
        Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(33))
            guard let self else { return }
            dirty = false
            if pictureSize.width > 0 { onFrame?(screen.image(), pictureSize) }
        }
    }

    private func endConnection() {
        output?.finish()
        output = nil
        connection = nil
        tasks.forEach { $0.cancel() }
        tasks = []
    }

    /// A new connection on the same screen (the picture stays until the first tiles of the new one).
    func reconnect() {
        guard case .closed = state else { return }
        quietAttempts = []
        if rdpBackend != nil { connectRdp() } else { connect() }
    }

    /// Leaves the screen: the Assistant on the computer stops with the connection.
    func close() {
        noticeTask?.cancel()
        connection?.close()
        rdp?.close()
        rdp = nil
        endConnection()
    }

    // MARK: input

    /// What was sent, for the line of diagnostics (Mouse → Input diagnostics): moves, presses, releases, and whether the left button is held.
    private(set) var sentMoves = 0
    private(set) var sentDowns = 0
    private(set) var sentUps = 0
    private(set) var leftHeld = false
    private(set) var lastPoint = CGPoint.zero

    func send(_ input: AssistInput) {
        switch input {
        case .move(let x, let y):
            sentMoves += 1
            lastPoint = CGPoint(x: x, y: y)
        case .button(let b, let down, let x, let y):
            if b == 0 {
                leftHeld = down
                if down { sentDowns += 1 } else { sentUps += 1 }
            }
            lastPoint = CGPoint(x: x, y: y)
        default: break
        }
        output?.yield(.input(input))
    }

    /// Whether the computer draws its own pointer into the picture (it is not by default: the phone draws its own).
    func setShowCursor(_ on: Bool) {
        guard let backend, var s = settings else { return }
        s.showCursor = on
        Task { [weak self] in
            do {
                try await backend.setSettings(s)
                self?.settings = s
            } catch {
                self?.show(L("remote.settings.failed"))
            }
        }
    }

    private func playInputs(_ inputs: [AssistInput]) {
        for i in inputs { output?.yield(.input(i)) }
    }

    private func play(_ steps: [AssistTyping.Step]) {
        for s in steps {
            switch s {
            case .input(let i): output?.yield(.input(i))
            case .clipboard(let t): output?.yield(.clipboard(t))
            }
        }
    }

    /// A key pressed and released, with the sticky modifiers (which are then let go).
    func press(_ key: KeyStroke) {
        if rdpBackend != nil { playInputs(RdpTyping.press(key, modifiers: modifiers)) } else { play(AssistTyping.press(key, modifiers: modifiers)) }
        modifiers = []
    }

    /// Text from the keyboard of the phone: keys of the US layout, and the clipboard for what the layout does not have.
    func typeText(_ text: String) {
        if rdpBackend != nil { playInputs(RdpTyping.steps(text, modifiers: modifiers)) } else { play(AssistTyping.plan(text, modifiers: modifiers)) }
        modifiers = []
    }

    func toggle(_ m: AssistModifiers) {
        if modifiers.contains(m) { modifiers.remove(m) } else { modifiers.insert(m) }
    }

    /// A shortcut: the keys together, whatever the sticky modifiers are.
    func shortcut(_ key: KeyStroke, with mods: AssistModifiers, label: String) {
        if rdpBackend != nil { playInputs(RdpTyping.press(key, modifiers: mods)) } else { play(AssistTyping.press(key, modifiers: mods)) }
        show(label)
    }

    func ctrlAltDel() {
        if rdpBackend != nil {
            // the remote desktop has no Ctrl+Alt+Del of the keyboard (Windows keeps it for itself): its own way is Ctrl+Alt+End
            playInputs(RdpTyping.press(ScanCodes.end, modifiers: [.ctrl, .alt]))
            show("Ctrl+Alt+End")
            return
        }
        send(.ctrlAltDel)
        show("Ctrl+Alt+Del")
    }

    func setPaused(_ on: Bool) {
        paused = on
        let c = connection
        Task { await c?.pause(on) }
    }

    // MARK: clipboard, monitor, settings, typing

    func sendClipboard() {
        guard let text = UIPasteboard.general.string, !text.isEmpty else { return show(L("remote.clip.empty")) }
        let c = connection
        Task { [weak self] in
            let ok = await c?.setClipboard(text) ?? false
            self?.show(ok ? L("remote.clip.sent") : L("remote.clip.big"))
        }
    }

    func fetchClipboard() {
        awaitingClip = true
        let c = connection
        Task { await c?.getClipboard() }
    }

    func selectMonitor(_ index: Int) {
        let c = connection
        Task { await c?.selectMonitor(index) }
    }

    func applyPreset(_ p: AssistSettings.Preset) {
        guard let backend else { return }
        var s = AssistSettings.preset(p)
        s.showCursor = settings?.showCursor
        Task { [weak self] in
            do {
                try await backend.setSettings(s)
                self?.settings = s
            } catch {
                self?.show(L("remote.settings.failed"))
            }
        }
    }

    /// Types a login or a password on the sign-in or the UAC screen (the text is sent as it is and is not kept anywhere).
    @discardableResult
    func typeOnSignIn(_ text: String, after: String) -> Bool {
        guard stats.isSignInDesktop, let c = connection else { return false }
        Task { await c.type(text, after: after, label: "iOS") }
        return true
    }

    /// Signs in with a key: what the key says to type, one thing after another, on the sign-in or UAC screen. Nothing is shown of the text.
    /// False when the screen is not one a text can be typed on.
    @discardableResult
    func signIn(_ steps: [SignInTyping]) -> Bool {
        guard stats.isSignInDesktop, let c = connection, !steps.isEmpty else { return false }
        typedAt = Date()
        Task { [weak self] in
            for (i, step) in steps.enumerated() {
                if i > 0 { try? await Task.sleep(for: .milliseconds(250)) } // the screen takes the focus to the next field
                let ok = await c.type(step.text, after: step.after, label: step.label)
                if !ok { self?.show(L("remote.type.failed")); return }
                self?.typedAt = Date()
            }
        }
        return true
    }

    func keyboardChanged(shown: Bool) { keyboardShown = shown }

    func show(_ text: String) {
        notice = text
        noticeTask?.cancel()
        noticeTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(1700))
            if !Task.isCancelled { self?.notice = nil }
        }
    }
}
