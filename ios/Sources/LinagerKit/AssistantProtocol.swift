import Foundation

// The wire format of the Assistant (internal/assist/frames.go, app/frontend/src/lib/assistant/protocol.ts). On the WebSocket one message is
// one frame: the type byte and the payload. Frames up to 15 go from the computer to the phone, from 16 on the other way.

public enum AssistFrameType {
    public static let hello: UInt8 = 1
    public static let tiles: UInt8 = 2
    public static let stats: UInt8 = 3
    public static let end: UInt8 = 4
    public static let clip: UInt8 = 5
    public static let waking: UInt8 = 6
    public static let typed: UInt8 = 7
    public static let input: UInt8 = 16
    public static let clipSet: UInt8 = 18
    public static let clipGet: UInt8 = 19
    public static let monitor: UInt8 = 20
    public static let pause: UInt8 = 21
    public static let type: UInt8 = 22
}

/// A monitor of the computer; the list has the primary one first.
public struct AssistMonitor: Decodable, Hashable, Sendable {
    public let x: Int
    public let y: Int
    public let w: Int
    public let h: Int
    public let primary: Bool

    public init(x: Int, y: Int, w: Int, h: Int, primary: Bool) {
        self.x = x
        self.y = y
        self.w = w
        self.h = h
        self.primary = primary
    }

    enum CodingKeys: String, CodingKey { case x, y, w, h, primary }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        x = try c.decodeIfPresent(Int.self, forKey: .x) ?? 0
        y = try c.decodeIfPresent(Int.self, forKey: .y) ?? 0
        w = try c.decode(Int.self, forKey: .w)
        h = try c.decode(Int.self, forKey: .h)
        primary = try c.decodeIfPresent(Bool.self, forKey: .primary) ?? false
    }
}

/// The size of the picture, sent first and after every change of it.
public struct AssistHello: Decodable, Hashable, Sendable {
    public let w: Int
    public let h: Int
    public let user: String
    public let monitors: [AssistMonitor]
    /// The index of the monitor shown, -1 for all of them as one picture.
    public let monitor: Int
    /// "linux" for a Linux computer, empty for Windows.
    public let platform: String

    public init(w: Int, h: Int, user: String = "", monitors: [AssistMonitor] = [], monitor: Int = -1, platform: String = "") {
        self.w = w
        self.h = h
        self.user = user
        self.monitors = monitors
        self.monitor = monitor
        self.platform = platform
    }

    enum CodingKeys: String, CodingKey { case w, h, user, monitors, monitor, platform }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        w = try c.decode(Int.self, forKey: .w)
        h = try c.decode(Int.self, forKey: .h)
        user = try c.decodeIfPresent(String.self, forKey: .user) ?? ""
        // a monitor that is not a rectangle is dropped
        monitors = ((try? c.decodeIfPresent([AssistMonitor].self, forKey: .monitors)) ?? nil)?.filter { $0.w > 0 && $0.h > 0 } ?? []
        monitor = try c.decodeIfPresent(Int.self, forKey: .monitor) ?? -1
        platform = try c.decodeIfPresent(String.self, forKey: .platform) ?? ""
    }

    public var isLinux: Bool { platform == "linux" }
}

public struct AssistStats: Decodable, Hashable, Sendable {
    public let fps: Double
    public let kbps: Int
    public let q: Int
    /// "default" normally, "winlogon" on the lock, sign-in or UAC screen, "screen-saver" with a screen saver.
    public let desktop: String

    public init(fps: Double = 0, kbps: Int = 0, q: Int = 0, desktop: String = "default") {
        self.fps = fps
        self.kbps = kbps
        self.q = q
        self.desktop = desktop
    }

    enum CodingKeys: String, CodingKey { case fps, kbps, q, desktop }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        fps = try c.decodeIfPresent(Double.self, forKey: .fps) ?? 0
        kbps = try c.decodeIfPresent(Int.self, forKey: .kbps) ?? 0
        q = try c.decodeIfPresent(Int.self, forKey: .q) ?? 0
        desktop = try c.decodeIfPresent(String.self, forKey: .desktop) ?? "default"
    }

    /// The screen of a sign-in or of a UAC prompt: the only one a login or a password can be typed on.
    public var isSignInDesktop: Bool { desktop == "winlogon" }
}

public struct AssistTile: Hashable, Sendable {
    public let x: Int
    public let y: Int
    public let w: Int
    public let h: Int
    public let jpeg: Data

    public init(x: Int, y: Int, w: Int, h: Int, jpeg: Data) {
        self.x = x
        self.y = y
        self.w = w
        self.h = h
        self.jpeg = jpeg
    }
}

/// What the computer says.
public enum AssistantEvent: Sendable, Equatable {
    case hello(AssistHello)
    case tiles([AssistTile])
    case stats(AssistStats)
    /// The helper stops; `reconnect` asks to connect again (the screen changed: the desktop opened after the sign-in, the screen was locked).
    case end(reason: String, reconnect: Bool)
    /// The text of the clipboard of the computer, the answer to `clipGet`.
    case clip(String)
    /// The screen of the computer is off and is being woken (`true`), or it is awake again.
    case waking(Bool)
    /// The answer to a typing: it was typed, or why it was not ("desktop": the screen is not a sign-in or a UAC prompt now; "unsupported"; "failed").
    case typed(ok: Bool, reason: String?)
    /// The connection is closed. `code` is 4031 when the session is already open in another Assistant.
    case closed(reason: String, code: Int)
}

public enum AssistProtocol {
    /// The tiles of a Tiles payload: u16 count, then x, y, w, h (u16) and the length (u32, big endian) with the JPEG bytes. Nil when the payload is
    /// cut or lies about a length.
    public static func parseTiles(_ data: Data) -> [AssistTile]? {
        let b = [UInt8](data)
        guard b.count >= 2 else { return nil }
        let count = Int(b[0]) << 8 | Int(b[1])
        var tiles: [AssistTile] = []
        tiles.reserveCapacity(count)
        var at = 2
        for _ in 0..<count {
            guard at + 12 <= b.count else { return nil }
            func u16(_ o: Int) -> Int { Int(b[o]) << 8 | Int(b[o + 1]) }
            let len = Int(b[at + 8]) << 24 | Int(b[at + 9]) << 16 | Int(b[at + 10]) << 8 | Int(b[at + 11])
            let start = at + 12
            guard len >= 0, len <= b.count - start else { return nil }
            tiles.append(AssistTile(x: u16(at), y: u16(at + 2), w: u16(at + 4), h: u16(at + 6), jpeg: Data(b[start..<start + len])))
            at = start + len
        }
        return tiles
    }

    /// A frame the computer sent, as an event; nil for a frame that is not understood (it is skipped).
    public static func event(from message: Data) -> AssistantEvent? {
        guard let type = message.first else { return nil }
        let body = message.dropFirst()
        let dec = JSONDecoder()
        switch type {
        case AssistFrameType.hello: return (try? dec.decode(AssistHello.self, from: body)).map { .hello($0) }
        case AssistFrameType.tiles: return parseTiles(Data(body)).map { .tiles($0) }
        case AssistFrameType.stats: return (try? dec.decode(AssistStats.self, from: body)).map { .stats($0) }
        case AssistFrameType.end:
            struct E: Decodable { let reason: String?; let reconnect: Bool? }
            guard let e = try? dec.decode(E.self, from: body) else { return nil }
            return .end(reason: e.reason ?? "", reconnect: e.reconnect ?? false)
        case AssistFrameType.clip:
            struct C: Decodable { let text: String? }
            return (try? dec.decode(C.self, from: body)).map { .clip($0.text ?? "") }
        case AssistFrameType.waking:
            struct W: Decodable { let on: Bool? }
            return (try? dec.decode(W.self, from: body)).map { .waking($0.on ?? false) }
        case AssistFrameType.typed:
            struct T: Decodable { let ok: Bool?; let reason: String? }
            return (try? dec.decode(T.self, from: body)).map { .typed(ok: $0.ok ?? false, reason: $0.reason) }
        default: return nil
        }
    }
}

/// An input event to the computer (the pixels are those of the picture).
public enum AssistInput: Sendable, Equatable {
    case move(x: Int, y: Int)
    /// A mouse button: 0 left, 1 middle, 2 right, 3 back, 4 forward.
    case button(Int, down: Bool, x: Int, y: Int)
    /// The wheel in notches; positive is down and right.
    case wheel(dx: Int, dy: Int)
    /// A key by its scan code (PC set 1); `ext` for the extended (E0) keys.
    case key(sc: Int, ext: Bool, down: Bool)
    /// Ctrl+Alt+Del.
    case ctrlAltDel
    /// A character by its Unicode value (the remote desktop carries it natively; the Assistant has no such frame and ignores it).
    case unicode(scalar: UInt32, down: Bool)

    /// The frame: the type byte, then JSON.
    public var frame: Data {
        var d: [String: Any]
        switch self {
        case .move(let x, let y): d = ["t": "m", "x": clamp(x), "y": clamp(y)]
        case .button(let b, let down, let x, let y): d = ["t": "b", "b": b, "d": down, "x": clamp(x), "y": clamp(y)]
        case .wheel(let dx, let dy): d = ["t": "w", "dx": max(-50, min(50, dx)), "dy": max(-50, min(50, dy))]
        case .key(let sc, let ext, let down): d = ["t": "k", "sc": sc, "ext": ext, "d": down]
        case .ctrlAltDel: d = ["t": "cad"]
        case .unicode: return Data()
        }
        return Self.build(AssistFrameType.input, d)
    }

    private func clamp(_ v: Int) -> Int { max(0, min(65535, v)) }

    static func build(_ type: UInt8, _ json: [String: Any]) -> Data {
        var out = Data([type])
        out.append((try? JSONSerialization.data(withJSONObject: json, options: [.sortedKeys])) ?? Data("{}".utf8))
        return out
    }
}

/// The frames the phone sends besides the input.
public enum AssistCommand {
    public static let clipMax = 1 << 20
    public static let typingMax = 512

    public static func clipSet(_ text: String) -> Data? {
        guard text.utf8.count <= clipMax else { return nil }
        return AssistInput.build(AssistFrameType.clipSet, ["text": text])
    }

    public static var clipGet: Data { Data([AssistFrameType.clipGet]) }

    /// Shows this monitor: its index, or -1 for all of them as one picture.
    public static func monitor(_ index: Int) -> Data { AssistInput.build(AssistFrameType.monitor, ["i": index]) }

    /// Stops the pictures (the screen is hidden) or lets them go on.
    public static func pause(_ on: Bool) -> Data { AssistInput.build(AssistFrameType.pause, ["on": on]) }

    /// A text to type on the sign-in or the UAC screen, then Tab or Enter or nothing. Nil when the text is empty, too long or has control characters.
    public static func typing(_ text: String, after: String = "", label: String = "") -> Data? {
        guard !text.isEmpty, text.count <= typingMax, !text.unicodeScalars.contains(where: { $0.value < 0x20 || $0.value == 0x7f }) else { return nil }
        var json: [String: Any] = ["text": text]
        if !after.isEmpty { json["after"] = after }
        let cleanLabel = String(label.unicodeScalars.map { $0.value < 0x20 || $0.value == 0x7f ? " " : Character($0) }.prefix(80))
        if !cleanLabel.isEmpty { json["label"] = cleanLabel }
        return AssistInput.build(AssistFrameType.type, json)
    }
}

/// The picture settings of a computer (kept on the server, one set per computer, shared with the desktop application).
public struct AssistSettings: Codable, Hashable, Sendable {
    public var colors: String
    public var fps: Int
    public var mbit: Double
    public var nolimit: Bool
    public var auto: Bool
    public var showCursor: Bool?

    public init(colors: String = "24", fps: Int = 15, mbit: Double = 4, nolimit: Bool = false, auto: Bool = true, showCursor: Bool? = false) {
        self.colors = colors
        self.fps = fps
        self.mbit = mbit
        self.nolimit = nolimit
        self.auto = auto
        self.showCursor = showCursor
    }

    enum CodingKeys: String, CodingKey {
        case colors, fps, mbit, nolimit, auto
        case showCursor = "show_cursor"
    }

    public enum Preset: String, CaseIterable, Sendable {
        case eco, normal, hq
    }

    /// The ready-made settings of the desktop application.
    public static func preset(_ p: Preset) -> AssistSettings {
        switch p {
        case .eco: AssistSettings(colors: "256", fps: 8, mbit: 1)
        case .normal: AssistSettings(colors: "24", fps: 15, mbit: 4)
        case .hq: AssistSettings(colors: "24", fps: 30, mbit: 12)
        }
    }

    /// The preset these settings equal (the pointer does not count), nil for custom ones.
    public var preset: Preset? {
        Preset.allCases.first { p in
            let s = Self.preset(p)
            return s.colors == colors && s.fps == fps && s.nolimit == nolimit && s.auto == auto && (nolimit || s.mbit == mbit)
        }
    }
}
