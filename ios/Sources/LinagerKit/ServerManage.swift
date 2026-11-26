import Foundation

// The management pages of a server: services, processes, Docker, the journal, the disk and the updates of the packages. The shapes are
// those of the agent as the server forwards them; what is missing in an answer is left empty (an older agent knows less).

// MARK: services

public struct ServiceUnit: Decodable, Hashable, Sendable, Identifiable {
    public let name: String
    public let description: String
    /// "active", "inactive", "failed", "activating"...
    public let active: String
    public let sub: String
    /// "enabled", "disabled", "static"...
    public let enabled: String

    public var id: String { name }
    public var isRunning: Bool { active == "active" }
    public var isFailed: Bool { active == "failed" }
    /// The name without ".service": what a person calls it.
    public var shortName: String { name.hasSuffix(".service") ? String(name.dropLast(8)) : name }

    public init(name: String, description: String = "", active: String = "inactive", sub: String = "dead", enabled: String = "disabled") {
        self.name = name
        self.description = description
        self.active = active
        self.sub = sub
        self.enabled = enabled
    }

    enum CodingKeys: String, CodingKey { case name, description, active, sub, enabled }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        description = try c.decodeIfPresent(String.self, forKey: .description) ?? ""
        active = try c.decodeIfPresent(String.self, forKey: .active) ?? ""
        sub = try c.decodeIfPresent(String.self, forKey: .sub) ?? ""
        enabled = try c.decodeIfPresent(String.self, forKey: .enabled) ?? ""
    }
}

public enum ServiceAction: String, CaseIterable, Sendable {
    case start, stop, restart, enable, disable
}

/// The services: failed ones first, then the running, then by name (what needs a look comes first).
public enum ServiceList {
    public enum Filter: String, CaseIterable, Sendable { case all, running, failed, stopped }

    public static func arranged(_ units: [ServiceUnit], query: String = "", only: Filter = .all) -> [ServiceUnit] {
        let q = query.trimmingCharacters(in: .whitespaces)
        func rank(_ u: ServiceUnit) -> Int { u.isFailed ? 0 : (u.isRunning ? 1 : 2) }
        func passes(_ u: ServiceUnit) -> Bool {
            switch only {
            case .all: true
            case .running: u.isRunning
            case .failed: u.isFailed
            case .stopped: !u.isRunning && !u.isFailed
            }
        }
        return units
            .filter(passes)
            .filter { q.isEmpty || $0.name.localizedCaseInsensitiveContains(q) || $0.description.localizedCaseInsensitiveContains(q) }
            .sorted { a, b in
                let (ra, rb) = (rank(a), rank(b))
                return ra != rb ? ra < rb : a.name.localizedStandardCompare(b.name) == .orderedAscending
            }
    }
}

// MARK: processes

public enum ProcessSort: String, CaseIterable, Sendable { case cpu, mem, name }

public struct ProcessItem: Decodable, Hashable, Sendable, Identifiable {
    public let pid: Int
    public let user: String
    public let cpu: Double
    public let memPct: Double
    public let rss: Int64
    public let state: String
    public let cmd: String

    public var id: Int { pid }
    /// The program without its path and arguments: "/usr/sbin/nginx -g daemon" gives "nginx", and `C:\Program Files\App\app.exe /x` gives "app.exe"
    /// (a Windows path has spaces and backslashes, so the first ".exe" ends the program there).
    public var program: String {
        var head = cmd.trimmingCharacters(in: .whitespaces)
        if head.hasPrefix("\"") { head.removeFirst() }
        if let r = head.range(of: ".exe", options: .caseInsensitive) {
            head = String(head[..<r.upperBound])
        } else if let space = head.firstIndex(of: " ") {
            head = String(head[..<space])
        }
        if let cut = head.lastIndex(where: { $0 == "/" || $0 == "\\" }) { head = String(head[head.index(after: cut)...]) }
        return head
    }

    public init(pid: Int, user: String = "", cpu: Double = 0, memPct: Double = 0, rss: Int64 = 0, state: String = "S", cmd: String = "") {
        self.pid = pid
        self.user = user
        self.cpu = cpu
        self.memPct = memPct
        self.rss = rss
        self.state = state
        self.cmd = cmd
    }

    enum CodingKeys: String, CodingKey {
        case pid, user, cpu, rss, state, cmd
        case memPct = "mem_pct"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        pid = try c.decode(Int.self, forKey: .pid)
        user = try c.decodeIfPresent(String.self, forKey: .user) ?? ""
        cpu = try c.decodeIfPresent(Double.self, forKey: .cpu) ?? 0
        memPct = try c.decodeIfPresent(Double.self, forKey: .memPct) ?? 0
        rss = try c.decodeIfPresent(Int64.self, forKey: .rss) ?? 0
        state = try c.decodeIfPresent(String.self, forKey: .state) ?? ""
        cmd = try c.decodeIfPresent(String.self, forKey: .cmd) ?? ""
    }
}

public struct ProcessList: Decodable, Hashable, Sendable {
    public let processes: [ProcessItem]
    public let total: Int

    public init(processes: [ProcessItem] = [], total: Int = 0) {
        self.processes = processes
        self.total = total
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        processes = try c.decodeIfPresent([ProcessItem].self, forKey: .processes) ?? []
        total = try c.decodeIfPresent(Int.self, forKey: .total) ?? processes.count
    }

    enum CodingKeys: String, CodingKey { case processes, total }
}

// MARK: updates of the packages

public struct AptPackage: Decodable, Hashable, Sendable, Identifiable {
    public let name: String
    public let current: String
    public let candidate: String
    public let security: Bool
    public let reboot: Bool
    public let ignored: Bool

    public var id: String { name }

    public init(name: String, current: String = "", candidate: String = "", security: Bool = false, reboot: Bool = false, ignored: Bool = false) {
        self.name = name
        self.current = current
        self.candidate = candidate
        self.security = security
        self.reboot = reboot
        self.ignored = ignored
    }

    enum CodingKeys: String, CodingKey { case name, current, candidate, security, reboot, ignored }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        current = try c.decodeIfPresent(String.self, forKey: .current) ?? ""
        candidate = try c.decodeIfPresent(String.self, forKey: .candidate) ?? ""
        security = try c.decodeIfPresent(Bool.self, forKey: .security) ?? false
        reboot = try c.decodeIfPresent(Bool.self, forKey: .reboot) ?? false
        ignored = try c.decodeIfPresent(Bool.self, forKey: .ignored) ?? false
    }
}

public struct AptList: Decodable, Hashable, Sendable {
    public let packages: [AptPackage]
    public let rebootRequired: Bool
    /// When the list of the packages was last refreshed (seconds since 1970; 0: never).
    public let lastCheck: Int64
    /// The agent is running an apt job just now.
    public let busy: Bool

    public init(packages: [AptPackage] = [], rebootRequired: Bool = false, lastCheck: Int64 = 0, busy: Bool = false) {
        self.packages = packages
        self.rebootRequired = rebootRequired
        self.lastCheck = lastCheck
        self.busy = busy
    }

    enum CodingKeys: String, CodingKey {
        case packages, busy
        case rebootRequired = "reboot_required"
        case lastCheck = "last_check"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        packages = try c.decodeIfPresent([AptPackage].self, forKey: .packages) ?? []
        rebootRequired = try c.decodeIfPresent(Bool.self, forKey: .rebootRequired) ?? false
        lastCheck = try c.decodeIfPresent(Int64.self, forKey: .lastCheck) ?? 0
        busy = try c.decodeIfPresent(Bool.self, forKey: .busy) ?? false
    }

    /// Packages that count: the ignored ones stay in the list but are not counted.
    public var counted: [AptPackage] { packages.filter { !$0.ignored } }
    public var securityCount: Int { counted.filter(\.security).count }
}

/// What a stream of the updates does: refresh the lists, install everything, install only what is security, install some packages.
public enum AptMode: Sendable, Equatable {
    case update, upgrade, security
    case packages([String])
}

// MARK: Windows Update

/// One update of Windows Update (the answer of `win/winupdate/list`).
public struct WinUpdate: Decodable, Hashable, Sendable, Identifiable {
    /// A GUID.
    public let id: String
    public let title: String
    public let kb: String
    public let size: Int64
    public let security: Bool
    public let downloaded: Bool
    public let needReboot: Bool

    public init(id: String, title: String, kb: String = "", size: Int64 = 0, security: Bool = false, downloaded: Bool = false, needReboot: Bool = false) {
        self.id = id
        self.title = title
        self.kb = kb
        self.size = size
        self.security = security
        self.downloaded = downloaded
        self.needReboot = needReboot
    }

    enum CodingKeys: String, CodingKey {
        case id, title, kb, size, security, downloaded
        case needReboot = "need_reboot"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? id
        kb = try c.decodeIfPresent(String.self, forKey: .kb) ?? ""
        size = try c.decodeIfPresent(Int64.self, forKey: .size) ?? 0
        security = try c.decodeIfPresent(Bool.self, forKey: .security) ?? false
        downloaded = try c.decodeIfPresent(Bool.self, forKey: .downloaded) ?? false
        needReboot = try c.decodeIfPresent(Bool.self, forKey: .needReboot) ?? false
    }
}

public struct WinUpdateList: Decodable, Hashable, Sendable {
    public let updates: [WinUpdate]
    public let rebootPending: Bool

    public init(updates: [WinUpdate] = [], rebootPending: Bool = false) {
        self.updates = updates
        self.rebootPending = rebootPending
    }

    enum CodingKeys: String, CodingKey {
        case updates
        case rebootPending = "reboot_pending"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        updates = try c.decodeIfPresent([WinUpdate].self, forKey: .updates) ?? []
        rebootPending = try c.decodeIfPresent(Bool.self, forKey: .rebootPending) ?? false
    }

    public var securityCount: Int { updates.filter(\.security).count }
}

// MARK: Docker

public struct DockerContainer: Decodable, Hashable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let image: String
    /// "running", "exited", "paused", "restarting"...
    public let state: String
    public let status: String
    public let ports: [String]
    public let cpu: Double
    public let memUsed: Int64
    public let memLimit: Int64
    /// "new" (a newer image exists), "current", "unknown" or empty (not checked).
    public let upd: String
    public let ignored: Bool

    public var isRunning: Bool { state == "running" }
    public var hasNewImage: Bool { upd == "new" && !ignored }

    public init(id: String, name: String, image: String = "", state: String = "running", status: String = "", ports: [String] = [], cpu: Double = 0, memUsed: Int64 = 0, memLimit: Int64 = 0, upd: String = "", ignored: Bool = false) {
        self.id = id
        self.name = name
        self.image = image
        self.state = state
        self.status = status
        self.ports = ports
        self.cpu = cpu
        self.memUsed = memUsed
        self.memLimit = memLimit
        self.upd = upd
        self.ignored = ignored
    }

    enum CodingKeys: String, CodingKey {
        case id, name, image, state, status, ports, cpu, upd, ignored
        case memUsed = "mem_used"
        case memLimit = "mem_limit"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? id
        image = try c.decodeIfPresent(String.self, forKey: .image) ?? ""
        state = try c.decodeIfPresent(String.self, forKey: .state) ?? ""
        status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        ports = try c.decodeIfPresent([String].self, forKey: .ports) ?? []
        cpu = try c.decodeIfPresent(Double.self, forKey: .cpu) ?? 0
        memUsed = try c.decodeIfPresent(Int64.self, forKey: .memUsed) ?? 0
        memLimit = try c.decodeIfPresent(Int64.self, forKey: .memLimit) ?? 0
        upd = try c.decodeIfPresent(String.self, forKey: .upd) ?? ""
        ignored = try c.decodeIfPresent(Bool.self, forKey: .ignored) ?? false
    }
}

public struct DockerImage: Decodable, Hashable, Sendable, Identifiable {
    public let id: String
    public let repoTags: [String]
    public let size: Int64
    /// Some container uses it.
    public let used: Bool

    public var title: String { repoTags.first ?? String(id.prefix(12)) }

    public init(id: String, repoTags: [String] = [], size: Int64 = 0, used: Bool = false) {
        self.id = id
        self.repoTags = repoTags
        self.size = size
        self.used = used
    }

    enum CodingKeys: String, CodingKey {
        case id, size, used
        case repoTags = "repo_tags"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        repoTags = try c.decodeIfPresent([String].self, forKey: .repoTags) ?? []
        size = try c.decodeIfPresent(Int64.self, forKey: .size) ?? 0
        used = try c.decodeIfPresent(Bool.self, forKey: .used) ?? false
    }
}

public struct DockerInfo: Decodable, Hashable, Sendable {
    /// Docker is there on the server.
    public let available: Bool
    public let containers: [DockerContainer]
    public let images: [DockerImage]

    public init(available: Bool = true, containers: [DockerContainer] = [], images: [DockerImage] = []) {
        self.available = available
        self.containers = containers
        self.images = images
    }

    enum CodingKeys: String, CodingKey { case available, containers, images }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        available = try c.decodeIfPresent(Bool.self, forKey: .available) ?? true
        containers = try c.decodeIfPresent([DockerContainer].self, forKey: .containers) ?? []
        images = try c.decodeIfPresent([DockerImage].self, forKey: .images) ?? []
    }
}

public enum DockerAction: String, CaseIterable, Sendable {
    case start, stop, restart, remove
}

// MARK: disk

public struct DiskFS: Decodable, Hashable, Sendable, Identifiable {
    public let mount: String
    public let device: String
    public let fs: String
    public let total: Int64
    public let used: Int64
    public let avail: Int64
    public let pct: Double
    /// How many days until it is full at the pace of the last weeks (0: unknown or not growing).
    public let daysLeft: Double
    public let perDay: Double

    public var id: String { mount }

    public init(mount: String, device: String = "", fs: String = "", total: Int64 = 0, used: Int64 = 0, avail: Int64 = 0, pct: Double = 0, daysLeft: Double = 0, perDay: Double = 0) {
        self.mount = mount
        self.device = device
        self.fs = fs
        self.total = total
        self.used = used
        self.avail = avail
        self.pct = pct
        self.daysLeft = daysLeft
        self.perDay = perDay
    }

    enum CodingKeys: String, CodingKey {
        case mount, device, fs, total, used, avail, pct
        case daysLeft = "days_left"
        case perDay = "per_day"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mount = try c.decode(String.self, forKey: .mount)
        device = try c.decodeIfPresent(String.self, forKey: .device) ?? ""
        fs = try c.decodeIfPresent(String.self, forKey: .fs) ?? ""
        total = try c.decodeIfPresent(Int64.self, forKey: .total) ?? 0
        used = try c.decodeIfPresent(Int64.self, forKey: .used) ?? 0
        avail = try c.decodeIfPresent(Int64.self, forKey: .avail) ?? 0
        pct = try c.decodeIfPresent(Double.self, forKey: .pct) ?? 0
        daysLeft = try c.decodeIfPresent(Double.self, forKey: .daysLeft) ?? 0
        perDay = try c.decodeIfPresent(Double.self, forKey: .perDay) ?? 0
    }
}

public struct DiskCleanItem: Decodable, Hashable, Sendable, Identifiable {
    /// "journal", "apt", "docker-images", "docker-build", "kernels"...
    public let kind: String
    /// What it takes now.
    public let size: Int64
    /// What cleaning it gives back.
    public let frees: Int64

    public var id: String { kind }

    public init(kind: String, size: Int64 = 0, frees: Int64 = 0) {
        self.kind = kind
        self.size = size
        self.frees = frees
    }

    enum CodingKeys: String, CodingKey { case kind, size, frees }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        kind = try c.decode(String.self, forKey: .kind)
        size = try c.decodeIfPresent(Int64.self, forKey: .size) ?? 0
        frees = try c.decodeIfPresent(Int64.self, forKey: .frees) ?? 0
    }
}

public struct DiskState: Decodable, Hashable, Sendable {
    public let filesystems: [DiskFS]
    public let cleanup: [DiskCleanItem]

    public init(filesystems: [DiskFS] = [], cleanup: [DiskCleanItem] = []) {
        self.filesystems = filesystems
        self.cleanup = cleanup
    }

    enum CodingKeys: String, CodingKey { case filesystems, cleanup }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        filesystems = try c.decodeIfPresent([DiskFS].self, forKey: .filesystems) ?? []
        cleanup = try c.decodeIfPresent([DiskCleanItem].self, forKey: .cleanup) ?? []
    }
}

public struct DiskCleanResult: Decodable, Hashable, Sendable {
    public let freed: Int64
    public let output: String

    public init(freed: Int64 = 0, output: String = "") {
        self.freed = freed
        self.output = output
    }

    enum CodingKeys: String, CodingKey { case freed, output }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        freed = try c.decodeIfPresent(Int64.self, forKey: .freed) ?? 0
        output = try c.decodeIfPresent(String.self, forKey: .output) ?? ""
    }
}

// MARK: streams (the journal, the jobs of the updates)

/// What a stream of a server carries as text: the journal of the system, or the output of a job of the packages.
public enum ServerStream: Sendable, Equatable {
    /// `unit` (a service of systemd, empty: all), `priority` (0 to 7, as journalctl), `kernel` (the log of the kernel), `lines` (how many of the last ones to start from).
    case logs(unit: String, priority: Int, kernel: Bool, lines: Int)
    case apt(AptMode)
    /// Windows Update: the ids of the updates to install (a GUID each; empty: all).
    case winUpdate(ids: [String])

    /// The query of the stream; nil when a value is not what the server accepts (nothing odd goes into the address).
    var query: [URLQueryItem]? {
        switch self {
        case .logs(let unit, let priority, let kernel, let lines):
            var items = [URLQueryItem(name: "kind", value: "logs"), URLQueryItem(name: "lines", value: String(min(max(lines, 1), 1000))),
                         URLQueryItem(name: "priority", value: String(min(max(priority, 0), 7)))]
            if !unit.isEmpty {
                guard unit.range(of: #"^[A-Za-z0-9:_.@-]{1,120}$"#, options: .regularExpression) != nil else { return nil }
                items.append(URLQueryItem(name: "unit", value: unit))
            }
            if kernel { items.append(URLQueryItem(name: "kernel", value: "1")) }
            return items
        case .winUpdate(let ids):
            guard ids.count <= 200, ids.allSatisfy({ $0.range(of: #"^[0-9a-fA-F-]{36}$"#, options: .regularExpression) != nil }) else { return nil }
            var items = [URLQueryItem(name: "kind", value: "winupdate")]
            if !ids.isEmpty { items.append(URLQueryItem(name: "ids", value: ids.joined(separator: ","))) }
            return items
        case .apt(let mode):
            switch mode {
            case .update: return [URLQueryItem(name: "kind", value: "apt"), URLQueryItem(name: "mode", value: "update")]
            case .upgrade: return [URLQueryItem(name: "kind", value: "apt"), URLQueryItem(name: "mode", value: "upgrade")]
            case .security: return [URLQueryItem(name: "kind", value: "apt"), URLQueryItem(name: "mode", value: "security")]
            case .packages(let names):
                guard !names.isEmpty, names.allSatisfy({ $0.range(of: #"^[a-z0-9][a-z0-9+.:~-]{0,100}$"#, options: .regularExpression) != nil }) else { return nil }
                return [URLQueryItem(name: "kind", value: "apt"), URLQueryItem(name: "mode", value: "packages"), URLQueryItem(name: "names", value: names.joined(separator: ","))]
            }
        }
    }
}
