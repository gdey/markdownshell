import Foundation

/// TermText turns what a tty block writes into the text the terminal
/// showed, by termrec's rules (termrec.Recording.Text): escape sequences
/// are dropped, a carriage return starts its line over, a backspace
/// steps back and erasing to the end of the line erases. So a typo fixed
/// with Backspace reads fixed, and a progress line redrawn with \r shows
/// once. Full-screen programs need a real terminal emulator; this isn't
/// one.
///
/// It is for the live terminal only: unlike termrec's Text it keeps
/// trailing blank lines and only the last scrollback lines, so the
/// saved output still comes from the bridge.
public struct TermText {
    /// scrollback is how many finished lines are kept, so each write
    /// costs the same however long the block has run.
    public static let scrollback = 1000
    /// maxPending caps an escape sequence left open across writes; past
    /// it, the sequence is dropped, as Go drops one open at the end.
    static let maxPending = 4096

    private var lines: [String] = []
    private var line: [Character] = []
    private var col = 0
    /// pending holds bytes of an escape sequence or a UTF-8 character
    /// cut off at the end of a write, until the next.
    private var pending: [UInt8] = []

    public init() {}

    /// text is the terminal's text so far, without trailing spaces.
    public var text: String {
        (lines + [trimRight(String(line))]).joined(separator: "\n")
    }

    /// textWithCursor is text with a block cursor where the next
    /// character goes, as a terminal waiting for input shows it.
    public var textWithCursor: String {
        var l = line
        while l.count < col { l.append(" ") }
        if col < l.count { l[col] = "\u{2588}" } else { l.append("\u{2588}") }
        return (lines + [String(l)]).joined(separator: "\n")
    }

    /// end drops what is left open, as the block is done writing.
    public mutating func end() { pending = [] }

    /// write adds what the block wrote.
    public mutating func write(_ data: Data) {
        let b = pending + [UInt8](data)
        pending = []
        var i = 0
        while i < b.count {
            let c = b[i]
            switch c {
            case 0x1B:
                guard let e = Self.escape(b, i) else {
                    if b.count - i <= Self.maxPending { pending = Array(b[i...]) }
                    return
                }
                if e.final == UInt8(ascii: "K") && (e.params.isEmpty || e.params == "0") {
                    line = Array(line.prefix(col))
                }
                i += e.length
                continue
            case 0x0D:  // \r
                col = 0
            case 0x0A:  // \n
                lines.append(trimRight(String(line)))
                if lines.count > Self.scrollback { lines.removeFirst(lines.count - Self.scrollback) }
                line = []
                col = 0
            case 0x08:  // backspace
                col = max(col - 1, 0)
            case 0x09:
                put("\t")
            case 0..<0x20, 0x7F:
                break
            default:
                switch Self.utf8Length(b, i) {
                case nil:
                    pending = Array(b[i...])
                    return
                case 0?:
                    // Not UTF-8: one byte, as Go's DecodeRune takes it.
                    put("\u{FFFD}")
                case let n?:
                    for ch in String(decoding: b[i..<i + n], as: UTF8.self) { put(ch) }
                    i += n
                    continue
                }
            }
            i += 1
        }
    }

    private mutating func put(_ ch: Character) {
        if col < line.count {
            line[col] = ch
        } else {
            while line.count < col { line.append(" ") }
            line.append(ch)
        }
        col += 1
    }

    private func trimRight(_ s: String) -> String {
        var s = Substring(s)
        while s.last == " " { s = s.dropLast() }
        return String(s)
    }

    /// utf8Length is the length of the UTF-8 character at b[i], 0 when
    /// it isn't one, or nil when it runs past the end.
    static func utf8Length(_ b: [UInt8], _ i: Int) -> Int? {
        let n: Int
        switch b[i] {
        case 0xC2...0xDF: n = 2
        case 0xE0...0xEF: n = 3
        case 0xF0...0xF4: n = 4
        default: return b[i] < 0x80 ? 1 : 0
        }
        for j in 1..<n {
            guard i + j < b.count else { return nil }
            if !(0x80...0xBF).contains(b[i + j]) { return 0 }
        }
        return n
    }

    /// escape reads the escape sequence at b[i], by termrec's escape:
    /// its length, its final byte and a CSI's parameters, or nil when it
    /// runs past the end.
    static func escape(_ b: [UInt8], _ i: Int) -> (length: Int, final: UInt8, params: String)? {
        guard i + 1 < b.count else { return nil }
        switch b[i + 1] {
        case UInt8(ascii: "["):
            var j = i + 2
            while j < b.count, !(0x40...0x7E).contains(b[j]) { j += 1 }
            guard j < b.count else { return nil }
            return (j - i + 1, b[j], String(decoding: b[i + 2..<j], as: UTF8.self))
        case UInt8(ascii: "]"), UInt8(ascii: "P"), UInt8(ascii: "_"), UInt8(ascii: "^"):
            // Strings, such as a window title, end with BEL or ESC \.
            var j = i + 2
            while j < b.count {
                if b[j] == 0x07 { return (j - i + 1, 0, "") }
                if b[j] == 0x1B {
                    guard j + 1 < b.count else { return nil }
                    if b[j + 1] == UInt8(ascii: "\\") { return (j - i + 2, 0, "") }
                }
                j += 1
            }
            return nil
        case UInt8(ascii: "("), UInt8(ascii: ")"), UInt8(ascii: "*"), UInt8(ascii: "+"), UInt8(ascii: "#"), UInt8(ascii: "%"):
            guard i + 2 < b.count else { return nil }
            return (3, 0, "")
        default:
            return (2, 0, "")
        }
    }
}

/// TermStream is the History pane's way through tty bytes: plain text
/// in order, with escapes dropped, holding back a UTF-8 character or
/// escape cut off at the end of a write until the next.
public struct TermStream {
    private var pending: [UInt8] = []

    public init() {}

    /// take returns the text of what was written that is complete.
    public mutating func take(_ data: Data) -> String {
        let b = pending + [UInt8](data)
        pending = []
        var out: [UInt8] = []
        var i = 0
        while i < b.count {
            if b[i] == 0x1B {
                guard let e = TermText.escape(b, i) else {
                    if b.count - i <= TermText.maxPending { pending = Array(b[i...]) }
                    break
                }
                i += e.length
                continue
            }
            switch TermText.utf8Length(b, i) {
            case nil:
                pending = Array(b[i...])
                i = b.count
            case 0?:
                out += [0xEF, 0xBF, 0xBD]
                i += 1
            case let n?:
                out += b[i..<i + n]
                i += n
            }
        }
        // A \r at the end may be the first half of a \r\n.
        if pending.isEmpty, out.last == 0x0D {
            out.removeLast()
            pending = [0x0D]
        }
        return String(decoding: out, as: UTF8.self).replacingOccurrences(of: "\r\n", with: "\n")
    }
}
