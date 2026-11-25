import Foundation
import Testing
@testable import LinagerKit

func fixture(_ name: String) throws -> Data {
    guard let url = Bundle.module.url(forResource: name, withExtension: nil, subdirectory: "Fixtures") else { throw CocoaError(.fileNoSuchFile) }
    return try Data(contentsOf: url)
}

struct Reply: Decodable { let items: [ServerCard] }

@Suite("Models")
struct ModelsTests {
    @Test func dashboardDecodes() throws {
        let cards = try JSONDecoder().decode(Reply.self, from: fixture("dashboard.json")).items
        #expect(cards.count == 4)
        let a = cards[0]
        #expect(a.name == "web-01")
        #expect(a.folderId == 2)
        #expect(a.tags.map(\.name) == ["web", "prod"])
        #expect(a.sample?.cpu == 41.5)
        #expect(a.sample?.updates == 3)
        #expect(a.sample?.imageUpdates == -1)
        #expect(a.spark == [10, 20, 41.5])
        // nine fraction digits, as Go writes them
        #expect(a.lastSeenAt != nil)
        #expect(abs((a.lastSeenAt ?? .distantPast).timeIntervalSince1970 - 1_795_601_730.123) < 1)
    }

    @Test func missingAndNullFieldsGetNeutralValues() throws {
        let cards = try JSONDecoder().decode(Reply.self, from: fixture("dashboard.json")).items
        let b = cards[1]
        #expect(b.tags.isEmpty) // tag_items: null
        #expect(b.sample?.securityUpdates == 0) // a field the server did not send
        let c = cards[2]
        #expect(c.sample == nil)
        #expect(c.folderId == nil)
        #expect(c.online == false)
        let d = cards[3]
        #expect(d.lastSeenAt == nil)
    }

    @Test func foldersDecode() throws {
        let list = try JSONDecoder().decode([Folder].self, from: fixture("groups.json"))
        #expect(list.count == 4)
        #expect(list[2].parentId == 2)
        #expect(list[0].parentId == nil)
    }

    @Test func sessionDecodes() throws {
        let s = try JSONDecoder().decode(SessionInfo.self, from: Data(#"{"csrf":"x","login":"admin","id":1,"role":"operator","modules":["files"],"totp_enabled":true,"recovery_left":3}"#.utf8))
        #expect(s.login == "admin")
        #expect(s.role == "operator")
        #expect(s.totpEnabled)
        let old = try JSONDecoder().decode(SessionInfo.self, from: Data(#"{"csrf":"x","login":"a"}"#.utf8))
        #expect(old.role == "admin")
    }

    @Test func cardSurvivesTheCache() throws {
        let cards = try JSONDecoder().decode(Reply.self, from: fixture("dashboard.json")).items
        let data = try JSONEncoder().encode(cards)
        let back = try JSONDecoder().decode([ServerCard].self, from: data)
        #expect(back.map(\.id) == cards.map(\.id))
        #expect(back[0].sample == cards[0].sample)
        #expect(back[0].tags == cards[0].tags)
        #expect(abs((back[0].lastSeenAt ?? .distantPast).timeIntervalSince(cards[0].lastSeenAt ?? .distantFuture)) < 0.01)
    }
}
