import Foundation
import Testing
@testable import LinagerKit

private let session = Credentials(token: "tok123", csrf: "csrf-abc")

@Suite("Management pages: logic")
struct ServerManageModelTests {
    @Test func theServicesThatNeedALookComeFirst() {
        let list = [
            ServiceUnit(name: "b.service", active: "active"), ServiceUnit(name: "a.service", active: "inactive"),
            ServiceUnit(name: "z.service", active: "failed"), ServiceUnit(name: "c.service", active: "active"),
        ]
        #expect(ServiceList.arranged(list).map(\.name) == ["z.service", "b.service", "c.service", "a.service"])
        #expect(ServiceList.arranged(list, only: .running).map(\.name) == ["b.service", "c.service"])
        #expect(ServiceList.arranged(list, only: .failed).map(\.name) == ["z.service"])
        #expect(ServiceList.arranged(list, only: .stopped).map(\.name) == ["a.service"])
        #expect(ServiceList.arranged(list, query: "  C.SERV ").map(\.name) == ["c.service"])
        #expect(ServiceUnit(name: "nginx.service").shortName == "nginx" && ServiceUnit(name: "x").shortName == "x")
        // the searches look in the description too
        #expect(ServiceList.arranged([ServiceUnit(name: "x.service", description: "A web server")], query: "web").count == 1)
    }

    @Test func numbersInTheNamesOfServicesAreSortedAsAPersonReads() {
        let list = [ServiceUnit(name: "getty@tty10.service", active: "active"), ServiceUnit(name: "getty@tty2.service", active: "active")]
        #expect(ServiceList.arranged(list).map(\.name) == ["getty@tty2.service", "getty@tty10.service"])
    }

    @Test func theProgramOfAWindowsProcessIsItsExe() {
        #expect(ProcessItem(pid: 4, cmd: #"C:\Windows\System32\svchost.exe -k netsvcs -p"#).program == "svchost.exe")
        #expect(ProcessItem(pid: 4, cmd: #""C:\Program Files\Some App\app.exe" /background"#).program == "app.exe")
        #expect(ProcessItem(pid: 4, cmd: #"C:\Program Files\Some App\APP.EXE /x"#).program == "APP.EXE")
        #expect(ProcessItem(pid: 4, cmd: "System").program == "System")
    }

    @Test func theProgramIsTheNameWithoutThePathAndTheArguments() {
        #expect(ProcessItem(pid: 1, cmd: "/usr/sbin/nginx -g daemon off;").program == "nginx")
        #expect(ProcessItem(pid: 1, cmd: "bash").program == "bash")
        #expect(ProcessItem(pid: 1, cmd: "").program == "")
    }

    @Test func theAnswersOfTheAgentAreReadWithWhatIsMissingTolerated() throws {
        let d = JSONDecoder()
        let svc = try d.decode(ServiceUnit.self, from: Data(#"{"name":"x.service"}"#.utf8))
        #expect(svc.active == "" && !svc.isRunning && !svc.isFailed)
        let procs = try d.decode(ProcessList.self, from: Data(#"{"processes":[{"pid":5,"cmd":"a","mem_pct":1.5}],"total":9}"#.utf8))
        #expect(procs.processes.first?.memPct == 1.5 && procs.total == 9)
        #expect(try d.decode(ProcessList.self, from: Data("{}".utf8)).processes.isEmpty)
        let apt = try d.decode(AptList.self, from: Data(#"{"packages":[{"name":"a","security":true},{"name":"b","ignored":true}],"reboot_required":true}"#.utf8))
        #expect(apt.counted.map(\.name) == ["a"] && apt.securityCount == 1 && apt.rebootRequired && apt.lastCheck == 0)
        let dock = try d.decode(DockerInfo.self, from: Data(#"{"containers":[{"id":"x","state":"running","upd":"new"},{"id":"y","upd":"new","ignored":true}]}"#.utf8))
        #expect(dock.available && dock.containers[0].isRunning && dock.containers[0].hasNewImage && !dock.containers[1].hasNewImage && dock.containers[0].name == "x")
        #expect(try d.decode(DockerInfo.self, from: Data(#"{"available":false}"#.utf8)).available == false)
        let image = try d.decode(DockerImage.self, from: Data(#"{"id":"sha256:abcdefabcdefabcdef"}"#.utf8))
        #expect(image.title == "sha256:abcde")
        let disk = try d.decode(DiskState.self, from: Data(#"{"filesystems":[{"mount":"/","pct":41,"days_left":12}],"cleanup":[{"kind":"journal","frees":5}]}"#.utf8))
        #expect(disk.filesystems[0].daysLeft == 12 && disk.cleanup[0].frees == 5)
    }

    @Test func aStreamAsksForNothingTheServerWouldRefuse() {
        func q(_ s: ServerStream) -> [String: String]? { s.query.map { Dictionary(uniqueKeysWithValues: $0.map { ($0.name, $0.value ?? "") }) } }
        #expect(q(.logs(unit: "nginx.service", priority: 4, kernel: true, lines: 200)) == ["kind": "logs", "lines": "200", "priority": "4", "unit": "nginx.service", "kernel": "1"])
        #expect(q(.logs(unit: "", priority: 99, kernel: false, lines: 0)) == ["kind": "logs", "lines": "1", "priority": "7"])
        #expect(q(.logs(unit: "a b", priority: 6, kernel: false, lines: 10)) == nil)
        #expect(q(.logs(unit: "x;rm -rf", priority: 6, kernel: false, lines: 10)) == nil)
        #expect(q(.apt(.update)) == ["kind": "apt", "mode": "update"])
        #expect(q(.apt(.security)) == ["kind": "apt", "mode": "security"])
        #expect(q(.apt(.packages(["openssl", "libc6:amd64"]))) == ["kind": "apt", "mode": "packages", "names": "openssl,libc6:amd64"])
        #expect(q(.apt(.packages([]))) == nil)
        #expect(q(.apt(.packages(["ok", "bad name"]))) == nil)
    }

    @Test func aWindowsUpdateStreamAsksForGuidsOnly() {
        func q(_ s: ServerStream) -> [String: String]? { s.query.map { Dictionary(uniqueKeysWithValues: $0.map { ($0.name, $0.value ?? "") }) } }
        let a = "11111111-1111-1111-1111-111111111111", b = "22222222-2222-2222-2222-222222222222"
        #expect(q(.winUpdate(ids: [])) == ["kind": "winupdate"])
        #expect(q(.winUpdate(ids: [a, b])) == ["kind": "winupdate", "ids": "\(a),\(b)"])
        #expect(q(.winUpdate(ids: ["not-a-guid"])) == nil)
        #expect(q(.winUpdate(ids: [a + ",x"])) == nil)
    }

    @Test func theSessionTellsWhichModulesItMayUse() {
        #expect(SessionInfo(csrf: "c", login: "a", role: "admin").can("docker"))
        #expect(SessionInfo(csrf: "c", login: "a", role: "operator", modules: ["docker"]).can("docker"))
        #expect(!SessionInfo(csrf: "c", login: "a", role: "operator", modules: ["docker"]).can("disk"))
        #expect(!SessionInfo(csrf: "c", login: "a", role: "auditor", modules: ["docker"]).can("docker"))
    }
}

@Suite("Management pages over a real connection", .serialized)
struct ServerManageConnectionTests {
    private func ops(_ server: FakeServer) async throws -> [[Any]] {
        let session = URLSession(configuration: .ephemeral, delegate: PinDelegate(mode: .pinned(server.fingerprint)), delegateQueue: nil)
        let (data, _) = try await session.data(from: try #require(server.endpoint.url(path: "/api/test/ops")))
        return try #require(try JSONSerialization.jsonObject(with: data) as? [[Any]])
    }

    @Test func theListsComeFromTheServer() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        let services = try await c.services(serverID: "srv-a")
        #expect(services.count == 4 && services.first { $0.isFailed }?.name == "cron.service")
        let byCpu = try await c.processes(serverID: "srv-a", sort: .cpu, count: 40)
        #expect(byCpu.processes.map(\.pid) == [20, 10, 30] && byCpu.total == 212)
        #expect(try await c.processes(serverID: "srv-a", sort: .mem, count: 40).processes.map(\.pid) == [10, 20, 30])
        #expect(try await c.processes(serverID: "srv-a", sort: .name, count: 40).processes.map(\.pid) == [10, 30, 20])
        let apt = try await c.apt(serverID: "srv-a")
        #expect(apt.counted.count == 2 && apt.securityCount == 1 && apt.rebootRequired && apt.lastCheck == 1_700_000_000)
        let docker = try await c.docker(serverID: "srv-a")
        #expect(docker.containers.map(\.name) == ["web", "db"] && docker.containers[0].hasNewImage && docker.images.count == 2)
        #expect(try await c.docker(serverID: "nodocker").available == false)
        let disk = try await c.disk(serverID: "srv-a")
        #expect(disk.filesystems.first?.pct == 41 && disk.cleanup.map(\.kind) == ["journal", "apt"])
        withExtendedLifetime(server) {}
    }

    @Test func actionsReachTheServerAndADangerousOneIsAskedAbout() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        try await c.serviceAction(serverID: "srv-a", unit: "nginx.service", action: .restart, confirm: false)
        // stopping the service the phone itself may be talking through is dangerous: the server wants it confirmed
        await #expect(throws: LinagerError.confirmRequired) { try await c.serviceAction(serverID: "srv-a", unit: "sshd.service", action: .stop, confirm: false) }
        try await c.serviceAction(serverID: "srv-a", unit: "sshd.service", action: .stop, confirm: true)
        try await c.killProcess(serverID: "srv-a", pid: 20, signal: "KILL")
        try await c.killProcess(serverID: "srv-a", pid: 21, signal: "anything")
        await #expect(throws: LinagerError.self) { try await c.killProcess(serverID: "srv-a", pid: 1, signal: "TERM") }
        try await c.dockerAction(serverID: "srv-a", containerID: "abc123", action: .restart)
        try await c.dockerAction(serverID: "srv-a", containerID: "abc123", action: .remove)
        let cleaned = try await c.diskClean(serverID: "srv-a", kind: "journal")
        #expect(cleaned.freed == 700_000_000 && cleaned.output == "done")
        let log = try await ops(server)
        #expect(log.map { $0[0] as? String } == ["service", "service", "kill", "kill", "docker", "docker", "clean"])
        #expect(log[0][1] as? String == "nginx.service" && log[0][2] as? String == "restart" && log[0][3] as? Bool == false)
        #expect(log[1][3] as? Bool == true)
        #expect(log[2][1] as? Int == 20 && log[2][2] as? String == "KILL" && log[3][2] as? String == "TERM")
        // a container is removed only with the confirmation, which the client gives for that action alone
        #expect(log[4][3] as? Bool == false && log[5][2] as? String == "remove" && log[5][3] as? Bool == true)
        withExtendedLifetime(server) {}
    }

    @Test func aRebootAndADeleteAreConfirmed() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        try await c.reboot(serverID: "srv-a")
        // the name has to be the name of the server, the server compares it
        await #expect(throws: LinagerError.self) { try await c.deleteServer(serverID: "srv-a", name: "other", uninstall: false) }
        try await c.deleteServer(serverID: "srv-a", name: "web-01", uninstall: true)
        let log = try await ops(server)
        #expect(log.map { $0[0] as? String } == ["reboot", "delete"])
        #expect(log[1][2] as? String == "web-01" && log[1][3] as? Bool == true)
        withExtendedLifetime(server) {}
    }

    @Test func theUpdatesOfWindowsComeFromTheServerAndTheirStreamIsOpened() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        let list = try await c.winUpdates(serverID: "srv-a")
        #expect(list.updates.count == 2 && list.securityCount == 1 && list.rebootPending)
        #expect(list.updates[0].kb == "KB5037771" && list.updates[0].needReboot && list.updates[1].downloaded)
        let stream = try await c.openStream(serverID: "srv-a", stream: .winUpdate(ids: [list.updates[1].id]))
        var it = stream.events.makeAsyncIterator()
        if case .output(let d)? = await it.next() {
            #expect(String(decoding: d, as: UTF8.self) == "stream kind=winupdate unit= priority= kernel= lines= mode= names= ids=22222222-2222-2222-2222-222222222222\n")
        } else { Issue.record("no greeting of the stream") }
        stream.close()
        withExtendedLifetime(server) {}
    }

    @Test func theJournalAndTheJobsOfTheUpdatesAreStreams() async throws {
        let server = try FakeServer()
        let c = server.client(credentials: session)
        let logs = try await c.openStream(serverID: "srv-a", stream: .logs(unit: "nginx.service", priority: 4, kernel: true, lines: 100))
        var it = logs.events.makeAsyncIterator()
        if case .output(let d)? = await it.next() {
            #expect(String(decoding: d, as: UTF8.self) == "stream kind=logs unit=nginx.service priority=4 kernel=1 lines=100 mode= names= ids=\n")
        } else { Issue.record("no greeting of the journal") }
        logs.close()
        let apt = try await c.openStream(serverID: "srv-a", stream: .apt(.packages(["openssl", "vim"])))
        var it2 = apt.events.makeAsyncIterator()
        if case .output(let d)? = await it2.next() {
            #expect(String(decoding: d, as: UTF8.self) == "stream kind=apt unit= priority= kernel= lines= mode=packages names=openssl,vim ids=\n")
        } else { Issue.record("no greeting of the job") }
        apt.close()
        await #expect(throws: LinagerError.self) { _ = try await c.openStream(serverID: "srv-a", stream: .logs(unit: "bad name", priority: 6, kernel: false, lines: 10)) }
        withExtendedLifetime(server) {}
    }
}
