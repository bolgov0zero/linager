import Testing
@testable import LinagerKit

private let tree: [Folder] = [
    Folder(id: 1, name: "Продакшен"),
    Folder(id: 2, name: "Базы данных", parentId: 1),
    Folder(id: 3, name: "Реплики", parentId: 2),
    Folder(id: 4, name: "Офис"),
    Folder(id: 5, name: "Пустая"),
]

private func srv(_ id: String, _ os: String, folder: Int64?, online: Bool = true, gui: Bool = false, desktopOff: Bool = false) -> ServerCard {
    ServerCard(id: id, name: id, os: os, online: online, sample: Sample(gui: gui, desktopOff: desktopOff), folderId: folder)
}

private let cards = [
    srv("web", "Debian GNU/Linux 12 (bookworm)", folder: 1),
    srv("db", "Debian GNU/Linux 12 (bookworm)", folder: 2),
    srv("rep", "Ubuntu 24.04", folder: 3, online: false),
    srv("desk", "Ubuntu 24.04", folder: 4, gui: true),
    srv("pc", "Windows 11 Pro", folder: 4),
    srv("srv", "Windows Server 2022", folder: 4, desktopOff: true),
    srv("pi", "Debian GNU/Linux 12 (bookworm)", folder: nil),
    srv("lost", "Windows 10", folder: 777, online: false),
]

private func ids(_ rows: [ConsoleRow]) -> [String] { rows.map(\.id) }

@Suite("Console")
struct ConsoleTests {
    @Test func kindsFollowTheRulesOfTheDesktopApp() {
        let linux = srv("a", "Debian 12", folder: nil)
        let guiLinux = srv("b", "Ubuntu", folder: nil, gui: true)
        let win = srv("c", "Windows 11", folder: nil)
        let winOff = srv("d", "Windows 11", folder: nil, desktopOff: true)
        #expect([linux, guiLinux, win, winOff].allSatisfy(ConnectKind.terminal.offers))
        #expect(ConnectKind.assistant.offers(linux) == false)
        #expect(ConnectKind.assistant.offers(guiLinux))
        #expect(ConnectKind.assistant.offers(win))
        #expect(ConnectKind.rdp.offers(win))
        #expect(ConnectKind.rdp.offers(winOff) == false)
        #expect(ConnectKind.rdp.offers(guiLinux) == false)
        #expect(ConnectKind.allCases.allSatisfy { $0.isReady })
    }

    @Test func treeOrderIsFoldersThenServers() {
        let r = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(), collapsed: [])
        // "Пустая" has no servers: it is not shown; the folder without a folder (pi) and the unknown one (lost) come last
        #expect(ids(r) == ["f:1", "f:2", "f:3", "s:rep", "s:db", "s:web", "f:4", "s:desk", "s:pc", "s:srv", "s:pi", "s:lost"])
        #expect(r.map(\.depth) == [0, 1, 2, 3, 2, 1, 0, 1, 1, 1, 0, 0])
        if case .folder(_, _, let n, _) = r[0].content { #expect(n == 3) } else { Issue.record("not a folder") } // nested servers are counted
        if case .folder(_, _, let n, _) = r[1].content { #expect(n == 2) } else { Issue.record("not a folder") }
    }

    @Test func aFoldedFolderHidesItsContentButKeepsTheCount() {
        let r = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(), collapsed: [1])
        #expect(ids(r).prefix(2) == ["f:1", "f:4"])
        if case .folder(_, _, let n, let open) = r[0].content { #expect(n == 3 && !open) } else { Issue.record("not a folder") }
    }

    @Test func theKindHidesServersWithoutItAndEmptyFolders() {
        let assistant = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(kind: .assistant), collapsed: [])
        // no Linux without a console, no Продакшен at all; lost is Windows 10 and stays (offline servers are not hidden by the kind)
        #expect(ids(assistant) == ["f:4", "s:desk", "s:pc", "s:srv", "s:lost"])
        let rdp = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(kind: .rdp), collapsed: [])
        #expect(ids(rdp) == ["f:4", "s:pc", "s:lost"]) // srv has the remote desktop turned off
    }

    @Test func systemAndStatusFiltersStack() {
        let linuxOn = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(system: .linux, status: .online), collapsed: [])
        #expect(ids(linuxOn) == ["f:1", "f:2", "s:db", "s:web", "f:4", "s:desk", "s:pi"])
        let winOff = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(system: .windows, status: .offline), collapsed: [])
        #expect(ids(winOff) == ["s:lost"])
        let none = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(kind: .rdp, system: .linux), collapsed: [])
        #expect(none.isEmpty)
        #expect(ConsoleFilter(system: .linux).narrowed && !ConsoleFilter(kind: .rdp).narrowed)
    }

    @Test func searchIsFlatWithPathsAndRespectsTheFilters() {
        let r = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(query: "базы"), collapsed: [1])
        // a folder name finds the servers under it, and the result is a plain list even if a folder is folded
        #expect(ids(r).contains("s:db"))
        if case .server(_, let path) = r.first(where: { $0.id == "s:db" })!.content { #expect(path == ["Продакшен", "Базы данных"]) } else { Issue.record("not a server") }
        #expect(r.allSatisfy { if case .server = $0.content { true } else { false } })
        // the system is a search word too, and the filters still apply
        let win = Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(status: .online, query: "windows"), collapsed: [])
        #expect(ids(win) == ["s:pc", "s:srv"])
        #expect(Console.rows(cards: cards, folders: tree, filter: ConsoleFilter(query: "  "), collapsed: []).count > 8) // blanks are no search
    }

    @Test func foldedFoldersAreStoredAsAString() {
        #expect(Console.encode([12, 3, 7]) == "3,7,12")
        #expect(Console.decode("3,7,12") == [3, 7, 12])
        #expect(Console.decode("") == [] && Console.decode("x,4") == [4])
        #expect(Console.allFolderIds(tree) == [1, 2, 3, 4, 5])
    }

    @Test func aCycleOrAnOrphanFolderDoesNotHangOrLoseServers() {
        let odd = [Folder(id: 1, name: "a", parentId: 2), Folder(id: 2, name: "b", parentId: 1), Folder(id: 3, name: "orphan", parentId: 99)]
        let cs = [srv("x", "Debian", folder: 1), srv("y", "Debian", folder: 3)]
        let r = Console.rows(cards: cs, folders: odd, filter: ConsoleFilter(), collapsed: [])
        // the folders of the cycle cannot be reached (nothing hangs), the orphan folder is shown on the top level
        #expect(ids(r).contains("f:3") && ids(r).contains("s:y"))
    }
}
