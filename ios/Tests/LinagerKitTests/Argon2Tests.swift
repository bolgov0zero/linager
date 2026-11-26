import Foundation
import Testing
@testable import LinagerKit

private struct Vector: Decodable {
    let password: String
    let salt: String
    let t: Int
    let m: Int
    let p: Int
    let tag: String
}

private func bytes(hex: String) -> [UInt8] {
    var out: [UInt8] = []
    var i = hex.startIndex
    while i < hex.endIndex {
        let j = hex.index(i, offsetBy: 2)
        out.append(UInt8(hex[i..<j], radix: 16)!)
        i = j
    }
    return out
}

private func hex(_ b: [UInt8]) -> String { b.map { String(format: "%02x", $0) }.joined() }

@Suite("Argon2id")
struct Argon2Tests {
    @Test func blake2bKnownAnswers() {
        // RFC 7693: BLAKE2b-512("abc") and the empty string
        #expect(hex(Blake2b.hash(Array("abc".utf8), outLen: 64)) == "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923")
        #expect(hex(Blake2b.hash([], outLen: 64)) == "786a02f742015903c6c6fd852552d272912f4740e15847618a86e217f71f5419d25e1031afee585313896444934eb04b903a685b1448b755d56f701afe9be2ce")
        // a message longer than one block
        #expect(hex(Blake2b.hash(Array(repeating: 0x61, count: 300), outLen: 32)).count == 64)
    }

    @Test func theVectorsOfTheDesktopImplementation() throws {
        let vectors = try JSONDecoder().decode([Vector].self, from: fixture("argon2-vectors.json"))
        #expect(vectors.count == 5)
        for v in vectors where v.m <= 8192 {
            let tag = Argon2id.hash(password: Array(v.password.utf8), salt: bytes(hex: v.salt), iterations: v.t, memoryKiB: v.m, parallelism: v.p)
            #expect(tag.map(hex) == v.tag, "t=\(v.t) m=\(v.m) p=\(v.p)")
        }
    }

    @Test func theDefaultCostOfTheVault() throws {
        // 64 MiB, 3 passes: what a vault of the desktop application asks for
        let vectors = try JSONDecoder().decode([Vector].self, from: fixture("argon2-vectors.json"))
        let v = try #require(vectors.first { $0.m == 65536 })
        let start = Date()
        let tag = Argon2id.hash(password: Array(v.password.utf8), salt: bytes(hex: v.salt), iterations: v.t, memoryKiB: v.m, parallelism: v.p)
        #expect(tag.map(hex) == v.tag)
        print("argon2id 64 MiB x 3: \(Date().timeIntervalSince(start)) s")
    }

    @Test func impossibleParametersAreRefused() {
        let pw = [UInt8]("x".utf8), salt = [UInt8](repeating: 1, count: 16)
        #expect(Argon2id.hash(password: pw, salt: salt, iterations: 0, memoryKiB: 64, parallelism: 1) == nil)
        #expect(Argon2id.hash(password: pw, salt: salt, iterations: 1, memoryKiB: 7, parallelism: 1) == nil) // less than 8 blocks a lane
        #expect(Argon2id.hash(password: pw, salt: salt, iterations: 1, memoryKiB: 64, parallelism: 0) == nil)
        #expect(Argon2id.hash(password: pw, salt: [1, 2, 3], iterations: 1, memoryKiB: 64, parallelism: 1) == nil) // a salt is at least 8 bytes
    }

    @Test func theSameInputGivesTheSameTagAndAnotherPasswordAnotherOne() {
        let salt = [UInt8](repeating: 7, count: 16)
        let a = Argon2id.hash(password: Array("one".utf8), salt: salt, iterations: 1, memoryKiB: 64, parallelism: 1)
        let b = Argon2id.hash(password: Array("one".utf8), salt: salt, iterations: 1, memoryKiB: 64, parallelism: 1)
        let c = Argon2id.hash(password: Array("two".utf8), salt: salt, iterations: 1, memoryKiB: 64, parallelism: 1)
        #expect(a == b)
        #expect(a != c)
        #expect(a?.count == 32)
    }
}
