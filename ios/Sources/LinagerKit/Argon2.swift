import Foundation

// Argon2id (RFC 9106, version 0x13), the key derivation of the vault of the keys. Written here because iOS has none and a library for it
// would be one more thing in the application to trust; it is checked against the vectors that the desktop application's own code
// (@noble/hashes) gives, with one, two and four lanes.

/// BLAKE2b (RFC 7693) without a key, for the lengths Argon2 needs (1 to 64 bytes).
enum Blake2b {
    private static let iv: [UInt64] = [
        0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
        0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
    ]

    private static let sigma: [[Int]] = [
        [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15],
        [14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3],
        [11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4],
        [7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8],
        [9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13],
        [2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9],
        [12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11],
        [13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10],
        [6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5],
        [10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0],
    ]

    @inline(__always) private static func rotr(_ x: UInt64, _ n: UInt64) -> UInt64 { (x >> n) | (x << (64 - n)) }

    private static func compress(_ h: inout [UInt64], _ block: [UInt64], t: UInt64, last: Bool) {
        var v = h + iv
        v[12] ^= t
        if last { v[14] = ~v[14] }
        @inline(__always) func g(_ a: Int, _ b: Int, _ c: Int, _ d: Int, _ x: UInt64, _ y: UInt64) {
            v[a] = v[a] &+ v[b] &+ x
            v[d] = rotr(v[d] ^ v[a], 32)
            v[c] = v[c] &+ v[d]
            v[b] = rotr(v[b] ^ v[c], 24)
            v[a] = v[a] &+ v[b] &+ y
            v[d] = rotr(v[d] ^ v[a], 16)
            v[c] = v[c] &+ v[d]
            v[b] = rotr(v[b] ^ v[c], 63)
        }
        for r in 0..<12 {
            let s = sigma[r % 10]
            g(0, 4, 8, 12, block[s[0]], block[s[1]])
            g(1, 5, 9, 13, block[s[2]], block[s[3]])
            g(2, 6, 10, 14, block[s[4]], block[s[5]])
            g(3, 7, 11, 15, block[s[6]], block[s[7]])
            g(0, 5, 10, 15, block[s[8]], block[s[9]])
            g(1, 6, 11, 12, block[s[10]], block[s[11]])
            g(2, 7, 8, 13, block[s[12]], block[s[13]])
            g(3, 4, 9, 14, block[s[14]], block[s[15]])
        }
        for i in 0..<8 { h[i] ^= v[i] ^ v[i + 8] }
    }

    /// The hash of `data`, `outLen` bytes long (1...64).
    static func hash(_ data: [UInt8], outLen: Int) -> [UInt8] {
        precondition((1...64).contains(outLen))
        var h = iv
        h[0] ^= 0x0101_0000 ^ UInt64(outLen)
        let blocks = max(1, (data.count + 127) / 128)
        for i in 0..<blocks {
            var words = [UInt64](repeating: 0, count: 16)
            let start = i * 128
            let end = min(data.count, start + 128)
            if start < end {
                for (k, byte) in data[start..<end].enumerated() { words[k / 8] |= UInt64(byte) << UInt64(8 * (k % 8)) }
            }
            let last = i == blocks - 1
            compress(&h, words, t: UInt64(last ? data.count : (i + 1) * 128), last: last)
        }
        var out = [UInt8](repeating: 0, count: 64)
        for i in 0..<8 { for k in 0..<8 { out[i * 8 + k] = UInt8(truncatingIfNeeded: h[i] >> UInt64(8 * k)) } }
        return Array(out[0..<outLen])
    }
}

public enum Argon2id {
    private static let blockWords = 128 // a block is 1024 bytes

    @inline(__always) private static func le32(_ v: Int) -> [UInt8] {
        let x = UInt32(truncatingIfNeeded: v)
        return [UInt8(x & 0xff), UInt8((x >> 8) & 0xff), UInt8((x >> 16) & 0xff), UInt8(x >> 24)]
    }

    /// H' of the RFC: a hash of any length.
    private static func longHash(_ input: [UInt8], outLen: Int) -> [UInt8] {
        let prefix = le32(outLen) + input
        if outLen <= 64 { return Blake2b.hash(prefix, outLen: outLen) }
        let r = (outLen + 31) / 32 - 2
        var v = Blake2b.hash(prefix, outLen: 64)
        var out = Array(v[0..<32])
        for _ in 1..<r {
            v = Blake2b.hash(v, outLen: 64)
            out += v[0..<32]
        }
        out += Blake2b.hash(v, outLen: outLen - 32 * r)
        return out
    }

    // MARK: the compression function G

    @inline(__always) private static func rotr(_ x: UInt64, _ n: UInt64) -> UInt64 { (x >> n) | (x << (64 - n)) }

    /// a + b + 2 * lo(a) * lo(b): the multiplication that makes the compression function hard to run on cheap hardware.
    @inline(__always) private static func blamka(_ a: UInt64, _ b: UInt64) -> UInt64 {
        let lo: UInt64 = 0xffff_ffff
        let product: UInt64 = (a & lo) &* (b & lo)
        let twice: UInt64 = product &* 2
        let sum: UInt64 = a &+ b
        return sum &+ twice
    }

    @inline(__always) private static func mix(_ a: inout UInt64, _ b: inout UInt64, _ c: inout UInt64, _ d: inout UInt64) {
        a = blamka(a, b)
        d = rotr(d ^ a, 32)
        c = blamka(c, d)
        b = rotr(b ^ c, 24)
        a = blamka(a, b)
        d = rotr(d ^ a, 16)
        c = blamka(c, d)
        b = rotr(b ^ c, 63)
    }

    /// The permutation P on sixteen words (indices into `v`).
    @inline(__always) private static func permute(_ v: UnsafeMutablePointer<UInt64>, _ i: [Int]) {
        mix(&v[i[0]], &v[i[4]], &v[i[8]], &v[i[12]])
        mix(&v[i[1]], &v[i[5]], &v[i[9]], &v[i[13]])
        mix(&v[i[2]], &v[i[6]], &v[i[10]], &v[i[14]])
        mix(&v[i[3]], &v[i[7]], &v[i[11]], &v[i[15]])
        mix(&v[i[0]], &v[i[5]], &v[i[10]], &v[i[15]])
        mix(&v[i[1]], &v[i[6]], &v[i[11]], &v[i[12]])
        mix(&v[i[2]], &v[i[7]], &v[i[8]], &v[i[13]])
        mix(&v[i[3]], &v[i[4]], &v[i[9]], &v[i[14]])
    }

    /// The words of a block as eight rows of sixteen, and as eight "columns" of two words from each row: P goes over both.
    private static let rowIndices: [[Int]] = {
        var out: [[Int]] = []
        for r in 0..<8 {
            var row: [Int] = []
            for k in 0..<16 { row.append(r * 16 + k) }
            out.append(row)
        }
        return out
    }()

    private static let columnIndices: [[Int]] = {
        var out: [[Int]] = []
        for c in 0..<8 {
            var col: [Int] = []
            for row in 0..<8 {
                col.append(row * 16 + 2 * c)
                col.append(row * 16 + 2 * c + 1)
            }
            out.append(col)
        }
        return out
    }()

    /// next = G(prev, ref), xored into the old content of `next` when `withXor` (the later passes).
    private static func fillBlock(_ prev: UnsafePointer<UInt64>, _ ref: UnsafePointer<UInt64>, _ next: UnsafeMutablePointer<UInt64>, withXor: Bool) {
        var r = [UInt64](repeating: 0, count: blockWords)
        var z = [UInt64](repeating: 0, count: blockWords)
        for i in 0..<blockWords {
            r[i] = ref[i] ^ prev[i]
            z[i] = r[i]
        }
        z.withUnsafeMutableBufferPointer { zp in
            let p = zp.baseAddress!
            for row in rowIndices { permute(p, row) }
            for col in columnIndices { permute(p, col) }
        }
        for i in 0..<blockWords {
            let v = z[i] ^ r[i]
            next[i] = withXor ? next[i] ^ v : v
        }
    }

    // MARK: the hash

    /// The Argon2id tag of a password. Nil when the parameters are not possible (the file of the vault limits them before this).
    /// `memoryKiB` is m, `iterations` t, `parallelism` p (the lanes are filled one after another here: the result is the same).
    public static func hash(password: [UInt8], salt: [UInt8], iterations t: Int, memoryKiB m: Int, parallelism p: Int, length: Int = 32) -> [UInt8]? {
        guard t >= 1, p >= 1, p <= 255, m >= 8 * p, length >= 4, salt.count >= 8, m <= 1 << 22 else { return nil }
        let blocksTotal = 4 * p * (m / (4 * p))
        let laneLength = blocksTotal / p
        let segmentLength = laneLength / 4

        var h0Input: [UInt8] = []
        for v in [p, length, m, t, 0x13, 2] { h0Input += le32(v) }
        h0Input += le32(password.count) + password
        h0Input += le32(salt.count) + salt
        h0Input += le32(0) // no secret
        h0Input += le32(0) // no associated data
        let h0 = Blake2b.hash(h0Input, outLen: 64)

        let memory = UnsafeMutablePointer<UInt64>.allocate(capacity: blocksTotal * blockWords)
        memory.initialize(repeating: 0, count: blocksTotal * blockWords)
        defer {
            // the memory held the key material: it is cleared before it goes back
            memory.update(repeating: 0, count: blocksTotal * blockWords)
            memory.deallocate()
        }

        func blockPointer(_ index: Int) -> UnsafeMutablePointer<UInt64> { memory + index * blockWords }

        // the first two blocks of every lane
        for lane in 0..<p {
            for k in 0..<2 {
                let bytes = longHash(h0 + le32(k) + le32(lane), outLen: 1024)
                let b = blockPointer(lane * laneLength + k)
                for i in 0..<blockWords {
                    var w: UInt64 = 0
                    for j in 0..<8 { w |= UInt64(bytes[i * 8 + j]) << UInt64(8 * j) }
                    b[i] = w
                }
            }
        }

        let zero = [UInt64](repeating: 0, count: blockWords)
        var inputBlock = [UInt64](repeating: 0, count: blockWords)
        var addressBlock = [UInt64](repeating: 0, count: blockWords)

        func nextAddresses() {
            inputBlock[6] &+= 1
            zero.withUnsafeBufferPointer { zp in
                inputBlock.withUnsafeBufferPointer { ip in
                    addressBlock.withUnsafeMutableBufferPointer { ap in fillBlock(zp.baseAddress!, ip.baseAddress!, ap.baseAddress!, withXor: false) }
                }
                addressBlock.withUnsafeMutableBufferPointer { ap in
                    fillBlock(zp.baseAddress!, UnsafePointer(ap.baseAddress!), ap.baseAddress!, withXor: false)
                }
            }
        }

        for pass in 0..<t {
            for slice in 0..<4 {
                for lane in 0..<p {
                    // the first half of the first pass takes its references from a counter, the rest from the data (the "i" of Argon2id)
                    let independent = pass == 0 && slice < 2
                    var startIndex = 0
                    if independent {
                        inputBlock = [UInt64](repeating: 0, count: blockWords)
                        inputBlock[0] = UInt64(pass)
                        inputBlock[1] = UInt64(lane)
                        inputBlock[2] = UInt64(slice)
                        inputBlock[3] = UInt64(blocksTotal)
                        inputBlock[4] = UInt64(t)
                        inputBlock[5] = 2
                    }
                    if pass == 0 && slice == 0 {
                        startIndex = 2
                        if independent { nextAddresses() }
                    }
                    var current = lane * laneLength + slice * segmentLength + startIndex
                    var previous = current % laneLength == 0 ? current + laneLength - 1 : current - 1
                    for index in startIndex..<segmentLength {
                        if current % laneLength == 1 { previous = current - 1 }
                        let pseudo: UInt64
                        if independent {
                            if index % blockWords == 0 { nextAddresses() }
                            pseudo = addressBlock[index % blockWords]
                        } else {
                            pseudo = blockPointer(previous)[0]
                        }
                        var refLane = Int((pseudo >> 32) % UInt64(p))
                        if pass == 0 && slice == 0 { refLane = lane }
                        let sameLane = refLane == lane
                        // the area of the lane the reference may point into
                        var area: Int
                        if pass == 0 {
                            if slice == 0 { area = index - 1 } else if sameLane { area = slice * segmentLength + index - 1 } else { area = slice * segmentLength + (index == 0 ? -1 : 0) }
                        } else if sameLane {
                            area = laneLength - segmentLength + index - 1
                        } else {
                            area = laneLength - segmentLength + (index == 0 ? -1 : 0)
                        }
                        var rel = pseudo & 0xffff_ffff
                        rel = (rel &* rel) >> 32
                        let relative = UInt64(area) &- 1 &- ((UInt64(area) &* rel) >> 32)
                        var startPosition = 0
                        if pass != 0 { startPosition = slice == 3 ? 0 : (slice + 1) * segmentLength }
                        let absolute = (startPosition + Int(relative)) % laneLength
                        fillBlock(UnsafePointer(blockPointer(previous)), UnsafePointer(blockPointer(laneLength * refLane + absolute)), blockPointer(current), withXor: pass != 0)
                        current += 1
                        previous += 1
                    }
                }
            }
        }

        // the tag: the last blocks of the lanes xored together, hashed
        var final = [UInt64](repeating: 0, count: blockWords)
        for lane in 0..<p {
            let b = blockPointer(lane * laneLength + laneLength - 1)
            for i in 0..<blockWords { final[i] ^= b[i] }
        }
        var bytes = [UInt8](repeating: 0, count: 1024)
        for i in 0..<blockWords { for j in 0..<8 { bytes[i * 8 + j] = UInt8(truncatingIfNeeded: final[i] >> UInt64(8 * j)) } }
        let tag = longHash(bytes, outLen: length)
        for i in 0..<bytes.count { bytes[i] = 0 }
        return tag
    }
}
