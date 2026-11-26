import CoreGraphics
import Foundation
import ImageIO

// What a keyboard and a screen need on the phone's side of the Assistant: the scan codes of the keys, the typing of a text, the picture of the
// computer put together from its tiles.

public struct KeyStroke: Hashable, Sendable {
    public let sc: Int
    public let ext: Bool
    /// The character needs Shift (on the US layout).
    public let shift: Bool

    public init(sc: Int, ext: Bool = false, shift: Bool = false) {
        self.sc = sc
        self.ext = ext
        self.shift = shift
    }
}

/// The PC set-1 scan codes (the same table as scancodes.ts). A character becomes a key of the US layout; the computer reads the key with its own
/// layout, as it does for a physical keyboard.
public enum ScanCodes {
    public static let esc = KeyStroke(sc: 0x01)
    public static let backspace = KeyStroke(sc: 0x0e)
    public static let tab = KeyStroke(sc: 0x0f)
    public static let enter = KeyStroke(sc: 0x1c)
    public static let ctrl = KeyStroke(sc: 0x1d)
    public static let shift = KeyStroke(sc: 0x2a)
    public static let alt = KeyStroke(sc: 0x38)
    public static let space = KeyStroke(sc: 0x39)
    public static let win = KeyStroke(sc: 0x5b, ext: true)
    public static let up = KeyStroke(sc: 0x48, ext: true)
    public static let down = KeyStroke(sc: 0x50, ext: true)
    public static let left = KeyStroke(sc: 0x4b, ext: true)
    public static let right = KeyStroke(sc: 0x4d, ext: true)
    public static let home = KeyStroke(sc: 0x47, ext: true)
    public static let end = KeyStroke(sc: 0x4f, ext: true)
    public static let pageUp = KeyStroke(sc: 0x49, ext: true)
    public static let pageDown = KeyStroke(sc: 0x51, ext: true)
    public static let insert = KeyStroke(sc: 0x52, ext: true)
    public static let delete = KeyStroke(sc: 0x53, ext: true)
    public static let printScreen = KeyStroke(sc: 0x37, ext: true)

    /// F1 to F12.
    public static func function(_ n: Int) -> KeyStroke? {
        switch n {
        case 1...10: KeyStroke(sc: 0x3a + n)
        case 11: KeyStroke(sc: 0x57)
        case 12: KeyStroke(sc: 0x58)
        default: nil
        }
    }

    private static let table: [Character: KeyStroke] = {
        var t: [Character: KeyStroke] = [:]
        func row(_ plain: String, _ shifted: String, from sc: Int) {
            for (i, (p, s)) in zip(plain, shifted).enumerated() {
                t[p] = KeyStroke(sc: sc + i)
                if s != " " { t[s] = KeyStroke(sc: sc + i, shift: true) }
            }
        }
        row("1234567890-=", "!@#$%^&*()_+", from: 0x02)
        row("qwertyuiop[]", "QWERTYUIOP{}", from: 0x10)
        row("asdfghjkl;'`", "ASDFGHJKL:\"~", from: 0x1e)
        row("zxcvbnm,./", "ZXCVBNM<>?", from: 0x2c)
        t["\\"] = KeyStroke(sc: 0x2b)
        t["|"] = KeyStroke(sc: 0x2b, shift: true)
        t[" "] = space
        return t
    }()

    /// The key of the US layout that types this character, nil for one it does not have (a letter of another alphabet, an accent, an emoji).
    public static func stroke(for c: Character) -> KeyStroke? { table[c] }
}

/// A keyboard modifier that is held for the next key (the sticky Ctrl, Alt and Win of the key bar).
public struct AssistModifiers: OptionSet, Sendable {
    public let rawValue: Int
    public init(rawValue: Int) { self.rawValue = rawValue }
    public static let ctrl = AssistModifiers(rawValue: 1)
    public static let alt = AssistModifiers(rawValue: 2)
    public static let win = AssistModifiers(rawValue: 4)
    public static let shift = AssistModifiers(rawValue: 8)

    var strokes: [KeyStroke] {
        var out: [KeyStroke] = []
        if contains(.ctrl) { out.append(ScanCodes.ctrl) }
        if contains(.alt) { out.append(ScanCodes.alt) }
        if contains(.win) { out.append(ScanCodes.win) }
        if contains(.shift) { out.append(ScanCodes.shift) }
        return out
    }
}

public enum AssistTyping {
    public enum Step: Equatable, Sendable {
        case input(AssistInput)
        /// Put this text on the clipboard of the computer; the next steps (Ctrl+V) come a moment later, when the computer has it.
        case clipboard(String)
    }

    /// A key pressed and released, with the modifiers held around it.
    public static func press(_ k: KeyStroke, modifiers: AssistModifiers = []) -> [Step] {
        var mods = modifiers.strokes
        if k.shift && !modifiers.contains(.shift) { mods.append(ScanCodes.shift) }
        var out: [Step] = mods.map { .input(.key(sc: $0.sc, ext: $0.ext, down: true)) }
        out.append(.input(.key(sc: k.sc, ext: k.ext, down: true)))
        out.append(.input(.key(sc: k.sc, ext: k.ext, down: false)))
        out += mods.reversed().map { .input(.key(sc: $0.sc, ext: $0.ext, down: false)) }
        return out
    }

    /// The steps that type a text. What the US layout has becomes keys; a run of other characters goes through the clipboard and Ctrl+V, which
    /// works whatever the layout of the computer is. The modifiers hold for the first key only.
    public static func plan(_ text: String, modifiers: AssistModifiers = []) -> [Step] {
        var steps: [Step] = []
        var pending = ""
        var mods = modifiers
        func flush() {
            guard !pending.isEmpty else { return }
            steps.append(.clipboard(pending))
            steps += press(KeyStroke(sc: 0x2f), modifiers: [.ctrl]) // Ctrl+V
            pending = ""
        }
        for c in text {
            let key: KeyStroke?
            switch c {
            case "\n", "\r", "\r\n": key = ScanCodes.enter
            case "\t": key = ScanCodes.tab
            case "\u{8}", "\u{7f}": key = ScanCodes.backspace
            default: key = ScanCodes.stroke(for: c)
            }
            if let key {
                flush()
                steps += press(key, modifiers: mods)
                mods = []
            } else {
                pending.append(c)
            }
        }
        flush()
        return steps
    }
}

/// The picture of the computer: a bitmap of the size of its screen, the tiles that come are drawn into it.
public final class AssistantScreen: @unchecked Sendable {
    private let lock = NSLock()
    private var context: CGContext?
    public private(set) var width = 0
    public private(set) var height = 0

    public init() {}

    private static func makeContext(_ w: Int, _ h: Int) -> CGContext? {
        guard w > 0, h > 0, w <= 16384, h <= 16384, let space = CGColorSpace(name: CGColorSpace.sRGB) else { return nil }
        let ctx = CGContext(data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0, space: space,
                            bitmapInfo: CGImageAlphaInfo.noneSkipFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue)
        ctx?.setFillColor(CGColor(red: 0, green: 0, blue: 0, alpha: 1))
        ctx?.fill(CGRect(x: 0, y: 0, width: w, height: h))
        return ctx
    }

    /// A new size of the screen: the picture starts black again, the computer sends the whole of it.
    public func resize(width w: Int, height h: Int) {
        lock.withLock {
            context = Self.makeContext(w, h)
            width = context == nil ? 0 : w
            height = context == nil ? 0 : h
        }
    }

    /// Draws the tiles; returns how many were drawn (one that is not a picture or lies outside the screen is skipped).
    @discardableResult
    public func apply(_ tiles: [AssistTile]) -> Int {
        // the pictures are decoded outside the lock: it is the slow part
        let decoded: [(AssistTile, CGImage)] = tiles.compactMap { t in
            guard let src = CGImageSourceCreateWithData(t.jpeg as CFData, nil), let img = CGImageSourceCreateImageAtIndex(src, 0, nil) else { return nil }
            return (t, img)
        }
        return lock.withLock {
            guard let ctx = context else { return 0 }
            var n = 0
            for (t, img) in decoded where t.w > 0 && t.h > 0 && t.x >= 0 && t.y >= 0 && t.x + t.w <= width && t.y + t.h <= height {
                // the bitmap has its origin at the bottom left, the tiles at the top left
                ctx.draw(img, in: CGRect(x: t.x, y: height - t.y - t.h, width: t.w, height: t.h))
                n += 1
            }
            return n
        }
    }

    /// A rectangle of pixels as the remote desktop (RDP) sends them: RGBA, rows packed. Returns false when it does not lie inside the screen.
    @discardableResult
    public func applyRGBA(x: Int, y: Int, w: Int, h: Int, rgba: Data) -> Bool {
        lock.withLock {
            guard let ctx = context, let base = ctx.data, w > 0, h > 0, x >= 0, y >= 0, x + w <= width, y + h <= height, rgba.count >= w * h * 4 else { return false }
            let stride = ctx.bytesPerRow
            rgba.withUnsafeBytes { (src: UnsafeRawBufferPointer) in
                let s = src.bindMemory(to: UInt8.self)
                let dst = base.assumingMemoryBound(to: UInt8.self)
                for row in 0..<h {
                    var si = row * w * 4
                    var di = (y + row) * stride + x * 4
                    for _ in 0..<w {
                        // the bitmap keeps B, G, R, X in memory (little-endian, alpha skipped first), the core gives R, G, B, A
                        dst[di] = s[si + 2]; dst[di + 1] = s[si + 1]; dst[di + 2] = s[si]; dst[di + 3] = 255
                        si += 4; di += 4
                    }
                }
            }
            return true
        }
    }

    /// The picture as it is now (a copy: later tiles do not change it).
    public func image() -> CGImage? { lock.withLock { context?.makeImage() } }

    /// The colour of a pixel, for the tests.
    public func pixel(x: Int, y: Int) -> (r: UInt8, g: UInt8, b: UInt8)? {
        lock.withLock {
            guard let ctx = context, let data = ctx.data, x >= 0, y >= 0, x < width, y < height else { return nil }
            let p = data.assumingMemoryBound(to: UInt8.self) + y * ctx.bytesPerRow + x * 4
            return (r: p[2], g: p[1], b: p[0])
        }
    }
}

/// Scrolling of the picture at the edges (the desktop application's "scroll at the edges"): when the pointer comes to an edge of the part of the
/// screen the picture has, the picture moves that way, and it stands still in the middle, so the pointer stays exact where it is needed. The
/// speed grows from nothing at the inner border of the zone to `maxSpeed` points per frame (at 60 frames a second) at the edge itself.
public enum EdgeScroll {
    public static let maxSpeed: Double = 20

    /// The width of the zone: the room is small with the keyboard up, so the zone never takes more than 30% of it from each side.
    public static func zone(of size: Double) -> Double { min(56, size * 0.3) }

    /// The speed for a pointer at `pos` in a box of `size`: negative toward the start, 0 in the middle. A pointer outside the box counts as at its edge.
    public static func speed(pos: Double, size: Double) -> Double {
        guard size > 0 else { return 0 }
        let zone = zone(of: size)
        if pos < zone { return -maxSpeed * min(1, (zone - max(0, pos)) / zone) }
        if pos > size - zone { return maxSpeed * min(1, (min(size, pos) - (size - zone)) / zone) }
        return 0
    }
}
