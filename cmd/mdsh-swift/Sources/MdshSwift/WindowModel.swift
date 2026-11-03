import AppKit
import MdshKit
import Observation
import SwiftUI
import UniformTypeIdentifiers

/// WindowTarget is what a window (a tab, in macOS's window tabs) opens
/// with: the start screen, or a file. Each start screen has its own id,
/// so New Tab always makes a new one, while opening a file that is open
/// already brings its window forward.
struct WindowTarget: Codable, Hashable {
    var file: String?
    var session = false
    var startID = UUID()

    static func start() -> WindowTarget { WindowTarget() }

    static func open(_ file: String, session: Bool = false) -> WindowTarget {
        // A file's target leaves startID fixed, so the same file is the
        // same window.
        WindowTarget(file: file, session: session, startID: UUID(uuidString: "00000000-0000-0000-0000-000000000000")!)
    }
}

/// Pane is a view the window can show of a document.
enum Pane: String {
    case rendered, source, both
}

/// WindowModel is one window: the start screen or a document, and how
/// it is shown.
@MainActor
@Observable
final class WindowModel {
    var doc: DocumentModel?
    var start: StartScreen?
    var startFilter = ""
    var error: String?

    var showRendered = true
    var showSource = false
    var showOutline = false
    var showHistory = false
    var showConsole = false
    var showCode = true
    var showOutput = true
    var frontMatterOpen = false
    /// viewedImage is the output image shown full size, if any.
    var viewedImage: NSImage?
    /// showDepLines shows the dependency lines in the margin.
    var showDepLines = true
    /// splitFraction is the source view's share of the width when both
    /// views show.
    var splitFraction: CGFloat = 0.5
    /// sourceLine is a line for the source view to show and put its
    /// cursor on, set by Edit in source.
    var sourceLine: Int?

    /// selected is the selected block or heading cell, by From.
    var selected: Int?
    /// folded are the cells folded in the rendered view, by From, and
    /// foldedOutput the blocks whose output is folded.
    var folded: Set<Int> = []
    var foldedOutput: Set<Int> = []
    /// sourceFolds are the source view's closed folds, by line from 1;
    /// sourceFoldsFile the file they were opened for.
    var sourceFolds: Set<Int> = []
    var sourceFoldsFile = ""
    /// moveBanner says what a move left behind, with Undo.
    var moveBanner: [String] = []

    // Sheets.
    var runOptionsOpen = false
    var reportOpen = false
    var dependsOpen = false
    var clearAllAsk = false
    var deleteAsk: Cell?
    var newFileOpen = false
    var folderSettings: FolderForm?

    /// window is the NSWindow, once shown, for sheets and closing.
    weak var window: NSWindow?

    private var app: AppModel { AppModel.shared }

    /// title is the window's title: the file's name, with • after it
    /// while the output isn't saved, as mdsh-gui shows it.
    var title: String {
        guard let doc else { return "Start" }
        // The tab shows what the file is doing, as mdsh-gui's tabs do:
        // ? waiting on a question, ▶ running, ◐ running in the background.
        let mark = doc.confirm != nil ? "? " : doc.running ? "▶ " : !doc.background.isEmpty ? "◐ " : ""
        return mark + (doc.info.dirty ? doc.name + " •" : doc.name)
    }

    // MARK: Opening

    /// target is what the window was opened with.
    var target: WindowTarget?

    func load(_ target: WindowTarget) async {
        self.target = target
        if let file = target.file {
            await open(file, session: target.session)
        } else {
            await loadStart()
        }
    }

    func loadStart() async {
        do {
            start = try await app.tryCall("startScreen", ["dir": .init(app.startFolderArg ?? "")])
            error = nil
            // MDSH_SWIFT_SHOW=newfile opens New File, for the screenshots.
            let show = (ProcessInfo.processInfo.environment["MDSH_SWIFT_SHOW"] ?? "").split(separator: ",")
            if show.contains("newfile") {
                newFileOpen = true
            }
            // And foldersettings the start folder's Folder Settings.
            if show.contains("foldersettings") {
                folderSettings = try await app.tryCall("folderSettings", ["rel": .init("")])
            }
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// open opens file in this window, closing the document it had.
    func open(_ file: String, session: Bool = false) async {
        // One window per file: the bridge has one document for it, and
        // two windows on it would close each other's shells.
        if let other = WindowRegistry.shared.window(showing: file), other !== self {
            other.window?.makeKeyAndOrderFront(nil)
            if session, !other.showHistory { other.toggleHistory() }
            if doc == nil, target?.file != nil { window?.close() }
            return
        }
        do {
            let info: DocInfo = try await app.tryCall(session ? "openSession" : "open", ["file": .init(file)])
            if let old = doc, old.id != info.id { await old.close() }
            let d = DocumentModel(info: info)
            doc = d
            applyCommandLineRun(file)
            await d.refresh()
            await d.refreshText()
            applyViewSetting()
            if info.openHistory || session {
                showHistory = true
                await d.setHistory(true)
            }
            if d.info.history { await d.loadPrompt() }
            window?.title = title
            window?.representedURL = URL(fileURLWithPath: d.file)
            NSDocumentController.shared.noteNewRecentDocumentURL(URL(fileURLWithPath: d.file))
            // Folds the file asks for.
            folded = Set(d.cells.filter { $0.collapsed == true }.map(\.from))
            foldedOutput = Set(d.cells.filter { $0.collapsedOutput == true }.map(\.from))
            // MDSH_SWIFT_AUTORUN runs the file as it opens, for the
            // screenshot script.
            let env = ProcessInfo.processInfo.environment
            if env["MDSH_SWIFT_AUTORUN"] == "1" { d.runAll() }
            if env["MDSH_SWIFT_AUTORUN"] == "rerun" { Task { await d.runAllThenFirst() } }
            // MDSH_SWIFT_SHOW opens panes and sheets, for the same script:
            // a comma-separated list of history, console, outline,
            // source, report, runoptions, frontmatter, depends, clearall,
            // delete, filechanged, selectlast, depsel, sourcefold,
            // fixfirst, movefirstdown and select:ID.
            for p in (env["MDSH_SWIFT_SHOW"] ?? "").split(separator: ",") {
                switch p {
                case "history": if !showHistory { toggleHistory() }
                case "console": showConsole = true
                case "outline": showOutline = true
                case "source": showSource = true
                case "report": reportOpen = true
                case "runoptions": runOptionsOpen = true
                case "frontmatter": frontMatterOpen = true
                case "depends":
                    // A block with depends=, so the numbering shows.
                    selected = (d.blocks.first { !($0.depends ?? []).isEmpty } ?? d.blocks.last)?.from
                    dependsOpen = true
                case "clearall": clearAllAsk = true
                case "delete": deleteAsk = d.blocks.last
                case "filechanged": d.fileChanged = true
                case "selectlast": selected = d.blocks.last?.from
                case "depsel": selected = d.blocks.first { !($0.depends ?? []).isEmpty }?.from
                case "sourcefold":
                    // The source view with the first block's fence folded.
                    showSource = true
                    sourceFoldsFile = d.file
                    sourceFolds = Set(d.blocks.prefix(1).map(\.line))
                case "fixfirst":
                    // Fix the first warning about this file that has a
                    // fix, once the console has it.
                    Task {
                        for _ in 0..<50 {
                            if let e = AppModel.shared.console.first(where: { $0.fix != nil && $0.file == d.file && $0.stale != true }) {
                                await fixWarning(e.message)
                                return
                            }
                            try? await Task.sleep(for: .milliseconds(100))
                        }
                    }
                case "movefirstdown":
                    selected = d.blocks.first?.from
                    moveSelected(dir: 1, byBlock: true)
                case let p where p.hasPrefix("select:"):
                    let id = String(p.dropFirst("select:".count))
                    selected = d.blocks.first { $0.blockID == id }?.from
                default: break
                }
            }
        } catch {
            self.error = error.localizedDescription
            app.log("error", "open \(file): \(error.localizedDescription)", file: file)
        }
    }

    private func applyCommandLineRun(_ file: String) {
        guard let d = doc, app.commandLineRunFor == file else { return }
        let r = app.commandLineRun
        app.commandLineRunFor = nil
        guard !r.only.isEmpty || r.step || r.stopOnError || !r.args.isEmpty else { return }
        let args = r.args.map { "'" + $0.replacingOccurrences(of: "'", with: "'\\''") + "'" }.joined(separator: " ")
        Task { _ = await d.setRunOptions(args: args, only: r.only, step: r.step, stopOnError: r.stopOnError) }
    }

    /// applyViewSetting shows the panes Settings › Open files in asks for.
    func applyViewSetting() {
        switch app.settings.view {
        case "rendered": showRendered = true; showSource = false
        case "source": showRendered = false; showSource = true
        case "both": showRendered = true; showSource = true
        default: break
        }
    }

    func closeDocument() async {
        if let d = doc { await d.close() }
        doc = nil
    }

    // MARK: Panes

    func toggleSource() {
        showSource.toggle()
        if !showSource && !showRendered { showRendered = true }
    }

    func toggleRendered() {
        showRendered.toggle()
        if !showSource && !showRendered { showSource = true }
    }

    func toggleConsole() {
        showConsole.toggle()
        if showConsole { app.unseenProblems = 0 }
    }

    func toggleHistory() {
        showHistory.toggle()
        if showHistory, let d = doc, !d.info.history {
            Task { await d.setHistory(true) }
        }
    }

    // MARK: Selection

    /// selectable are the cells j and k move between: blocks and
    /// headings.
    var selectable: [Cell] { doc?.cells.filter { $0.isBlock || $0.isHeading } ?? [] }

    var selectedCell: Cell? { selected.flatMap { s in doc?.cells.first { $0.from == s } } }

    func moveSelection(_ by: Int) {
        let list = selectable
        guard !list.isEmpty else { return }
        let i = list.firstIndex { $0.from == selected } ?? (by > 0 ? -1 : list.count)
        let j = min(max(i + by, 0), list.count - 1)
        selected = list[j].from
    }

    /// currentBlock is the block Run Current Block runs: the selected
    /// card's.
    var currentBlock: Int? {
        guard let c = selectedCell, c.isBlock else { return nil }
        return c.block
    }

    func runCurrent() {
        guard let d = doc else { return }
        if let b = currentBlock { d.runBlock(b) }
    }

    /// runSection runs the section of the selected heading, or the one
    /// the selected block is in.
    func runSection() {
        guard let d = doc, let sel = selectedCell else { return }
        if sel.isHeading {
            d.runSection(sel)
            return
        }
        let heading = d.cells.last { $0.isHeading && $0.from <= sel.from && $0.hasSection && (sel.block >= ($0.sectionFrom ?? 0)) && (sel.block < ($0.sectionTo ?? 0)) }
        if let h = heading { d.runSection(h) }
    }

    func copySelected() {
        guard let c = selectedCell, c.isBlock else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(c.body ?? "", forType: .string)
        doc?.status = "copied"
    }

    func moveSelected(dir: Int, byBlock: Bool) {
        guard let d = doc, let c = selectedCell, c.isBlock else { return }
        Task {
            if let m = await d.moveBlock(c.block, dir: dir, byBlock: byBlock) {
                selected = d.cells.first { $0.block == m.block }?.from
                showMoved(m)
            }
        }
    }

    /// renderedFoldLines are the source lines of the rendered view's
    /// folds: a heading's, a block's code at its fence, its output at
    /// its output fence. fromFile takes the folds the file saved.
    func renderedFoldLines(fromFile: Bool = false) -> Set<Int> {
        guard let d = doc else { return [] }
        var lines = Set<Int>()
        for c in d.cells {
            let code = fromFile ? c.collapsed == true : folded.contains(c.from)
            if c.isHeading && code { lines.insert(c.line) }
            if c.isBlock {
                if code { lines.insert(c.line) }
                let out = fromFile ? c.collapsedOutput == true : foldedOutput.contains(c.from)
                if out, let l = c.outputLine { lines.insert(l) }
            }
        }
        return lines
    }

    /// foldsToSource is View › Folding › Copy Folds to Source View.
    func foldsToSource() {
        showSource = true
        sourceFoldsFile = doc?.file ?? ""
        sourceFolds = renderedFoldLines()
    }

    /// foldsToRendered is View › Folding › Copy Folds to Rendered View,
    /// saving the folds in the file as folding there does.
    func foldsToRendered() {
        guard let d = doc else { return }
        guard showSource else {
            d.status = "open the source view to copy its folds"
            return
        }
        let lines = sourceFolds
        var list: [Fold] = []
        func set(_ s: inout Set<Int>, _ from: Int, _ on: Bool) -> Bool {
            guard s.contains(from) != on else { return false }
            if on { s.insert(from) } else { s.remove(from) }
            return true
        }
        for c in d.cells {
            if c.isHeading && c.hasSection {
                let on = lines.contains(c.line)
                if set(&folded, c.from, on) { list.append(Fold(block: -1, output: false, heading: c.from, on: on)) }
            }
            if c.isBlock {
                let on = lines.contains(c.line)
                if set(&folded, c.from, on) { list.append(Fold(block: c.block, output: false, heading: 0, on: on)) }
                let out = c.outputLine.map { lines.contains($0) } ?? false
                if set(&foldedOutput, c.from, out) { list.append(Fold(block: c.block, output: true, heading: 0, on: out)) }
            }
        }
        showRendered = true
        if !list.isEmpty { Task { await d.setFolds(list) } }
    }

    /// showMoved says what a move changed that depends on block order,
    /// in the banner with Undo, or just that it moved, on the status
    /// line, as mdsh-gui's showMoved does.
    /// fixWarning makes the fix for warning, a console entry's message,
    /// and says so in the banner, with Undo, as mdsh-gui does.
    func fixWarning(_ warning: String) async {
        guard let d = doc else { return }
        if await d.edit("fixWarning", ["warning": .init(warning)]) {
            moveBanner = ["Fixed: \(warning)."]
            d.status = "fixed: \(warning)"
        }
    }

    func showMoved(_ m: Move) {
        let warnings = m.warnings ?? []
        if warnings.isEmpty {
            moveBanner = []
            doc?.status = m.note.map { "moved. \($0)" } ?? "moved"
        } else {
            moveBanner = warnings + (m.note.map { [$0] } ?? [])
            doc?.status = "moved"
        }
    }

    // MARK: File commands

    func save() {
        guard let d = doc else { return }
        Task {
            if !(await d.save()) {
                if FileManager.default.isWritableFile(atPath: d.file) == false {
                    saveAs()
                }
            }
        }
    }

    func saveAs() {
        guard let d = doc else { return }
        let panel = NSSavePanel()
        panel.message = "Save a copy with its output"
        panel.directoryURL = URL(fileURLWithPath: (d.file as NSString).deletingLastPathComponent)
        panel.nameFieldStringValue = (d.file as NSString).lastPathComponent
        panel.allowedContentTypes = [UTType(filenameExtension: "md") ?? .plainText]
        panel.begin { resp in
            guard resp == .OK, let url = panel.url else { return }
            Task { @MainActor in
                if await d.saveAs(url.path) {
                    self.window?.title = self.title
                    self.window?.representedURL = url
                }
            }
        }
    }

    // MARK: Closing

    /// CloseChoice is what to do with unsaved output as a window closes.
    enum CloseChoice { case close, save, cancel }

    /// closeChoice asks about unsaved output, if there is any.
    func closeChoice() -> CloseChoice {
        guard let d = doc, d.info.dirty else { return .close }
        let alert = NSAlert()
        alert.messageText = "Unsaved output"
        alert.informativeText = "Save the output in \(d.name) before closing?"
        alert.addButton(withTitle: "Save")
        alert.addButton(withTitle: "Don't Save")
        alert.addButton(withTitle: "Cancel")
        switch alert.runModal() {
        case .alertFirstButtonReturn: return .save
        case .alertSecondButtonReturn: return .close
        default: return .cancel
        }
    }

    /// saveForClose saves before the window closes, and reports whether
    /// it may: a failed save keeps the window, with the error showing.
    func saveForClose() async -> Bool {
        guard let d = doc else { return true }
        if await d.save() { return true }
        error = d.status
        return false
    }
}
