import AppKit
import MdshKit
import SwiftUI
import UniformTypeIdentifiers

/// BlockCard is a block: its name, language and status, its code, the
/// question waiting on it, and its output, live while it runs.
struct BlockCard: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    var cell: Cell
    @Environment(\.theme) private var theme
    @State private var hover = false
    /// shownUntil is when a secret block shown with its eye button masks
    /// itself again.
    @State private var shownUntil: Date?
    @State private var copied = false

    private var selected: Bool { model.selected == cell.from }
    /// masked is set while a secret block's code (with secretCode) and
    /// output are blurred.
    private var masked: Bool { cell.secret == true && shownUntil == nil }
    private var codeFolded: Bool { model.folded.contains(cell.from) }
    private var outputFolded: Bool { model.foldedOutput.contains(cell.from) }
    private var isRunning: Bool { doc.current == cell.block || doc.background.contains(cell.block) || cell.running }
    private var background: Bool { (cell.attrs ?? "").split(separator: " ").contains("background") }
    /// edge is the state mdsh-gui's card carries in its left border.
    private var edge: CardFrame.Edge? {
        if doc.runStarts[cell.block] != nil { return .timing }
        if cell.running || doc.background.contains(cell.block) { return .background }
        if cell.failed { return .failed }
        if cell.ran { return cell.stale == true ? .stale : .ran }
        return nil
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            if model.showCode && !codeFolded {
                if masked && cell.secretCode == true {
                    Masked(text: cell.body ?? "").padding(.horizontal, 10).padding(.vertical, 8)
                } else {
                    CodeView(code: cell.body ?? "", lang: cell.lang ?? "", firstLine: cell.firstLine ?? 1)
                }
            }
            if let q = doc.confirm, q.block == cell.block {
                ConfirmView(doc: doc, question: q)
                    .id(ConfirmView.anchor(q.block))
            }
            if let t = doc.terminal[cell.block] {
                TerminalView(doc: doc, text: t)
            }
            if model.showOutput { output }
        }
        .modifier(CardFrame(theme: theme, selected: selected, dashed: background || cell.hidden != nil, edge: edge))
        .anchorPreference(key: CardAnchors.self, value: .bounds) { [cell.block: $0] }
        .modifier(DepRings(rings: depRings, theme: theme))
        .contentShape(Rectangle())
        .onTapGesture { model.selected = cell.from }
        .onHover { hover = $0 }
        // A shown secret masks itself again when the window loses focus.
        .onReceive(NotificationCenter.default.publisher(for: NSWindow.didResignKeyNotification)) { n in
            if (n.object as? NSWindow) === model.window { shownUntil = nil }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(accessibilityName)
        .contextMenu { menu }
        .draggable(BlockDrag(block: cell.block)) {
            Text(cell.displayName).padding(6).background(theme.surface)
        }
        .dropDestination(for: BlockDrag.self) { items, _ in
            guard let d = items.first, d.block != cell.block, cell.drop == true else { return false }
            Task {
                if let m = await doc.moveTo(d.block, at: cell.from) { model.showMoved(m) }
            }
            return true
        }
    }

    /// depRings are the k of each of the selected block's depends= that
    /// names this block, to outline it in their colors.
    private var depRings: [Int] {
        guard let sel = model.selectedCell, sel.isBlock, sel.block != cell.block, !cell.blockID.isEmpty else { return [] }
        return (sel.depends ?? []).enumerated().filter { $0.element == cell.blockID }.map(\.offset)
    }

    static let staleNote = "A block this depends on ran again or was cleared since this ran. Running a block that depends on this one runs it again first."

    private var accessibilityName: String {
        var parts = [cell.displayName, cell.lang ?? ""]
        if let a = cell.attrs, !a.isEmpty { parts.append(a) }
        if isRunning { parts.append("running") } else if cell.ran { parts.append(cell.failed ? "failed, status \(cell.status)" : "ran") }
        if cell.stale == true { parts.append("stale") }
        return parts.filter { !$0.isEmpty }.joined(separator: ", ")
    }

    // MARK: Header

    private var header: some View {
        HStack(spacing: 6) {
            // mdsh-gui's grip: first in the head, shown on hover or when
            // selected; the card drags by any of it.
            if cell.movable == true && !isRunning {
                Text("⋮⋮")
                    .font(theme.uiFont(size: 12 * theme.textScale))
                    .kerning(-2)
                    .foregroundStyle(theme.muted)
                    .opacity(hover || selected ? 1 : 0)
                    .help("Drag to move this block (⌥↑ or ⌥↓)")
                    .accessibilityHidden(true)
            }
            Button {
                model.folded.formSymmetricDifference([cell.from])
                Task { await doc.setFold(block: cell.block, on: model.folded.contains(cell.from)) }
            } label: {
                Image(systemName: codeFolded ? "chevron.right" : "chevron.down").foregroundStyle(theme.muted)
            }
            .buttonStyle(.borderless)
            .help(codeFolded ? "Show the code" : "Fold the code")
            .accessibilityLabel(codeFolded ? "Show code" : "Fold code")

            Text(cell.displayName)
                .font(theme.uiFont(weight: .semibold))
                .lineLimit(1)
            if let lang = cell.lang, !lang.isEmpty {
                // Teletype brackets the language and attributes; Blueprint
                // letters the language in capitals.
                if theme.look == .tty {
                    Text("[\(lang)]").font(theme.codeFont(size: theme.codeSize)).foregroundStyle(theme.muted)
                } else {
                    Badge(text: theme.look == .blueprint ? lang.uppercased() : lang, color: theme.look == .blueprint ? theme.text : theme.muted)
                }
            }
            if let a = cell.attrs, !a.isEmpty {
                Text(theme.look == .tty ? "[\(a)]" : a).font(theme.codeFont(size: theme.codeSize * 0.9)).foregroundStyle(theme.muted).lineLimit(1)
            }
            if cell.hidden != nil {
                Badge(text: "hidden when printed", color: theme.muted).help(cell.hidden ?? "")
            }
            if cell.nosave == true {
                NosaveBadge(kind: .block)
            }
            Spacer(minLength: 8)
            if cell.stale == true {
                // mdsh-gui's stale badge, right before the exit: its run is
                // out of date.
                Text("stale")
                    .font(theme.uiFont(size: theme.bodySize * 0.85))
                    .padding(.horizontal, 8)
                    .background(theme.warnBackground, in: Capsule())
                    .overlay(Capsule().stroke(theme.warnBorder))
                    .help(Self.staleNote)
                    .accessibilityLabel("Stale: " + Self.staleNote)
            }
            // The exit status sits at the right, before the buttons, then
            // the run timer: how long a running block has run, or, muted,
            // how long its last run took, as mdsh-gui shows them.
            if isRunning {
                if background { Text("running in the background").foregroundStyle(theme.muted).font(theme.uiFont(size: theme.bodySize * 0.85)) }
                if let start = doc.runStarts[cell.block] {
                    RunTimer(start: start, timeout: cell.timeout ?? 0)
                } else {
                    ProgressView().controlSize(.small)
                }
            } else if cell.ran {
                Text("exit \(cell.status)")
                    .font(theme.codeFont(size: theme.codeSize * 0.85))
                    .foregroundStyle(cell.failed ? theme.danger : theme.ok)
                    .accessibilityLabel(cell.failed ? "Failed, exit status \(cell.status)" : "Ran, exit status 0")
                if let ms = cell.ms, ms > 0 { tookLabel(ms) }
            }
            buttons
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        // With timeout=, a bar under the head fills toward it.
        .overlay(alignment: .bottomLeading) {
            if isRunning, let t = cell.timeout, t > 0, let start = doc.runStarts[cell.block] {
                TimeoutBar(start: start, timeout: t)
            }
        }
    }

    /// tookLabel is how long a finished block's last run took, or that
    /// its timeout ended it.
    @ViewBuilder
    private func tookLabel(_ ms: Int) -> some View {
        let limit = cell.timeout ?? 0
        let secs = Double(ms) / 1000
        if cell.timedOut == true {
            Text("timed out at " + limitTime(limit > 0 ? limit : ms))
                .font(theme.codeFont(size: theme.codeSize * 0.9))
                .foregroundStyle(theme.danger)
                .help("Timed out after " + elapsed(secs))
        } else {
            let title = "Took " + elapsed(secs) + (limit > 0 ? ", of a " + limitTime(limit) + " timeout" : "")
            Text(elapsed(secs))
                .font(theme.codeFont(size: theme.codeSize * 0.9))
                .monospacedDigit()
                .foregroundStyle(theme.muted)
                .help(title)
                .accessibilityLabel(title)
        }
    }

    private var buttons: some View {
        HStack(spacing: 2) {
            if cell.secret == true {
                iconButton(masked ? "eye" : "eye.slash", masked ? "Show the secret" : "Hide the secret") {
                    if masked { show() } else { shownUntil = nil }
                }
            }
            iconButton("doc.on.doc", "Copy (c)") {
                Clipboard.copy(cell.body ?? "", secret: cell.secretCode == true)
                doc.status = "copied \(cell.displayName)"
            }
            iconButton("pencil", "Edit in source (e)") {
                model.showSource = true
                model.sourceLine = cell.line
            }
            iconButton("play.fill", "Run (enter)") { doc.runBlock(cell.block) }
                .accessibilityLabel("Run \(cell.displayName)")
                .disabled(doc.running && doc.confirm?.block != cell.block)
            iconButton("forward.end", "Run from here (a)") { doc.runFrom(cell.block) }
                .disabled(doc.running)
            // Delete shows on the selected card only, and always on
            // History's blocks, as in mdsh-gui.
            if cell.movable == true && (selected || cell.history == true) {
                iconButton("trash", cell.undeletable?.isEmpty == false ? cell.undeletable! : "Delete (⌫)") {
                    if cell.history == true {
                        Task { await doc.deleteBlock(cell.block) }
                    } else {
                        model.deleteAsk = cell
                    }
                }
                .disabled(cell.undeletable?.isEmpty == false)
            }
        }
        .opacity(hover || selected ? 1 : 0.75)
    }

    private func iconButton(_ symbol: String, _ help: String, action: @escaping () -> Void) -> some View {
        Button(action: action) { Image(systemName: symbol).foregroundStyle(theme.muted) }
            .buttonStyle(.borderless)
            .help(help)
            .accessibilityLabel(help.replacingOccurrences(of: #" \(.*\)$"#, with: "", options: .regularExpression))
    }

    // MARK: Output

    @ViewBuilder
    private var output: some View {
        let live = doc.live[cell.block]
        let rendered = cell.output ?? ""
        // A tty block's terminal takes the output's place, as in
        // mdsh-gui, until the card is drawn again with its recording.
        let term = doc.terminal[cell.block] != nil
        let liveShown = live != nil && !term
        if liveShown || (!rendered.isEmpty && !term) {
            Divider().overlay(theme.border)
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Button {
                        model.foldedOutput.formSymmetricDifference([cell.from])
                        Task { await doc.setFold(block: cell.block, output: true, on: model.foldedOutput.contains(cell.from)) }
                    } label: {
                        Image(systemName: outputFolded ? "chevron.right" : "chevron.down").foregroundStyle(theme.muted)
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel(outputFolded ? "Show output" : "Fold output")
                    Text(theme.look == .tty ? "-- output --" : "OUTPUT")
                        .font(theme.look == .tty ? theme.codeFont(size: theme.codeSize * 0.85) : theme.uiFont(size: theme.bodySize * 0.72, weight: .semibold))
                        .kerning(theme.look == .tty ? 0 : 0.6)
                        .foregroundStyle(theme.muted)
                        .accessibilityLabel("Output")
                    Spacer()
                    // mdsh-gui's copy button, in the output's top right
                    // corner while the card is hovered or selected.
                    if hover || selected || copied {
                        Button {
                            let text = liveShown ? (live ?? "") : HTMLCache.attributed(rendered, css: "").string
                            Clipboard.copy(text, secret: cell.secret == true)
                            copied = true
                            Task {
                                try? await Task.sleep(for: .seconds(1.5))
                                copied = false
                            }
                        } label: {
                            if copied {
                                Text("Copied").font(theme.uiFont(size: theme.bodySize * 0.75)).foregroundStyle(theme.muted)
                            } else {
                                Image(systemName: "doc.on.doc").foregroundStyle(theme.muted)
                            }
                        }
                        .buttonStyle(.borderless)
                        .help("Copy the output as text")
                        .accessibilityLabel(copied ? "Copied" : "Copy output")
                    }
                    if live == nil {
                        Button { Task { await doc.clearOutput(cell.block) } } label: { Image(systemName: "trash").foregroundStyle(theme.muted) }
                            .buttonStyle(.borderless)
                            .help("Clear this output")
                            .accessibilityLabel("Clear output")
                    }
                }
                if !outputFolded && masked {
                    Masked(text: liveShown ? (live ?? "") : HTMLCache.attributed(rendered, css: "").string)
                } else if !outputFolded {
                    VStack(alignment: .leading, spacing: 4) {
                    if let live, liveShown {
                        Text(live.isEmpty ? " " : live)
                            .font(theme.codeFont())
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    } else if let src = CastPlayer.source(in: rendered) {
                        // A tty block's recording plays in the card.
                        CastPlayer(src: src) { n in
                            Task { _ = await doc.edit("setCastFrame", ["block": .init(cell.block), "body": .init(""), "frame": .init(n)]) }
                        }
                    } else if let items = TaskList.items(rendered, count: cell.outputTasks?.count ?? 0) {
                        TaskList(doc: doc, tasks: cell.outputTasks ?? [], items: items, mono: true)
                    } else {
                        HTMLView(html: rendered, mono: true, onLink: { url in
                            // Links in output open only with the links attribute.
                            cell.links != true
                        })
                        if !(cell.outputTasks ?? []).isEmpty {
                            TaskToggles(doc: doc, tasks: cell.outputTasks ?? [], html: rendered)
                        }
                    }
                    }
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(theme.codeBackground.opacity(0.5))
        }
    }

    /// show unmasks a secret block until Settings › Editing › Show
    /// secrets for has passed.
    private func show() {
        let until = Date().addingTimeInterval(AppModel.shared.settings.secretShow)
        shownUntil = until
        Task {
            try? await Task.sleep(for: .seconds(AppModel.shared.settings.secretShow))
            if shownUntil == until { shownUntil = nil }
        }
    }

    // MARK: Menu

    @ViewBuilder
    private var menu: some View {
        Button("Run") { doc.runBlock(cell.block) }
        Button("Run From Here") { doc.runFrom(cell.block) }
        Button("Copy Code") { Clipboard.copy(cell.body ?? "", secret: cell.secretCode == true) }
        Button("Edit in Source") {
            model.showSource = true
            model.sourceLine = cell.line
        }
        Divider()
        Button("Depends On…") {
            model.selected = cell.from
            model.dependsOpen = true
        }
        if cell.movable == true {
            Button("Move Up") { model.selected = cell.from; model.moveSelected(dir: -1, byBlock: false) }
            Button("Move Down") { model.selected = cell.from; model.moveSelected(dir: 1, byBlock: false) }
            Divider()
            Button("Delete…", role: .destructive) { model.deleteAsk = cell }
                .disabled(cell.undeletable?.isEmpty == false)
        }
    }
}

/// BlockDrag is a block being dragged to a new place.
struct BlockDrag: Codable, Transferable {
    var block: Int
    static var transferRepresentation: some TransferRepresentation {
        CodableRepresentation(contentType: .data)
    }
}

/// Badge is a small label on a card's header.
/// Masked stands in for a secret block's code or output until its eye
/// button shows it: the text's shape, each character a dot, blurred, so
/// the secret itself is never drawn, selected or read out.
struct Masked: View {
    var text: String
    @Environment(\.theme) private var theme

    var body: some View {
        Text(String(text.prefix(4000).map { $0.isWhitespace ? $0 : "•" }))
            .font(theme.codeFont())
            .foregroundStyle(theme.muted)
            .blur(radius: 3)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityLabel("Secret, hidden")
    }
}

/// NosaveBadge marks a block or input whose value stays in the session:
/// saving leaves what the file holds.
struct NosaveBadge: View {
    enum Kind { case block, textbox, choice }
    var kind: Kind
    @Environment(\.theme) private var theme

    private var note: String {
        switch kind {
        case .block: "Marked nosave: its output lives in this session. Saving keeps what the file holds after it."
        case .textbox: "Marked nosave: what you type lives in this session. Saving keeps the file's text."
        case .choice: "Marked nosave: what you pick lives in this session. Saving keeps the file's marks."
        }
    }

    var body: some View {
        Badge(text: "session only", color: theme.muted)
            .help(note)
            .accessibilityLabel("Session only: " + note)
    }
}

struct Badge: View {
    var text: String
    var color: Color
    @Environment(\.theme) private var theme

    var body: some View {
        Text(text)
            .font(theme.uiFont(size: theme.bodySize * 0.75, weight: .medium))
            .padding(.horizontal, 6)
            .padding(.vertical, 1)
            .foregroundStyle(color)
            .overlay {
                if theme.look == .blueprint {
                    Rectangle().stroke(color)
                } else {
                    Capsule().stroke(color.opacity(0.6))
                }
            }
    }
}

/// CardFrame is a block card's edge in each look: plain's rounded card
/// with a soft shadow, Teletype's hard ink edge with an offset shadow
/// (in ink when selected), and Blueprint's transparent card the grid
/// shows through, with a dashed outline when selected.
struct CardFrame: ViewModifier {
    var theme: Theme
    var selected: Bool
    var dashed: Bool
    var edge: Edge? = nil

    /// Edge is mdsh-gui's 3px left border: green once the block ran,
    /// red when it failed, warn when its run is stale, accent while its
    /// run timer goes, and dashed accent while it runs in the background
    /// without one.
    enum Edge { case ran, failed, stale, background, timing }

    @ViewBuilder
    private var leftEdge: some View {
        switch edge {
        case .ran?: Rectangle().fill(theme.ok).frame(width: 3)
        case .failed?: Rectangle().fill(theme.danger).frame(width: 3)
        case .stale?: Rectangle().fill(theme.warnBorder).frame(width: 3)
        case .timing?: Rectangle().fill(theme.accent).frame(width: 3)
        case .background?: VLine().stroke(theme.accent, style: StrokeStyle(lineWidth: 3, dash: [6, 3])).frame(width: 3)
        case nil: EmptyView()
        }
    }

    func body(content: Content) -> some View {
        let noShadows = AppModel.shared.settings.noShadows
        switch theme.look {
        case .plain:
            content
                .background(theme.surface)
                .overlay(alignment: .leading) { leftEdge }
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay {
                    RoundedRectangle(cornerRadius: 8)
                        .strokeBorder(selected ? theme.accent : theme.border,
                                      style: StrokeStyle(lineWidth: selected ? 2 : 1, dash: dashed ? [5, 3] : []))
                }
        case .tty:
            content
                .background(theme.surface)
                .overlay { Rectangle().strokeBorder(theme.text, style: StrokeStyle(lineWidth: 1, dash: dashed ? [5, 3] : [])) }
                .overlay(alignment: .leading) { leftEdge }
                .background {
                    if !noShadows || selected {
                        Rectangle()
                            .fill(selected ? theme.text : theme.color(theme.dark ? "#1f3020" : "#cfc5a5"))
                            .offset(x: selected ? 5 : 4, y: selected ? 5 : 4)
                    }
                }
        case .gum:
            // Puffy: a pale pink fill, a two-point edge and a chunky
            // shadow underneath, the selected card ringed in the accent.
            content
                .background(theme.surface)
                .overlay(alignment: .leading) { leftEdge.frame(width: 4) }
                .clipShape(RoundedRectangle(cornerRadius: 16))
                .overlay {
                    RoundedRectangle(cornerRadius: 16)
                        .strokeBorder(theme.border, style: StrokeStyle(lineWidth: 2, dash: dashed ? [5, 3] : []))
                }
                .background {
                    if !noShadows { RoundedRectangle(cornerRadius: 16).fill(theme.gumShadow).offset(y: 4) }
                }
                .overlay {
                    if selected { RoundedRectangle(cornerRadius: 18).stroke(theme.accent, lineWidth: 3).padding(-4) }
                }
        case .witch:
            // A grimoire page: the pale card with a soft violet glow
            // around it, the selected one ringed in the amethyst.
            content
                .background(theme.surface)
                .overlay(alignment: .leading) { leftEdge }
                .clipShape(RoundedRectangle(cornerRadius: 4))
                .overlay {
                    RoundedRectangle(cornerRadius: 4)
                        .strokeBorder(theme.border, style: StrokeStyle(lineWidth: 1, dash: dashed ? [5, 3] : []))
                }
                .shadow(color: noShadows ? .clear : theme.witchGlow, radius: 9, y: 4)
                .overlay {
                    if selected { RoundedRectangle(cornerRadius: 6).stroke(theme.accent, lineWidth: 2).padding(-4) }
                }
        case .blueprint:
            content
                .overlay { Rectangle().strokeBorder(theme.text, style: StrokeStyle(lineWidth: 1, dash: dashed ? [5, 3] : [])) }
                .overlay(alignment: .leading) { leftEdge }
                .overlay {
                    if selected {
                        Rectangle().stroke(theme.accent, style: StrokeStyle(lineWidth: 2, dash: [6, 4])).padding(-4)
                    }
                }
        }
    }
}

/// RunTimer is a running block's spinner and time, ticking each tenth
/// of a second, against its timeout when it has one: accent, then amber
/// at three quarters of it and red at nine tenths.
struct RunTimer: View {
    var start: Date
    var timeout: Int
    @Environment(\.theme) private var theme

    var body: some View {
        TimelineView(.periodic(from: start, by: 0.1)) { t in
            let secs = t.date.timeIntervalSince(start)
            let color = TimeoutBar.color(theme, secs: secs, timeout: timeout)
            let title = "Running for " + elapsed(secs) + (timeout > 0 ? ", of a " + limitTime(timeout) + " timeout" : "")
            HStack(spacing: 6) {
                ProgressView().controlSize(.mini).tint(color)
                Text(timeout > 0 ? elapsed(secs) + " / " + limitTime(timeout) : elapsed(secs))
                    .font(theme.codeFont(size: theme.codeSize * 0.9).weight(.semibold))
                    .monospacedDigit()
                    .foregroundStyle(color)
            }
            .help(title)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(title)
        }
    }
}

/// TimeoutBar fills along the bottom of a running block's head toward
/// its timeout.
struct TimeoutBar: View {
    var start: Date
    var timeout: Int
    @Environment(\.theme) private var theme

    static func color(_ theme: Theme, secs: TimeInterval, timeout: Int) -> Color {
        guard timeout > 0 else { return theme.accent }
        let f = secs * 1000 / Double(timeout)
        return f >= 0.9 ? theme.danger : f >= 0.75 ? theme.warnBorder : theme.accent
    }

    var body: some View {
        TimelineView(.periodic(from: start, by: 0.1)) { t in
            let secs = t.date.timeIntervalSince(start)
            GeometryReader { g in
                Rectangle()
                    .fill(Self.color(theme, secs: secs, timeout: timeout))
                    .frame(width: g.size.width * min(secs * 1000 / Double(timeout), 1), height: 2)
                    .frame(maxHeight: .infinity, alignment: .bottom)
            }
            .frame(height: 2)
        }
        .accessibilityHidden(true)
    }
}

/// VLine is a vertical line down the middle of its frame.
struct VLine: Shape {
    func path(in r: CGRect) -> Path {
        Path { p in
            p.move(to: CGPoint(x: r.midX, y: r.minY))
            p.addLine(to: CGPoint(x: r.midX, y: r.maxY))
        }
    }
}

/// ConfirmView asks, in the card, whether to run the block.
struct ConfirmView: View {
    /// anchor names a block's question, for scrolling to it.
    static func anchor(_ block: Int) -> String { "confirm-\(block)" }

    var doc: DocumentModel
    var question: ConfirmQuestion
    @Environment(\.theme) private var theme
    @FocusState private var runFocused: Bool

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "info.circle").foregroundStyle(theme.muted)
                .help(question.of != nil ? "Another block needs this one, and it hasn't run yet" : "This block is marked confirm")
            Text(question.question).frame(maxWidth: .infinity, alignment: .leading)
            Button("Run") { doc.answer("run") }
                .keyButton(theme, primary: true)
                .keyboardShortcut(.defaultAction)
                .focused($runFocused)
            // Bubble Gum's question buttons are all gumballs, and
            // Witchcraft's all amethyst.
            Button("Skip") { doc.answer("skip") }.keyButton(theme, primary: theme.look == .gum || theme.look == .witch)
            Button("Stop") { doc.answer("stop") }.keyButton(theme, primary: theme.look == .gum || theme.look == .witch)
        }
        .font(theme.uiFont())
        .padding(10)
        // Warm yellow, as mdsh-gui tints the question, or in Bubble Gum
        // a bubble of the look's --hover pink, in Witchcraft a candlelit
        // slip of its violet.
        .background(theme.look == .gum ? theme.color(theme.dark ? "#4a1a3e" : "#ffb3d1")
                    : theme.look == .witch ? theme.witchHover
                    : theme.dark ? Color(nsColor: Theme.nsColor("#2b2611")) : Color(nsColor: Theme.nsColor("#fff8c5")))
        .onAppear {
            runFocused = true
            NSAccessibility.post(element: NSApp as Any, notification: .announcementRequested,
                                 userInfo: [.announcement: question.question, .priority: NSAccessibilityPriorityLevel.high.rawValue])
        }
    }
}

/// TerminalView shows what a tty block running in its card draws, and
/// sends it what is typed.
struct TerminalView: View {
    var doc: DocumentModel
    var text: String
    @Environment(\.theme) private var theme
    @FocusState private var focused: Bool

    var body: some View {
        ScrollView {
            Text(lastLines)
                .font(theme.codeFont())
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(8)
        }
        .frame(height: CGFloat(max(doc.info.termRows, 24)) * theme.codeSize * 1.25)
        .background(Color.black)
        .foregroundStyle(Color.white)
        .focusable()
        .focused($focused)
        .onKeyPress(phases: .down) { press in
            if press.key == KeyEquivalent("]") && press.modifiers.contains(.control) {
                doc.stop()
                return .handled
            }
            var s = press.characters
            if press.key == .return { s = "\r" }
            if press.key == .delete { s = "\u{7f}" }
            if press.key == .escape { s = "\u{1b}" }
            doc.type(s)
            return .handled
        }
        .onAppear { focused = true }
        .accessibilityLabel("Terminal")
    }

    private var lastLines: String {
        text.split(separator: "\n", omittingEmptySubsequences: false).suffix(max(doc.info.termRows, 24)).joined(separator: "\n")
    }
}

/// TextboxCard is a textbox block: a field whose text is the block's body.
struct TextboxCard: View {
    var model: WindowModel
    var doc: DocumentModel
    var cell: Cell
    var box: TextboxCell
    @Environment(\.theme) private var theme
    @State private var text = ""
    @FocusState private var editing: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(box.id.replacingOccurrences(of: "_", with: " ")).font(theme.uiFont(weight: .semibold))
            // mdsh-gui's badge goes on its own line, over the field.
            if box.nosave == true { NosaveBadge(kind: .textbox) }
            if box.lines > 1 || box.fit == true || grown > 0 {
                // Saved as one edit when the box loses the keyboard, as
                // mdsh-gui does, not on every key. A fit box is as tall
                // as its text: the hidden copy under the editor sizes it.
                ZStack(alignment: .topLeading) {
                    if box.fit == true {
                        Text(text.isEmpty || text.hasSuffix("\n") ? text + " " : text)
                            .font(theme.codeFont())
                            .lineSpacing(theme.codeSize * 0.3)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .fixedSize(horizontal: false, vertical: true)
                            .hidden()
                            .accessibilityHidden(true)
                    }
                    TextEditor(text: $text)
                        .font(theme.codeFont())
                        .scrollDisabled(box.fit == true && grown == 0)
                        // lines= is the most lines the text takes.
                        .onKeyPress(.return) {
                            box.fit != true && text.split(separator: "\n", omittingEmptySubsequences: false).count >= box.lines ? .handled : .ignored
                        }
                        .focused($editing)
                        .onChange(of: editing) { _, on in if !on { save() } }
                }
                .frame(minHeight: box.fit == true ? max(lineHeight, CGFloat(grown) * lineHeight) : nil)
                .frame(height: box.fit == true ? nil : CGFloat(max(Double(box.lines), grown)) * lineHeight)
                .overlay(RoundedRectangle(cornerRadius: 4).stroke(theme.border))
                TextboxGrip(rows: rowsBinding, natural: Double(box.fit == true ? 1 : box.lines), id: box.id, lineHeight: lineHeight)
            } else {
                TextField(box.placeholder ?? "", text: $text)
                    .textFieldStyle(.roundedBorder)
                    .focused($editing)
                    .onSubmit(save)
                    .onChange(of: editing) { _, on in if !on { save() } }
                TextboxGrip(rows: rowsBinding, natural: 1, id: box.id, lineHeight: lineHeight)
            }
            if let p = box.problem, !p.isEmpty { Text("! " + p).foregroundStyle(theme.danger) }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(theme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(model.selected == cell.from ? theme.accent : theme.border))
        .onAppear { text = box.text }
        .onChange(of: box.text) { _, t in if t != text { text = t } }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Textbox \(box.id)")
    }

    private func save() {
        guard text != box.text else { return }
        Task { await doc.setTextbox(box.block, text) }
    }

    private var lineHeight: CGFloat { theme.codeSize * 1.5 }

    /// grown is the height, in lines, the grip was dragged to, or 0.
    private var grown: Double { model.textboxRows[rowsKey] ?? 0 }
    private var rowsKey: String { doc.file + "\u{0}" + box.id }
    private var rowsBinding: Binding<Double> {
        Binding(get: { model.textboxRows[rowsKey] ?? 0 }, set: { model.textboxRows[rowsKey] = $0 > 0 ? $0 : nil })
    }
}

/// TextboxGrip is the bar under a textbox that makes it taller, as in
/// mdsh-gui: drag it, or focus it and press down and up; a double click
/// puts the box back to its own height. rows is the height dragged to,
/// in lines, 0 for none; natural is the box's own height in lines.
struct TextboxGrip: View {
    @Binding var rows: Double
    var natural: Double
    var id: String
    var lineHeight: CGFloat
    @Environment(\.theme) private var theme
    @State private var start: Double?
    @State private var hover = false
    @FocusState private var focused: Bool

    var body: some View {
        Capsule()
            .fill(hover || focused ? theme.accent : theme.border)
            .frame(width: 36, height: 4)
            .frame(maxWidth: .infinity)
            .frame(height: 10)
            .contentShape(Rectangle())
            .padding(.vertical, -5)
            .focusable()
            .focusEffectDisabled()
            .focused($focused)
            .onHover { on in
                hover = on
                if on { NSCursor.resizeUpDown.push() } else { NSCursor.pop() }
            }
            .gesture(DragGesture(minimumDistance: 1)
                .onChanged { g in
                    let from = start ?? max(rows, natural)
                    start = from
                    set(from + g.translation.height / lineHeight)
                }
                .onEnded { _ in start = nil })
            .onTapGesture(count: 2) { rows = 0 }
            .onKeyPress(.downArrow) { set((max(rows, natural)).rounded() + 1); return .handled }
            .onKeyPress(.upArrow) { set((max(rows, natural)).rounded() - 1); return .handled }
            .help("Drag to make it taller; double-click for its own height")
            .accessibilityElement()
            .accessibilityLabel("Height of \(id), in lines")
            .accessibilityValue("\(Int(max(rows, natural).rounded()))")
            .accessibilityAdjustableAction { dir in
                switch dir {
                case .increment: set(max(rows, natural).rounded() + 1)
                case .decrement: set(max(rows, natural).rounded() - 1)
                @unknown default: break
                }
            }
    }

    /// set keeps a height of n lines, or none when that is no taller than
    /// the box would be anyway.
    private func set(_ n: Double) {
        rows = n <= natural + 0.25 ? 0 : n
    }
}

/// ChoiceCard is a choice block: a dropdown, radio buttons or checkboxes
/// whose checks are written to the block's [x] list.
struct ChoiceCard: View {
    var model: WindowModel
    var doc: DocumentModel
    var cell: Cell
    var choice: ChoiceCell
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(choice.id.replacingOccurrences(of: "_", with: " ")).font(theme.uiFont(weight: .semibold))
            // mdsh-gui's badge goes on its own line, over the field.
            if choice.nosave == true { NosaveBadge(kind: .choice) }
            switch choice.mode {
            case "multi":
                ForEach(Array(choice.options.enumerated()), id: \.offset) { i, o in
                    Toggle(o.label, isOn: Binding(get: { o.checked }, set: { v in
                        var checked = choice.options.map(\.checked)
                        checked[i] = v
                        Task { await doc.setChoice(choice.block, checked) }
                    }))
                    .toggleStyle(.checkbox)
                }
            case "radio":
                Picker("", selection: pick) {
                    ForEach(Array(choice.options.enumerated()), id: \.offset) { i, o in Text(o.label).tag(i) }
                }
                .pickerStyle(.radioGroup)
                .labelsHidden()
            default:
                Picker("", selection: pick) {
                    Text(choice.placeholder ?? "Choose…").tag(-1)
                    ForEach(Array(choice.options.enumerated()), id: \.offset) { i, o in Text(o.label).tag(i) }
                }
                .labelsHidden()
                .frame(maxWidth: 320)
            }
            if let p = choice.problem, !p.isEmpty { Text("! " + p).foregroundStyle(theme.danger) }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(theme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(model.selected == cell.from ? theme.accent : theme.border))
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Choice \(choice.id)")
    }

    private var pick: Binding<Int> {
        Binding(get: { choice.options.firstIndex(where: \.checked) ?? -1 }, set: { i in
            let checked = choice.options.indices.map { $0 == i }
            Task { await doc.setChoice(choice.block, checked) }
        })
    }
}
