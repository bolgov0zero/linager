import Foundation

/// The last dashboard that was received, kept to be shown when the server cannot be reached ("data from 14:02").
public struct DashboardSnapshot: Codable, Equatable, Sendable {
    public var endpoint: Endpoint
    public var savedAt: Date
    public var cards: [ServerCard]
    public var folders: [Folder]

    public init(endpoint: Endpoint, savedAt: Date, cards: [ServerCard], folders: [Folder]) {
        self.endpoint = endpoint
        self.savedAt = savedAt
        self.cards = cards
        self.folders = folders
    }
}

public protocol SnapshotStore: Sendable {
    func load(for endpoint: Endpoint) -> DashboardSnapshot?
    func save(_ snapshot: DashboardSnapshot)
    func clear()
}

/// A file in the Caches folder: not in backups, locked while the phone is locked.
public struct FileSnapshotStore: SnapshotStore {
    private let url: URL

    public init(url: URL? = nil) {
        self.url = url ?? (FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory).appendingPathComponent("dashboard.json")
    }

    public func load(for endpoint: Endpoint) -> DashboardSnapshot? {
        guard let data = try? Data(contentsOf: url), let s = try? JSONDecoder().decode(DashboardSnapshot.self, from: data), s.endpoint == endpoint else { return nil }
        return s
    }

    public func save(_ snapshot: DashboardSnapshot) {
        guard let data = try? JSONEncoder().encode(snapshot) else { return }
        try? data.write(to: url, options: [.atomic, .completeFileProtection])
    }

    public func clear() { try? FileManager.default.removeItem(at: url) }
}

public final class MemorySnapshotStore: SnapshotStore, @unchecked Sendable {
    private let lock = NSLock()
    private var snapshot: DashboardSnapshot?

    public init() {}

    public func load(for endpoint: Endpoint) -> DashboardSnapshot? { lock.withLock { snapshot?.endpoint == endpoint ? snapshot : nil } }
    public func save(_ s: DashboardSnapshot) { lock.withLock { snapshot = s } }
    public func clear() { lock.withLock { snapshot = nil } }
}
