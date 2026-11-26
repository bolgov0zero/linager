import Foundation
import Testing
@testable import LinagerKit

private let mark = "\u{1E}"

private func line(_ json: String) -> String { mark + json + "\n" }

@Suite("Events of a job of the updates")
struct AptStreamTests {
    private let sample = "Reading package lists...\n"
        + line(#"{"t":"queue","pkgs":[{"name":"openssl","current":"3.0.1","candidate":"3.0.2","security":true},{"name":"vim"}]}"#)
        + "Get:1 http://deb.example/ openssl 3.0.2\n"
        + line(#"{"t":"progress","stage":"download","pct":40,"cur":1,"total":2}"#)
        + line(#"{"t":"pkg","name":"openssl","state":"running"}"#)
        + line(#"{"t":"pkg","name":"openssl","state":"done"}"#)
        + line(#"{"t":"pkg","name":"vim","state":"error","msg":"dpkg failed"}"#)
        + "E: Sub-process /usr/bin/dpkg returned an error code (1)\n"

    @Test func eventsAreCutOutOfTheText() {
        var p = AptStreamParser()
        let (text, events) = p.feed(sample)
        #expect(text == "Reading package lists...\nGet:1 http://deb.example/ openssl 3.0.2\nE: Sub-process /usr/bin/dpkg returned an error code (1)\n")
        #expect(events.count == 5)
        #expect(events[0] == .queue([AptPackage(name: "openssl", current: "3.0.1", candidate: "3.0.2", security: true), AptPackage(name: "vim")]))
        #expect(events[1] == .progress(stage: "download", pct: 40, current: 1, total: 2))
        #expect(events[4] == .package(name: "vim", state: .error, message: "dpkg failed"))
    }

    @Test func theSameEventsComeWhereverTheStreamIsCut() {
        var whole = AptStreamParser()
        let (wholeText, wholeEvents) = whole.feed(sample)
        let chars = Array(sample)
        for size in [1, 2, 3, 5, 7, 11, 17, 40] {
            var p = AptStreamParser()
            var text = ""
            var events: [AptStreamParser.Event] = []
            var i = 0
            while i < chars.count {
                let (t, e) = p.feed(String(chars[i..<min(i + size, chars.count)]))
                text += t
                events += e
                i += size
            }
            #expect(text == wholeText && events == wholeEvents, "pieces of \(size)")
        }
    }

    @Test func aDamagedOrUnknownEventLineIsDropped() {
        var p = AptStreamParser()
        let (text, events) = p.feed("a\n" + line("{not json") + line(#"{"t":"nothing"}"#) + line(#"{"t":"pkg","name":"x","state":"weird"}"#) + "b\n")
        #expect(text == "a\nb\n" && events.isEmpty)
    }

    @Test func theRunKeepsAStateOnceItIsFinal() {
        var run = AptRun()
        var p = AptStreamParser()
        for e in p.feed(sample).events { run.apply(e) }
        #expect(run.order == ["openssl", "vim"])
        #expect(run.state("openssl") == .done && run.state("vim") == .error)
        #expect(run.messages["vim"] == "dpkg failed" && run.info["openssl"]?.security == true)
        #expect(run.stage == "download" && run.pct == 40 && run.overall == 0.4)
        var unknown = AptRun()
        unknown.apply(.progress(stage: "install", pct: -1, current: 0, total: 3))
        #expect(unknown.overall == nil && unknown.stage == "install")
        #expect(run.doneCount == 1 && run.failedCount == 1 && run.hasFailure && run.total == 2)
        // apt reports a package several times: what is done stays done
        run.apply(.package(name: "openssl", state: .running, message: nil))
        #expect(run.state("openssl") == .done)
    }

    @Test func aPackageThatAptAddsOnTheFlyIsListed() {
        var run = AptRun()
        run.apply(.queue([AptPackage(name: "a")]))
        run.apply(.package(name: "new-dependency", state: .running, message: nil))
        #expect(run.order == ["a", "new-dependency"] && run.state("new-dependency") == .running && run.state("a") == .queued)
        #expect(run.info["new-dependency"]?.name == "new-dependency")
    }

    @Test func aFailureOfAJobWithNoPackagesIsKnownFromTheText() {
        var run = AptRun()
        #expect(!run.hasFailure)
        run.note(text: "Fetched 1 kB\nE: Failed to fetch http://x  404\n")
        #expect(run.hasFailure && run.lastError == "Failed to fetch http://x  404")
    }
}

@Suite("Installation of Windows Update")
struct WinUpdateStreamTests {
    private let log = "Found 2 update(s).\r\n  2024-05 Cumulative Update (KB5037771)\r\n  Defender definition update\r\nDownloading...\r\n"
        + "Download finished (code 2).\r\nInstalling...\r\n"
        + "  2024-05 Cumulative Update (KB5037771) -> result 2 (0)\r\n  Defender definition update -> result 4 (80240022)\r\n"
        + "Installation finished (code 3).\r\nA restart is required.\r\n"

    private func run(_ pieces: [String]) -> (AptRun, [AptStreamParser.Event]) {
        var p = WinUpdateStreamParser()
        var r = AptRun()
        var all: [AptStreamParser.Event] = []
        for piece in pieces { for e in p.feed(piece) { r.apply(e); all.append(e) } }
        return (r, all)
    }

    @Test func theUpdatesAreQueuedWorkedOnAndSettledOneByOne() {
        let (r, events) = run([log])
        #expect(r.order == ["2024-05 Cumulative Update (KB5037771)", "Defender definition update"])
        #expect(r.state("2024-05 Cumulative Update (KB5037771)") == .done)
        #expect(r.state("Defender definition update") == .error && r.messages["Defender definition update"] == "4 (80240022)")
        #expect(r.stage == "install" && r.overall == nil && r.hasFailure)
        // the order of the events is what a page needs: queue, queue, download, running, running, install, done, error
        #expect(events.first == .queue([AptPackage(name: "2024-05 Cumulative Update (KB5037771)")]))
        #expect(events.contains(.progress(stage: "download", pct: -1, current: 0, total: 2)))
    }

    @Test func theSameResultWhereverTheTextIsCut() {
        let (whole, _) = run([log])
        let chars = Array(log)
        for size in [1, 3, 7, 19, 50] {
            var pieces: [String] = []
            var i = 0
            while i < chars.count { pieces.append(String(chars[i..<min(i + size, chars.count)])); i += size }
            #expect(run(pieces).0 == whole, "pieces of \(size)")
        }
    }

    @Test func aResultOfSucceededWithErrorsCountsAsDone() {
        let (r, _) = run(["Found 1 update(s).\n  U\nDownloading...\nInstalling...\n  U -> result 3 (0)\n"])
        #expect(r.state("U") == .done && !r.hasFailure)
    }

    @Test func nothingDownloadedIsAFailureOfTheJob() {
        #expect(WinUpdateStreamParser.failure(in: "Found 1 update(s).\nNothing was downloaded.\n") == "Nothing was downloaded.")
        #expect(WinUpdateStreamParser.failure(in: "Nothing to install.\n") == nil)
    }
}
