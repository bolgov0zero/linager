import Foundation

/// The two modes of the main screen (the bar at the bottom): the console is a bare list to connect from, the dashboard shows the state.
public enum AppMode: String, CaseIterable, Sendable {
    case console, dashboard
}

/// How a server of the console is opened. The chosen one decides which servers the list shows.
public enum ConnectKind: String, CaseIterable, Sendable {
    case terminal, rdp, assistant

    /// Whether the app can open it at all (every kind can now; a kind that cannot yet is shown grey and explains itself when tapped).
    public var isReady: Bool { true }

    /// The same rules as the desktop app: the terminal for every server; the remote desktop for Windows while the service is not turned
    /// off; the Assistant for Windows and for a Linux computer with a graphical console.
    public func offers(_ c: ServerCard) -> Bool {
        switch self {
        case .terminal: return true
        case .rdp: return isWindows(c.os) && c.sample?.desktopOff != true
        case .assistant: return assistantOffered(c)
        }
    }
}

public enum SystemFilter: String, CaseIterable, Sendable {
    case all, windows, linux
}

public enum StatusFilter: String, CaseIterable, Sendable {
    case all, online, offline
}

/// What narrows the list of the console: the kind of connection, the menu filters and the search.
public struct ConsoleFilter: Equatable, Sendable {
    public var kind: ConnectKind
    public var system: SystemFilter
    public var status: StatusFilter
    public var query: String

    public init(kind: ConnectKind = .terminal, system: SystemFilter = .all, status: StatusFilter = .all, query: String = "") {
        self.kind = kind
        self.system = system
        self.status = status
        self.query = query
    }

    /// A menu filter is on (the kind of connection does not count: it has its own switch).
    public var narrowed: Bool { system != .all || status != .all }

    public var searching: Bool { !query.trimmingCharacters(in: .whitespaces).isEmpty }

    /// A server passes the kind, the system and the status.
    public func passes(_ c: ServerCard) -> Bool {
        guard kind.offers(c) else { return false }
        switch system {
        case .all: break
        case .windows: if !isWindows(c.os) { return false }
        case .linux: if isWindows(c.os) { return false }
        }
        switch status {
        case .all: break
        case .online: if !c.online { return false }
        case .offline: if c.online { return false }
        }
        return true
    }
}

/// A row of the console list.
public struct ConsoleRow: Identifiable, Hashable, Sendable {
    public enum Content: Hashable, Sendable {
        /// `count`: the servers inside, the nested folders included, as far as they pass the filters.
        case folder(id: Int64, name: String, count: Int, open: Bool)
        /// `path`: the folders above it; shown under the name in a search result.
        case server(ServerCard, path: [String])
    }

    public let id: String
    /// How deep it lies: 0 on the top level.
    public let depth: Int
    public let content: Content

    public init(id: String, depth: Int, content: Content) {
        self.id = id
        self.depth = depth
        self.content = content
    }
}

public enum Console {
    /// The rows of the console. A search gives a plain list of servers with their paths. Otherwise it is the tree: a folder, then its
    /// nested folders, then its servers; a folder with no server that passes the filters (at any depth) is left out, and the servers of a
    /// folder that is folded are not listed. A server whose folder is unknown goes to the top level, after the folders.
    public static func rows(cards: [ServerCard], folders: [Folder], filter: ConsoleFilter, collapsed: Set<Int64>) -> [ConsoleRow] {
        let flat = Dashboard.flatten(folders)
        let pathOf = Dictionary(uniqueKeysWithValues: flat.map { ($0.folder.id, $0.path) })
        let passing = cards.filter(filter.passes)

        if filter.searching {
            let q = filter.query.trimmingCharacters(in: .whitespaces)
            return passing.filter { c in
                let path = c.folderId.flatMap { pathOf[$0] } ?? []
                return c.name.localizedCaseInsensitiveContains(q) || path.contains { $0.localizedCaseInsensitiveContains(q) }
                    || systemWord(c).localizedCaseInsensitiveContains(q) || c.os.localizedCaseInsensitiveContains(q)
            }.map { ConsoleRow(id: "s:" + $0.id, depth: 0, content: .server($0, path: $0.folderId.flatMap { pathOf[$0] } ?? [])) }
        }

        let known = Set(folders.map(\.id))
        var serversIn: [Int64: [ServerCard]] = [:]
        var loose: [ServerCard] = []
        for c in passing {
            if let f = c.folderId, known.contains(f) { serversIn[f, default: []].append(c) } else { loose.append(c) }
        }
        // the servers under a folder at any depth, to count them and to tell an empty folder
        var count: [Int64: Int] = [:]
        for item in flat.reversed() {
            let id = item.folder.id
            count[id, default: 0] += serversIn[id]?.count ?? 0
            if let p = item.folder.parentId, known.contains(p), p != id { count[p, default: 0] += count[id] ?? 0 }
        }

        var out: [ConsoleRow] = []
        var children: [Int64?: [Folder]] = [:]
        for f in folders {
            let p = f.parentId.flatMap { known.contains($0) && $0 != f.id ? $0 : nil }
            children[p, default: []].append(f)
        }
        func walk(_ parent: Int64?, _ depth: Int, _ seen: Set<Int64>) {
            for f in children[parent] ?? [] where !seen.contains(f.id) && (count[f.id] ?? 0) > 0 {
                let open = !collapsed.contains(f.id)
                out.append(ConsoleRow(id: "f:\(f.id)", depth: depth, content: .folder(id: f.id, name: f.name, count: count[f.id] ?? 0, open: open)))
                guard open else { continue }
                walk(f.id, depth + 1, seen.union([f.id]))
                for c in serversIn[f.id] ?? [] {
                    out.append(ConsoleRow(id: "s:" + c.id, depth: depth + 1, content: .server(c, path: [])))
                }
            }
        }
        walk(nil, 0, [])
        for c in loose { out.append(ConsoleRow(id: "s:" + c.id, depth: 0, content: .server(c, path: []))) }
        return out
    }

    /// The ids of every folder, to fold them all.
    public static func allFolderIds(_ folders: [Folder]) -> Set<Int64> { Set(folders.map(\.id)) }

    /// "windows" or "linux": what a search by the system finds.
    static func systemWord(_ c: ServerCard) -> String { isWindows(c.os) ? "windows" : "linux" }

    /// Folded folders as a string for the storage of the app: "3,7,12".
    public static func encode(_ ids: Set<Int64>) -> String { ids.sorted().map(String.init).joined(separator: ",") }

    public static func decode(_ s: String) -> Set<Int64> { Set(s.split(separator: ",").compactMap { Int64($0) }) }
}
