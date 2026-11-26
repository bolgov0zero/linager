import Foundation
import Testing
@testable import LinagerKit

private let session = Credentials(token: "tok123", csrf: "csrf-abc")

@Suite("Page of a server")
struct ServerPageModelTests {
    @Test func theDetailIsReadWithWhatIsMissingTolerated() throws {
        let full = try JSONDecoder().decode(ServerDetail.self, from: Data(#"{"id":"a","name":"web","online":true,"sample":{"cpu":41.5,"mem_used":4,"mem_total":8,"disks":[{"mount":"/","total":10,"used":4,"pct":40}],"load":[1,2,3],"updates":3},"system":{"cpu_cores":4},"modules":["files"]}"#.utf8))
        #expect(full.name == "web" && full.online)
        #expect(full.sample?.cpu == 41.5 && full.sample?.load == [1, 2, 3] && full.sample?.disks.first?.mount == "/")
        #expect(full.system?.cpuCores == 4 && full.modules == ["files"])
        // a bare answer: nothing is required but the id
        let bare = try JSONDecoder().decode(ServerDetail.self, from: Data(#"{"id":"b"}"#.utf8))
        #expect(bare.name == "b" && !bare.online && bare.sample == nil && bare.spark.isEmpty && bare.tags.isEmpty)
    }

    @Test func foldersComeFirstAndNamesAreSortedAsAPersonReads() {
        let list = FileList(path: "/", entries: [
            FileEntry(name: "file10", kind: .file), FileEntry(name: "file2", kind: .file), FileEntry(name: "zeta", kind: .dir), FileEntry(name: "Alpha", kind: .dir),
        ])
        #expect(list.sorted.map(\.name) == ["Alpha", "zeta", "file2", "file10"])
    }

    @Test func pathsOfFolders() {
        #expect(FilePath.child("/", "var") == "/var")
        #expect(FilePath.child("/var", "www") == "/var/www")
        #expect(FilePath.parent("/var/www") == "/var" && FilePath.parent("/var") == "/" && FilePath.parent("/") == "/")
        #expect(FilePath.parts("/var/www") == ["var", "www"] && FilePath.parts("/").isEmpty)
    }

    @Test func aValueOfTheQueryCannotBreakTheAddress() {
        #expect(LinagerClient.queryValue("/a b&c=d?#+é") == "/a%20b%26c%3Dd%3F%23%2B%C3%A9")
    }
}

@Suite("Page of a server over a real connection", .serialized)
struct ServerPageConnectionTests {
    @Test func theDetailTheHistoryAndTheFilesComeFromTheServer() async throws {
        let server = try FakeServer()
        let client = try await server.client(credentials: session)
        let d = try await client.serverDetail(serverID: "srv-a")
        #expect(d.name == "web-01" && d.sample?.memTotal == 8_589_934_592 && d.sample?.updates == 3 && d.sample?.rebootRequired == true)
        #expect(d.tags.first?.name == "web" && d.system?.kernel == "6.1.0-18-amd64")

        let h = try await client.history(serverID: "srv-a", period: .week)
        #expect(h.period == "7d" && h.points.count == 5 && h.points.last?.cpuAvg == 14)
        #expect(h.stats["cpu"]?.max == 20 && h.events.first?.kind == "reboot")

        let root = try await client.files(serverID: "srv-a", path: "/")
        #expect(root.sorted.map(\.name) == ["Docs", "var", "a.txt", "link"])
        #expect(root.entries.first { $0.name == "link" }?.linkTarget == "/var")
        // a name with signs of the address in it goes through the query whole
        let inner = try await client.files(serverID: "srv-a", path: "/var")
        #expect(inner.entries.first?.name == "b & c=d.txt")
        withExtendedLifetime(server) {}
    }

    @Test func aRefusalIsToldInOurTerms() async throws {
        let server = try FakeServer()
        let client = try await server.client(credentials: session)
        await #expect(throws: LinagerError.accessDenied) { _ = try await client.files(serverID: "srv-a", path: "/denied") }
        await #expect(throws: LinagerError.self) { _ = try await client.serverDetail(serverID: "missing") }
        withExtendedLifetime(server) {}
    }

    @Test func aFileIsDownloadedUnderItsOwnName() async throws {
        let server = try FakeServer()
        let client = try await server.client(credentials: session)
        let url = try await client.downloadFile(serverID: "srv-a", path: "/var/www/a b.txt")
        defer { try? FileManager.default.removeItem(at: url.deletingLastPathComponent()) }
        #expect(url.lastPathComponent == "a b.txt")
        #expect(try String(contentsOf: url, encoding: .utf8) == "content of /var/www/a b.txt")
        await #expect(throws: LinagerError.accessDenied) { _ = try await client.downloadFile(serverID: "srv-a", path: "/denied") }
        withExtendedLifetime(server) {}
    }
}

@Suite("File operations over a real connection", .serialized)
struct FileOperationsConnectionTests {
    private func ops(_ server: FakeServer) async throws -> [[Any]] {
        let session = URLSession(configuration: .ephemeral, delegate: PinDelegate(mode: .pinned(server.fingerprint)), delegateQueue: nil)
        let (data, _) = try await session.data(from: try #require(server.endpoint.url(path: "/api/test/ops")))
        return try #require(try JSONSerialization.jsonObject(with: data) as? [[Any]])
    }

    @Test func aTextIsReadAndSavedWithItsVersion() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session)
        let f = try await c.fileContent(serverID: "srv-a", path: "/etc/motd")
        #expect(f.content == "hello\nworld\n" && f.mtime == 1_700_000_000_000 && f.mode == "0644")
        let m2 = try await c.saveFile(serverID: "srv-a", path: "/etc/motd", content: "changed\n", mtime: f.mtime)
        #expect(m2 == f.mtime + 1000)
        // the version that was read is stale now: a second save with it is refused, and the file stays as the first save left it
        await #expect(throws: LinagerError.fileChanged) { _ = try await c.saveFile(serverID: "srv-a", path: "/etc/motd", content: "again", mtime: f.mtime) }
        let again = try await c.fileContent(serverID: "srv-a", path: "/etc/motd")
        #expect(again.content == "changed\n" && again.mtime == m2)
        withExtendedLifetime(server) {}
    }

    @Test func whatTheEditorCannotOpenIsToldApart() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session)
        await #expect(throws: LinagerError.notText) { _ = try await c.fileContent(serverID: "srv-a", path: "/bin.dat") }
        await #expect(throws: LinagerError.tooLarge) { _ = try await c.fileContent(serverID: "srv-a", path: "/huge.log") }
        await #expect(throws: LinagerError.notFound) { _ = try await c.fileContent(serverID: "srv-a", path: "/gone") }
        await #expect(throws: LinagerError.accessDenied) { _ = try await c.fileContent(serverID: "srv-a", path: "/denied") }
        withExtendedLifetime(server) {}
    }

    @Test func createDeleteRenameAndTransferReachTheServer() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session)
        try await c.createFile(serverID: "srv-a", path: "/tmp/new file.txt")
        try await c.createFolder(serverID: "srv-a", path: "/tmp/dir")
        try await c.renameEntry(serverID: "srv-a", path: "/tmp/dir", name: "dir2")
        try await c.transferEntry(serverID: "srv-a", path: "/tmp/new file.txt", to: "/home", move: false, mode: .keep)
        try await c.transferEntry(serverID: "srv-a", path: "/tmp/dir2", to: "/home", move: true, mode: .overwrite)
        try await c.deleteEntry(serverID: "srv-a", path: "/tmp/new file.txt")
        let log = try await ops(server)
        #expect(log.map { $0[0] as? String } == ["create", "mkdir", "rename", "copy", "move", "delete"])
        let rename = try #require(log[2][1] as? [String: Any])
        #expect(rename["path"] as? String == "/tmp/dir" && rename["name"] as? String == "dir2")
        // overwriting is confirmed, keeping both is not; a delete is always confirmed
        #expect((log[3][1] as? [String: Any])?["confirm"] as? Bool == false && (log[3][1] as? [String: Any])?["mode"] as? String == "keep")
        #expect((log[4][1] as? [String: Any])?["confirm"] as? Bool == true)
        #expect((log[5][1] as? [String: Any])?["confirm"] as? Bool == true)
        withExtendedLifetime(server) {}
    }

    @Test func aNameThatIsTakenIsToldApart() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session)
        await #expect(throws: LinagerError.exists) { try await c.createFile(serverID: "srv-a", path: "/tmp/exists") }
        await #expect(throws: LinagerError.exists) { try await c.createFolder(serverID: "srv-a", path: "/tmp/exists") }
        await #expect(throws: LinagerError.exists) { try await c.transferEntry(serverID: "srv-a", path: "/tmp/a", to: "/taken", move: false, mode: .fail) }
        try await c.transferEntry(serverID: "srv-a", path: "/tmp/a", to: "/taken", move: false, mode: .keep)
        withExtendedLifetime(server) {}
    }

    @Test func permissionsAndOwnerAreSetAndRead() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session)
        let st = try await c.stat(serverID: "srv-a", path: "/suid")
        #expect(st.mode == "4755" && FileMode(octal: st.mode)?.has(.setuid) == true && st.owner == "root" && st.group == "adm" && !st.protected)
        #expect(try await c.stat(serverID: "srv-a", path: "/etc").protected)
        try await c.setPermissions(serverID: "srv-a", path: "/tmp/x", mode: "0755", recursive: true)
        try await c.setOwner(serverID: "srv-a", path: "/tmp/x", owner: "anna", group: "adm", recursive: false)
        let log = try await ops(server)
        #expect(log.map { $0[0] as? String } == ["chmod", "chown"])
        let chmod = try #require(log[0][1] as? [String: Any])
        #expect(chmod["mode"] as? String == "0755" && chmod["recursive"] as? Bool == true)
        let chown = try #require(log[1][1] as? [String: Any])
        #expect(chown["owner"] as? String == "anna" && chown["group"] as? String == "adm" && chown["recursive"] as? Bool == false)
        withExtendedLifetime(server) {}
    }

    @Test func aFileIsUploadedWholeToAPathWithOddSigns() async throws {
        let server = try FakeServer()
        let c = try await server.client(credentials: session)
        let file = FileManager.default.temporaryDirectory.appendingPathComponent("up-\(UUID().uuidString).bin")
        try Data(repeating: 7, count: 300_000).write(to: file)
        defer { try? FileManager.default.removeItem(at: file) }
        let seen = LockedValue(0.0)
        try await c.uploadFile(serverID: "srv-a", to: "/tmp/a b&c.bin", from: file) { seen.set($0) }
        let log = try await ops(server)
        #expect(log.count == 1 && log[0][0] as? String == "upload" && log[0][1] as? String == "/tmp/a b&c.bin" && log[0][2] as? Int == 300_000)
        #expect(seen.get() > 0)
        await #expect(throws: LinagerError.exists) { try await c.uploadFile(serverID: "srv-a", to: "/tmp/exists", from: file) { _ in } }
        withExtendedLifetime(server) {}
    }
}

@Suite("Permission bits")
struct FileModeTests {
    @Test func bitsAreReadAndSetPerPerson() {
        var m = FileMode(octal: "0644")!
        #expect(m.has(.read, for: .owner) && m.has(.write, for: .owner) && !m.has(.execute, for: .owner))
        #expect(m.has(.read, for: .others) && !m.has(.write, for: .others))
        m.set(.execute, for: .owner, true)
        m.set(.write, for: .group, true)
        m.set(.read, for: .others, false)
        #expect(m.octal == "0760" && m.symbolic == "rwxrw----")
    }

    @Test func specialBitsShowAsLsShowsThem() {
        #expect(FileMode(octal: "4755")!.symbolic == "rwsr-xr-x")
        #expect(FileMode(octal: "4644")!.symbolic == "rwSr--r--")
        #expect(FileMode(octal: "2755")!.symbolic == "rwxr-sr-x")
        #expect(FileMode(octal: "1777")!.symbolic == "rwxrwxrwt")
        #expect(FileMode(octal: "1776")!.symbolic == "rwxrwxrwT")
        #expect(FileMode(octal: "4755")!.isDangerous && !FileMode(octal: "0755")!.isDangerous)
    }

    @Test func octalIsParsedStrictly() {
        #expect(FileMode(octal: "755")?.bits == 0o755 && FileMode(octal: "0755")?.octal == "0755")
        #expect(FileMode(octal: "9") == nil && FileMode(octal: "") == nil && FileMode(octal: "17777") == nil && FileMode(octal: "-1") == nil)
        var m = FileMode(bits: 0o755)
        m.set(.setgid, true)
        #expect(m.octal == "2755")
        m.set(.setgid, false)
        #expect(m.octal == "0755")
    }
}

@Suite("Files and notes of a server page")
struct ServerFilesAndNotesTests {
    private let list = FileList(path: "/", entries: [
        FileEntry(name: ".env", kind: .file, size: 300, mtime: 50),
        FileEntry(name: "big.log", kind: .file, size: 9000, mtime: 10),
        FileEntry(name: "small.txt", kind: .file, size: 5, mtime: 90),
        FileEntry(name: "www", kind: .dir, mtime: 5),
        FileEntry(name: "Apps", kind: .dir, mtime: 80),
    ])

    @Test func foldersAreFirstInEveryOrder() {
        #expect(list.arranged(sort: .name, showHidden: false).map(\.name) == ["Apps", "www", "big.log", "small.txt"])
        #expect(list.arranged(sort: .date, showHidden: false).map(\.name) == ["Apps", "www", "small.txt", "big.log"]) // the newest first
        #expect(list.arranged(sort: .size, showHidden: false).map(\.name) == ["Apps", "www", "big.log", "small.txt"]) // the biggest first
    }

    @Test func hiddenAndSearch() {
        #expect(list.arranged(sort: .name, showHidden: true).map(\.name).contains(".env"))
        #expect(!list.arranged(sort: .name, showHidden: false).map(\.name).contains(".env"))
        #expect(list.hiddenCount(showHidden: false) == 1 && list.hiddenCount(showHidden: true) == 0)
        #expect(list.arranged(sort: .name, showHidden: false, query: "LOG").map(\.name) == ["big.log"])
        #expect(list.arranged(sort: .name, showHidden: false, query: "  ").count == 4) // blanks are no search
    }

    @Test func theWayToAFolder() {
        #expect(FilePath.chain("/var/www") == ["/var", "/var/www"])
        #expect(FilePath.chain("/").isEmpty)
    }

    @Test func aNoteIsSplitIntoBlocks() {
        let b = Markdown.parse("# Title\nline one\nline two\n\n- a\n  - b\n* c\n1. first\n2. second\n> quoted\n> more\n---\n```\ncode here\n  indented\n```\nend")
        #expect(b == [
            .heading(level: 1, text: "Title"), .paragraph("line one\nline two"),
            .bullet(depth: 0, text: "a"), .bullet(depth: 1, text: "b"), .bullet(depth: 0, text: "c"),
            .numbered(number: 1, text: "first"), .numbered(number: 2, text: "second"),
            .quote("quoted\nmore"), .rule, .code("code here\n  indented"), .paragraph("end"),
        ])
    }

    @Test func oddNotes() {
        #expect(Markdown.parse("").isEmpty)
        #expect(Markdown.parse("**Сервер N8n**\nУведомления в телеграм") == [.paragraph("**Сервер N8n**\nУведомления в телеграм")])
        // an unclosed fence keeps what was written; "#hash" is not a heading; a dash line without a space is text
        #expect(Markdown.parse("```\nx") == [.code("x")])
        #expect(Markdown.parse("#hash") == [.paragraph("#hash")])
        #expect(Markdown.parse("-x") == [.paragraph("-x")])
        #expect(Markdown.parse("a\r\nb") == [.paragraph("a\nb")])
    }
}

/// A value written from the thread of a progress callback and read by the test.
private final class LockedValue<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: T
    init(_ v: T) { value = v }
    func set(_ v: T) { lock.lock(); value = v; lock.unlock() }
    func get() -> T { lock.lock(); defer { lock.unlock() }; return value }
}
