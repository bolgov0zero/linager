import CryptoKit
import Foundation
import Testing
@testable import LinagerKit

private struct Expected: Decodable {
    struct Ref: Decodable { let id: String; let name: String }
    struct Entry: Decodable {
        let id: String, name: String, type: String, login: String, domain: String, password: String, note: String
        let groups: [Ref], servers: [Ref]
    }
    let passphrase: String
    let normalized: String
    let entries: [Entry]
}

private func expected() throws -> Expected { try JSONDecoder().decode(Expected.self, from: fixture("vault-expected.json")) }

/// A vault made here (a test must be able to make a damaged content that only a valid file carries): the same layout as the desktop's.
private func makeVault(json: String, passphrase: String = "a test passphrase", m: Int = 8192, t: Int = 1, p: Int = 1) -> Data {
    let salt = [UInt8](repeating: 3, count: 16), nonce = [UInt8](repeating: 5, count: 12)
    var head: [UInt8] = [0x4c, 0x47, 0x52, 0x4b, 1, 1]
    for v in [m, t, p] { head += [UInt8(v >> 24 & 0xff), UInt8(v >> 16 & 0xff), UInt8(v >> 8 & 0xff), UInt8(v & 0xff)] }
    head += salt + nonce
    let key = Argon2id.hash(password: Array(passphrase.precomposedStringWithCompatibilityMapping.utf8), salt: salt, iterations: t, memoryKiB: m, parallelism: p)!
    let box = try! AES.GCM.seal(Data(json.utf8), using: SymmetricKey(data: key), nonce: AES.GCM.Nonce(data: nonce), authenticating: Data(head))
    return Data(head) + box.ciphertext + box.tag
}

@Suite("Vault of the keys")
struct VaultTests {
    @Test func aFileMadeByTheDesktopApplicationIsOpened() throws {
        let e = try expected()
        let vault = try VaultFile.open(fixture("sample-small.lvault"), passphrase: e.passphrase)
        #expect(vault.entries.count == e.entries.count)
        for (got, want) in zip(vault.entries, e.entries) {
            #expect(got.id == want.id)
            #expect(got.name == want.name)
            #expect(got.type.rawValue == want.type)
            #expect(got.login == want.login)
            #expect(got.domain == want.domain)
            #expect(got.password == want.password)
            #expect(got.note == want.note)
            #expect(got.groups.map(\.id) == want.groups.map(\.id))
            #expect(got.servers.map(\.name) == want.servers.map(\.name))
        }
        #expect(vault.entries[0].password == "s3cret!ё") // not only ASCII
        #expect(vault.entries[0].account == "CORP\\admin")
        #expect(vault.entries[1].account == "root") // no domain
    }

    @Test func aFileWithTheDefaultCostOfTheDesktopIsOpened() throws {
        let e = try expected()
        let vault = try VaultFile.open(fixture("sample-default.lvault"), passphrase: e.passphrase)
        #expect(vault.entries.count == 5)
    }

    @Test func thePassphraseIsComparedInOneUnicodeForm() throws {
        let e = try expected()
        #expect(e.passphrase != e.normalized) // the file was made with a ligature, the second one is the plain letters
        #expect(try VaultFile.open(fixture("sample-small.lvault"), passphrase: e.normalized).entries.count == 5)
    }

    @Test func aWrongPassphraseIsAnAuthError() throws {
        #expect(throws: VaultError.auth) { _ = try VaultFile.open(try fixture("sample-small.lvault"), passphrase: "not the one") }
        #expect(throws: VaultError.auth) { _ = try VaultFile.open(try fixture("sample-small.lvault"), passphrase: "") }
    }

    @Test func everyChangedByteIsRefused() throws {
        let e = try expected()
        let good = try fixture("sample-small.lvault")
        // a byte of the header (the salt, the nonce), of the content and of the tag: none gives a damaged vault
        for at in [18, 30, 34, 45, 46, 60, good.count - 1] {
            var bad = good
            bad[at] ^= 0x01
            #expect(throws: VaultError.auth, "byte \(at)") { _ = try VaultFile.open(bad, passphrase: e.passphrase) }
        }
    }

    @Test func notAVaultFile() throws {
        let e = try expected()
        let good = try fixture("sample-small.lvault")
        #expect(throws: VaultError.format) { _ = try VaultFile.open(Data(), passphrase: e.passphrase) }
        #expect(throws: VaultError.format) { _ = try VaultFile.open(good.prefix(40), passphrase: e.passphrase) }
        var magic = good; magic[0] = 0x00
        #expect(throws: VaultError.format) { _ = try VaultFile.open(magic, passphrase: e.passphrase) }
        var version = good; version[4] = 2
        #expect(throws: VaultError.version) { _ = try VaultFile.open(version, passphrase: e.passphrase) }
        var kdf = good; kdf[5] = 2
        #expect(throws: VaultError.version) { _ = try VaultFile.open(kdf, passphrase: e.passphrase) }
        #expect(throws: VaultError.limit) { _ = try VaultFile.open(Data(count: VaultFile.maxFile + 1), passphrase: e.passphrase) }
    }

    @Test func aFileMustNotAskForTooMuchMemoryOrWork() throws {
        let e = try expected()
        let good = try fixture("sample-small.lvault")
        func with(_ offset: Int, _ value: Int) -> Data {
            var d = good
            for k in 0..<4 { d[offset + k] = UInt8(value >> (8 * (3 - k)) & 0xff) }
            return d
        }
        #expect(throws: VaultError.format) { _ = try VaultFile.open(with(6, 262_145), passphrase: e.passphrase) } // above 256 MiB
        #expect(throws: VaultError.format) { _ = try VaultFile.open(with(6, 8191), passphrase: e.passphrase) }
        #expect(throws: VaultError.format) { _ = try VaultFile.open(with(10, 0), passphrase: e.passphrase) }
        #expect(throws: VaultError.format) { _ = try VaultFile.open(with(10, 11), passphrase: e.passphrase) }
        #expect(throws: VaultError.format) { _ = try VaultFile.open(with(14, 0), passphrase: e.passphrase) }
        #expect(throws: VaultError.format) { _ = try VaultFile.open(with(14, 5), passphrase: e.passphrase) }
    }

    @Test func theContentIsCheckedLikeTheDesktopChecksIt() throws {
        func open(_ json: String) throws -> Vault { try VaultFile.open(makeVault(json: json), passphrase: "a test passphrase") }
        let ok = #"{"v":1,"entries":[{"id":"1","name":"n","type":"uac","login":"l"}]}"#
        #expect(try open(ok).entries.count == 1) // domain, password, note, bindings are optional
        #expect(try open(#"{"v":1,"entries":[]}"#).entries.isEmpty)
        #expect(throws: VaultError.version) { _ = try open(#"{"v":2,"entries":[]}"#) }
        #expect(throws: VaultError.version) { _ = try open(#"{"entries":[]}"#) }
        #expect(throws: VaultError.format) { _ = try open(#"{"v":1}"#) }
        #expect(throws: VaultError.format) { _ = try open("not json") }
        #expect(throws: VaultError.format) { _ = try open(#"{"v":1,"entries":[{"id":"1","name":"n","type":"ssh","login":"l"}]}"#) } // an unknown type
        #expect(throws: VaultError.format) { _ = try open(#"{"v":1,"entries":[{"id":"","name":"n","type":"rdp","login":"l"}]}"#) }
        #expect(throws: VaultError.format) { _ = try open(#"{"v":1,"entries":[{"id":"1","name":"n","type":"rdp"}]}"#) } // no login
        let long = String(repeating: "a", count: 81)
        #expect(throws: VaultError.format) { _ = try open(#"{"v":1,"entries":[{"id":"1","name":"\#(long)","type":"rdp","login":"l"}]}"#) }
        let dup = #"{"v":1,"entries":[{"id":"1","name":"n","type":"rdp","login":"l","servers":[{"id":"s"},{"id":"s"}]}]}"#
        #expect(throws: VaultError.format) { _ = try open(dup) } // a binding twice
        let many = (0..<501).map { #"{"id":"\#($0)","name":"n","type":"rdp","login":"l"}"# }.joined(separator: ",")
        #expect(throws: VaultError.limit) { _ = try open(#"{"v":1,"entries":[\#(many)]}"#) }
    }

    @Test func theLimitsCountLikeJavaScriptDoes() throws {
        func open(_ json: String) throws -> Vault { try VaultFile.open(makeVault(json: json), passphrase: "a test passphrase") }
        // 80 UTF-16 units: eighty letters pass, and an emoji is two units
        let eighty = String(repeating: "я", count: 80)
        #expect(try open(#"{"v":1,"entries":[{"id":"1","name":"\#(eighty)","type":"rdp","login":"l"}]}"#).entries.count == 1)
        let emojis = String(repeating: "😀", count: 41) // 82 units
        #expect(throws: VaultError.format) { _ = try open(#"{"v":1,"entries":[{"id":"1","name":"\#(emojis)","type":"rdp","login":"l"}]}"#) }
    }
}

@Suite("Keys for a computer")
struct VaultKeysTests {
    private func e(_ id: String, _ type: VaultType, groups: [String] = [], servers: [String] = [], name: String? = nil) -> VaultEntry {
        VaultEntry(id: id, name: name ?? id, type: type, groups: groups.map { VaultRef(id: $0, name: "g") }, servers: servers.map { VaultRef(id: $0, name: "s") }, login: "l", password: "p")
    }

    private let known = VaultKnown(servers: ["srv-a", "srv-b"], groups: ["2", "3"])

    @Test func theOrderIsThisServerItsFolderAllThenTheRest() {
        let entries = [
            e("other", .uac, servers: ["srv-b"]), e("all", .uac), e("folder", .uac, groups: ["3"]), e("here", .uac, servers: ["srv-a"]),
            e("rdp", .rdp), e("any", .any), e("lost", .uac, servers: ["gone"]), e("parent", .uac, groups: ["2"]),
        ]
        let s = VaultKeys.sections(entries, serverId: "srv-a", groupChain: [3, 2], known: known, use: .uac)
        #expect(s.here.map(\.id) == ["here"])
        #expect(s.group.map(\.id) == ["folder", "parent"]) // the folder of the server and the ones above it
        #expect(s.all.map(\.id) == ["all", "any", "lost"]) // "lost" is bound to nothing that exists: for all
        #expect(s.other.map(\.id) == ["other"])
        #expect(s.fitting.map(\.id) == ["here", "folder", "parent", "all", "any", "lost"])
        #expect(!(s.here + s.group + s.all + s.other).contains { $0.id == "rdp" }) // the type RDP is not for the sign-in screen
    }

    @Test func theTypeDecidesWhatFits() {
        let entries = [e("r", .rdp), e("u", .uac), e("a", .any)]
        #expect(VaultKeys.sections(entries, serverId: "x", groupChain: [], known: known, use: .rdp).all.map(\.id) == ["a", "r"])
        #expect(VaultKeys.sections(entries, serverId: "x", groupChain: [], known: known, use: .uac).all.map(\.id) == ["a", "u"])
    }

    @Test func aKeyBoundToOneLivingThingIsNotLost() {
        let entry = e("k", .uac, groups: ["gone-group"], servers: ["srv-b"])
        #expect(!VaultKeys.isLost(entry, known: known))
        #expect(VaultKeys.isLost(e("k", .uac, groups: ["x"], servers: ["y"]), known: known))
        #expect(!VaultKeys.isLost(e("k", .uac), known: known))
    }

    @Test func namesSortTheWayAPersonReadsThem() {
        let entries = [e("1", .uac, name: "Key 10"), e("2", .uac, name: "Key 2"), e("3", .uac, name: "alpha")]
        #expect(VaultKeys.sections(entries, serverId: "x", groupChain: [], known: known, use: .uac).all.map(\.name) == ["alpha", "Key 2", "Key 10"])
    }

    @Test func theFolderChain() {
        let folders = [Folder(id: 1, name: "a"), Folder(id: 2, name: "b", parentId: 1), Folder(id: 3, name: "c", parentId: 2)]
        #expect(VaultKeys.chain(of: 3, in: folders) == [3, 2, 1])
        #expect(VaultKeys.chain(of: nil, in: folders).isEmpty)
        #expect(VaultKeys.chain(of: 99, in: folders).isEmpty)
        #expect(VaultKeys.chain(of: 1, in: [Folder(id: 1, name: "loop", parentId: 1)]) == [1]) // a loop does not hang
    }

    @Test func theThreeActionsOfAKey() {
        let k = VaultEntry(id: "1", name: "Office", type: .uac, login: "admin", domain: "CORP", password: "pw")
        #expect(k.typing(for: .login) == [SignInTyping(text: "CORP\\admin", after: "tab", label: "Office")])
        #expect(k.typing(for: .password) == [SignInTyping(text: "pw", after: "enter", label: "Office")])
        #expect(k.typing(for: .signIn).map(\.after) == ["tab", "enter"])
        #expect(k.typing(for: .signIn).map(\.text) == ["CORP\\admin", "pw"])
        // what is typed passes the check of the Assistant (no control characters, in the limit)
        for step in k.typing(for: .signIn) { #expect(AssistCommand.typing(step.text, after: step.after, label: step.label) != nil) }
    }
}
