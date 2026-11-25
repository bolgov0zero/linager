import Foundation

public enum DashboardFilter: String, CaseIterable, Sendable {
    case all, problems, offline
}

public enum DashboardOrder: String, CaseIterable, Sendable {
    /// The order of the dashboard on the server.
    case dashboard
    /// The worst first.
    case state
    case name
}

/// A part of the dashboard: the servers of one folder (id 0 = servers that lie in no folder).
public struct DashboardSection: Identifiable, Hashable, Sendable {
    public let id: Int64
    /// The names from the top level down.
    public let path: [String]
    public let servers: [ServerCard]

    public var title: String { path.joined(separator: " › ") }

    public init(id: Int64, path: [String], servers: [ServerCard]) {
        self.id = id
        self.path = path
        self.servers = servers
    }
}

public enum Dashboard {
    /// The folders as one list in display order (a folder, then everything under it), each with its path.
    public static func flatten(_ list: [Folder]) -> [(folder: Folder, path: [String], depth: Int)] {
        let ids = Set(list.map(\.id))
        var byParent: [Int64?: [Folder]] = [:]
        for f in list {
            // a folder whose parent is unknown (a stale list) is shown on the top level rather than lost
            let p = f.parentId.flatMap { ids.contains($0) ? $0 : nil }
            byParent[p, default: []].append(f)
        }
        var out: [(Folder, [String], Int)] = []
        func walk(_ parent: Int64?, _ path: [String], _ seen: Set<Int64>) {
            for f in byParent[parent] ?? [] where !seen.contains(f.id) {
                let p = path + [f.name]
                out.append((f, p, path.count))
                walk(f.id, p, seen.union([f.id]))
            }
        }
        walk(nil, [], [])
        return out
    }

    /// The id of the folder and of every folder under it.
    public static func subtree(_ list: [Folder], of id: Int64) -> Set<Int64> {
        var out: Set<Int64> = [id]
        var grew = true
        while grew {
            grew = false
            for f in list {
                if let p = f.parentId, out.contains(p), !out.contains(f.id) {
                    out.insert(f.id)
                    grew = true
                }
            }
        }
        return out
    }

    public static func matches(_ c: ServerCard, query: String) -> Bool {
        let q = query.trimmingCharacters(in: .whitespaces)
        if q.isEmpty { return true }
        return c.name.localizedCaseInsensitiveContains(q) || c.address.localizedCaseInsensitiveContains(q)
    }

    /// The cards that pass the filter, the search and the folder filter, in the chosen order.
    public static func visible(_ cards: [ServerCard], filter: DashboardFilter, query: String, folder: Int64?, folders: [Folder], order: DashboardOrder) -> [ServerCard] {
        let inside = folder.map { subtree(folders, of: $0) }
        var out = cards.filter { c in
            switch filter {
            case .all: break
            case .problems: if levelOf(c) == .ok { return false }
            case .offline: if levelOf(c) != .off { return false }
            }
            if let inside, !(c.folderId.map(inside.contains) ?? false) { return false }
            return matches(c, query: query)
        }
        switch order {
        case .dashboard: break
        case .name: out.sort { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        case .state: out = out.enumerated().sorted { a, b in
                let (x, y) = (weight(levelOf(a.element)), weight(levelOf(b.element)))
                return x != y ? x < y : a.offset < b.offset
            }.map(\.element)
        }
        return out
    }

    /// crit, then offline, then warn, then ok.
    private static func weight(_ l: Level) -> Int {
        switch l {
        case .crit: return 0
        case .off: return 1
        case .warn: return 2
        case .ok: return 3
        }
    }

    /// The servers grouped by folder, in the order of the tree; folders with nothing to show are left out. A server whose folder is
    /// unknown (or none) goes to the section with id 0, which comes last.
    public static func sections(_ cards: [ServerCard], folders: [Folder]) -> [DashboardSection] {
        let known = Set(folders.map(\.id))
        var out: [DashboardSection] = []
        for (f, path, _) in flatten(folders) {
            let here = cards.filter { $0.folderId == f.id }
            if !here.isEmpty { out.append(DashboardSection(id: f.id, path: path, servers: here)) }
        }
        let loose = cards.filter { $0.folderId.map { !known.contains($0) } ?? true }
        if !loose.isEmpty { out.append(DashboardSection(id: 0, path: [], servers: loose)) }
        return out
    }
}
