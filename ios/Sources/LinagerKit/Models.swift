import Foundation

// The shapes of the server API (the same ones as app/frontend/src/lib/api.ts). The decoding is forgiving: a field a server
// does not send (an older one) gets a neutral value instead of failing the whole dashboard.

public struct TagItem: Codable, Hashable, Sendable, Identifiable {
    public let id: Int64
    public let name: String
    /// A key of the tag palette (see TagPalette); unknown keys are drawn gray.
    public let color: String

    public init(id: Int64, name: String, color: String) {
        self.id = id
        self.name = name
        self.color = color
    }
}

/// A folder of servers (the API calls it a group). `parentId` makes the tree.
public struct Folder: Codable, Hashable, Sendable, Identifiable {
    public let id: Int64
    public let name: String
    public let parentId: Int64?

    public init(id: Int64, name: String, parentId: Int64? = nil) {
        self.id = id
        self.name = name
        self.parentId = parentId
    }

    enum CodingKeys: String, CodingKey {
        case id, name
        case parentId = "parent_id"
    }
}

/// The last measurement of a server. Only what the dashboard shows.
public struct Sample: Codable, Hashable, Sendable {
    public var ts: Int64
    public var cpu: Double
    public var memPct: Double
    public var diskPct: Double
    public var rxBps: Double
    public var txBps: Double
    public var uptime: Int64
    public var failedServices: Int
    public var rebootRequired: Bool
    /// -1 = unknown
    public var updates: Int
    public var securityUpdates: Int
    /// -1 = unknown or no Docker
    public var imageUpdates: Int
    /// A Linux agent with a graphical console the Assistant can show.
    public var gui: Bool
    /// A Windows agent whose remote desktop (RDP) is off or the edition has none.
    public var desktopOff: Bool

    public init(ts: Int64 = 0, cpu: Double = 0, memPct: Double = 0, diskPct: Double = 0, rxBps: Double = 0, txBps: Double = 0, uptime: Int64 = 0,
                failedServices: Int = 0, rebootRequired: Bool = false, updates: Int = 0, securityUpdates: Int = 0, imageUpdates: Int = 0, gui: Bool = false, desktopOff: Bool = false) {
        self.ts = ts
        self.cpu = cpu
        self.memPct = memPct
        self.diskPct = diskPct
        self.rxBps = rxBps
        self.txBps = txBps
        self.uptime = uptime
        self.failedServices = failedServices
        self.rebootRequired = rebootRequired
        self.updates = updates
        self.securityUpdates = securityUpdates
        self.imageUpdates = imageUpdates
        self.gui = gui
        self.desktopOff = desktopOff
    }

    enum CodingKeys: String, CodingKey {
        case ts, cpu, uptime, updates
        case memPct = "mem_pct"
        case diskPct = "disk_pct"
        case rxBps = "rx_bps"
        case txBps = "tx_bps"
        case failedServices = "failed_services"
        case rebootRequired = "reboot_required"
        case securityUpdates = "security_updates"
        case imageUpdates = "image_updates"
        case gui
        case desktopOff = "desktop_off"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ts = try c.decodeIfPresent(Int64.self, forKey: .ts) ?? 0
        cpu = try c.decodeIfPresent(Double.self, forKey: .cpu) ?? 0
        memPct = try c.decodeIfPresent(Double.self, forKey: .memPct) ?? 0
        diskPct = try c.decodeIfPresent(Double.self, forKey: .diskPct) ?? 0
        rxBps = try c.decodeIfPresent(Double.self, forKey: .rxBps) ?? 0
        txBps = try c.decodeIfPresent(Double.self, forKey: .txBps) ?? 0
        uptime = try c.decodeIfPresent(Int64.self, forKey: .uptime) ?? 0
        failedServices = try c.decodeIfPresent(Int.self, forKey: .failedServices) ?? 0
        rebootRequired = try c.decodeIfPresent(Bool.self, forKey: .rebootRequired) ?? false
        updates = try c.decodeIfPresent(Int.self, forKey: .updates) ?? 0
        securityUpdates = try c.decodeIfPresent(Int.self, forKey: .securityUpdates) ?? 0
        imageUpdates = try c.decodeIfPresent(Int.self, forKey: .imageUpdates) ?? 0
        gui = try c.decodeIfPresent(Bool.self, forKey: .gui) ?? false
        desktopOff = try c.decodeIfPresent(Bool.self, forKey: .desktopOff) ?? false
    }
}

/// A server on the dashboard.
public struct ServerCard: Codable, Hashable, Sendable, Identifiable {
    public let id: String
    public var name: String
    public var address: String
    public var os: String
    public var agentVersion: String
    public var online: Bool
    public var lastSeenAt: Date?
    public var sample: Sample?
    /// The last CPU values, 5 seconds apart, oldest first.
    public var spark: [Double]
    public var folderId: Int64?
    public var tags: [TagItem]

    public init(id: String, name: String, address: String = "", os: String = "", agentVersion: String = "", online: Bool = true, lastSeenAt: Date? = nil,
                sample: Sample? = nil, spark: [Double] = [], folderId: Int64? = nil, tags: [TagItem] = []) {
        self.id = id
        self.name = name
        self.address = address
        self.os = os
        self.agentVersion = agentVersion
        self.online = online
        self.lastSeenAt = lastSeenAt
        self.sample = sample
        self.spark = spark
        self.folderId = folderId
        self.tags = tags
    }

    enum CodingKeys: String, CodingKey {
        case id, name, address, os, online, sample, spark
        case agentVersion = "agent_version"
        case lastSeenAt = "last_seen_at"
        case folderId = "group_id"
        case tags = "tag_items"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? id
        address = try c.decodeIfPresent(String.self, forKey: .address) ?? ""
        os = try c.decodeIfPresent(String.self, forKey: .os) ?? ""
        agentVersion = try c.decodeIfPresent(String.self, forKey: .agentVersion) ?? ""
        online = try c.decodeIfPresent(Bool.self, forKey: .online) ?? false
        lastSeenAt = (try c.decodeIfPresent(String.self, forKey: .lastSeenAt)).flatMap(Self.parseDate)
        sample = try c.decodeIfPresent(Sample.self, forKey: .sample)
        spark = try c.decodeIfPresent([Double].self, forKey: .spark) ?? []
        folderId = try c.decodeIfPresent(Int64.self, forKey: .folderId)
        tags = try c.decodeIfPresent([TagItem].self, forKey: .tags) ?? []
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(id, forKey: .id)
        try c.encode(name, forKey: .name)
        try c.encode(address, forKey: .address)
        try c.encode(os, forKey: .os)
        try c.encode(agentVersion, forKey: .agentVersion)
        try c.encode(online, forKey: .online)
        if let d = lastSeenAt {
            let f = ISO8601DateFormatter()
            f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            try c.encode(f.string(from: d), forKey: .lastSeenAt)
        }
        try c.encodeIfPresent(sample, forKey: .sample)
        try c.encode(spark, forKey: .spark)
        try c.encodeIfPresent(folderId, forKey: .folderId)
        try c.encode(tags, forKey: .tags)
    }

    /// Go writes times as RFC 3339 with up to nine fraction digits.
    static func parseDate(_ s: String) -> Date? {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = f.date(from: s) { return d }
        f.formatOptions = [.withInternetDateTime]
        if let d = f.date(from: s) { return d }
        // more fraction digits than the formatter takes: cut them
        if let dot = s.firstIndex(of: "."), let end = s[dot...].firstIndex(where: { !$0.isNumber && $0 != "." }) {
            return f.date(from: String(s[..<dot]) + String(s[end...]))
        }
        return nil
    }
}

/// What the server answers to a sign-in and to `api/me`.
public struct SessionInfo: Decodable, Hashable, Sendable {
    public let csrf: String
    public let login: String
    /// "admin", "operator" or "auditor"
    public let role: String
    public let totpEnabled: Bool
    /// The modules an operator may use (an administrator has all of them, an auditor none).
    public let modules: [String]
    /// What the server says of this device (nil: an older server that does not know devices).
    public let device: DeviceState?

    public init(csrf: String, login: String, role: String = "admin", totpEnabled: Bool = false, modules: [String] = [], device: DeviceState? = nil) {
        self.csrf = csrf
        self.login = login
        self.role = role
        self.totpEnabled = totpEnabled
        self.modules = modules
        self.device = device
    }

    /// The device waits for an administrator to approve it: nothing but "who am I" and the sign-out is allowed until then.
    public var isWaiting: Bool { device?.confirm == true && device?.status == "pending" }

    /// A module of the server ("services", "processes", "docker", "logs", "disk", "apt"...): for an administrator, and for an operator who was given it. An auditor has none.
    public func can(_ module: String) -> Bool { role == "admin" || (role == "operator" && modules.contains(module)) }

    /// The terminal is for an administrator and for an operator who was given the module "terminal".
    public var canUseTerminal: Bool { role == "admin" || (role == "operator" && modules.contains("terminal")) }

    /// The Assistant is for an administrator and for an operator who was given the module "assistant".
    public var canUseAssistant: Bool { role == "admin" || (role == "operator" && modules.contains("assistant")) }

    /// The remote desktop is for an administrator and for an operator who was given the module "desktop".
    public var canUseDesktop: Bool { role == "admin" || (role == "operator" && modules.contains("desktop")) }

    enum CodingKeys: String, CodingKey {
        case csrf, login, role, modules, device
        case totpEnabled = "totp_enabled"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        csrf = try c.decode(String.self, forKey: .csrf)
        login = try c.decode(String.self, forKey: .login)
        role = try c.decodeIfPresent(String.self, forKey: .role) ?? "admin"
        totpEnabled = try c.decodeIfPresent(Bool.self, forKey: .totpEnabled) ?? false
        modules = try c.decodeIfPresent([String].self, forKey: .modules) ?? []
        device = try c.decodeIfPresent(DeviceState.self, forKey: .device)
    }
}

/// The device of this phone as the server sees it (the answer of the sign-in and of `api/me`).
public struct DeviceState: Decodable, Hashable, Sendable {
    /// The confirmation of devices is on.
    public let confirm: Bool
    public let id: Int64?
    /// "pending", "approved" or "denied"; with the confirmation off nothing waits, and the server says "approved".
    public let status: String?

    public init(confirm: Bool, id: Int64? = nil, status: String? = nil) {
        self.confirm = confirm
        self.id = id
        self.status = status
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        confirm = try c.decodeIfPresent(Bool.self, forKey: .confirm) ?? false
        id = try c.decodeIfPresent(Int64.self, forKey: .id)
        status = try c.decodeIfPresent(String.self, forKey: .status)
    }

    enum CodingKeys: String, CodingKey { case confirm, id, status }
}

/// A command saved on the server (Settings, Snippets of the desktop application): it can be typed into a terminal.
public struct Snippet: Codable, Hashable, Sendable, Identifiable {
    public let id: Int64
    public let name: String
    public let command: String

    public init(id: Int64, name: String, command: String) {
        self.id = id
        self.name = name
        self.command = command
    }
}

/// The state of the agent of a server, as `api/updates` reports it (administrators only).
public enum AgentFlag: String, Sendable, Hashable {
    case available
    case unsupported
}
