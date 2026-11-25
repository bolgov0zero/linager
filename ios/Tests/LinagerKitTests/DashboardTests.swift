import Testing
@testable import LinagerKit

private let tree: [Folder] = [
    Folder(id: 1, name: "Офис"),
    Folder(id: 2, name: "Продакшен"),
    Folder(id: 3, name: "Базы данных", parentId: 2),
    Folder(id: 4, name: "Пустая"),
    Folder(id: 5, name: "Реплики", parentId: 3),
    Folder(id: 6, name: "Сирота", parentId: 99), // its parent is not in the list
]

private func srv(_ id: String, folder: Int64?, cpu: Double = 10, online: Bool = true, address: String = "10.0.0.1") -> ServerCard {
    ServerCard(id: id, name: id, address: address, online: online, sample: Sample(cpu: cpu), folderId: folder)
}

@Suite("Dashboard")
struct DashboardTests {
    @Test func treeOrderAndPaths() {
        let flat = Dashboard.flatten(tree)
        #expect(flat.map(\.folder.id) == [1, 2, 3, 5, 4, 6])
        #expect(flat[3].path == ["Продакшен", "Базы данных", "Реплики"])
        #expect(flat[3].depth == 2)
        // a folder whose parent is unknown is shown on the top level rather than lost
        #expect(flat[5].path == ["Сирота"])
    }

    @Test func aCycleDoesNotHangOrLoseFolders() {
        let loop = [Folder(id: 1, name: "a", parentId: 2), Folder(id: 2, name: "b", parentId: 1)]
        // both have a known parent, so neither is a root: nothing is reachable, and nothing hangs
        #expect(Dashboard.flatten(loop).isEmpty)
    }

    @Test func subtreeIncludesEveryLevel() {
        #expect(Dashboard.subtree(tree, of: 2) == [2, 3, 5])
        #expect(Dashboard.subtree(tree, of: 4) == [4])
    }

    @Test func sectionsFollowTheTreeAndSkipEmptyFolders() {
        let cards = [srv("a", folder: 3), srv("b", folder: 1), srv("c", folder: nil), srv("d", folder: 3), srv("e", folder: 777), srv("f", folder: 2)]
        let s = Dashboard.sections(cards, folders: tree)
        #expect(s.map(\.id) == [1, 2, 3, 0])
        #expect(s[2].title == "Продакшен › Базы данных")
        #expect(s[2].servers.map(\.id) == ["a", "d"]) // the order of the dashboard is kept
        #expect(s[3].servers.map(\.id) == ["c", "e"]) // no folder, or a folder that is not known
        #expect(s[3].title == "")
    }

    @Test func filters() {
        let cards = [srv("ok", folder: 1), srv("hot", folder: 1, cpu: 95), srv("warm", folder: 1, cpu: 80), srv("off", folder: 1, online: false)]
        func ids(_ f: DashboardFilter) -> [String] {
            Dashboard.visible(cards, filter: f, query: "", folder: nil, folders: tree, order: .dashboard).map(\.id)
        }
        #expect(ids(.all) == ["ok", "hot", "warm", "off"])
        #expect(ids(.problems) == ["hot", "warm", "off"])
        #expect(ids(.offline) == ["off"])
    }

    @Test func searchByNameOrAddressIgnoresCase() {
        let cards = [srv("Web-01", folder: nil, address: "10.0.1.21"), srv("db", folder: nil, address: "10.0.2.5")]
        func ids(_ q: String) -> [String] { Dashboard.visible(cards, filter: .all, query: q, folder: nil, folders: [], order: .dashboard).map(\.id) }
        #expect(ids("web") == ["Web-01"])
        #expect(ids("10.0.2") == ["db"])
        #expect(ids("  ") == ["Web-01", "db"])
        #expect(ids("zzz").isEmpty)
    }

    @Test func folderFilterTakesTheWholeSubtree() {
        let cards = [srv("a", folder: 2), srv("b", folder: 3), srv("c", folder: 5), srv("d", folder: 1), srv("e", folder: nil)]
        let ids = Dashboard.visible(cards, filter: .all, query: "", folder: 2, folders: tree, order: .dashboard).map(\.id)
        #expect(ids == ["a", "b", "c"])
    }

    @Test func orders() {
        let cards = [srv("b-ok", folder: nil), srv("a-warm", folder: nil, cpu: 80), srv("c-off", folder: nil, online: false), srv("d-hot", folder: nil, cpu: 99), srv("e-ok", folder: nil)]
        func ids(_ o: DashboardOrder) -> [String] { Dashboard.visible(cards, filter: .all, query: "", folder: nil, folders: [], order: o).map(\.id) }
        #expect(ids(.dashboard) == ["b-ok", "a-warm", "c-off", "d-hot", "e-ok"])
        #expect(ids(.name) == ["a-warm", "b-ok", "c-off", "d-hot", "e-ok"])
        #expect(ids(.state) == ["d-hot", "c-off", "a-warm", "b-ok", "e-ok"]) // critical, offline, warning, ok; ties keep their order
    }
}
