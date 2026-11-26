import Foundation

// The remote desktop (RDP) of a Windows computer. The protocol itself is the RDP core written in Rust (IronRDP, `ios/rdp`); this file is what
// the rest of the application knows about it: the parameters, the events, what a typed text becomes. The core is behind `RdpEngine`, so that the
// package can be tested on a Mac without it.

public struct RdpParams: Sendable, Equatable {
    public var username: String
    public var password: String
    public var domain: String
    /// "address:port" of the computer as the server knows it.
    public var destination: String
    public var width: Int
    public var height: Int

    public init(username: String, password: String, domain: String = "", destination: String, width: Int = 1280, height: Int = 720) {
        self.username = username
        self.password = password
        self.domain = domain
        self.destination = destination
        self.width = width
        self.height = height
    }
}

public enum RdpEngineEvent: Sendable, Equatable {
    case connected(width: Int, height: Int)
    /// A changed rectangle of the picture: RGBA, rows packed.
    case frame(x: Int, y: Int, w: Int, h: Int, rgba: Data)
    /// The session is over; the text is empty for a plain end.
    case ended(String)
}

/// The RDP core. `send` carries a message to the WebSocket of the server, `event` tells what happens; both may be called from any thread.
public protocol RdpEngine: AnyObject, Sendable {
    /// Starts a session; false when it cannot be started.
    func start(_ params: RdpParams, send: @escaping @Sendable (Data) -> Void, event: @escaping @Sendable (RdpEngineEvent) -> Void) -> Bool
    /// A message from the WebSocket.
    func feed(_ data: Data)
    /// The WebSocket is closed.
    func feedEnd()
    func input(_ input: AssistInput)
    /// Ends the session and frees it. Blocks until the core has stopped, so it is not called from the callbacks.
    func stop()
}

public enum RdpEvent: Sendable, Equatable {
    case connected(width: Int, height: Int)
    case frame(x: Int, y: Int, w: Int, h: Int, rgba: Data)
    /// The session is over. `code` is 4030 when the server refused a second remote desktop of the computer, 0 otherwise.
    case closed(reason: String, code: Int)
}

/// What a typed text becomes for a remote desktop. A character of the US layout is a key (a shortcut, a game, a program that reads keys
/// takes it); any other is sent as the character itself, which RDP carries natively, so there is no clipboard in between as in the Assistant.
public enum RdpTyping {
    public static func steps(_ text: String, modifiers: AssistModifiers = []) -> [AssistInput] {
        var out: [AssistInput] = []
        var mods = modifiers
        for c in text {
            let key: KeyStroke?
            switch c {
            case "\n", "\r", "\r\n": key = ScanCodes.enter
            case "\t": key = ScanCodes.tab
            case "\u{8}", "\u{7f}": key = ScanCodes.backspace
            default: key = ScanCodes.stroke(for: c)
            }
            if let key {
                for case .input(let i) in AssistTyping.press(key, modifiers: mods) { out.append(i) }
            } else {
                for s in c.unicodeScalars {
                    out.append(.unicode(scalar: s.value, down: true))
                    out.append(.unicode(scalar: s.value, down: false))
                }
            }
            mods = []
        }
        return out
    }

    /// A key with the modifiers held around it.
    public static func press(_ key: KeyStroke, modifiers: AssistModifiers = []) -> [AssistInput] {
        AssistTyping.press(key, modifiers: modifiers).compactMap { if case .input(let i) = $0 { i } else { nil } }
    }
}
