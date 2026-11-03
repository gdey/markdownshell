import Foundation

/// Readline gives a text field the emacs keys of a shell's prompt that
/// macOS's own text keys lack, as mdsh-gui's History prompt has them:
/// ctrl+w, ctrl+u, ctrl+k and alt+d kill text, alt+backspace kills the
/// word before the cursor, ctrl+y yanks it back, alt+b and alt+f move by
/// word, ctrl+p and ctrl+n recall commands from the first or last line,
/// ctrl+l clears the transcript, and ctrl+d with nothing typed closes
/// the pane. ctrl+a, ctrl+e, ctrl+b, ctrl+f and ctrl+h are left to the
/// system, which already does them. Offsets are in UTF-16, as NSString's.
public struct Readline: Sendable {
    /// Key is a key with ctrl or alt (option) held, named by its place
    /// on the keyboard, not what it types: option+b types ∫ on a Mac.
    public enum Key: Equatable, Sendable {
        case ctrl(String)
        case alt(String)
    }

    /// Action is what a key does.
    public enum Action: Equatable, Sendable {
        /// nothing takes the key and does nothing, as ctrl+w at the start.
        case nothing
        /// move puts the cursor at an offset.
        case move(Int)
        /// replace puts text in place of a range, the cursor after it.
        case replace(NSRange, String)
        case eof, clear, prev, next
    }

    /// killed is what the last kill took, for ctrl+y. Kills one after
    /// another add to it, as readline's do.
    public private(set) var killed = ""
    private var killing = false

    public init() {}

    /// other is for any key key doesn't see, so a kill after it starts
    /// the kill text afresh.
    public mutating func other() { killing = false }

    /// key says what k does for text with the selection sel, or nil when
    /// it leaves the key to the system.
    public mutating func key(_ k: Key, text: String, selection sel: NSRange) -> Action? {
        let appending = killing
        killing = false
        let v = Array(text.utf16)
        let from = min(sel.location, v.count)
        let to = min(sel.location + sel.length, v.count)

        func kill(_ a: Int, _ b: Int, backward: Bool) -> Action {
            guard a < b else { return .nothing }
            let t = String(decoding: v[a..<b], as: UTF16.self)
            killed = !appending ? t : backward ? t + killed : killed + t
            killing = true
            return .replace(NSRange(location: a, length: b - a), "")
        }

        switch k {
        case .ctrl("w"):
            return from != to ? kill(from, to, backward: true) : kill(Self.spaceBack(v, from), from, backward: true)
        case .ctrl("u"):
            return kill(Self.lineStart(v, from), from, backward: true)
        case .ctrl("k"):
            // At the end of a line, ctrl+k joins the next line to it.
            let end = Self.lineEnd(v, from)
            return kill(from, end == from ? min(end + 1, v.count) : end, backward: false)
        case .ctrl("y"):
            return killed.isEmpty ? Action.nothing : .replace(NSRange(location: from, length: to - from), killed)
        case .ctrl("d"):
            return v.isEmpty ? .eof : nil
        case .ctrl("l"):
            return .clear
        case .ctrl("p"):
            return Self.lineMove(v, from, -1).map { .move($0) } ?? .prev
        case .ctrl("n"):
            return Self.lineMove(v, from, 1).map { .move($0) } ?? .next
        case .alt("b"):
            return .move(Self.wordBack(v, from))
        case .alt("f"):
            return .move(Self.wordForth(v, to))
        case .alt("d"):
            return kill(from, Self.wordForth(v, to), backward: false)
        case .alt("backspace"):
            return kill(Self.wordBack(v, from), from, backward: true)
        default:
            return nil
        }
    }

    static func isWord(_ u: UInt16) -> Bool {
        guard let s = Unicode.Scalar(u) else { return false }
        return s == "_" || CharacterSet.alphanumerics.contains(s)
    }

    static func isSpace(_ u: UInt16) -> Bool {
        guard let s = Unicode.Scalar(u) else { return false }
        return CharacterSet.whitespacesAndNewlines.contains(s)
    }

    /// lineStart and lineEnd are where the line holding p starts and ends.
    static func lineStart(_ v: [UInt16], _ p: Int) -> Int {
        var p = p
        while p > 0 && v[p - 1] != 10 { p -= 1 }
        return p
    }

    static func lineEnd(_ v: [UInt16], _ p: Int) -> Int {
        var p = p
        while p < v.count && v[p] != 10 { p += 1 }
        return p
    }

    /// wordBack is where alt+b goes: back over what isn't a word, then
    /// over the word. wordForth is alt+f: on over what isn't a word, then
    /// to the word's end.
    static func wordBack(_ v: [UInt16], _ p: Int) -> Int {
        var p = p
        while p > 0 && !isWord(v[p - 1]) { p -= 1 }
        while p > 0 && isWord(v[p - 1]) { p -= 1 }
        return p
    }

    static func wordForth(_ v: [UInt16], _ p: Int) -> Int {
        var p = p
        while p < v.count && !isWord(v[p]) { p += 1 }
        while p < v.count && isWord(v[p]) { p += 1 }
        return p
    }

    /// spaceBack is where ctrl+w kills back to: over spaces, then to the
    /// space before the word, so a path or flag goes whole.
    static func spaceBack(_ v: [UInt16], _ p: Int) -> Int {
        var p = p
        while p > 0 && isSpace(v[p - 1]) { p -= 1 }
        while p > 0 && !isSpace(v[p - 1]) { p -= 1 }
        return p
    }

    /// lineMove is where the cursor goes on the line above or below,
    /// keeping its column where that line is long enough, or nil on the
    /// first or last line.
    static func lineMove(_ v: [UInt16], _ p: Int, _ dir: Int) -> Int? {
        let start = lineStart(v, p)
        let col = p - start
        if dir < 0 {
            if start == 0 { return nil }
            let above = lineStart(v, start - 1)
            return min(above + col, start - 1)
        }
        let end = lineEnd(v, p)
        if end == v.count { return nil }
        return min(end + 1 + col, lineEnd(v, end + 1))
    }
}
