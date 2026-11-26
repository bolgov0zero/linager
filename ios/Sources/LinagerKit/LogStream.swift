import Foundation

/// The bytes of a stream come in pieces that can cut a character in two: this keeps the cut part until the rest arrives.
public struct Utf8Stream: Sendable {
    private var pending = Data()

    public init() {}

    /// Adds a piece and returns the text that is complete now.
    public mutating func append(_ data: Data) -> String {
        pending.append(data)
        // the longest start that is whole text: at most three bytes at the end may be the start of a character
        var cut = pending.count
        for back in 0..<min(3, pending.count) {
            let i = pending.count - 1 - back
            let byte = pending[pending.startIndex + i]
            if byte & 0b1100_0000 == 0b1000_0000 { continue } // a continuation byte: look further back for its first byte
            let needed: Int = byte >= 0b1111_0000 ? 4 : (byte >= 0b1110_0000 ? 3 : (byte >= 0b1100_0000 ? 2 : 1))
            if needed > back + 1 { cut = i }
            break
        }
        let head = pending.prefix(cut)
        pending = Data(pending.dropFirst(cut))
        return String(decoding: head, as: UTF8.self)
    }
}

/// Text of a journal or of a job as it is shown: lines without the signs a terminal would obey.
public enum LogText {
    /// Takes out the colour and cursor sequences (ESC [ ... letter) and the carriage returns.
    public static func clean(_ text: String) -> String {
        var s = text.replacingOccurrences(of: "\r\n", with: "\n")
        s = s.replacingOccurrences(of: #"\u001B\[[0-9;?]*[ -/]*[@-~]"#, with: "", options: .regularExpression)
        // a bare carriage return redraws a line (a progress bar): the last drawing stays
        if s.contains("\r") {
            s = s.split(separator: "\n", omittingEmptySubsequences: false).map { line in
                line.split(separator: "\r", omittingEmptySubsequences: false).last(where: { !$0.isEmpty }).map(String.init) ?? ""
            }.joined(separator: "\n")
        }
        return s
    }

    /// Adds text to the lines of a view: the last line is the one still being written, the rest are whole. `limit` lines are kept.
    public static func append(_ text: String, to lines: inout [String], limit: Int) {
        guard !text.isEmpty else { return }
        let parts = clean(text).split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
        if lines.isEmpty { lines = [""] }
        lines[lines.count - 1] += parts[0]
        lines.append(contentsOf: parts.dropFirst())
        if lines.count > limit { lines.removeFirst(lines.count - limit) }
    }
}
