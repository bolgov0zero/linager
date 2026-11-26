import Foundation
import Testing
@testable import LinagerKit

@Suite("Text of a stream")
struct LogStreamTests {
    @Test func aCharacterCutInTwoWaitsForItsOtherHalf() {
        var s = Utf8Stream()
        let bytes = Array("a→я😀z".utf8)
        var out = ""
        // every cut of the bytes gives the same text in the end
        for size in 1...bytes.count {
            s = Utf8Stream()
            out = ""
            var i = 0
            while i < bytes.count {
                out += s.append(Data(bytes[i..<min(i + size, bytes.count)]))
                i += size
            }
            #expect(out == "a→я😀z", "pieces of \(size)")
        }
    }

    @Test func wrongBytesAreNotKeptForever() {
        var s = Utf8Stream()
        // a lone continuation byte and a start with nothing after it do not stall the stream
        #expect(s.append(Data([0x80])).contains("\u{FFFD}"))
        #expect(s.append(Data("ok".utf8)) == "ok")
    }

    @Test func theSignsOfATerminalAreTakenOut() {
        #expect(LogText.clean("\u{1B}[31mred\u{1B}[0m text") == "red text")
        #expect(LogText.clean("a\r\nb") == "a\nb")
        #expect(LogText.clean("10%\r50%\r100%\nnext") == "100%\nnext")
        #expect(LogText.clean("plain") == "plain")
    }

    @Test func linesGrowAndAreCapped() {
        var lines: [String] = []
        LogText.append("one\ntw", to: &lines, limit: 3)
        #expect(lines == ["one", "tw"])
        LogText.append("o\nthree\nfour\n", to: &lines, limit: 3)
        // the line that was being written is finished; the empty one at the end is the next line to write
        #expect(lines == ["three", "four", ""])
        LogText.append("", to: &lines, limit: 3)
        #expect(lines == ["three", "four", ""])
    }
}
