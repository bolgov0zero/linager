import Foundation

// What the page of a server shows: the detail of the server, the history of its metrics, the files of a folder.

/// Static information about the host.
public struct SystemInfo: Decodable, Hashable, Sendable {
    public var hostname: String
    public var os: String
    public var kernel: String
    public var arch: String
    public var cpuModel: String
    public var cpuCores: Int
    public var memTotal: Int64

    enum CodingKeys: String, CodingKey {
        case hostname, os, kernel, arch
        case cpuModel = "cpu_model"
        case cpuCores = "cpu_cores"
        case memTotal = "mem_total"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        hostname = try c.decodeIfPresent(String.self, forKey: .hostname) ?? ""
        os = try c.decodeIfPresent(String.self, forKey: .os) ?? ""
        kernel = try c.decodeIfPresent(String.self, forKey: .kernel) ?? ""
        arch = try c.decodeIfPresent(String.self, forKey: .arch) ?? ""
        cpuModel = try c.decodeIfPresent(String.self, forKey: .cpuModel) ?? ""
        cpuCores = try c.decodeIfPresent(Int.self, forKey: .cpuCores) ?? 0
        memTotal = try c.decodeIfPresent(Int64.self, forKey: .memTotal) ?? 0
    }

    public init(hostname: String = "", os: String = "", kernel: String = "", arch: String = "", cpuModel: String = "", cpuCores: Int = 0, memTotal: Int64 = 0) {
        self.hostname = hostname
        self.os = os
        self.kernel = kernel
        self.arch = arch
        self.cpuModel = cpuModel
        self.cpuCores = cpuCores
        self.memTotal = memTotal
    }
}

public struct DiskUsage: Decodable, Hashable, Sendable, Identifiable {
    public var mount: String
    public var total: Int64
    public var used: Int64
    public var pct: Double
    public var id: String { mount }

    enum CodingKeys: String, CodingKey { case mount, total, used, pct }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mount = try c.decodeIfPresent(String.self, forKey: .mount) ?? ""
        total = try c.decodeIfPresent(Int64.self, forKey: .total) ?? 0
        used = try c.decodeIfPresent(Int64.self, forKey: .used) ?? 0
        pct = try c.decodeIfPresent(Double.self, forKey: .pct) ?? 0
    }

    public init(mount: String, total: Int64, used: Int64, pct: Double) {
        self.mount = mount
        self.total = total
        self.used = used
        self.pct = pct
    }
}

/// The last measurement with all it carries (the dashboard keeps only part of it: `Sample`).
public struct SampleDetail: Decodable, Hashable, Sendable {
    public var cpu: Double
    public var memUsed: Int64
    public var memTotal: Int64
    public var memPct: Double
    public var swapUsed: Int64
    public var swapTotal: Int64
    public var disks: [DiskUsage]
    public var diskPct: Double
    public var rxBps: Double
    public var txBps: Double
    public var load: [Double]
    public var uptime: Int64
    public var procs: Int
    public var failedServices: Int
    public var rebootRequired: Bool
    public var updates: Int
    public var securityUpdates: Int
    public var imageUpdates: Int

    enum CodingKeys: String, CodingKey {
        case cpu, disks, load, uptime, procs, updates
        case memUsed = "mem_used"
        case memTotal = "mem_total"
        case memPct = "mem_pct"
        case swapUsed = "swap_used"
        case swapTotal = "swap_total"
        case diskPct = "disk_pct"
        case rxBps = "rx_bps"
        case txBps = "tx_bps"
        case failedServices = "failed_services"
        case rebootRequired = "reboot_required"
        case securityUpdates = "security_updates"
        case imageUpdates = "image_updates"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        cpu = try c.decodeIfPresent(Double.self, forKey: .cpu) ?? 0
        memUsed = try c.decodeIfPresent(Int64.self, forKey: .memUsed) ?? 0
        memTotal = try c.decodeIfPresent(Int64.self, forKey: .memTotal) ?? 0
        memPct = try c.decodeIfPresent(Double.self, forKey: .memPct) ?? 0
        swapUsed = try c.decodeIfPresent(Int64.self, forKey: .swapUsed) ?? 0
        swapTotal = try c.decodeIfPresent(Int64.self, forKey: .swapTotal) ?? 0
        disks = try c.decodeIfPresent([DiskUsage].self, forKey: .disks) ?? []
        diskPct = try c.decodeIfPresent(Double.self, forKey: .diskPct) ?? 0
        rxBps = try c.decodeIfPresent(Double.self, forKey: .rxBps) ?? 0
        txBps = try c.decodeIfPresent(Double.self, forKey: .txBps) ?? 0
        load = try c.decodeIfPresent([Double].self, forKey: .load) ?? []
        uptime = try c.decodeIfPresent(Int64.self, forKey: .uptime) ?? 0
        procs = try c.decodeIfPresent(Int.self, forKey: .procs) ?? 0
        failedServices = try c.decodeIfPresent(Int.self, forKey: .failedServices) ?? 0
        rebootRequired = try c.decodeIfPresent(Bool.self, forKey: .rebootRequired) ?? false
        updates = try c.decodeIfPresent(Int.self, forKey: .updates) ?? 0
        securityUpdates = try c.decodeIfPresent(Int.self, forKey: .securityUpdates) ?? 0
        imageUpdates = try c.decodeIfPresent(Int.self, forKey: .imageUpdates) ?? 0
    }
}

/// `GET api/servers/{id}`: the page of a server.
public struct ServerDetail: Decodable, Hashable, Sendable {
    public var id: String
    public var name: String
    public var hostname: String
    public var address: String
    public var os: String
    public var agentVersion: String
    public var online: Bool
    public var system: SystemInfo?
    public var sample: SampleDetail?
    /// The last CPU values, oldest first.
    public var spark: [Double]
    /// The modules of this server that are on for accounts that are not administrators.
    public var modules: [String]
    public var tags: [TagItem]
    public var note: String

    enum CodingKeys: String, CodingKey {
        case id, name, hostname, address, os, online, system, sample, spark, modules, note
        case agentVersion = "agent_version"
        case tags = "tag_items"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? id
        hostname = try c.decodeIfPresent(String.self, forKey: .hostname) ?? ""
        address = try c.decodeIfPresent(String.self, forKey: .address) ?? ""
        os = try c.decodeIfPresent(String.self, forKey: .os) ?? ""
        agentVersion = try c.decodeIfPresent(String.self, forKey: .agentVersion) ?? ""
        online = try c.decodeIfPresent(Bool.self, forKey: .online) ?? false
        system = try c.decodeIfPresent(SystemInfo.self, forKey: .system)
        sample = try c.decodeIfPresent(SampleDetail.self, forKey: .sample)
        spark = try c.decodeIfPresent([Double].self, forKey: .spark) ?? []
        modules = try c.decodeIfPresent([String].self, forKey: .modules) ?? []
        tags = try c.decodeIfPresent([TagItem].self, forKey: .tags) ?? []
        note = try c.decodeIfPresent(String.self, forKey: .note) ?? ""
    }
}

// MARK: history

/// The periods of the history of the metrics that the page offers.
public enum HistoryPeriod: String, CaseIterable, Sendable {
    case hour = "1h", day = "24h", week = "7d", month = "30d"
}

public struct HistoryPoint: Decodable, Hashable, Sendable, Identifiable {
    public var ts: Int64
    public var cpuAvg: Double
    public var cpuMax: Double
    public var memAvg: Double
    public var memMax: Double
    public var diskAvg: Double
    public var rxAvg: Double
    public var txAvg: Double
    public var id: Int64 { ts }

    enum CodingKeys: String, CodingKey {
        case ts
        case cpuAvg = "cpu_avg"
        case cpuMax = "cpu_max"
        case memAvg = "mem_avg"
        case memMax = "mem_max"
        case diskAvg = "disk_avg"
        case rxAvg = "rx_avg"
        case txAvg = "tx_avg"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ts = try c.decode(Int64.self, forKey: .ts)
        cpuAvg = try c.decodeIfPresent(Double.self, forKey: .cpuAvg) ?? 0
        cpuMax = try c.decodeIfPresent(Double.self, forKey: .cpuMax) ?? 0
        memAvg = try c.decodeIfPresent(Double.self, forKey: .memAvg) ?? 0
        memMax = try c.decodeIfPresent(Double.self, forKey: .memMax) ?? 0
        diskAvg = try c.decodeIfPresent(Double.self, forKey: .diskAvg) ?? 0
        rxAvg = try c.decodeIfPresent(Double.self, forKey: .rxAvg) ?? 0
        txAvg = try c.decodeIfPresent(Double.self, forKey: .txAvg) ?? 0
    }

    public init(ts: Int64, cpuAvg: Double = 0, cpuMax: Double = 0, memAvg: Double = 0, memMax: Double = 0, diskAvg: Double = 0, rxAvg: Double = 0, txAvg: Double = 0) {
        self.ts = ts
        self.cpuAvg = cpuAvg
        self.cpuMax = cpuMax
        self.memAvg = memAvg
        self.memMax = memMax
        self.diskAvg = diskAvg
        self.rxAvg = rxAvg
        self.txAvg = txAvg
    }
}

public struct MetricStats: Decodable, Hashable, Sendable {
    public var min: Double
    public var avg: Double
    public var p95: Double
    public var max: Double

    public init(min: Double = 0, avg: Double = 0, p95: Double = 0, max: Double = 0) {
        self.min = min
        self.avg = avg
        self.p95 = p95
        self.max = max
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        min = try c.decodeIfPresent(Double.self, forKey: .min) ?? 0
        avg = try c.decodeIfPresent(Double.self, forKey: .avg) ?? 0
        p95 = try c.decodeIfPresent(Double.self, forKey: .p95) ?? 0
        max = try c.decodeIfPresent(Double.self, forKey: .max) ?? 0
    }

    enum CodingKeys: String, CodingKey { case min, avg, p95, max }
}

/// Something that happened in the period (a reboot, a failed service, an offline spell).
public struct HistoryEvent: Decodable, Hashable, Sendable, Identifiable {
    public var ts: Int64
    public var kind: String
    public var message: String
    public var id: String { "\(ts)-\(kind)" }
}

public struct History: Decodable, Hashable, Sendable {
    public var period: String
    public var points: [HistoryPoint]
    /// By metric: "cpu", "mem", "disk", "rx", "tx".
    public var stats: [String: MetricStats]
    public var events: [HistoryEvent]

    enum CodingKeys: String, CodingKey { case period, points, stats, events }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        period = try c.decodeIfPresent(String.self, forKey: .period) ?? ""
        points = try c.decodeIfPresent([HistoryPoint].self, forKey: .points) ?? []
        stats = try c.decodeIfPresent([String: MetricStats].self, forKey: .stats) ?? [:]
        events = try c.decodeIfPresent([HistoryEvent].self, forKey: .events) ?? []
    }

    public init(period: String, points: [HistoryPoint], stats: [String: MetricStats] = [:], events: [HistoryEvent] = []) {
        self.period = period
        self.points = points
        self.stats = stats
        self.events = events
    }
}

// MARK: files

public struct FileEntry: Decodable, Hashable, Sendable, Identifiable {
    public enum Kind: String, Sendable { case dir, file, link, other }

    public var name: String
    public var kind: Kind
    public var size: Int64
    public var modeStr: String
    public var owner: String
    public var group: String
    public var mtime: Int64
    public var linkTarget: String?
    public var id: String { name }

    public var isFolder: Bool { kind == .dir }

    enum CodingKeys: String, CodingKey {
        case name, type, size, owner, group, mtime
        case modeStr = "mode_str"
        case linkTarget = "link_target"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        kind = Kind(rawValue: try c.decodeIfPresent(String.self, forKey: .type) ?? "") ?? .other
        size = try c.decodeIfPresent(Int64.self, forKey: .size) ?? 0
        modeStr = try c.decodeIfPresent(String.self, forKey: .modeStr) ?? ""
        owner = try c.decodeIfPresent(String.self, forKey: .owner) ?? ""
        group = try c.decodeIfPresent(String.self, forKey: .group) ?? ""
        mtime = try c.decodeIfPresent(Int64.self, forKey: .mtime) ?? 0
        linkTarget = try c.decodeIfPresent(String.self, forKey: .linkTarget)
    }

    public init(name: String, kind: Kind, size: Int64 = 0, modeStr: String = "", owner: String = "", group: String = "", mtime: Int64 = 0, linkTarget: String? = nil) {
        self.name = name
        self.kind = kind
        self.size = size
        self.modeStr = modeStr
        self.owner = owner
        self.group = group
        self.mtime = mtime
        self.linkTarget = linkTarget
    }
}

public struct FileList: Decodable, Hashable, Sendable {
    public var path: String
    public var entries: [FileEntry]

    /// Folders first, then files, each in the order of names as a person reads them.
    public var sorted: [FileEntry] {
        entries.sorted { a, b in
            if a.isFolder != b.isFolder { return a.isFolder }
            return a.name.localizedStandardCompare(b.name) == .orderedAscending
        }
    }
}

/// How the files of a folder are ordered (folders always come first).
public enum FileSort: String, CaseIterable, Sendable {
    /// By name, as a person reads it.
    case name
    /// The newest first.
    case date
    /// The biggest first.
    case size
}

extension FileList {
    /// The entries to show: the hidden ones (a name that starts with a dot) only when asked for, the ones that match the search, in the chosen order.
    public func arranged(sort: FileSort, showHidden: Bool, query: String = "") -> [FileEntry] {
        let q = query.trimmingCharacters(in: .whitespaces)
        let shown = entries.filter { e in
            (showHidden || !e.name.hasPrefix(".")) && (q.isEmpty || e.name.localizedCaseInsensitiveContains(q))
        }
        func byName(_ a: FileEntry, _ b: FileEntry) -> Bool { a.name.localizedStandardCompare(b.name) == .orderedAscending }
        return shown.sorted { a, b in
            if a.isFolder != b.isFolder { return a.isFolder }
            switch sort {
            case .name: return byName(a, b)
            case .date: return a.mtime != b.mtime ? a.mtime > b.mtime : byName(a, b)
            case .size:
                // folders have no size worth sorting by: they stay by name
                if a.isFolder { return byName(a, b) }
                return a.size != b.size ? a.size > b.size : byName(a, b)
            }
        }
    }

    /// How many entries the "hidden" filter keeps out of view.
    public func hiddenCount(showHidden: Bool) -> Int { showHidden ? 0 : entries.filter { $0.name.hasPrefix(".") }.count }
}

public struct FileContent: Decodable, Hashable, Sendable {
    public var path: String
    public var content: String
    public var size: Int64
    /// The time of this version in milliseconds: the save sends it back.
    public var mtime: Int64
    public var mode: String

    enum CodingKeys: String, CodingKey { case path, content, size, mtime, mode }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        path = try c.decodeIfPresent(String.self, forKey: .path) ?? ""
        content = try c.decodeIfPresent(String.self, forKey: .content) ?? ""
        size = try c.decodeIfPresent(Int64.self, forKey: .size) ?? 0
        mtime = try c.decodeIfPresent(Int64.self, forKey: .mtime) ?? 0
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? ""
    }

    public init(path: String, content: String, size: Int64 = 0, mtime: Int64 = 0, mode: String = "0644") {
        self.path = path
        self.content = content
        self.size = size
        self.mtime = mtime
        self.mode = mode
    }
}

/// What the server knows of one entry (`api/servers/{id}/files/stat`).
public struct FileStat: Decodable, Hashable, Sendable {
    public var name: String
    public var path: String
    public var type: String
    public var size: Int64
    /// Octal with the special bits: "0644", "4755".
    public var mode: String
    public var modeStr: String
    public var owner: String
    public var group: String
    public var mtime: Int64
    public var linkTarget: String?
    public var mime: String?
    /// A system folder: its permissions and owner cannot be changed.
    public var protected: Bool

    enum CodingKeys: String, CodingKey {
        case name, path, type, size, mode, owner, group, mtime, mime, protected
        case modeStr = "mode_str"
        case linkTarget = "link_target"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        path = try c.decodeIfPresent(String.self, forKey: .path) ?? ""
        type = try c.decodeIfPresent(String.self, forKey: .type) ?? ""
        size = try c.decodeIfPresent(Int64.self, forKey: .size) ?? 0
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? "0644"
        modeStr = try c.decodeIfPresent(String.self, forKey: .modeStr) ?? ""
        owner = try c.decodeIfPresent(String.self, forKey: .owner) ?? ""
        group = try c.decodeIfPresent(String.self, forKey: .group) ?? ""
        mtime = try c.decodeIfPresent(Int64.self, forKey: .mtime) ?? 0
        linkTarget = try c.decodeIfPresent(String.self, forKey: .linkTarget)
        mime = try c.decodeIfPresent(String.self, forKey: .mime)
        protected = try c.decodeIfPresent(Bool.self, forKey: .protected) ?? false
    }

    public var isFolder: Bool { type == "dir" }
}

public enum TransferMode: String, Sendable {
    case fail, keep, overwrite
}

/// The permission bits of an entry as a person edits them: nine of read, write and execute, and the three special ones.
public struct FileMode: Equatable, Sendable {
    public enum Who: Int, CaseIterable, Sendable { case owner = 2, group = 1, others = 0 }
    public enum What: Int, CaseIterable, Sendable { case read = 4, write = 2, execute = 1 }
    public enum Special: Int, CaseIterable, Sendable { case setuid = 0o4000, setgid = 0o2000, sticky = 0o1000 }

    public var bits: Int

    public init(bits: Int = 0o644) { self.bits = bits & 0o7777 }

    /// From the text the server gives ("0644", "4755"); nil when it is not octal.
    public init?(octal: String) {
        guard let v = Int(octal.trimmingCharacters(in: .whitespaces), radix: 8), v >= 0, v <= 0o7777 else { return nil }
        self.bits = v
    }

    public func has(_ what: What, for who: Who) -> Bool { bits & (what.rawValue << (who.rawValue * 3)) != 0 }

    public mutating func set(_ what: What, for who: Who, _ on: Bool) {
        let mask = what.rawValue << (who.rawValue * 3)
        if on { bits |= mask } else { bits &= ~mask }
    }

    public func has(_ s: Special) -> Bool { bits & s.rawValue != 0 }

    public mutating func set(_ s: Special, _ on: Bool) {
        if on { bits |= s.rawValue } else { bits &= ~s.rawValue }
    }

    /// "0644": four digits, as the server takes it.
    public var octal: String { String(format: "%04o", bits) }

    /// "rw-r--r--": as `ls` shows it (the special bits as s, S, t, T).
    public var symbolic: String {
        var out = ""
        for who in Who.allCases {
            let r = has(.read, for: who) ? "r" : "-"
            let w = has(.write, for: who) ? "w" : "-"
            var x = has(.execute, for: who) ? "x" : "-"
            switch who {
            case .owner: if has(.setuid) { x = x == "x" ? "s" : "S" }
            case .group: if has(.setgid) { x = x == "x" ? "s" : "S" }
            case .others: if has(.sticky) { x = x == "x" ? "t" : "T" }
            }
            out += r + w + x
        }
        return out
    }

    /// The server wants such a change confirmed: the special bits, or a change of a whole tree.
    public var isDangerous: Bool { bits & 0o7000 != 0 }
}

public enum FilePath {
    /// The path of `name` inside `folder` ("/" and "var" give "/var").
    public static func child(_ folder: String, _ name: String) -> String {
        folder == "/" ? "/" + name : folder + "/" + name
    }

    /// The folder above a path ("/" for the top).
    public static func parent(_ path: String) -> String {
        guard let i = path.lastIndex(of: "/"), i != path.startIndex else { return "/" }
        return String(path[..<i])
    }

    /// The names from the top down: "/var/www" gives ["var", "www"].
    public static func parts(_ path: String) -> [String] { path.split(separator: "/").map(String.init) }

    /// Every folder on the way to a path, from the top: "/var/www" gives ["/var", "/var/www"] (the top "/" is not in it).
    public static func chain(_ path: String) -> [String] {
        var out: [String] = []
        var cur = "/"
        for p in parts(path) {
            cur = child(cur, p)
            out.append(cur)
        }
        return out
    }
}


// MARK: notes

/// A block of the note of a server, written in Markdown. The inline marks (bold, italic, code, links) stay in the text and are drawn by the screen.
public enum MarkdownBlock: Equatable, Sendable {
    case heading(level: Int, text: String)
    case paragraph(String)
    case bullet(depth: Int, text: String)
    case numbered(number: Int, text: String)
    case quote(String)
    case code(String)
    case rule
}

public enum Markdown {
    /// Splits a text into blocks. Lines of one paragraph keep their line breaks (a note is written by a person who means them).
    public static func parse(_ source: String) -> [MarkdownBlock] {
        var blocks: [MarkdownBlock] = []
        var para: [String] = []
        var quote: [String] = []
        var code: [String]?
        func flushPara() { if !para.isEmpty { blocks.append(.paragraph(para.joined(separator: "\n"))); para = [] } }
        func flushQuote() { if !quote.isEmpty { blocks.append(.quote(quote.joined(separator: "\n"))); quote = [] } }
        for raw in source.replacingOccurrences(of: "\r\n", with: "\n").replacingOccurrences(of: "\r", with: "\n").components(separatedBy: "\n") {
            let t = raw.trimmingCharacters(in: .whitespaces)
            if code != nil {
                if t.hasPrefix("```") { blocks.append(.code((code ?? []).joined(separator: "\n"))); code = nil } else { code?.append(raw) }
                continue
            }
            if t.hasPrefix("```") { flushPara(); flushQuote(); code = []; continue }
            if t.isEmpty { flushPara(); flushQuote(); continue }
            if let h = heading(t) { flushPara(); flushQuote(); blocks.append(h); continue }
            if t.count >= 3, t.allSatisfy({ $0 == "-" || $0 == "*" || $0 == "_" }), Set(t).count == 1 { flushPara(); flushQuote(); blocks.append(.rule); continue }
            if t.hasPrefix(">") {
                flushPara()
                quote.append(String(t.dropFirst()).trimmingCharacters(in: .whitespaces))
                continue
            }
            flushQuote()
            if let item = listItem(raw) { flushPara(); blocks.append(item); continue }
            para.append(t)
        }
        if let c = code { blocks.append(.code(c.joined(separator: "\n"))) }
        flushPara()
        flushQuote()
        return blocks
    }

    private static func heading(_ t: String) -> MarkdownBlock? {
        let hashes = t.prefix { $0 == "#" }.count
        guard (1...6).contains(hashes), t.dropFirst(hashes).first == " " else { return nil }
        return .heading(level: hashes, text: String(t.dropFirst(hashes)).trimmingCharacters(in: .whitespaces))
    }

    private static func listItem(_ raw: String) -> MarkdownBlock? {
        let indent = raw.prefix { $0 == " " || $0 == "\t" }.reduce(0) { $0 + ($1 == "\t" ? 4 : 1) }
        let t = raw.trimmingCharacters(in: .whitespaces)
        for mark in ["- ", "* ", "+ "] where t.hasPrefix(mark) {
            return .bullet(depth: indent / 2, text: String(t.dropFirst(2)).trimmingCharacters(in: .whitespaces))
        }
        let digits = t.prefix { $0.isNumber }
        if !digits.isEmpty, digits.count <= 9, t.dropFirst(digits.count).hasPrefix(". "), let n = Int(digits) {
            return .numbered(number: n, text: String(t.dropFirst(digits.count + 2)).trimmingCharacters(in: .whitespaces))
        }
        return nil
    }
}
