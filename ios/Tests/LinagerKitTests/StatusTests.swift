import Testing
@testable import LinagerKit

private func card(_ s: Sample?, online: Bool = true, id: String = "x") -> ServerCard {
    ServerCard(id: id, name: id, online: online, sample: s)
}

@Suite("Status")
struct StatusTests {
    @Test func levelThresholdsAreTheOnesOfTheDesktopApp() {
        #expect(levelOf(card(Sample(cpu: 10))) == .ok)
        #expect(levelOf(card(Sample(cpu: 75))) == .ok)
        #expect(levelOf(card(Sample(cpu: 75.1))) == .warn)
        #expect(levelOf(card(Sample(cpu: 90))) == .warn)
        #expect(levelOf(card(Sample(cpu: 90.1))) == .crit)
        #expect(levelOf(card(Sample(memPct: 85))) == .ok)
        #expect(levelOf(card(Sample(memPct: 86))) == .warn)
        #expect(levelOf(card(Sample(memPct: 92.5))) == .crit)
        #expect(levelOf(card(Sample(diskPct: 80))) == .ok)
        #expect(levelOf(card(Sample(diskPct: 81))) == .warn)
        #expect(levelOf(card(Sample(diskPct: 91))) == .crit)
        #expect(levelOf(card(Sample(failedServices: 1))) == .warn)
        #expect(levelOf(card(Sample(rebootRequired: true))) == .warn)
        #expect(levelOf(card(Sample(updates: 9))) == .ok)
        #expect(levelOf(card(Sample(updates: 10))) == .warn)
        #expect(levelOf(card(Sample(updates: -1))) == .ok) // unknown is not a problem
        #expect(levelOf(card(nil)) == .ok)
        #expect(levelOf(card(Sample(cpu: 99), online: false)) == .off)
    }

    @Test func meterColours() {
        #expect(meter(74.9) == .ok)
        #expect(meter(75) == .warn)
        #expect(meter(89.9) == .warn)
        #expect(meter(90) == .crit)
    }

    @Test func chipsFollowTheOrderAndTonesOfTheDesktopApp() {
        let c = card(Sample(failedServices: 2, rebootRequired: true, updates: 12, imageUpdates: 6))
        let got = chips(for: c, agent: .available)
        #expect(got.map(\.id) == [.failed, .reboot, .updates, .images, .agentUpdate])
        #expect(got.map(\.tone) == [.crit, .warn, .warn, .warn, .warn])
        #expect(got[0].n == 2)
        #expect(got[2].n == 12)
    }

    @Test func smallCountsAreInfoAndUnknownCountsAreHidden() {
        let few = chips(for: card(Sample(updates: 3, imageUpdates: 4)))
        #expect(few.map(\.tone) == [.info, .info])
        #expect(chips(for: card(Sample(updates: -1, imageUpdates: -1))).isEmpty)
        #expect(chips(for: card(nil)).isEmpty)
        #expect(chips(for: card(Sample()), agent: .unsupported).map(\.id) == [.agentOld])
        #expect(chips(for: card(Sample()), agent: .unsupported).first?.tone == .crit)
    }

    @Test func offlineServerShowsOnlyItsStateAndPackages() {
        let off = chips(for: card(Sample(failedServices: 3, rebootRequired: true, updates: 5), online: false))
        #expect(off.map(\.id) == [.offline, .updates])
        #expect(off.allSatisfy { $0.tone == .mute })
        #expect(chips(for: card(nil, online: false)).map(\.id) == [.offline])
    }

    @Test func extraChipsFoldIntoPlusN() {
        let all = (0..<6).map { Chip(.updates, n: $0, tone: .info) }
        let v = visibleChips(all, max: 4)
        #expect(v.shown.count == 4)
        #expect(v.rest.count == 2)
        #expect(visibleChips(Array(all.prefix(3)), max: 4).rest.isEmpty)
    }

    @Test func totals() {
        let cards = [
            card(Sample(cpu: 40, updates: 3), id: "a"),
            card(Sample(cpu: 95, rebootRequired: true, updates: 12), id: "b"),
            card(Sample(cpu: 80), online: false, id: "c"),
            card(Sample(cpu: 20, updates: -1), id: "d"),
        ]
        let t = summarize(cards)
        #expect(t.total == 4)
        #expect(t.online == 3)
        #expect(t.offline == 1)
        #expect(t.problems == 2) // b is critical, c is offline
        #expect(t.critical == 1)
        #expect(t.updates == 15)
        #expect(t.rebootPending == 1)
        #expect(abs((t.cpuAvg ?? 0) - 51.666) < 0.01) // offline servers do not count
        #expect(summarize([]).cpuAvg == nil)
    }

    @Test func osNames() {
        #expect(shortOs("Debian GNU/Linux 12 (bookworm)") == "Debian 12")
        #expect(shortOs("Microsoft Windows 11 Pro") == "Windows 11 Pro")
        #expect(shortOs("Ubuntu 24.04.1 LTS") == "Ubuntu 24.04.1 LTS")
        #expect(shortOs("") == "")
        #expect(isWindows("Windows 11 Pro"))
        #expect(!isWindows("Debian 12"))
    }

    @Test func tagBadges() {
        #expect(tagLetter("prod") == "P")
        #expect(tagLetter("  база") == "Б")
        #expect(tagLetter("") == "?")
        #expect(TagPalette.rgb("blue") == 0x3B6FE6)
        #expect(TagPalette.rgb("no-such-colour") == TagPalette.rgb("gray"))
    }
}
