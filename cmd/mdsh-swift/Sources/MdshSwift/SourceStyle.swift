import AppKit
import SwiftUI

/// SourceStyle colors the source view's markdown as mdsh-gui's editor
/// does with its tok-* classes: headings, links, inline code and
/// emphasis in the prose, the #! line, the front matter as YAML, fences
/// and their info strings, and fenced code in its language.
enum SourceStyle {
    /// Segment is a piece of the text, by what it is.
    enum Segment {
        case shebang(NSRange)
        case mark(NSRange)  // a fence or front matter line
        case info(NSRange)  // a fence's info string
        case code(NSRange, String)  // fenced code, or front matter as yaml
        case prose(NSRange)  // one line of prose
    }

    /// scan cuts text into segments in one pass over its lines.
    static func scan(_ text: NSString) -> [Segment] {
        var out: [Segment] = []
        var start = 0
        var lineNo = 0
        var shebang = false
        var front: Int?  // where the front matter's body starts
        var fence: (mark: String, lang: String, body: Int)?
        while start < text.length {
            let line = text.lineRange(for: NSRange(location: start, length: 0))
            let s = text.substring(with: line).trimmingCharacters(in: .whitespacesAndNewlines)
            defer {
                start = NSMaxRange(line)
                lineNo += 1
            }
            if lineNo == 0 && s.hasPrefix("#!") {
                shebang = true
                out.append(.shebang(line))
            } else if (lineNo == 0 || (lineNo == 1 && shebang)) && s == "---" {
                front = NSMaxRange(line)
                out.append(.mark(line))
            } else if let f = front {
                if s == "---" || s == "..." {
                    out.append(.code(NSRange(location: f, length: line.location - f), "yaml"))
                    out.append(.mark(line))
                    front = nil
                }
            } else if let f = fence {
                if s.hasPrefix(f.mark) && s.allSatisfy({ $0 == f.mark.first }) {
                    out.append(.code(NSRange(location: f.body, length: line.location - f.body), f.lang))
                    out.append(.mark(line))
                    fence = nil
                }
            } else if s.hasPrefix("```") || s.hasPrefix("~~~") {
                let mark = String(s.prefix { $0 == s.first })
                let info = s.dropFirst(mark.count).trimmingCharacters(in: .whitespaces)
                fence = (mark, String(info.prefix { !$0.isWhitespace && $0 != "{" }), NSMaxRange(line))
                out.append(.mark(line))
                let at = (text.substring(with: line) as NSString).range(of: info)
                if !info.isEmpty, at.location != NSNotFound {
                    out.append(.info(NSRange(location: line.location + at.location, length: at.length)))
                }
            } else {
                out.append(.prose(line))
            }
        }
        // A front matter or fence left open runs to the end.
        if let f = front { out.append(.code(NSRange(location: f, length: text.length - f), "yaml")) }
        if let f = fence { out.append(.code(NSRange(location: f.body, length: text.length - f.body), f.lang)) }
        return out
    }

    private static let skipped = try! NSRegularExpression(pattern: [
        #"(`+)[^`\n][\s\S]*?\1"#,           // inline code
        #"<!--[\s\S]*?-->"#,                  // comment
        #"<[A-Za-z][A-Za-z0-9+.-]*:[^>\s]*>"#, // autolink
        #"<[^>\s@]+@[^>\s]+>"#,               // email autolink
        #"</?[A-Za-z][^>]*>"#,                 // HTML tag
        #"\b[A-Za-z][A-Za-z0-9+.-]*://[^\s<>()]+"#, // URL
        #"\]\([^)\s]*"#,                     // link destination
        #"\]\[[^\]]*\]"#,                   // link label
        #"^ {0,3}\[[^\]]+\]:.*$"#,           // link definition
        #"&(?:#[0-9]+|#[xX][0-9A-Fa-f]+|[A-Za-z][A-Za-z0-9]*);"#, // entity
    ].joined(separator: "|"))

    /// unspelled is where spell checking leaves a prose line alone, as
    /// mdsh-gui's spell.ts skips InlineCode, URL, Autolink, HTMLTag,
    /// Comment, LinkLabel and Entity.
    static func unspelled(_ line: String) -> [NSRange] {
        skipped.matches(in: line, range: NSRange(location: 0, length: (line as NSString).length)).map(\.range)
    }

    /// codeLines is the ranges, in order and merged, of the lines that
    /// aren't prose, for spell checking: besides fences, front matter
    /// and the #! line, the blocks mdsh-gui's spell.ts skips, indented
    /// code and HTML blocks, comments and processing instructions.
    static func codeLines(_ segments: [Segment], in text: NSString) -> [NSRange] {
        var ranges: [NSRange] = []
        var blocks = ProseBlocks()
        for s in segments {
            switch s {
            case .shebang(let r), .mark(let r), .code(let r, _):
                ranges.append(r)
                blocks.reset()
            case .info: break
            case .prose(let r):
                if blocks.skips(text.substring(with: r)) { ranges.append(r) }
            }
        }
        ranges.sort { $0.location < $1.location }
        var out: [NSRange] = []
        for r in ranges where r.length > 0 {
            if let last = out.last, NSMaxRange(last) >= r.location {
                out[out.count - 1] = NSUnionRange(last, r)
            } else {
                out.append(r)
            }
        }
        return out
    }

    /// apply sets the colors and faces of the text in storage, only
    /// within limit when it is given, so an edit restyles only its
    /// lines. It changes no characters, so it leaves undo and the
    /// selection alone. It returns the segments it found.
    @discardableResult
    static func apply(_ storage: NSTextStorage, theme: Theme, font: NSFont, limit: NSRange? = nil) -> [Segment] {
        let text = storage.string as NSString
        let all = NSRange(location: 0, length: text.length)
        let lim = limit.map { NSIntersectionRange($0, all) } ?? all
        let segments = scan(text)
        let p = Highlighter.SyntaxPalette(look: theme.look, dark: theme.dark)
        let fm = NSFontManager.shared
        let bold = fm.convert(font, toHaveTrait: .boldFontMask)
        let italic = fm.convert(font, toHaveTrait: .italicFontMask)
        func clip(_ r: NSRange) -> NSRange? {
            let x = NSIntersectionRange(r, lim)
            return x.length > 0 ? x : nil
        }
        func color(_ r: NSRange, _ c: Color) {
            if let x = clip(r) { storage.addAttribute(.foregroundColor, value: NSColor(c), range: x) }
        }
        func face(_ r: NSRange, _ f: NSFont) {
            if let x = clip(r) { storage.addAttribute(.font, value: f, range: x) }
        }

        storage.beginEditing()
        defer { storage.endEditing() }
        if lim.length > 0 {
            storage.setAttributes([.font: font, .foregroundColor: Theme.nsColor(theme.palette.text)], range: lim)
        }
        for seg in segments {
            switch seg {
            case .shebang(let r):
                color(r, p.comment)
                face(r, italic)
            case .mark(let r):
                color(r, p.punct)
            case .info(let r):
                // The info string and its attributes, in the headings'
                // blue, as mdsh-gui draws them.
                color(r, p.heading)
            case .code(let r, let lang):
                guard r.length > 0, clip(r) != nil else { continue }
                for (s, c) in Highlighter.spans(text.substring(with: r), lang: lang, palette: p) {
                    let at = NSRange(location: r.location + s.location, length: s.length)
                    color(at, c)
                    if c == p.comment { face(at, italic) }
                }
            case .prose(let r):
                guard clip(r) != nil else { continue }
                prose(r, text: storage.string, palette: p, bold: bold, italic: italic, color: color, face: face)
            }
        }
        return segments
    }

    private static let heading = try! NSRegularExpression(pattern: #"^ {0,3}#{1,6}(\s|$)"#)
    private static let inline: [(NSRegularExpression, Kind)] = [
        (try! NSRegularExpression(pattern: #"\*\*[^*\n]+\*\*|__[^_\n]+__"#), .bold),
        (try! NSRegularExpression(pattern: #"(?<![*\w])\*[^*\s][^*\n]*\*(?!\*)|(?<![_\w])_[^_\s][^_\n]*_(?!\w)"#), .italic),
        (try! NSRegularExpression(pattern: #"!?\[[^\]\n]*\]\([^)\n]*\)|<https?://[^>\s]+>"#), .link),
        (try! NSRegularExpression(pattern: #"\{\{[^}\n]*\}\}"#), .placeholder),
        (try! NSRegularExpression(pattern: #"`[^`\n]+`"#), .code),
        (try! NSRegularExpression(pattern: #"<!--.*?-->"#), .comment),
    ]

    private enum Kind { case bold, italic, link, placeholder, code, comment }

    private static func prose(_ line: NSRange, text s: String, palette p: Highlighter.SyntaxPalette, bold: NSFont, italic: NSFont,
                              color: (NSRange, Color) -> Void, face: (NSRange, NSFont) -> Void) {
        if heading.firstMatch(in: s, range: line) != nil {
            color(line, p.heading)
            face(line, bold)
            return
        }
        for (re, kind) in inline {
            for m in re.matches(in: s, range: line) {
                switch kind {
                case .bold: face(m.range, bold)
                case .italic: face(m.range, italic)
                case .link: color(m.range, p.link)
                case .placeholder: color(m.range, p.variable)
                case .code: color(m.range, p.string)
                case .comment:
                    color(m.range, p.comment)
                    face(m.range, italic)
                }
            }
        }
    }

    /// risky says whether an edit of these lines, before or after it,
    /// can change how lines past them are read: a fence, the front
    /// matter or the #! line.
    static func risky(_ s: String) -> Bool {
        s.split(separator: "\n", omittingEmptySubsequences: false).contains { l in
            // A fence may sit in a quote or a list item.
            var t = l.drop(while: { " \t>".contains($0) })
            if let f = t.first, "-*+".contains(f), t.dropFirst().first == " " {
                t = t.dropFirst(2)
            } else {
                let d = t.drop(while: { $0.isASCII && $0.isNumber })
                if d.count < t.count, let p = d.first, ".)".contains(p) { t = d.dropFirst() }
            }
            t = t.drop(while: { $0 == " " })
            // scan() reads the front matter and #! lines trimmed.
            let lt = l.drop(while: { $0 == " " || $0 == "\t" })
            return t.hasPrefix("```") || t.hasPrefix("~~~") || lt.hasPrefix("---") || lt.hasPrefix("...") || lt.hasPrefix("#!")
        }
    }
}

/// ProseBlocks reads prose lines in order and says which are in an
/// indented code block or an HTML block, by CommonMark's rules as far
/// as one line at a time can follow them.
struct ProseBlocks {
    private var blank = true
    private var paragraph = false
    private var list = false
    private var indented = false
    /// html is how the open HTML block ends: a marker in a line, or ""
    /// for a blank line.
    private var html: String?

    private static let rawTag = try! NSRegularExpression(pattern: #"^<(script|pre|style|textarea)(\s|>|$)"#, options: .caseInsensitive)
    private static let tag = try! NSRegularExpression(pattern: #"^</?[A-Za-z][A-Za-z0-9-]*(\s|/?>|$)"#)
    private static let listItem = try! NSRegularExpression(pattern: #"^ {0,3}([-*+]|\d{1,9}[.)])(\s|$)"#)

    mutating func reset() { self = ProseBlocks() }

    /// skips says whether spell checking leaves this line alone.
    mutating func skips(_ raw: String) -> Bool {
        let line = raw.trimmingCharacters(in: .newlines)
        let isBlank = line.trimmingCharacters(in: .whitespaces).isEmpty
        if let end = html {
            if end.isEmpty {
                if isBlank { html = nil; blank = true; return false }
            } else if line.contains(end) {
                html = nil
                blank = false
            }
            return true
        }
        if isBlank {
            blank = true
            return false
        }
        var indent = 0
        for c in line {
            if c == " " { indent += 1 } else if c == "\t" { indent += 4 - indent % 4 } else { break }
        }
        defer { blank = false }
        if indent >= 4 && (indented || (blank && !list) || (!paragraph && !list)) {
            indented = true
            paragraph = false
            return true
        }
        indented = false
        let ns = line as NSString
        let all = NSRange(location: 0, length: ns.length)
        if Self.listItem.firstMatch(in: line, range: all) != nil { list = true } else if indent == 0 { list = false }
        guard indent < 4 else { paragraph = true; return false }
        let t = String(line.drop(while: { $0 == " " }))
        let tns = t as NSString
        let tall = NSRange(location: 0, length: tns.length)
        var end: String?
        if t.hasPrefix("<!--") {
            end = "-->"
        } else if t.hasPrefix("<?") {
            end = "?>"
        } else if t.hasPrefix("<![CDATA[") {
            end = "]]>"
        } else if t.hasPrefix("<!"), t.dropFirst(2).first?.isLetter == true {
            end = ">"
        } else if let m = Self.rawTag.firstMatch(in: t, range: tall) {
            end = "</" + tns.substring(with: m.range(at: 1)).lowercased() + ">"
        } else if Self.tag.firstMatch(in: t, range: tall) != nil, !paragraph || blank {
            end = ""
        }
        guard let e = end else {
            paragraph = true
            return false
        }
        paragraph = false
        // A block that ends on its first line is done with it.
        let after = e.isEmpty ? "" : String(t.dropFirst(2))
        if e.isEmpty || !after.lowercased().contains(e) { html = e }
        return true
    }
}

/// SourceFolds is where the source view folds, as mdsh-gui's fold.ts
/// works it out: a heading folds its section, down to the next heading
/// of the same or a higher level, and a fence or an mdsh:output region
/// folds to its closing line.
enum SourceFolds {
    private static let fenceOpen = try! NSRegularExpression(pattern: #"^(\s*(?:>\s*)*(?:[-*+]\s+|\d+[.)]\s+)?)(`{3,}|~{3,})"#)
    private static let atx = try! NSRegularExpression(pattern: #"^ {0,3}(#{1,6})(?:\s|$)"#)
    private static let regionOpen = try! NSRegularExpression(pattern: #"^\s*(?:>\s*)*<!-- mdsh:output\b.*-->\s*$"#)
    private static let regionClose = try! NSRegularExpression(pattern: #"^\s*(?:>\s*)*<!-- /mdsh:output -->\s*$"#)
    private static let listOrQuote = try! NSRegularExpression(pattern: #"^\s*([-*+]|\d+[.)])\s|^\s*(>|<!--)"#)
    private static let setext1 = try! NSRegularExpression(pattern: #"^ {0,3}=+\s*$"#)
    private static let setext2 = try! NSRegularExpression(pattern: #"^ {0,3}-+\s*$"#)

    /// closers are the closing-fence patterns, by fence.
    private static var closers: [String: NSRegularExpression] = [:]
    private static let closersLock = NSLock()

    private static func closer(_ fence: String) -> NSRegularExpression {
        closersLock.lock()
        defer { closersLock.unlock() }
        if let c = closers[fence] { return c }
        let c = try! NSRegularExpression(pattern: #"^\s*(?:>\s*)*"# + (fence.first == "`" ? "`" : "~") + "{\(fence.count),}\\s*$")
        closers[fence] = c
        return c
    }

    private static func test(_ re: NSRegularExpression, _ s: String) -> Bool {
        re.firstMatch(in: s, range: NSRange(location: 0, length: (s as NSString).length)) != nil
    }

    /// outline is, for each line that opens a fold, counted from 1, the
    /// last line it hides.
    static func outline(_ lines: [String]) -> [Int: Int] {
        var folds: [Int: Int] = [:]
        var headings: [(line: Int, level: Int)] = []
        func blank(_ n: Int) -> Bool { lines[n - 1].trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        func close(_ line: Int, _ level: Int) {
            while let h = headings.last, h.level >= level {
                headings.removeLast()
                var end = line - 1
                while end > h.line && blank(end) { end -= 1 }
                if end > h.line { folds[h.line] = end }
            }
        }
        var n = 1
        while n <= lines.count {
            let t = lines[n - 1]
            let ns = t as NSString
            if let f = fenceOpen.firstMatch(in: t, range: NSRange(location: 0, length: ns.length)) {
                let fence = ns.substring(with: f.range(at: 2))
                let closing = closer(fence)
                var end = n + 1
                while end <= lines.count && !test(closing, lines[end - 1]) { end += 1 }
                end = min(end, lines.count)
                if end > n { folds[n] = end }
                n = end + 1
                continue
            }
            if test(regionOpen, t) {
                var end = n + 1
                while end <= lines.count && !test(regionClose, lines[end - 1]) { end += 1 }
                if end <= lines.count {
                    folds[n] = end
                    // A region holds markdown, so its headings are left alone.
                    n = end + 1
                    continue
                }
            }
            let level = headingLevel(lines, n)
            if level > 0 {
                close(n, level)
                headings.append((n, level))
                if !test(atx, t) { n += 1 }  // the setext underline
            }
            n += 1
        }
        close(lines.count + 1, 0)
        return folds
    }

    /// headingLevel is the level of the heading on line n, or 0. A
    /// setext heading's level comes from the underline after it.
    private static func headingLevel(_ lines: [String], _ n: Int) -> Int {
        let t = lines[n - 1]
        let ns = t as NSString
        if let m = atx.firstMatch(in: t, range: NSRange(location: 0, length: ns.length)) { return m.range(at: 1).length }
        if n >= lines.count || t.trimmingCharacters(in: .whitespaces).isEmpty || test(listOrQuote, t) { return 0 }
        let under = lines[n]
        if test(setext1, under) { return 1 }
        if test(setext2, under) { return 2 }
        return 0
    }
}

/// SourceTextView is the source view's text view: it shades the line
/// the cursor is on, as mdsh-gui's editor does.
final class SourceTextView: NSTextView {
    var currentLineColor: NSColor = .clear { didSet { needsDisplay = true } }

    override func drawBackground(in rect: NSRect) {
        super.drawBackground(in: rect)
        guard let lm = layoutManager, let tc = textContainer, selectedRange().length == 0 else { return }
        let text = string as NSString
        let loc = min(selectedRange().location, text.length)
        var r: NSRect
        if loc == text.length && (text.length == 0 || text.character(at: text.length - 1) == 10) {
            r = lm.extraLineFragmentRect
        } else {
            let line = text.lineRange(for: NSRange(location: loc, length: 0))
            let glyphs = lm.glyphRange(forCharacterRange: line, actualCharacterRange: nil)
            r = lm.boundingRect(forGlyphRange: glyphs, in: tc)
        }
        guard !r.isEmpty || r.height > 0 else { return }
        r.origin.y += textContainerOrigin.y
        r.origin.x = 0
        r.size.width = bounds.width
        currentLineColor.setFill()
        r.intersection(rect).fill(using: .sourceOver)
    }

    override func setSelectedRanges(_ ranges: [NSValue], affinity: NSSelectionAffinity, stillSelecting: Bool) {
        super.setSelectedRanges(ranges, affinity: affinity, stillSelecting: stillSelecting)
        needsDisplay = true
    }
}

/// SourceGutter is the source view's gutter, as mdsh-gui's editor has
/// one: the line numbers, a run button on the opening line of each block
/// and on each heading whose section has blocks, and fold arrows.
final class SourceGutter: NSRulerView {
    /// blocks and sections are the lines, from 1, with a run button.
    var blocks: Set<Int> = [] { didSet { if blocks != oldValue { needsDisplay = true } } }
    var sections: Set<Int> = [] { didSet { if sections != oldValue { needsDisplay = true } } }
    /// foldable lines open a fold; folded ones are closed, hiding the
    /// hidden lines.
    var foldable: Set<Int> = [] { didSet { if foldable != oldValue { needsDisplay = true } } }
    var folded: Set<Int> = [] { didSet { if folded != oldValue { needsDisplay = true } } }
    var hiddenLines: Set<Int> = [] { didSet { if hiddenLines != oldValue { needsDisplay = true } } }
    var onRun: (Int) -> Void = { _ in }
    var onFold: (Int) -> Void = { _ in }
    var numberColor: NSColor = .secondaryLabelColor { didSet { if numberColor != oldValue { chevrons = [:]; needsDisplay = true } } }
    /// chevrons are the fold chevrons tinted in numberColor, open and
    /// folded, made once per color and font.
    private var chevrons: [Bool: NSImage] = [:]
    private var chevronSize: CGFloat = 0

    private func chevron(folded: Bool) -> NSImage? {
        let size = numberFont.pointSize * 0.75
        if size != chevronSize { chevrons = [:]; chevronSize = size }
        if let c = chevrons[folded] { return c }
        guard let img = NSImage(systemSymbolName: folded ? "chevron.right" : "chevron.down", accessibilityDescription: nil)?
            .withSymbolConfiguration(NSImage.SymbolConfiguration(pointSize: size, weight: .semibold)) else { return nil }
        let color = numberColor
        let tinted = NSImage(size: img.size, flipped: false) { b in
            img.draw(in: b)
            color.set()
            b.fill(using: .sourceAtop)
            return true
        }
        chevrons[folded] = tinted
        return tinted
    }
    var runColor: NSColor = .controlAccentColor { didSet { if runColor != oldValue { playCache = nil; needsDisplay = true } } }
    var background: NSColor = .textBackgroundColor { didSet { if background != oldValue { needsDisplay = true } } }
    var border: NSColor = .separatorColor { didSet { if border != oldValue { needsDisplay = true } } }

    private let column: CGFloat = 14
    private var digits: CGFloat = 2
    private var playCache: NSImage?
    private weak var textView: NSTextView?

    init(textView: NSTextView) {
        self.textView = textView
        super.init(scrollView: textView.enclosingScrollView, orientation: .verticalRuler)
        clientView = textView
        changed()
        NotificationCenter.default.addObserver(self, selector: #selector(changed), name: NSText.didChangeNotification, object: textView)
    }

    required init(coder: NSCoder) { fatalError("not used") }

    /// changed makes room for the line count's digits, as the file
    /// grows; call it too when the text is set from outside.
    @objc func changed() {
        var lines = 1
        for b in (textView?.string ?? "").utf8 where b == 10 { lines += 1 }
        digits = CGFloat(max(String(lines).count, 2))
        let want = numbersRight + 2 * column + 2
        if abs(want - ruleThickness) > 0.5 { ruleThickness = want }
        needsDisplay = true
    }

    private var numberFont: NSFont {
        let size = (textView?.font?.pointSize ?? 13) * 0.85
        return .monospacedDigitSystemFont(ofSize: size, weight: .regular)
    }

    private var numbersRight: CGFloat { 6 + digits * ("8" as NSString).size(withAttributes: [.font: numberFont]).width }
    private var runX: CGFloat { numbersRight + 2 }
    private var foldX: CGFloat { runX + column }

    /// lines calls f with each line on screen that isn't folded away:
    /// its number, from 1, and its rect in the gutter.
    private func lines(_ f: (Int, NSRect) -> Void) {
        guard let tv = textView, let lm = tv.layoutManager, let tc = tv.textContainer else { return }
        let text = tv.string as NSString
        let glyphs = lm.glyphRange(forBoundingRect: tv.visibleRect, in: tc)
        let chars = lm.characterRange(forGlyphRange: glyphs, actualGlyphRange: nil)
        // The number of the first line on screen.
        var n = 1
        text.enumerateSubstrings(in: NSRange(location: 0, length: chars.location), options: [.byLines, .substringNotRequired]) { _, _, _, _ in n += 1 }
        let dy = tv.textContainerOrigin.y + convert(NSPoint.zero, from: tv).y
        func put(_ r: NSRect) {
            var r = r
            r.origin.y += dy
            r.origin.x = 0
            r.size.width = ruleThickness
            f(n, r)
        }
        var at = chars.location
        while at < text.length && (at < NSMaxRange(chars) || at == chars.location) {
            let line = text.lineRange(for: NSRange(location: at, length: 0))
            if !hiddenLines.contains(n) {
                let g = lm.glyphIndexForCharacter(at: line.location)
                put(lm.lineFragmentRect(forGlyphAt: g, effectiveRange: nil))
            }
            n += 1
            at = NSMaxRange(line)
        }
        // The empty last line after a final newline, or of an empty file.
        if at >= text.length, text.length == 0 || text.character(at: text.length - 1) == 10, !lm.extraLineFragmentRect.isEmpty {
            put(lm.extraLineFragmentRect)
        }
    }

    private var play: NSImage? {
        if let playCache { return playCache }
        guard let base = NSImage(systemSymbolName: "play.fill", accessibilityDescription: nil)?
            .withSymbolConfiguration(.init(pointSize: numberFont.pointSize * 0.8, weight: .regular)) else { return nil }
        let color = runColor
        let img = NSImage(size: base.size, flipped: false) { b in
            base.draw(in: b)
            color.set()
            b.fill(using: .sourceAtop)
            return true
        }
        playCache = img
        return img
    }

    override func drawHashMarksAndLabels(in rect: NSRect) {
        background.setFill()
        bounds.fill()
        border.setFill()
        NSRect(x: bounds.maxX - 1, y: bounds.minY, width: 1, height: bounds.height).fill()
        let attrs: [NSAttributedString.Key: Any] = [.font: numberFont, .foregroundColor: numberColor]
        lines { n, r in
            // The number first, then the run button, as mdsh-gui orders them.
            let s = String(n) as NSString
            let size = s.size(withAttributes: attrs)
            s.draw(at: NSPoint(x: numbersRight - size.width, y: r.minY + (r.height - size.height) / 2), withAttributes: attrs)
            if blocks.contains(n) || sections.contains(n), let play {
                let y = r.minY + (r.height - play.size.height) / 2
                play.draw(in: NSRect(x: runX + (column - play.size.width) / 2, y: y, width: play.size.width, height: play.size.height),
                          from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
            }
            if foldable.contains(n), let tinted = chevron(folded: folded.contains(n)) {
                let img = tinted
                let y = r.minY + (r.height - img.size.height) / 2
                tinted.draw(in: NSRect(x: foldX + (column - img.size.width) / 2, y: y, width: img.size.width, height: img.size.height),
                            from: .zero, operation: .sourceOver, fraction: folded.contains(n) ? 1 : 0.6, respectFlipped: true, hints: nil)
            }
        }
    }

    override func mouseDown(with event: NSEvent) {
        // Not passed on: NSTextView would take a click on the ruler as
        // setting a tab stop.
        let p = convert(event.locationInWindow, from: nil)
        var hit: Int?
        lines { n, r in if r.minY <= p.y && p.y < r.maxY { hit = n } }
        guard let n = hit else { return }
        if p.x >= runX && p.x < foldX, blocks.contains(n) || sections.contains(n) {
            onRun(n)
        } else if p.x >= foldX, foldable.contains(n) {
            onFold(n)
        }
    }

    override var isFlipped: Bool { true }
}
