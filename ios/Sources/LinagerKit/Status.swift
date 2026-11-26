import Foundation

// The meaning of the numbers of a server. It is the logic of app/frontend/src/lib/status.ts, with the same thresholds; if one
// of them changes there, it changes here (the tests hold the values).

/// ok = normal, warn = needs a look, crit = critical, off = no connection.
public enum Level: Int, Sendable, Comparable {
    case ok, warn, crit, off
    public static func < (a: Level, b: Level) -> Bool { a.rawValue < b.rawValue }
}

public func levelOf(_ c: ServerCard) -> Level {
    guard c.online else { return .off }
    guard let s = c.sample else { return .ok }
    if s.cpu > 90 || s.memPct > 92 || s.diskPct > 90 { return .crit }
    if s.cpu > 75 || s.memPct > 85 || s.diskPct > 80 || s.failedServices > 0 || s.rebootRequired || s.updates >= 10 { return .warn }
    return .ok
}

/// The colour of a meter: warn from 75, critical from 90.
public enum MeterLevel: Sendable { case ok, warn, crit }

public func meter(_ v: Double) -> MeterLevel { v >= 90 ? .crit : v >= 75 ? .warn : .ok }

// MARK: badges

public enum ChipID: String, Sendable, CaseIterable {
    case offline, failed, reboot, updates, images, agentUpdate, agentOld
}

/// info = there is something, warn = a lot (10 packages, 5 images) or it needs an action, crit = a failure, mute = no answer.
public enum ChipTone: Int, Sendable, Comparable {
    case mute, info, warn, crit
    public static func < (a: ChipTone, b: ChipTone) -> Bool { a.rawValue < b.rawValue }
}

public struct Chip: Hashable, Sendable, Identifiable {
    public let id: ChipID
    public let n: Int?
    public let tone: ChipTone

    public init(_ id: ChipID, n: Int? = nil, tone: ChipTone) {
        self.id = id
        self.n = n
        self.tone = tone
    }
}

/// One chip per thing that needs a look, in the order of their weight.
public func chips(for c: ServerCard, agent: AgentFlag? = nil) -> [Chip] {
    let s = c.sample
    if !c.online {
        var out = [Chip(.offline, tone: .mute)]
        if let s, s.updates > 0 { out.append(Chip(.updates, n: s.updates, tone: .mute)) }
        return out
    }
    var out: [Chip] = []
    if let s {
        if s.failedServices > 0 { out.append(Chip(.failed, n: s.failedServices, tone: .crit)) }
        if s.rebootRequired { out.append(Chip(.reboot, tone: .warn)) }
        if s.updates > 0 { out.append(Chip(.updates, n: s.updates, tone: s.updates >= 10 ? .warn : .info)) }
        if s.imageUpdates > 0 { out.append(Chip(.images, n: s.imageUpdates, tone: s.imageUpdates >= 5 ? .warn : .info)) }
    }
    if let agent { out.append(Chip(agent == .unsupported ? .agentOld : .agentUpdate, tone: agent == .unsupported ? .crit : .warn)) }
    return out
}

/// The chips that fit and the rest, which are folded into "+N".
public func visibleChips(_ all: [Chip], max: Int = 4) -> (shown: [Chip], rest: [Chip]) {
    all.count <= max ? (all, []) : (Array(all.prefix(max)), Array(all.dropFirst(max)))
}

// MARK: totals

public struct Totals: Equatable, Sendable {
    public var total = 0
    public var online = 0
    public var offline = 0
    /// Servers that are not "ok" (an offline one included).
    public var problems = 0
    public var critical = 0
    public var updates = 0
    public var rebootPending = 0
    public var cpuAvg: Double?
}

public func summarize(_ cards: [ServerCard]) -> Totals {
    var t = Totals()
    t.total = cards.count
    var cpu = 0.0
    for c in cards {
        let lv = levelOf(c)
        if c.online { t.online += 1 }
        if lv != .ok { t.problems += 1 }
        if lv == .crit { t.critical += 1 }
        guard let s = c.sample else { continue }
        if s.updates > 0 { t.updates += s.updates }
        if s.rebootRequired { t.rebootPending += 1 }
        if c.online { cpu += s.cpu }
    }
    t.offline = t.total - t.online
    t.cpuAvg = t.online > 0 ? cpu / Double(t.online) : nil
    return t
}

// MARK: text

/// The name of the system without what does not fit a row: "Debian GNU/Linux 12 (bookworm)" becomes "Debian 12".
public func shortOs(_ os: String) -> String {
    var s = os
    s = s.replacingOccurrences(of: #"\s*\([^)]*\)"#, with: "", options: .regularExpression)
    s = s.replacingOccurrences(of: #"GNU/Linux\s*"#, with: "", options: [.regularExpression, .caseInsensitive])
    s = s.replacingOccurrences(of: #"^Microsoft\s+"#, with: "", options: .regularExpression)
    s = s.replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression).trimmingCharacters(in: .whitespaces)
    return s.isEmpty ? os : s
}

public func isWindows(_ os: String) -> Bool { os.hasPrefix("Windows") }

/// The Assistant is offered for a Windows computer and for a Linux computer whose agent says it has a graphical console.
public func assistantOffered(_ c: ServerCard) -> Bool { isWindows(c.os) || c.sample?.gui == true }

/// The first letter of a tag for its badge.
public func tagLetter(_ name: String) -> String {
    guard let ch = name.trimmingCharacters(in: .whitespaces).first else { return "?" }
    return String(ch).uppercased()
}

/// The tag palette keys the server accepts (store.TagColors) and the colour of each (the same as tag-colors.ts).
public enum TagPalette {
    public static let hex: [String: UInt32] = [
        "gray": 0x8A919D, "slate": 0x64748B, "stone": 0x8D7B68, "red": 0xE5484D, "rose": 0xE5486F, "pink": 0xE0568F, "orange": 0xF0803C, "amber": 0xE5A00D,
        "yellow": 0xD4B106, "lime": 0x84B82A, "green": 0x3FA356, "emerald": 0x14A77A, "teal": 0x0E9AA7, "cyan": 0x1FA3D1, "sky": 0x4A9CF0, "blue": 0x3B6FE6,
    ]

    /// A key the palette knows, or gray for anything else (a newer server may have more colours).
    public static func rgb(_ key: String) -> UInt32 { hex[key] ?? hex["gray"]! }
}
