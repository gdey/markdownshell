import AppKit
import MdshKit
import SwiftUI

/// OutlineView is the drawer listing the document's headings and blocks
/// with each block's state.
struct OutlineView: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(\.theme) private var theme

    var body: some View {
        List(selection: $model.selected) {
            ForEach(rows, id: \.cell.id) { r in
                row(r.cell, depth: r.depth)
            }
        }
        .listStyle(.sidebar)
        .accessibilityLabel("Outline")
    }

    /// rows are the headings and blocks, each block indented under its
    /// heading, leaving out what folded headings hold.
    private var rows: [(cell: Cell, depth: Int)] {
        var out: [(cell: Cell, depth: Int)] = []
        var level = 0
        var hideBelow: Int?
        for c in doc.cells where c.isHeading || c.isBlock {
            if c.isHeading {
                if let h = hideBelow, c.headingLevel > h { continue }
                hideBelow = model.folded.contains(c.from) ? c.headingLevel : nil
                level = c.headingLevel
                out.append((c, c.headingLevel - 1))
            } else {
                if hideBelow != nil { continue }
                out.append((c, level))
            }
        }
        return out
    }

    /// row is mdsh-gui's outline row: the state mark in a column on the
    /// left, then H for a heading with its fold chevron, a run square for
    /// a block, or a purple control for a textbox or choice.
    private func row(_ c: Cell, depth: Int) -> some View {
        let input = c.textbox != nil || c.choice != nil
        return HStack(spacing: 6) {
            Text(mark(c))
                .foregroundStyle(c.failed ? theme.danger : theme.muted)
                .frame(width: 12)
                .accessibilityLabel(markName(c))
            HStack(spacing: 6) {
                if c.isHeading {
                    Button {
                        model.folded.formSymmetricDifference([c.from])
                        Task { await doc.setFold(block: -1, heading: c.from, on: model.folded.contains(c.from)) }
                    } label: {
                        Image(systemName: model.folded.contains(c.from) ? "chevron.right" : "chevron.down")
                            .font(.system(size: 9))
                            .foregroundStyle(theme.muted)
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel(model.folded.contains(c.from) ? "Unfold section" : "Fold section")
                    Text("H").font(theme.uiFont(size: theme.bodySize * 0.75, weight: .bold)).foregroundStyle(theme.muted)
                    Text(plain(c.html ?? "")).lineLimit(1)
                } else {
                    Image(systemName: input ? "slider.horizontal.3" : "play.square")
                        .foregroundStyle(input ? Color.purple : theme.muted)
                    Text(c.displayName).lineLimit(1)
                }
            }
            .padding(.leading, CGFloat(depth) * 12)
            Spacer(minLength: 0)
        }
        // The block waiting for an answer is ringed, as in mdsh-gui.
        .padding(.horizontal, 2)
        .background {
            if c.isBlock, doc.confirm?.block == c.block {
                RoundedRectangle(cornerRadius: 4).fill(theme.accent.opacity(0.1))
                    .overlay(RoundedRectangle(cornerRadius: 4).stroke(theme.accent, lineWidth: 1.5))
            }
        }
        .font(theme.uiFont(size: theme.bodySize * 0.9))
        .contextMenu {
            if c.isBlock { Button("Run") { doc.runBlock(c.block) } }
            if c.isHeading && c.hasSection { Button("Run Section") { doc.runSection(c) } }
        }
    }

    private func mark(_ c: Cell) -> String {
        if doc.confirm?.block == c.block { return "?" }
        if doc.current == c.block { return "▶" }
        if doc.background.contains(c.block) || c.running { return "◐" }
        if c.ran { return c.failed ? "✗" : "✓" }
        return ""
    }

    private func markName(_ c: Cell) -> String {
        switch mark(c) {
        case "?": "waiting for you"
        case "▶": "running"
        case "◐": "running in the background"
        case "✗": "failed"
        case "✓": "ran"
        default: ""
        }
    }

    /// plain is a heading's text without its tags.
    private func plain(_ html: String) -> String {
        let s = html.replacingOccurrences(of: "<[^>]+>", with: "", options: .regularExpression)
        return CFXMLCreateStringByUnescapingEntities(nil, s as CFString, nil) as String? ?? s
    }
}

/// SourceView is the whole file as text, to edit. What is typed goes to
/// the document after a pause.
struct SourceView: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(\.theme) private var theme
    @Environment(AppModel.self) private var app
    @State private var editor = TextViewRef()

    var body: some View {
        VStack(spacing: 0) {
            MarkdownBar(insert: insert)
            SourceEditor(text: Binding(get: { doc.text }, set: { doc.textChanged($0) }),
                         font: theme.nsCodeFont, textColor: Theme.nsColor(theme.palette.text),
                         background: Theme.nsColor(theme.palette.background),
                         spellCheck: !app.settings.noSpellCheck,
                         line: $model.sourceLine,
                         ref: editor,
                         onRunLine: runLine,
                         theme: theme,
                         blockLines: Set(doc.cells.filter(\.isBlock).map(\.line)),
                         sectionLines: Set(doc.cells.filter { $0.isHeading && $0.hasSection }.map(\.line)),
                         onRunGutter: runGutter,
                         folds: $model.sourceFolds)
        }
        .onAppear {
            // The pane opens with the folds the file asks for, then keeps
            // its own, as mdsh-gui's does.
            if model.sourceFoldsFile != doc.file {
                model.sourceFoldsFile = doc.file
                model.sourceFolds = model.renderedFoldLines(fromFile: true)
            }
        }
    }

    /// insert puts s at the source view's cursor, whatever has the
    /// keyboard.
    private func insert(_ s: String) {
        guard let tv = editor.textView else { return }
        tv.window?.makeFirstResponder(tv)
        tv.insertText(s, replacementRange: tv.selectedRange())
    }

    /// runGutter runs the block or section whose gutter button was
    /// clicked, once what was typed is applied, as mdsh-gui's runLine.
    private func runGutter(_ line: Int) {
        guard !doc.running else { return }
        Task {
            await doc.sendText()
            if let c = doc.cells.first(where: { $0.isBlock && $0.line == line }) {
                doc.runBlock(c.block)
            } else if let c = doc.cells.first(where: { $0.isHeading && $0.line == line }) {
                doc.runSection(c)
            }
        }
    }

    /// runLine runs the block at a line, as ⌘↩ in the editor does.
    private func runLine(_ line: Int) {
        Task {
            await doc.sendText()
            if let b: Int = try? await app.tryCall("blockAt", ["doc": .init(doc.id), "line": .init(line)]), b >= 0 {
                doc.runBlock(b)
            }
        }
    }
}

/// TextViewRef holds the source view's NSTextView, for the markdown bar.
final class TextViewRef {
    weak var textView: NSTextView?
}

/// MarkdownInsert is something the markdown bar and the source view's
/// Insert menu put at the cursor.
struct MarkdownInsert {
    var symbol: String
    var name: String
    var key: String = ""
    var text: String

    static let items: [MarkdownInsert] = [
        .init(symbol: "textformat.size", name: "Heading", text: "# "),
        .init(symbol: "bold", name: "Bold", key: "⌘B", text: "**bold**"),
        .init(symbol: "italic", name: "Italic", key: "⌘I", text: "*italic*"),
        .init(symbol: "chevron.left.forwardslash.chevron.right", name: "Code", key: "⌘E", text: "`code`"),
        .init(symbol: "link", name: "Link", key: "⌘K", text: "[text](https://)"),
        .init(symbol: "photo", name: "Image", text: "![alt](image.png)"),
        .init(symbol: "list.bullet", name: "Bulleted list", text: "- "),
        .init(symbol: "list.number", name: "Numbered list", text: "1. "),
        .init(symbol: "checklist", name: "Task list", text: "- [ ] "),
        .init(symbol: "text.quote", name: "Quote", text: "> "),
        .init(symbol: "tablecells", name: "Table", text: "| A | B |\n| --- | --- |\n|  |  |\n"),
        .init(symbol: "curlybraces", name: "Code block", text: "```\n\n```\n"),
    ]

    static let blocks: [MarkdownInsert] = [
        .init(symbol: "terminal", name: "sh block", text: "```sh {#name}\n\n```\n"),
        .init(symbol: "terminal", name: "sh block, confirm first", text: "```sh {#name confirm}\n\n```\n"),
        .init(symbol: "terminal", name: "zsh block", text: "```zsh {#name}\n\n```\n"),
        .init(symbol: "character.textbox", name: "Textbox", text: "```textbox {#name var=NAME}\n```\n"),
        .init(symbol: "checklist", name: "Choice", text: "```choice {#name var=NAME}\n- [ ] one\n- [ ] two\n```\n"),
    ]

    /// menu is the Insert submenu for the source view's right-click
    /// menu, as mdsh-gui has.
    static func menu(target: AnyObject, action: Selector) -> NSMenuItem {
        let sub = NSMenu(title: "Insert")
        for list in [items, blocks] {
            if !sub.items.isEmpty { sub.addItem(.separator()) }
            for i in list {
                let mi = NSMenuItem(title: i.name, action: action, keyEquivalent: "")
                mi.target = target
                mi.representedObject = i.text
                sub.addItem(mi)
            }
        }
        let item = NSMenuItem(title: "Insert", action: nil, keyEquivalent: "")
        item.submenu = sub
        return item
    }
}

/// MarkdownBar inserts markdown and mdsh blocks at the cursor.
struct MarkdownBar: View {
    var insert: (String) -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 2) {
            ForEach(MarkdownInsert.items, id: \.name) { i in
                Button { insert(i.text) } label: { Image(systemName: i.symbol) }
                    .buttonStyle(.borderless)
                    .help(i.key.isEmpty ? i.name : "\(i.name) (\(i.key))")
                    .accessibilityLabel(i.name)
            }
            Menu {
                ForEach(MarkdownInsert.blocks, id: \.name) { i in
                    Button(i.name) { insert(i.text) }
                }
            } label: { Image(systemName: "terminal") }
                .menuStyle(.borderlessButton)
                .fixedSize()
                .help("mdsh block")
            Spacer()
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 3)
        .background(theme.surface)
    }
}

/// SourceEditor is an NSTextView for the source, which SwiftUI's
/// TextEditor can't scroll to a line or take ⌘↩ in.
struct SourceEditor: NSViewRepresentable {
    @Binding var text: String
    var font: NSFont
    var textColor: NSColor
    var background: NSColor
    var spellCheck: Bool
    @Binding var line: Int?
    var ref: TextViewRef
    var onRunLine: (Int) -> Void
    var theme: Theme
    /// blockLines and sectionLines get run buttons in the gutter.
    var blockLines: Set<Int> = []
    var sectionLines: Set<Int> = []
    var onRunGutter: (Int) -> Void = { _ in }
    /// folds are the lines, from 1, whose folds are closed.
    @Binding var folds: Set<Int>

    func makeNSView(context: Context) -> NSScrollView {
        let sv = NSScrollView()
        sv.hasVerticalScroller = true
        sv.borderType = .noBorder
        sv.drawsBackground = false
        // TextKit 1, for the layout manager the gutter and folds use.
        let tv = SourceTextView(usingTextLayoutManager: false)
        tv.minSize = .zero
        tv.maxSize = NSSize(width: CGFloat.greatestFiniteMagnitude, height: .greatestFiniteMagnitude)
        tv.isVerticallyResizable = true
        tv.isHorizontallyResizable = false
        tv.autoresizingMask = [.width]
        tv.textContainer?.widthTracksTextView = true
        tv.textContainer?.containerSize = NSSize(width: 0, height: CGFloat.greatestFiniteMagnitude)
        sv.documentView = tv
        tv.delegate = context.coordinator
        tv.layoutManager?.delegate = context.coordinator
        tv.textStorage?.delegate = context.coordinator
        tv.isRichText = false
        tv.allowsUndo = true
        tv.isAutomaticQuoteSubstitutionEnabled = false
        tv.isAutomaticDashSubstitutionEnabled = false
        tv.isAutomaticTextReplacementEnabled = false
        // Blocks are code: nothing may rewrite what is typed. Spelling is
        // only underlined, in the prose, per the setting.
        tv.isAutomaticSpellingCorrectionEnabled = false
        tv.isAutomaticTextCompletionEnabled = false
        tv.isAutomaticDataDetectionEnabled = false
        tv.isAutomaticLinkDetectionEnabled = false
        tv.isGrammarCheckingEnabled = false
        tv.textContainerInset = NSSize(width: 8, height: 8)
        tv.setAccessibilityLabel("Source")
        context.coordinator.textView = tv
        ref.textView = tv
        let gutter = SourceGutter(textView: tv)
        sv.verticalRulerView = gutter
        sv.hasVerticalRuler = true
        sv.rulersVisible = true
        gutter.onFold = { [weak coordinator = context.coordinator] n in coordinator?.toggleFold(n) }
        context.coordinator.gutter = gutter
        return sv
    }

    func updateNSView(_ sv: NSScrollView, context: Context) {
        guard let tv = sv.documentView as? SourceTextView else { return }
        let c = context.coordinator
        c.parent = self
        // The text's faces and colors come from SourceStyle, so only
        // what is typed takes these until it is styled.
        tv.typingAttributes = [.font: font, .foregroundColor: textColor]
        tv.backgroundColor = background
        tv.insertionPointColor = textColor
        tv.currentLineColor = Self.activeLine(theme)
        tv.isContinuousSpellCheckingEnabled = spellCheck
        if tv.string != text && !c.editing {
            let sel = tv.selectedRanges
            tv.string = text
            c.styled = ""
            c.edited = nil
            c.full = false
            c.reindex()
            c.gutter?.changed()
            tv.selectedRanges = sel.filter { $0.rangeValue.upperBound <= (text as NSString).length }
        }
        c.restyle(force: false)
        c.applyFolds(folds)
        if let g = c.gutter {
            g.blocks = blockLines
            g.sections = sectionLines
            g.onRun = onRunGutter
            g.numberColor = NSColor(theme.muted)
            g.runColor = NSColor(theme.accent)
            g.background = background
            g.border = NSColor(theme.border)
        }
        if let l = line {
            DispatchQueue.main.async {
                c.go(to: l)
                line = nil
            }
        }
    }

    /// activeLine is mdsh-gui's --active-line for the look.
    static func activeLine(_ theme: Theme) -> NSColor {
        let (rgb, a): (String, CGFloat)
        switch (theme.look, theme.dark) {
        case (.plain, false): (rgb, a) = ("#d0d7de", 0.35)
        case (.plain, true): (rgb, a) = ("#6e7681", 0.14)
        case (.tty, false): (rgb, a) = ("#2a241c", 0.05)
        case (.tty, true): (rgb, a) = ("#9cff9c", 0.06)
        case (.blueprint, false): (rgb, a) = ("#ffffff", 0.07)
        case (.blueprint, true): (rgb, a) = ("#8fd3ff", 0.06)
        case (.gum, false): (rgb, a) = ("#d8006f", 0.07)
        case (.gum, true): (rgb, a) = ("#ff5fae", 0.07)
        }
        return Theme.nsColor(rgb).withAlphaComponent(a)
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    final class Coordinator: NSObject, NSTextViewDelegate, NSLayoutManagerDelegate, NSTextStorageDelegate {
        var parent: SourceEditor
        weak var textView: NSTextView?
        var editing = false
        /// code is the line ranges outside the prose, from the last
        /// styling pass, for the spell checker.
        var code: [NSRange] = []
        weak var gutter: SourceGutter?
        /// styled says which look the text was last styled in; "" after
        /// the text is set from outside.
        var styled = ""
        /// edited is where the last edit's characters went, and full
        /// says it touched a fence, the front matter or the #! line, so
        /// the whole text is restyled.
        var edited: NSRange?
        var full = false
        /// outline is where the text folds; starts are its lines' first
        /// characters.
        var outline: [Int: Int] = [:]
        var starts: [Int] = [0]
        /// applied are the folds the layout hides; hidden their
        /// characters, in order.
        var applied: Set<Int> = []
        var hidden: [NSRange] = []

        init(_ p: SourceEditor) { parent = p }

        /// restyle colors the whole text, when the look changed since or
        /// force says so.
        func restyle(force: Bool) {
            guard let tv = textView, let storage = tv.textStorage else { return }
            let f = parent.font
            let key = "\(parent.theme.look)|\(parent.theme.dark)|\(f.fontName)|\(f.pointSize)"
            guard force || key != styled else { return }
            styled = key
            code = SourceStyle.codeLines(SourceStyle.apply(storage, theme: parent.theme, font: f), in: storage.string as NSString)
        }

        func textStorage(_ storage: NSTextStorage, didProcessEditing mask: NSTextStorageEditActions, range: NSRange, changeInLength delta: Int) {
            if mask.contains(.editedCharacters) { edited = edited.map { NSUnionRange($0, range) } ?? range }
        }

        func textView(_ tv: NSTextView, shouldChangeTextIn range: NSRange, replacementString: String?) -> Bool {
            let s = tv.string as NSString
            let lines = s.substring(with: s.lineRange(for: NSRange(location: min(range.location, s.length), length: min(range.length, s.length - min(range.location, s.length)))))
            if SourceStyle.risky(lines + (replacementString ?? "")) { full = true }
            // An edit reaching into a fold opens it, so nothing changes
            // out of sight.
            let open = parent.folds.filter { n in foldRange(n, s.length).map { Self.reaches(range, $0) } ?? false }
            if !open.isEmpty {
                let f = parent.folds.subtracting(open)
                parent.folds = f
                applyFolds(f)
            }
            return true
        }

        /// reaches says an edit of range changes what the hidden range h
        /// holds: h starts at the newline ending the fold's line, which
        /// stays in sight, so typing just before it doesn't count.
        static func reaches(_ range: NSRange, _ h: NSRange) -> Bool {
            if range.length == 0 { return range.location > h.location && range.location <= NSMaxRange(h) }
            return range.location < NSMaxRange(h) && NSMaxRange(range) > h.location
        }

        /// The caret steps over a fold rather than into it: past it going
        /// forward, onto the fold's line going back.
        func textView(_ tv: NSTextView, willChangeSelectionFromCharacterRanges old: [NSValue], toCharacterRanges new: [NSValue]) -> [NSValue] {
            guard !hidden.isEmpty, new.count == 1, new[0].rangeValue.length == 0 else { return new }
            let at = new[0].rangeValue.location
            guard let h = hidden.first(where: { at > $0.location && at <= NSMaxRange($0) }) else { return new }
            let from = old.first?.rangeValue.location ?? 0
            let length = (tv.string as NSString).length
            let forward = from <= h.location && NSMaxRange(h) < length
            return [NSValue(range: NSRange(location: forward ? NSMaxRange(h) + 1 : h.location, length: 0))]
        }

        func textDidChange(_ n: Notification) {
            guard let tv = textView, let storage = tv.textStorage else { return }
            let s = tv.string as NSString
            // Only the edited lines are restyled, unless the edit touched
            // what changes how the lines after it read.
            let at = edited.map { NSRange(location: min($0.location, s.length), length: min($0.length, s.length - min($0.location, s.length))) }
            let lines = at.map { s.lineRange(for: $0) }
            if let lines, !full, !SourceStyle.risky(s.substring(with: lines)) {
                code = SourceStyle.codeLines(SourceStyle.apply(storage, theme: parent.theme, font: parent.font, limit: lines), in: storage.string as NSString)
            } else {
                restyle(force: true)
            }
            edited = nil
            full = false
            reindex()
            // Folds whose line no longer opens one are dropped.
            let kept = parent.folds.filter { outline[$0] != nil }
            applyFolds(kept)
            editing = true
            parent.text = tv.string
            if kept != parent.folds { parent.folds = kept }
            editing = false
        }

        // MARK: Folding

        /// reindex works out the lines and folds of the text.
        func reindex() {
            guard let tv = textView else { return }
            let s = tv.string
            var st = [0]
            var i = 0
            for u in s.utf16 {
                i += 1
                if u == 10 { st.append(i) }
            }
            starts = st
            outline = SourceFolds.outline(s.components(separatedBy: "\n"))
            gutter?.foldable = Set(outline.keys)
            // So applyFolds works out the hidden ranges again, and lets go
            // of the old ones.
            applied = []
        }

        func toggleFold(_ n: Int) {
            var f = parent.folds
            if f.contains(n) { f.remove(n) } else { f.insert(n) }
            parent.folds = f
            applyFolds(f)
        }

        /// contentEnd is where line n's text ends, before its newline.
        private func contentEnd(_ n: Int, _ length: Int) -> Int {
            n < starts.count ? starts[n] - 1 : length
        }

        /// foldRange is what fold n hides: from the end of its line to the
        /// end of its last.
        func foldRange(_ n: Int, _ length: Int) -> NSRange? {
            guard let end = outline[n], n <= starts.count, end <= starts.count else { return nil }
            let from = contentEnd(n, length), to = contentEnd(end, length)
            return to > from ? NSRange(location: from, length: to - from) : nil
        }

        /// applyFolds hides the lines the closed folds hide.
        func applyFolds(_ folds: Set<Int>) {
            guard let tv = textView, let lm = tv.layoutManager else { return }
            if outline.isEmpty && starts.count == 1 && !tv.string.isEmpty { reindex() }
            let want = folds.filter { outline[$0] != nil }
            gutter?.folded = want
            guard want != applied || (want.isEmpty && !hidden.isEmpty) else { return }
            let length = (tv.string as NSString).length
            var ranges: [NSRange] = []
            var lines = Set<Int>()
            for n in want.sorted() {
                guard let end = outline[n] else { continue }
                if let r = foldRange(n, length) { ranges.append(r) }
                lines.formUnion((n + 1)...end)
            }
            // Nested folds: keep the outer range.
            ranges.sort { $0.location < $1.location }
            var merged: [NSRange] = []
            for r in ranges {
                if let last = merged.last, NSMaxRange(last) >= r.location {
                    merged[merged.count - 1] = NSUnionRange(last, r)
                } else {
                    merged.append(r)
                }
            }
            let old = hidden
            hidden = merged
            applied = want
            gutter?.hiddenLines = lines
            let all = NSRange(location: 0, length: length)
            let touched = (old + merged).reduce(nil as NSRange?) { acc, r in acc.map { NSUnionRange($0, r) } ?? r } ?? all
            let range = NSIntersectionRange(touched, all)
            lm.invalidateGlyphs(forCharacterRange: range, changeInLength: 0, actualCharacterRange: nil)
            lm.invalidateLayout(forCharacterRange: range, actualCharacterRange: nil)
            tv.needsDisplay = true
            gutter?.needsDisplay = true
        }

        private func isHidden(_ c: Int) -> Bool {
            var lo = 0, hi = hidden.count
            while lo < hi {
                let mid = (lo + hi) / 2
                if NSMaxRange(hidden[mid]) <= c { lo = mid + 1 } else { hi = mid }
            }
            return lo < hidden.count && hidden[lo].location <= c
        }

        func layoutManager(_ lm: NSLayoutManager, shouldGenerateGlyphs glyphs: UnsafePointer<CGGlyph>,
                           properties props: UnsafePointer<NSLayoutManager.GlyphProperty>,
                           characterIndexes chars: UnsafePointer<Int>, font: NSFont, forGlyphRange range: NSRange) -> Int {
            guard !hidden.isEmpty else { return 0 }
            var p = Array(UnsafeBufferPointer(start: props, count: range.length))
            var any = false
            for i in 0..<range.length where isHidden(chars[i]) {
                p[i] = .null
                any = true
            }
            guard any else { return 0 }
            p.withUnsafeBufferPointer { lm.setGlyphs(glyphs, properties: $0.baseAddress!, characterIndexes: chars, font: font, forGlyphRange: range) }
            return range.length
        }

        func layoutManager(_ lm: NSLayoutManager, shouldUse action: NSLayoutManager.ControlCharacterAction,
                           forControlCharacterAt c: Int) -> NSLayoutManager.ControlCharacterAction {
            // A hidden newline doesn't break the line, so the fold's
            // lines collapse into the line that opens it.
            isHidden(c) ? .zeroAdvancement : action
        }

        // MARK: Keys and spelling

        func textView(_ tv: NSTextView, doCommandBy sel: Selector) -> Bool {
            // ⌘↩ runs the block at the cursor.
            if sel == #selector(NSResponder.insertNewline(_:)), NSEvent.modifierFlags.contains(.command) {
                parent.onRunLine(currentLine(tv))
                return true
            }
            return false
        }

        /// Spelling is checked in the prose only, as mdsh-gui does: not
        /// in the #! line, the front matter, or code fences. The code
        /// lines come from the styling pass, so the two agree.
        func textView(_ tv: NSTextView, shouldSetSpellingState value: Int, range: NSRange) -> Int {
            guard value != 0 else { return value }
            guard Self.isProse(code, at: range.location), let tv = textView else { return 0 }
            // Within prose, the spans mdsh-gui's spell.ts skips: code,
            // URLs, tags, comments, link labels and entities.
            let s = tv.string as NSString
            let lineRange = s.lineRange(for: NSRange(location: min(range.location, s.length), length: 0))
            let line = s.substring(with: lineRange)
            for r in SourceStyle.unspelled(line) {
                let at = NSRange(location: r.location + lineRange.location, length: r.length)
                if NSIntersectionRange(at, range).length > 0 { return 0 }
            }
            return value
        }

        /// isProse says whether loc is in none of the code line ranges,
        /// which are in order.
        static func isProse(_ code: [NSRange], at loc: Int) -> Bool {
            var lo = 0, hi = code.count
            while lo < hi {
                let mid = (lo + hi) / 2
                if NSMaxRange(code[mid]) <= loc { lo = mid + 1 } else { hi = mid }
            }
            return lo == code.count || loc < code[lo].location
        }

        /// The right-click menu: AppKit's, with mdsh-gui's Insert
        /// submenu after Cut, Copy and Paste.
        func textView(_ view: NSTextView, menu: NSMenu, for event: NSEvent, at charIndex: Int) -> NSMenu? {
            let at = (menu.items.firstIndex { $0.action == #selector(NSText.paste(_:)) } ?? -1) + 1
            menu.insertItem(.separator(), at: at)
            menu.insertItem(MarkdownInsert.menu(target: self, action: #selector(insertItem(_:))), at: at + 1)
            return menu
        }

        @objc func insertItem(_ sender: NSMenuItem) {
            guard let s = sender.representedObject as? String, let tv = textView else { return }
            tv.insertText(s, replacementRange: tv.selectedRange())
        }

        func currentLine(_ tv: NSTextView) -> Int {
            let s = tv.string as NSString
            let loc = min(tv.selectedRange().location, s.length)
            var n = 1
            s.enumerateSubstrings(in: NSRange(location: 0, length: loc), options: [.byLines, .substringNotRequired]) { _, _, _, _ in n += 1 }
            return max(1, n - (loc > 0 && s.character(at: loc - 1) != 10 ? 1 : 0))
        }

        func go(to line: Int) {
            guard let tv = textView else { return }
            let s = tv.string as NSString
            var loc = 0, n = 1
            while n < line, loc < s.length {
                let r = s.lineRange(for: NSRange(location: loc, length: 0))
                loc = NSMaxRange(r)
                n += 1
            }
            let r = NSRange(location: min(loc, s.length), length: 0)
            tv.setSelectedRange(r)
            tv.scrollRangeToVisible(r)
            tv.window?.makeFirstResponder(tv)
        }
    }
}

/// HistoryPane is the terminal along the bottom: type a command and it
/// runs in the file's shell and is added to the end of the file.
struct HistoryPane: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(\.theme) private var theme
    @State private var input = ""
    @State private var past: [String] = []
    @State private var pastIndex: Int?
    @FocusState private var focused: Bool
    /// cleared is how much of the transcript ctrl+l cleared, in UTF-16.
    @State private var cleared = 0

    var body: some View {
        // The transcript, and under it the prompt line, which is the bar
        // History opens from. Stop, Restart and the arrow sit in a box
        // on the line between the two, as in mdsh-gui.
        VStack(spacing: 0) {
            ScrollViewReader { proxy in
                ScrollView {
                    Text(shownTranscript)
                        .textSelection(.enabled)
                        // A click, not a drag, puts the keyboard in the
                        // input; a drag still selects lines to copy.
                        .simultaneousGesture(TapGesture().onEnded { focused = true })
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(8)
                        // Room under the last line for the buttons' top half.
                        .padding(.bottom, 10)
                        .id("end")
                }
                .defaultScrollAnchor(.bottom)
                .onChange(of: doc.historyTranscript) { _, _ in proxy.scrollTo("end", anchor: .bottom) }
            }
            // All but the prompt's last line goes above the input, as
            // mdsh-gui's prompt head, unwrapped, so a rule drawn to the
            // pane's width runs across it.
            VStack(alignment: .leading, spacing: 0) {
            if !promptHead.isEmpty {
                Text(promptHead)
                    .fixedSize()
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .clipped()
                    .opacity(doc.historyRunning ? 0.4 : 1)
                    // A click on the prompt puts the keyboard in the
                    // input, as in mdsh-gui.
                    .contentShape(Rectangle())
                    .onTapGesture { focused = true }
            }
            HStack(alignment: .firstTextBaseline, spacing: 0) {
                Text(promptLine)
                    // While a command typed here runs, the prompt dims and
                    // the input gives its place to a clock, as in mdsh-gui.
                    .opacity(doc.historyRunning ? 0.4 : 1)
                    .onTapGesture { focused = true }
                TextField("", text: $input, axis: .vertical)
                    .textFieldStyle(.plain)
                    // The placeholder is drawn here, in the pane's muted
                    // color; the field's own is too dark on the dark pane.
                    .overlay(alignment: .leading) {
                        if input.isEmpty {
                            Text("A command to run and add to the file, or # a note")
                                .foregroundStyle(paneMuted)
                                .allowsHitTesting(false)
                                .accessibilityHidden(true)
                        }
                    }
                    .focused($focused)
                    .onSubmit(submit)
                    .background {
                        ReadlineKeys(
                            eof: { model.toggleHistory() },
                            clear: { cleared = (doc.historyTranscript as NSString).length },
                            prev: { _ = recall(-1) },
                            next: { _ = recall(1) }
                        )
                    }
                    .onKeyPress(.upArrow) { recall(-1) }
                    .onKeyPress(.downArrow) { recall(1) }
                    .onKeyPress(.tab) {
                        complete()
                        return .handled
                    }
                    .accessibilityLabel("Command")
                    // Settings › Look › History prompt focus: an outline
                    // around the field, or a block cursor where you type.
                    // With Increase Contrast the outline and the system's
                    // caret come back, as with forced colors in mdsh-gui.
                    .overlay {
                        if focused && !doc.historyRunning {
                            if blockCursor {
                                BlockCursor(fg: NSColor(paneText), bg: NSColor(paneSurface), placeholder: "A")
                                    .allowsHitTesting(false)
                                    .accessibilityHidden(true)
                            } else {
                                RoundedRectangle(cornerRadius: 3)
                                    .stroke(theme.accent, lineWidth: 2)
                                    .padding(-3)
                                    .allowsHitTesting(false)
                            }
                        }
                    }
                    .opacity(doc.historyRunning ? 0 : 1)
                    .overlay(alignment: .leading) {
                        if doc.historyRunning, let start = doc.historyStarted { clock(start) }
                    }
                    .padding(.trailing, 80)
            }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(paneSurface)
            .overlay(alignment: .top) { Rectangle().fill(paneMuted.opacity(0.4)).frame(height: 1) }
            .overlay(alignment: .topTrailing) {
                HStack(spacing: 8) {
                    Button { doc.stop() } label: { Image(systemName: "stop.fill") }
                        .disabled(!doc.running)
                        .help("Stop")
                        .accessibilityLabel("Stop")
                    Button { Task { _ = await doc.edit("restart") } } label: { Image(systemName: "arrow.clockwise") }
                        .disabled(doc.running)
                        .help("Start the shells again")
                        .accessibilityLabel("Restart")
                    Button { model.toggleHistory() } label: { Image(systemName: "chevron.down") }
                        .help("Close History")
                        .accessibilityLabel("Close History")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(paneMuted)
                .padding(.horizontal, 6)
                .padding(.vertical, 3)
                .background(paneBackground, in: RoundedRectangle(cornerRadius: 5))
                .overlay(RoundedRectangle(cornerRadius: 5).stroke(paneMuted.opacity(0.4), lineWidth: 1))
                .padding(.trailing, 8)
                // The line runs through the middle of the box.
                .alignmentGuide(.top) { $0.height / 2 }
            }
            .zIndex(1)
        }
        .font(theme.codeFont())
        .background(paneBackground)
        // The pane's size in characters, for a prompt drawn to fit it.
        .background {
            GeometryReader { g in
                Color.clear
                    .onAppear { sized(g.size) }
                    .onChange(of: g.size) { _, s in sized(s) }
            }
        }
        .foregroundStyle(paneText)
        .onAppear { focused = true }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("History")
    }

    /// The pane's colors: dark for the plain look unless Settings says
    /// otherwise, as in mdsh-gui.
    private var paneDark: Bool {
        switch AppModel.shared.settings.historyPane {
        case "match": theme.dark
        case "invert": !theme.dark
        case "dark": true
        default: theme.look == .plain ? true : theme.dark
        }
    }
    /// shownTranscript is the transcript after what ctrl+l cleared.
    private var shownTranscript: String {
        let t = doc.historyTranscript as NSString
        return cleared > 0 && cleared <= t.length ? t.substring(from: cleared) : doc.historyTranscript
    }

    private var blockCursor: Bool {
        AppModel.shared.settings.historyFocus == "cursor" && !NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast
    }
    private var panePalette: Palette { Palette.of(theme.look, dark: paneDark) }
    private var paneBackground: Color { Color(nsColor: Theme.nsColor(panePalette.background)) }
    private var paneSurface: Color { Color(nsColor: Theme.nsColor(panePalette.surface)) }
    private var paneText: Color { Color(nsColor: Theme.nsColor(panePalette.text)) }
    private var paneMuted: Color { Color(nsColor: Theme.nsColor(panePalette.muted)) }

    /// promptHead and promptLine split the shell's PS1 at its last line.
    private var promptHead: String {
        let ps1 = doc.prompt.ps1
        guard let nl = ps1.lastIndex(of: "\n") else { return "" }
        return String(ps1[..<nl])
    }

    private var promptLine: String {
        let ps1 = doc.prompt.ps1.isEmpty ? "$ " : doc.prompt.ps1
        guard let nl = ps1.lastIndex(of: "\n") else { return ps1 }
        return String(ps1[ps1.index(after: nl)...])
    }

    private func sized(_ size: CGSize) {
        let f = theme.nsCodeFont
        let w = ("M" as NSString).size(withAttributes: [.font: f]).width
        let h = NSLayoutManager().defaultLineHeight(for: f)
        guard w > 0, h > 0 else { return }
        doc.setPaneSize(cols: Int((size.width - 16) / w), rows: Int(size.height / h))
    }

    /// clock is how long the History command has run, ticking each
    /// tenth of a second, or each second with reduced motion.
    private func clock(_ start: Date) -> some View {
        let calm = NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
        return TimelineView(.periodic(from: start, by: calm ? 1 : 0.1)) { t in
            (Text("running ").foregroundStyle(paneMuted) + Text(elapsed(t.date.timeIntervalSince(start), tenths: !calm)))
                .monospacedDigit()
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Time the command has run")
    }

    private func submit() {
        let line = input
        Task {
            if await doc.historyIsComplete(line) {
                input = ""
                if !line.trimmingCharacters(in: .whitespaces).isEmpty { past.append(line) }
                pastIndex = nil
                doc.historyEnter(line)
            } else {
                input += "\n"
            }
            focused = true
        }
    }

    private func recall(_ by: Int) -> KeyPress.Result {
        guard !past.isEmpty else { return .ignored }
        let i = (pastIndex ?? past.count) + by
        if i >= past.count {
            pastIndex = nil
            input = ""
        } else if i >= 0 {
            pastIndex = i
            input = past[i]
        }
        return .handled
    }

    private func complete() {
        let before = input
        Task {
            guard let c = await doc.historyCompletions(before), let items = c.items, !items.isEmpty else { return }
            if items.count == 1 {
                input = String(before.dropLast(c.word.count)) + items[0]
            } else {
                doc.historyTranscript += items.joined(separator: "  ") + "\n"
                let common = items.dropFirst().reduce(items[0]) { a, b in String(a.commonPrefix(with: b)) }
                if common.count > c.word.count { input = String(before.dropLast(c.word.count)) + common }
            }
        }
    }
}

/// ConsolePane lists what the app logged: errors and notes.
struct ConsolePane: View {
    var model: WindowModel
    @Environment(AppModel.self) private var app
    @Environment(\.theme) private var theme
    @State private var selection = Set<Int>()

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Console").font(theme.uiFont(size: theme.bodySize * 0.85, weight: .semibold))
                Spacer()
                Button("Copy") { copy() }
                Button("Clear") { app.clearConsole() }
                Button { model.showConsole = false } label: { Image(systemName: "xmark") }
                    .buttonStyle(.borderless)
                    .accessibilityLabel("Close Console")
            }
            .controlSize(.small)
            .padding(.horizontal, 10)
            .padding(.vertical, 4)
            .background(theme.surface)
            if app.console.isEmpty {
                Text("Errors and notes from mdsh show up here.")
                    .foregroundStyle(theme.muted)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List(app.console, selection: $selection) { e in
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Text(Self.clock(e.time)).foregroundStyle(theme.muted)
                        // mdsh-gui's colored three-letter tags.
                        Text(Self.tag(e.level))
                            .font(theme.codeFont(size: theme.codeSize * 0.85).weight(.bold))
                            .foregroundStyle(e.level == "error" ? theme.danger : e.level == "warn" ? .orange : theme.muted)
                            .accessibilityLabel(e.level)
                            .frame(width: 40, alignment: .leading)
                        let stale = e.stale == true
                        if let f = e.file {
                            let name = "\((f as NSString).lastPathComponent)\(e.line.map { ":\($0)" } ?? "")"
                            // A fixed warning's line may since have moved, so it is no link.
                            if stale {
                                Text(name).foregroundStyle(theme.muted)
                            } else {
                                Button(name) { goTo(f, e.line) }
                                    .buttonStyle(.link)
                            }
                        }
                        Text(e.message)
                            .strikethrough(stale)
                            .foregroundStyle(stale ? theme.muted : theme.text)
                            .textSelection(.enabled)
                        if stale {
                            Text("(fixed)").italic().foregroundStyle(theme.muted)
                        }
                        if let fix = e.fix, !stale, let f = e.file, f == model.doc?.file {
                            Spacer()
                            Button("Fix") { Task { await askFix(e.message) } }
                                .controlSize(.small)
                                .help(fix)
                                .accessibilityLabel("Fix: \(fix)")
                        }
                    }
                    .font(theme.codeFont(size: theme.codeSize * 0.9))
                }
                .listStyle(.plain)
            }
        }
        .onAppear { app.unseenProblems = 0 }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Console")
    }

    private func copy() {
        let entries = selection.isEmpty ? app.console : app.console.filter { selection.contains($0.seq) }
        let text = entries.map { "\($0.time) \(Self.tag($0.level)) \($0.message)\($0.stale == true ? " (fixed)" : "")" }.joined(separator: "\n")
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }

    /// askFix says what the fix for warning does, asks first, and makes
    /// it. Undo takes it back.
    private func askFix(_ warning: String) async {
        guard let d = model.doc else { return }
        if d.running {
            d.status = "can't change the file while a block runs"
            return
        }
        await d.sendText()
        let f: FixInfo
        do {
            f = try await app.tryCall("describeFix", ["doc": .init(d.id), "warning": .init(warning)])
        } catch {
            d.status = error.localizedDescription
            return
        }
        let alert = NSAlert()
        alert.messageText = f.label + "?"
        alert.informativeText = f.detail
        alert.addButton(withTitle: f.label)
        alert.addButton(withTitle: "Cancel")
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        await model.fixWarning(warning)
    }

    private func goTo(_ file: String, _ line: Int?) {
        if model.doc?.file == file {
            model.showSource = true
            model.sourceLine = line
        }
    }
}

/// editing is the text view that has the keyboard when it edits the
/// field under view: the window's field editor, or the field itself.
@MainActor
func editing(under view: NSView) -> NSTextView? {
    guard let win = view.window, let tv = win.firstResponder as? NSTextView,
          let edited = tv.isFieldEditor ? tv.delegate as? NSView : tv,
          edited.convert(edited.bounds, to: nil).intersects(view.convert(view.bounds, to: nil))
    else { return nil }
    return tv
}

/// ReadlineKeys gives the field it lies under the emacs keys of a shell's
/// prompt that macOS lacks (see Readline), as mdsh-gui's History prompt
/// has. It watches keys as they reach the window, so it sees option+b
/// before it types ∫.
struct ReadlineKeys: NSViewRepresentable {
    var eof: () -> Void
    var clear: () -> Void
    var prev: () -> Void
    var next: () -> Void

    func makeNSView(context: Context) -> ReadlineKeysView { ReadlineKeysView() }

    func updateNSView(_ v: ReadlineKeysView, context: Context) {
        v.eof = eof
        v.clear = clear
        v.prev = prev
        v.next = next
    }

    static func dismantleNSView(_ v: ReadlineKeysView, coordinator: ()) { v.stop() }
}

final class ReadlineKeysView: NSView {
    var eof: () -> Void = {}
    var clear: () -> Void = {}
    var prev: () -> Void = {}
    var next: () -> Void = {}
    private var readline = Readline()
    private var monitor: Any?

    override func hitTest(_ point: NSPoint) -> NSView? { nil }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        stop()
        guard window != nil else { return }
        // Local monitors run on the main thread, as this view does.
        monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] e in
            guard let self else { return e }
            return self.handle(e)
        }
    }

    func stop() {
        if let m = monitor { NSEvent.removeMonitor(m) }
        monitor = nil
    }

    /// The keys Readline takes, by their place on the keyboard.
    private static let names: [UInt16: String] = [
        11: "b", 3: "f", 2: "d", 13: "w", 32: "u", 40: "k", 16: "y", 37: "l", 35: "p", 45: "n", 51: "backspace",
    ]

    private func handle(_ e: NSEvent) -> NSEvent? {
        guard e.window === window, let tv = editing(under: self), !tv.hasMarkedText() else { return e }
        let f = e.modifierFlags.intersection([.control, .option, .command, .shift])
        guard f == .control || f == .option, let name = Self.names[e.keyCode],
              let a = readline.key(f == .control ? .ctrl(name) : .alt(name), text: tv.string, selection: tv.selectedRange())
        else {
            readline.other()
            return e
        }
        switch a {
        case .nothing: break
        case .move(let p): tv.setSelectedRange(NSRange(location: p, length: 0))
        case .replace(let r, let t):
            // Through the text view, so Undo takes it back and the field's
            // binding hears of it.
            if tv.shouldChangeText(in: r, replacementString: t) {
                tv.replaceCharacters(in: r, with: t)
                tv.didChangeText()
                tv.setSelectedRange(NSRange(location: r.location + (t as NSString).length, length: 0))
            }
        case .eof: eof()
        case .clear: clear()
        case .prev: prev()
        case .next: next()
        }
        return nil
    }
}

/// BlockCursor is the History prompt's block cursor, for Settings › Look
/// › History prompt focus set to the cursor: a block over the character
/// at the caret, as a terminal's, in the pane's text color with that
/// character in the pane's background. It stays solid while it moves,
/// blinks once it stops, and doesn't blink with Reduce Motion. It lies
/// over the field and follows the window's field editor while that
/// edits the field, hiding the editor's own caret.
struct BlockCursor: NSViewRepresentable {
    var fg: NSColor
    var bg: NSColor
    /// placeholder is the character under the block while the field is
    /// empty: the placeholder's first.
    var placeholder: String

    func makeNSView(context: Context) -> BlockCursorView { BlockCursorView() }

    func updateNSView(_ v: BlockCursorView, context: Context) {
        v.fg = fg
        v.bg = bg
        v.placeholder = placeholder
        v.place()
    }

    static func dismantleNSView(_ v: BlockCursorView, coordinator: ()) { v.stop() }
}

final class BlockCursorView: NSView {
    var fg = NSColor.textColor
    var bg = NSColor.textBackgroundColor
    var placeholder = ""
    private weak var editor: NSTextView?
    private var savedCaret: NSColor?
    private var rect: NSRect?
    private var under = " "
    private var font: NSFont?
    private var on = true
    private var timer: Timer?
    private var observers: [NSObjectProtocol] = []
    private static let blink: TimeInterval = 0.53

    override var isFlipped: Bool { true }
    override func hitTest(_ point: NSPoint) -> NSView? { nil }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        stop()
        guard window != nil else { return }
        let nc = NotificationCenter.default
        for name in [NSTextView.didChangeSelectionNotification, NSText.didChangeNotification,
                     NSWindow.didBecomeKeyNotification, NSWindow.didResignKeyNotification] {
            observers.append(nc.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.place() }
            })
        }
        // Focus moving has no notification, so the blink checks too.
        timer = Timer.scheduledTimer(withTimeInterval: Self.blink, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.tick() }
        }
        place()
    }

    override func layout() {
        super.layout()
        place()
    }

    func stop() {
        observers.forEach { NotificationCenter.default.removeObserver($0) }
        observers = []
        timer?.invalidate()
        timer = nil
        release()
    }

    private func tick() {
        place()
        on = NSWorkspace.shared.accessibilityDisplayShouldReduceMotion ? true : !on
        needsDisplay = true
    }

    /// field is the window's field editor while it edits the field under
    /// this view, with nothing selected and no input method composing.
    private func field() -> NSTextView? {
        guard let win = window, win.isKeyWindow, let tv = editing(under: self),
              !tv.hasMarkedText(), tv.selectedRange().length == 0
        else { return nil }
        return tv
    }

    func place() {
        let was = (rect, under)
        rect = nil
        if let tv = field(), let win = window {
            if editor !== tv {
                release()
                editor = tv
                savedCaret = tv.insertionPointColor
            }
            tv.insertionPointColor = .clear
            let s = tv.string as NSString
            let at = min(tv.selectedRange().location, s.length)
            // The character under the block: the one at the caret, the
            // placeholder's first while the field is empty, or a space.
            var ch = " "
            if at < s.length {
                let c = s.substring(with: s.rangeOfComposedCharacterSequence(at: at))
                if c != "\n" && c != "\t" { ch = c }
            } else if s.length == 0, let p = placeholder.first {
                ch = String(p)
            }
            let f = tv.font ?? NSFont.monospacedSystemFont(ofSize: NSFont.systemFontSize, weight: .regular)
            let caret = tv.firstRect(forCharacterRange: NSRange(location: at, length: 0), actualRange: nil)
            var r = convert(win.convertFromScreen(caret), from: nil)
            if r.height < 1 { r.size.height = NSLayoutManager().defaultLineHeight(for: f) }
            r.size.width = max(2, (ch as NSString).size(withAttributes: [.font: f]).width)
            rect = r
            under = ch
            font = f
        } else {
            release()
        }
        if was.0 != rect || was.1 != under {
            // Solid while it moves, as a terminal's.
            on = true
            timer?.fireDate = Date().addingTimeInterval(Self.blink)
            needsDisplay = true
        }
    }

    private func release() {
        if let tv = editor, let c = savedCaret {
            tv.insertionPointColor = c
        }
        editor = nil
        savedCaret = nil
        if rect != nil {
            rect = nil
            needsDisplay = true
        }
    }

    override func draw(_ dirtyRect: NSRect) {
        guard let r = rect, on, let f = font else { return }
        fg.setFill()
        r.fill()
        (under as NSString).draw(at: r.origin, withAttributes: [.font: f, .foregroundColor: bg])
    }
}

/// HistoryBar is the line across the bottom of a document window that
/// History opens from, as in mdsh-gui: what History is doing, Stop,
/// Restart, and the arrow that opens and closes the pane.
struct HistoryBar: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(\.theme) private var theme

    private var text: String {
        if doc.historyRunning { return "running…" }
        if doc.confirm != nil { return "waiting for you" }
        return "History: ⇧⌘H, or the arrow on the right"
    }

    var body: some View {
        HStack(spacing: 8) {
            // The shell's own prompt once History has one, as in mdsh-gui.
            Text(doc.info.history && !doc.prompt.ps1.isEmpty ? doc.prompt.ps1.trimmingCharacters(in: .whitespaces) : "$")
                .foregroundStyle(theme.muted)
                .lineLimit(1)
            Button(text) { model.toggleHistory() }
                .buttonStyle(.plain)
                .foregroundStyle(theme.muted)
                .lineLimit(1)
                .accessibilityHint("Opens or closes History")
            Spacer()
            Button { doc.stop() } label: { Image(systemName: "stop.fill") }
                .disabled(!doc.running)
                .help("Stop")
                .accessibilityLabel("Stop")
            Button { Task { _ = await doc.edit("restart") } } label: { Image(systemName: "arrow.clockwise") }
                .disabled(doc.running)
                .help("Start the shells again")
                .accessibilityLabel("Restart")
            Button { model.toggleHistory() } label: {
                Image(systemName: model.showHistory ? "chevron.down" : "chevron.up")
            }
            .help(model.showHistory ? "Close History" : "Open History")
            .accessibilityLabel(model.showHistory ? "Close History" : "Open History")
        }
        .buttonStyle(.borderless)
        .font(theme.codeFont(size: theme.bodySize * 0.8))
        .padding(.horizontal, 10)
        .frame(height: 24)
        .background(theme.surface)
        .overlay(alignment: .top) { Rectangle().fill(theme.border).frame(height: 1) }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("History bar")
    }
}

/// SplitPair lays out two views side by side with a grip between them
/// that moves the split, and a double-click on the grip that shares the
/// width evenly again.
struct SplitPair<Leading: View, Trailing: View>: View {
    @Binding var fraction: CGFloat
    var minLeading: CGFloat
    var minTrailing: CGFloat
    @ViewBuilder var leading: Leading
    @ViewBuilder var trailing: Trailing
    @Environment(\.theme) private var theme
    @State private var start: CGFloat?
    @State private var cursorPushed = false

    var body: some View {
        GeometryReader { geo in
            let w = geo.size.width
            let lw = clamp(w * fraction, w)
            HStack(spacing: 0) {
                leading.frame(width: lw)
                grip(w)
                trailing.frame(maxWidth: .infinity)
            }
        }
    }

    private func clamp(_ x: CGFloat, _ w: CGFloat) -> CGFloat {
        let hi = max(minLeading, w - minTrailing - 5)
        return min(max(x, minLeading), hi)
    }

    private func grip(_ w: CGFloat) -> some View {
        Rectangle()
            .fill(theme.border)
            .frame(width: 1)
            .padding(.horizontal, 2)
            .contentShape(Rectangle())
            .onHover { inside in
                if inside && !cursorPushed {
                    NSCursor.resizeLeftRight.push()
                    cursorPushed = true
                } else if !inside && cursorPushed {
                    NSCursor.pop()
                    cursorPushed = false
                }
            }
            // A grip removed under the pointer gets no hover end.
            .onDisappear {
                if cursorPushed {
                    NSCursor.pop()
                    cursorPushed = false
                }
            }
            .gesture(
                DragGesture(minimumDistance: 1)
                    .onChanged { g in
                        let s = start ?? w * fraction
                        if start == nil { start = s }
                        fraction = clamp(s + g.translation.width, w) / max(w, 1)
                    }
                    .onEnded { _ in start = nil }
            )
            .onTapGesture(count: 2) { fraction = 0.5 }
            .accessibilityElement()
            .accessibilityLabel("Split")
            .accessibilityValue("\(Int(fraction * 100)) percent source")
            .accessibilityAdjustableAction { dir in
                fraction = min(max(fraction + (dir == .increment ? 0.05 : -0.05), 0.1), 0.9)
            }
    }
}

extension ConsolePane {
    /// clock is an entry's time as mdsh-gui shows it, 12-hour with AM/PM.
    static func clock(_ iso: String) -> String {
        let date = isoFractional.date(from: iso) ?? isoPlain.date(from: iso)
        guard let date else { return String(iso.dropFirst(11).prefix(8)) }
        return clockFormat.string(from: date)
    }

    private static let isoFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
    private static let isoPlain = ISO8601DateFormatter()

    private static let clockFormat: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "h:mm:ss a"
        return f
    }()

    /// tag is a level's three-letter tag, as mdsh-gui's Console shows it.
    static func tag(_ level: String) -> String {
        switch level {
        case "error": "ERR"
        case "warn": "WRN"
        case "debug": "DBG"
        default: "INF"
        }
    }
}
