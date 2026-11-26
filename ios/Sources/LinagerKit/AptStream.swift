import Foundation

/// What a job of the updates tells while it runs. The agent mixes event lines into the text of the job: the mark, a line of JSON, a newline.
/// This reads them out, so that the packages can show their own state (waiting, working, done, failed) and the page needs no console.
public enum PackageState: String, Sendable, Equatable {
    case queued, running, done, error
    /// apt stopped on an earlier error and never came to this one.
    case skipped

    public var isFinal: Bool { self == .done || self == .error || self == .skipped }
}

public struct AptStreamParser: Sendable {
    /// The mark that starts an event line (the same the desktop application cuts out).
    public static let mark: Character = "\u{1E}"

    public enum Event: Equatable, Sendable {
        /// The packages the job is going to do.
        case queue([AptPackage])
        case package(name: String, state: PackageState, message: String?)
        /// The overall progress of the phase: "download" or "install", in percent.
        case progress(stage: String, pct: Int, current: Int, total: Int)
    }

    private var pending = ""

    public init() {}

    /// Takes a piece of the stream. Returns the text of the job without the event lines, and the events in it. An event line cut in two by the
    /// piece waits for its rest.
    public mutating func feed(_ chunk: String) -> (text: String, events: [Event]) {
        var data = pending + chunk
        pending = ""
        var text = ""
        var events: [Event] = []
        while let start = data.firstIndex(of: Self.mark) {
            text += data[..<start]
            let after = data.index(after: start)
            guard let end = data[after...].firstIndex(of: "\n") else {
                pending = String(data[start...])
                return (text, events)
            }
            if let e = Self.decode(String(data[after..<end])) { events.append(e) } // a damaged line is dropped
            data = String(data[data.index(after: end)...])
        }
        text += data
        return (text, events)
    }

    private struct Raw: Decodable {
        let t: String
        let name: String?
        let state: String?
        let msg: String?
        let stage: String?
        let pct: Int?
        let cur: Int?
        let total: Int?
        let pkgs: [AptPackage]?
    }

    static func decode(_ line: String) -> Event? {
        guard let d = line.data(using: .utf8), let r = try? JSONDecoder().decode(Raw.self, from: d) else { return nil }
        switch r.t {
        case "queue": return .queue(r.pkgs ?? [])
        case "pkg":
            guard let name = r.name, let s = r.state, let st = PackageState(rawValue: s) else { return nil }
            return .package(name: name, state: st, message: r.msg)
        case "progress":
            guard let stage = r.stage else { return nil }
            return .progress(stage: stage, pct: min(100, max(0, r.pct ?? 0)), current: r.cur ?? 0, total: r.total ?? 0)
        default: return nil
        }
    }
}

/// The state of a job of the updates as the events build it up.
public struct AptRun: Equatable, Sendable {
    public private(set) var order: [String] = []
    public private(set) var states: [String: PackageState] = [:]
    public private(set) var info: [String: AptPackage] = [:]
    public private(set) var messages: [String: String] = [:]
    /// "download", "install" or nil before the first word.
    public private(set) var stage: String?
    public private(set) var pct = 0
    /// The last line of the text that told of an error ("E: ...").
    public private(set) var lastError: String?

    public init() {}

    public mutating func apply(_ event: AptStreamParser.Event) {
        switch event {
        case .queue(let pkgs):
            for p in pkgs where states[p.name] == nil {
                order.append(p.name)
                states[p.name] = .queued
                info[p.name] = p
            }
        case .package(let name, let state, let message):
            if states[name] == nil {
                order.append(name)
                info[name] = AptPackage(name: name)
            } else if let cur = states[name], cur.isFinal { return } // a state does not go back
            states[name] = state
            if let message, !message.isEmpty { messages[name] = message }
        case .progress(let stage, let pct, _, _):
            self.stage = stage
            self.pct = pct // -1: the phase has no percent (Windows Update says only which phase it is in)
        }
    }

    /// Marks the job as failed with this text (Windows Update tells it in plain words, not with "E: ").
    public mutating func fail(_ text: String) { lastError = text }

    /// The text of the job (without the event lines): only what tells of a failure is kept.
    public mutating func note(text: String) {
        for line in text.split(separator: "\n") where line.hasPrefix("E: ") { lastError = String(line.dropFirst(3)) }
    }

    public func state(_ name: String) -> PackageState { states[name] ?? .queued }
    public var total: Int { order.count }
    public var doneCount: Int { states.values.filter { $0 == .done }.count }
    public var failedCount: Int { states.values.filter { $0 == .error || $0 == .skipped }.count }
    public var hasFailure: Bool { failedCount > 0 || lastError != nil }

    /// How far the phase is, 0 to 1; nil when it says no percent.
    public var overall: Double? { pct >= 0 ? Double(pct) / 100 : nil }
}

/// Reads the text of an installation of Windows Update (PowerShell lines: "Found 3 update(s).", the titles, "Downloading...", "Installing...", one
/// "<title> -> result 2 (0)" for each update) and tells it as the same events the packages of apt have, so one page shows both. An update is
/// named by its title, as the log names it.
public struct WinUpdateStreamParser: Sendable {
    private var partial = ""
    private var listing = false
    private var titles: [String] = []

    public init() {}

    /// Takes a piece of the text; returns the events of the whole lines in it.
    public mutating func feed(_ chunk: String) -> [AptStreamParser.Event] {
        partial += chunk.replacingOccurrences(of: "\r\n", with: "\n").replacingOccurrences(of: "\r", with: "\n")
        var events: [AptStreamParser.Event] = []
        while let nl = partial.firstIndex(of: "\n") {
            let line = String(partial[..<nl])
            partial = String(partial[partial.index(after: nl)...])
            events += handle(line)
        }
        return events
    }

    private mutating func handle(_ line: String) -> [AptStreamParser.Event] {
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        if trimmed.hasPrefix("Found "), trimmed.hasSuffix("update(s).") {
            listing = true
            titles = []
            return []
        }
        if listing, line.hasPrefix("  "), !trimmed.isEmpty, !line.contains(" -> result ") {
            titles.append(trimmed)
            return [.queue([AptPackage(name: trimmed)])]
        }
        if trimmed == "Downloading..." {
            listing = false
            return [.progress(stage: "download", pct: -1, current: 0, total: titles.count)] + titles.map { .package(name: $0, state: .running, message: nil) }
        }
        if trimmed == "Installing..." {
            listing = false
            return [.progress(stage: "install", pct: -1, current: 0, total: titles.count)]
        }
        if let r = trimmed.range(of: " -> result ") {
            let title = String(trimmed[..<r.lowerBound])
            let rest = trimmed[r.upperBound...]
            let code = Int(rest.prefix { $0.isNumber }) ?? 0
            // 2: succeeded, 3: succeeded with errors; 4 and above: failed or aborted
            if code == 2 || code == 3 { return [.package(name: title, state: .done, message: nil)] }
            return [.package(name: title, state: .error, message: String(rest))]
        }
        return []
    }

    /// The lines of the text that tell the job came to nothing (the installation stops there).
    public static func failure(in text: String) -> String? {
        for line in text.split(separator: "\n") {
            let t = line.trimmingCharacters(in: .whitespaces)
            if t == "Nothing was downloaded." { return t }
        }
        return nil
    }
}
