import AppKit
import MdshKit
import SwiftUI

/// RenderedView is the document drawn as a notebook: prose, and a card
/// for each block with its code and output.
struct RenderedView: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(\.theme) private var theme
    @FocusState private var focused: Bool
    /// following is false once the view is scrolled by hand during a
    /// run, so it stops jumping to the running block.
    @State private var following = true
    /// reopened are the folded sections opened for a block's question,
    /// to fold again once the run moves past it.
    @State private var reopened: Set<Int> = []
    @State private var reopenedFor: Int?

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                // The front matter form slides the page down, with its tab
                // at the form's bottom edge; only the page scrolls.
                if model.frontMatterOpen {
                    FrontMatterForm(model: model, doc: doc)
                        .transition(.move(edge: .top))
                    FrontMatterTab(model: model).padding(.top, -1)
                }
                // Lazy, as RichText's sizing needs the width a lazy stack
                // proposes; so a dependency line shows only while both of
                // its cards are laid out, near the screen.
                LazyVStack(alignment: .leading, spacing: 10) {
                    if !model.frontMatterOpen {
                        FrontMatterTab(model: model).padding(.top, -16)
                    }
                    ForEach(visibleCells) { cell in
                        CellView(model: model, doc: doc, cell: cell)
                            .id(cell.from)
                    }
                }
                .padding(.horizontal, theme.look == .tty && !theme.noSprockets ? 44 : 28)
                .padding(.vertical, 16)
                .frame(maxWidth: 980, alignment: .leading)
                .frame(maxWidth: .infinity)
                .backgroundPreferenceValue(CardAnchors.self) { anchors in
                    if model.showDepLines {
                        GeometryReader { geo in
                            DepLines(edges: DepEdge.of(doc.cells), cards: anchors.mapValues { geo[$0] },
                                     lit: model.selectedCell.flatMap { $0.isBlock ? $0.block : nil }, theme: theme)
                        }
                    }
                }
                .background { Paper(theme: theme) }
                .overlay(alignment: .topLeading) { depToggle }
            }
            .background(theme.background)
            .focusable()
            .focused($focused)
            .focusEffectDisabled()
            .onKeyPress(phases: .down) { press in key(press) }
            .onChange(of: model.selected) { _, s in
                if let s { withAnimation { proxy.scrollTo(s, anchor: .center) } }
            }
            .onChange(of: doc.current) { _, b in
                if !reopened.isEmpty, doc.confirm == nil, b != reopenedFor {
                    model.folded.formUnion(reopened)
                    reopened = []
                }
                guard following, let b, let c = doc.cell(block: b) else { return }
                withAnimation { proxy.scrollTo(c.from, anchor: .center) }
            }
            .onChange(of: doc.confirm) { _, q in
                guard let q, let c = doc.cell(block: q.block) else { return }
                // A section folded over the block opens for its question,
                // and the question itself goes to the middle of the view,
                // as mdsh-gui centers it rather than the card.
                let over = foldsOver(c)
                if !over.isEmpty {
                    model.folded.subtract(over)
                    reopened.formUnion(over)
                    reopenedFor = q.block
                }
                withAnimation { proxy.scrollTo(c.from, anchor: .center) }
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                    withAnimation { proxy.scrollTo(ConfirmView.anchor(q.block), anchor: .center) }
                }
            }
            .onChange(of: doc.running) { _, r in
                if !r, !reopened.isEmpty {
                    model.folded.formUnion(reopened)
                    reopened = []
                }
            }
            .modifier(StopFollowing(running: doc.running, following: $following))
            .onChange(of: doc.running) { _, r in if !r { following = true } }
        }
        .onAppear { focused = true }
    }

    /// depToggle hides the dependency lines and shows them again, at the
    /// top of the margin, when the file has any.
    @ViewBuilder
    private var depToggle: some View {
        if doc.cells.contains(where: { !($0.depends ?? []).isEmpty }) {
            Button { model.showDepLines.toggle() } label: {
                Image(systemName: "point.3.connected.trianglepath.dotted")
                    .foregroundStyle(model.showDepLines ? DepEdge.color(0, dark: theme.dark) : theme.muted)
            }
            .buttonStyle(.borderless)
            .padding(6)
            .help(model.showDepLines ? "Hide the dependency lines" : "Show the dependency lines")
            .accessibilityLabel(model.showDepLines ? "Hide the dependency lines" : "Show the dependency lines")
        }
    }

    /// foldsOver is the folded headings whose sections hold cell.
    private func foldsOver(_ cell: Cell) -> Set<Int> {
        var open: [Cell] = []  // the headings above cell, outermost first
        for c in doc.cells {
            if c.from >= cell.from { break }
            if c.isHeading {
                open.removeAll { $0.headingLevel >= c.headingLevel }
                open.append(c)
            }
        }
        return Set(open.map(\.from).filter { model.folded.contains($0) })
    }

    /// visibleCells leaves out the cells inside folded sections.
    private var visibleCells: [Cell] {
        var out: [Cell] = []
        var hideBelow: Int?  // a folded heading's level
        for c in doc.cells {
            if c.isHeading {
                if let h = hideBelow, c.headingLevel > h { continue }
                hideBelow = model.folded.contains(c.from) ? c.headingLevel : nil
                out.append(c)
                continue
            }
            if hideBelow != nil, c.hidden == nil { continue }
            out.append(c)
        }
        return out
    }

    // MARK: Keys

    /// typing says the keys go to text being edited: a field in the
    /// front matter form, a textbox or choice, the History input, or a
    /// sheet. The page's keys leave those alone, as mdsh-gui's do for
    /// input, textarea, select and dialog.
    static var typing: Bool {
        guard let w = NSApp.keyWindow else { return false }
        if w.attachedSheet != nil || w.isSheet { return true }
        // A field's editor is the window's NSTextView field editor.
        return w.firstResponder is NSText || w.firstResponder is NSTextField
    }

    private func key(_ press: KeyPress) -> KeyPress.Result {
        guard !Self.typing else { return .ignored }
        guard press.modifiers.subtracting([.shift, .numericPad]).isEmpty || press.modifiers.contains(.option) else { return .ignored }
        let sel = model.selectedCell
        if press.modifiers.contains(.option) {
            switch press.key {
            case .upArrow: model.moveSelected(dir: -1, byBlock: press.modifiers.contains(.shift)); return .handled
            case .downArrow: model.moveSelected(dir: 1, byBlock: press.modifiers.contains(.shift)); return .handled
            default: return .ignored
            }
        }
        switch press.key {
        case .return:
            guard let sel else { return .ignored }
            if sel.isBlock { doc.runBlock(sel.block) } else if sel.hasSection { doc.runSection(sel) }
            return .handled
        case .delete, .deleteForward:
            guard let sel, sel.isBlock, sel.movable == true else { return .ignored }
            if sel.history == true {
                Task { await doc.deleteBlock(sel.block) }
            } else if let why = sel.undeletable, !why.isEmpty {
                doc.status = why
            } else {
                model.deleteAsk = sel
            }
            return .handled
        case .escape:
            model.frontMatterOpen = false
            return .handled
        default:
            break
        }
        switch press.characters {
        case "j": model.moveSelection(1)
        case "k": model.moveSelection(-1)
        case "a": if let sel, sel.isBlock { doc.runFrom(sel.block) }
        case "s": model.runSection()
        case "c": model.copySelected()
        case "e": editInSource(sel)
        case "d": if sel?.isBlock == true { model.dependsOpen = true }
        default: return .ignored
        }
        return .handled
    }

    private func editInSource(_ c: Cell?) {
        guard c != nil else { return }
        model.showSource = true
        model.sourceLine = c?.line
    }
}

/// CellView draws one cell: prose, a heading, or a block's card.
struct CellView: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    var cell: Cell
    @Environment(\.theme) private var theme

    var body: some View {
        if let t = cell.textbox {
            TextboxCard(model: model, doc: doc, cell: cell, box: t)
        } else if let c = cell.choice {
            ChoiceCard(model: model, doc: doc, cell: cell, choice: c)
        } else if cell.isBlock {
            BlockCard(model: model, doc: doc, cell: cell)
        } else {
            prose
        }
    }

    @ViewBuilder
    private var prose: some View {
        if cell.isHeading {
            heading
        } else {
            if let items = TaskList.items(cell.html ?? "", count: cell.tasks?.count ?? 0) {
                TaskList(doc: doc, tasks: cell.tasks ?? [], items: items)
                    .onTapGesture(count: 2) {
                        model.showSource = true
                        model.sourceLine = cell.line
                    }
                    .help("Double-click to show it in the source")
            } else if let parts = DiagramHTML.parts(cell.html ?? "") {
                // d2 and svg fences, drawn as their pictures.
                VStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(parts.enumerated()), id: \.offset) { _, part in
                        switch part {
                        case .html(let h): HTMLView(html: h)
                        case .diagram(let f):
                            DiagramView(figure: f) { body, n in
                                Task { _ = await doc.edit("setCastFrame", ["block": .init(-1), "body": .init(body), "frame": .init(n)]) }
                            }
                        }
                    }
                }
                .onTapGesture(count: 2) {
                    model.showSource = true
                    model.sourceLine = cell.line
                }
                .help("Double-click to show it in the source")
            } else {
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    HTMLView(html: cell.html ?? "")
                        .onTapGesture(count: 2) {
                            model.showSource = true
                            model.sourceLine = cell.line
                        }
                        .help("Double-click to show it in the source")
                    if !(cell.tasks ?? []).isEmpty {
                        TaskToggles(doc: doc, tasks: cell.tasks ?? [], html: cell.html ?? "")
                    }
                }
            }
        }
    }

    /// heading is a heading: its fold chevron in the left margin on the
    /// baseline, its text, and the section's play button right after
    /// it, as in mdsh-gui.
    private var heading: some View {
        let folded = model.folded.contains(cell.from)
        let level = cell.headingLevel
        let scale: CGFloat = [1.9, 1.5, 1.25, 1.1, 1, 0.9][min(max(level, 1), 6) - 1]
        // A heading of plain text is drawn as Text, so the play button
        // can sit right after it; one with code, emphasis or a link in
        // it goes through the HTML import to keep them.
        let inner = Self.headingInner(cell.html ?? "")
        let rich = inner.contains("<")
        var title = Self.unescape(inner).trimmingCharacters(in: .whitespacesAndNewlines)
        if theme.look == .tty { title = String(repeating: "#", count: level) + " " + title }
        return HStack(alignment: .firstTextBaseline, spacing: 6) {
            Button {
                model.folded.formSymmetricDifference([cell.from])
                Task { await doc.setFold(block: -1, heading: cell.from, on: model.folded.contains(cell.from)) }
            } label: {
                Image(systemName: folded ? "chevron.right" : "chevron.down")
                    .font(.system(size: theme.bodySize * 0.8))
                    .foregroundStyle(theme.muted)
                    .frame(width: 16)
            }
            .buttonStyle(.borderless)
            .help(folded ? "Unfold this section" : "Fold this section")
            .accessibilityLabel(folded ? "Unfold section" : "Fold section")
            Group {
                if rich {
                    // Teletype keeps its #s on a heading with markup too.
                    HTMLView(html: theme.look == .tty ? Self.withHashes(cell.html ?? "", level) : cell.html ?? "")
                } else {
                    Text(title)
                        .font(theme.uiFont(size: theme.bodySize * scale, weight: theme.look == .gum ? .black : .bold))
                        .textCase(theme.look == .blueprint ? .uppercase : nil)
                        // Bubble Gum's headings are bubble letters, with a
                        // pop of pink behind them.
                        .shadow(color: theme.look == .gum && level <= 3 ? theme.gumPop : .clear, radius: 0, x: 2, y: 2)
                        .textSelection(.enabled)
                        .accessibilityAddTraits(.isHeader)
                }
            }
            .onTapGesture(count: 2) {
                model.showSource = true
                model.sourceLine = cell.line
            }
            .help("Double-click to show it in the source")
            if cell.hasSection {
                Button { doc.runSection(cell) } label: { Image(systemName: "play.fill").font(.system(size: theme.bodySize * 0.7)) }
                    .buttonStyle(.borderless)
                    .foregroundStyle(theme.accent)
                    .help("Run this section (s)")
                    .accessibilityLabel("Run this section")
            }
            Spacer(minLength: 0)
        }
        .padding(.top, level <= 2 ? 8 : 4)
        .padding(.bottom, 2)
        .background(model.selected == cell.from ? theme.accent.opacity(0.08) : .clear)
        // The hit shape takes in the chevron before it is moved into the
        // margin: a content shape set after would end at the heading's
        // text and leave the chevron unclickable.
        .contentShape(Rectangle())
        .onTapGesture { model.selected = cell.from }
        // The chevron hangs in the margin, so the heading lines up with
        // the prose.
        .padding(.leading, -22)
    }

    /// headingInner is what is inside a heading's <hN> tag.
    static func headingInner(_ html: String) -> String {
        guard let open = html.range(of: #"<h[1-6][^>]*>"#, options: .regularExpression),
              let close = html.range(of: #"</h[1-6]>"#, options: [.regularExpression, .backwards]),
              open.upperBound <= close.lowerBound
        else { return html }
        return String(html[open.upperBound..<close.lowerBound])
    }

    static func withHashes(_ html: String, _ level: Int) -> String {
        guard let open = html.range(of: #"<h[1-6][^>]*>"#, options: .regularExpression) else { return html }
        return html.replacingCharacters(in: open, with: html[open] + String(repeating: "#", count: level) + " ")
    }

    static func unescape(_ s: String) -> String {
        CFXMLCreateStringByUnescapingEntities(nil, s as CFString, nil) as String? ?? s
    }
}

/// TaskList draws prose or output that is only a task list as
/// checkboxes with their text, in place, as mdsh-gui does, since the
/// HTML import can't draw a checkbox.
struct TaskList: View {
    var doc: DocumentModel
    var tasks: [Int]
    var items: [(checked: Bool, text: String)]
    var mono = false
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            ForEach(Array(items.enumerated()), id: \.offset) { i, item in
                Toggle(isOn: Binding(get: { item.checked }, set: { v in
                    if i < tasks.count { Task { await doc.setTask(tasks[i], done: v) } }
                })) {
                    Text(item.text)
                        .font(mono ? theme.codeFont() : theme.uiFont())
                }
                .toggleStyle(.checkbox)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// items reads html as a task list: one item per checkbox, with its
    /// text. It is nil unless html is a list of exactly count task
    /// items and nothing else, which the HTML import draws instead.
    static func items(_ html: String, count: Int) -> [(checked: Bool, text: String)]? {
        guard count > 0 else { return nil }
        // The notebook leaves comments where it drops raw HTML.
        let trimmed = html.replacingOccurrences(of: "<!--[\\s\\S]*?-->", with: "", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.hasPrefix("<ul"), trimmed.hasSuffix("</ul>"),
              trimmed.components(separatedBy: "<ul").count == 2
        else { return nil }
        let re = try! NSRegularExpression(pattern: #"<li[^>]*>(.*?)</li>"#, options: [.dotMatchesLineSeparators])
        let ns = trimmed as NSString
        var out: [(checked: Bool, text: String)] = []
        for m in re.matches(in: trimmed, range: NSRange(location: 0, length: ns.length)) {
            let li = ns.substring(with: m.range(at: 1))
            guard let input = li.range(of: #"<input[^>]*>"#, options: .regularExpression) else { return nil }
            let tag = li[input]
            guard tag.contains("checkbox") else { return nil }
            // Only plain text: an item with a link or code keeps its
            // markup through the HTML import instead.
            let rest = li.replacingCharacters(in: input, with: "")
            guard !rest.contains("<") else { return nil }
            let text = rest
            out.append((tag.contains("checked"), CellView.unescape(text).trimmingCharacters(in: .whitespacesAndNewlines)))
        }
        return out.count == count ? out : nil
    }
}

/// TaskToggles lists a cell's task list items as checkboxes, which the
/// HTML import can't draw, so they can be checked off.
struct TaskToggles: View {
    var doc: DocumentModel
    var tasks: [Int]
    var html: String

    var body: some View {
        let states = TaskToggles.states(html)
        VStack(alignment: .leading, spacing: 2) {
            ForEach(Array(tasks.enumerated()), id: \.offset) { i, t in
                let on = i < states.count ? states[i] : false
                Toggle(isOn: Binding(get: { on }, set: { v in Task { await doc.setTask(t, done: v) } })) { EmptyView() }
                    .toggleStyle(.checkbox)
                    .accessibilityLabel("Task \(i + 1)")
            }
        }
    }

    /// states reads the checked state of each task checkbox in html.
    static func states(_ html: String) -> [Bool] {
        var out: [Bool] = []
        var rest = html[...]
        while let r = rest.range(of: "<input") {
            let tail = rest[r.upperBound...]
            let end = tail.firstIndex(of: ">") ?? tail.endIndex
            let tag = tail[..<end]
            if tag.contains("checkbox") { out.append(tag.contains("checked")) }
            rest = tail[end...]
        }
        return out
    }
}

/// StopFollowing stops the view following a run once it is scrolled by
/// hand, where the system can say so (macOS 15).
private struct StopFollowing: ViewModifier {
    var running: Bool
    @Binding var following: Bool

    func body(content: Content) -> some View {
        if #available(macOS 15.0, *) {
            content.onScrollPhaseChange { _, phase in
                if phase == .interacting, running { following = false }
            }
        } else {
            content
        }
    }
}

/// FrontMatterTab is the tab that opens the front matter form: at the
/// top edge of the page while it is closed, and at the form's bottom
/// edge while it is open, as in mdsh-gui.
struct FrontMatterTab: View {
    @Bindable var model: WindowModel
    @Environment(\.theme) private var theme

    var body: some View {
        HStack {
            Spacer()
            Button {
                withAnimation(.easeOut(duration: 0.2)) { model.frontMatterOpen.toggle() }
            } label: {
                HStack(spacing: 4) {
                    Text("Front matter")
                    Image(systemName: model.frontMatterOpen ? "chevron.up" : "chevron.down")
                }
                .font(theme.uiFont(size: theme.bodySize * 0.8))
                .foregroundStyle(theme.muted)
                .padding(.horizontal, 12)
                .padding(.vertical, 3)
                .background(theme.surface, in: UnevenRoundedRectangle(bottomLeadingRadius: 6, bottomTrailingRadius: 6))
                .overlay(UnevenRoundedRectangle(bottomLeadingRadius: 6, bottomTrailingRadius: 6).stroke(theme.border))
            }
            .buttonStyle(.plain)
            .help("Front matter (⇧⌘M)")
            .accessibilityAddTraits(model.frontMatterOpen ? .isSelected : [])
            Spacer()
        }
    }
}
