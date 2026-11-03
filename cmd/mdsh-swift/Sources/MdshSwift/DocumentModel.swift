import AppKit
import MdshKit
import Observation
import SwiftUI

/// DocumentModel is an open file as a window shows it: the bridge's
/// document, its cells, and what the run going on has sent so far.
@MainActor
@Observable
final class DocumentModel {
    let id: Int
    var info: DocInfo
    var cells: [Cell] = []
    /// live is the output a block has written so far in this run, ANSI
    /// escapes taken out, until it ends and its rendered output comes.
    var live: [Int: String] = [:]
    /// current is the block running now, in the foreground.
    var current: Int?
    /// background are the background blocks still running.
    var background: Set<Int> = []
    /// confirm is the question waiting for an answer, if any.
    var confirm: ConfirmQuestion?
    /// fileChanged is set when the file changed on disk.
    var fileChanged = false
    /// text is the whole file, for the source view, and base the text
    /// the bridge last had, which SetText checks.
    var text = ""
    var base = ""
    /// terminal is what the tty block running in its card has shown.
    var terminal: [Int: String] = [:]
    /// termState reads the tty bytes into terminal's text.
    @ObservationIgnored private var termState: [Int: TermText] = [:]
    @ObservationIgnored private var historyStream = TermStream()
    /// historyTranscript is the History pane's terminal.
    var historyTranscript = ""
    var historyRunning = false
    /// historyStarted is when the command typed in History started, for
    /// its clock.
    var historyStarted: Date?
    /// runStarts is when each running block started, for its card's
    /// run timer.
    var runStarts: [Int: Date] = [:]
    var prompt = Prompt(ps1: "$ ", ps2: "> ")
    /// runError is the last run's error, for the status line.
    var status: String?
    /// events is the file's event log, for the Console's Events tab.
    var events: [AuditEvent] = []
    /// lostNote says what the run going on lost of what blocks set in
    /// the shells, for the status line once it ends.
    @ObservationIgnored private var lostNote: String?
    /// shellLost is the last event that lost what blocks set in a
    /// shell, for the History bar, until the next block starts.
    var shellLost: String?
    /// killAsk is the question about killing a stopped block's shell,
    /// while it shows.
    @ObservationIgnored private var killAsk: NSAlert?
    @ObservationIgnored private var killTask: Task<Void, Never>?

    private var app: AppModel { AppModel.shared }
    private var textTask: Task<Void, Never>?

    init(info: DocInfo) {
        id = info.id
        self.info = info
        AppModel.shared.register(self)
        Task { await loadEvents() }
    }

    var name: String { info.name }
    var file: String { info.file }
    var running: Bool { info.running || current != nil }

    /// blocks are the cells that are blocks.
    var blocks: [Cell] { cells.filter(\.isBlock) }

    func cell(block: Int) -> Cell? { cells.first { $0.block == block } }

    // MARK: Loading

    func refresh() async {
        if let i: DocInfo = await app.call("info", ["doc": .init(id)]) { info = i }
        if let c: [Cell] = await app.call("cells", ["doc": .init(id)]) { cells = c }
    }

    /// refreshText reads the file's text again. Text typed in the source
    /// view and not yet sent is kept and sent on top of it; blocks whose
    /// code it doesn't change keep their output.
    func refreshText() async {
        guard let t: String = await app.call("text", ["doc": .init(id)]) else { return }
        if text == base {
            text = t
            base = t
        } else if t != base {
            base = t
            if !running { await sendText() }
        }
    }

    func close() async {
        app.unregister(self)
        await app.call("close", ["doc": .init(id)], as: Empty.self)
    }

    // MARK: Events

    func handle(_ ev: BridgeEvent) {
        switch ev.name {
        case "block":
            guard let e = try? ev.decode(BlockEvent.self) else { return }
            blockEvent(e)
        case "confirm":
            confirm = try? ev.decode(ConfirmQuestion.self)
            if confirm != nil { status = "waiting for you" }
        case "ran":
            current = nil
            confirm = nil
            // The block stopped on its own: no need to ask about its shell.
            dismissKill()
            if let note = lostNote {
                status = note
                lostNote = nil
            }
            // In case a block's done never came; background blocks go on.
            runStarts = runStarts.filter { background.contains($0.key) }
            historyRunning = false
            Task {
                await refresh()
                await refreshText()
                await loadPrompt()
            }
        case "saved":
            Task { await refresh() }
        case "opened":
            fileChanged = false
            Task {
                await refresh()
                await refreshText()
            }
        case "file-changed":
            fileChanged = true
        case "audit":
            if let e = try? ev.decode(AuditEvent.self) { addEvent(e) }
        default:
            break
        }
    }

    private func blockEvent(_ e: BlockEvent) {
        switch e.kind {
        case "started":
            current = e.block
            shellLost = nil
            runStarts[e.block] = Date()
            live[e.block] = ""
            confirm = nil
            status = nil
        case "stdout", "stderr":
            let s = ANSI.strip(e.data ?? "")
            live[e.block, default: ""] += s
            if historyRunning { historyTranscript += s }
        case "background":
            background.insert(e.block)
            if runStarts[e.block] == nil { runStarts[e.block] = Date() }
            if current == e.block { current = nil }
        case "done":
            if current == e.block { current = nil }
            runStarts[e.block] = nil
            background.remove(e.block)
            // The cell's rendered output replaces the live text once the
            // cells are read again, unless the block started again since.
            let b = e.block
            Task {
                await refresh()
                if current != b {
                    live[b] = nil
                    terminal[b] = nil
                    termState[b] = nil
                }
            }
        case "terminal":
            termState[e.block] = TermText()
            terminal[e.block] = TermText().textWithCursor
        case "tty":
            if let b = e.bytes {
                termState[e.block, default: TermText()].write(b)
                terminal[e.block] = termState[e.block]?.textWithCursor
                if historyRunning { historyTranscript += historyStream.take(b) }
            }
        case "terminal-end":
            // The block is done with it: drop the cursor, as mdsh-gui
            // stops its terminal's.
            termState[e.block]?.end()
            if let t = termState[e.block] { terminal[e.block] = t.text }
            historyStream = TermStream()
        default:
            break
        }
    }

    // MARK: Running

    func runBlock(_ block: Int) { run("runBlock", ["block": .init(block)]) }
    func runFrom(_ block: Int) { run("runFrom", ["block": .init(block)]) }
    func runAll() { run("runAll", [:]) }

    /// runAllThenFirst runs the file, then its first block again, which
    /// leaves the blocks that depend on it stale: for the screenshot
    /// script.
    func runAllThenFirst() async {
        try? await app.tryCall("runAll", ["doc": .init(id)])
        if let b = blocks.first?.block {
            try? await app.tryCall("runBlock", ["doc": .init(id), "block": .init(b)])
        }
        await refresh()
    }

    func runSection(_ heading: Cell) {
        run("runSection", ["from": .init(heading.sectionFrom ?? 0), "to": .init(heading.sectionTo ?? 0)])
    }

    private func run(_ method: String, _ params: [String: JSONValue]) {
        // A block's own Run button answers its question.
        if let q = confirm, method == "runBlock", case .number(let b)? = params["block"], Int(b) == q.block {
            answer("run")
            return
        }
        var p = params
        p["doc"] = .init(id)
        info.running = true
        status = nil
        Task {
            do {
                try await app.tryCall(method, p)
            } catch {
                status = error.localizedDescription
                app.log("error", "\(name): \(error.localizedDescription)", file: file)
            }
            info.running = false
            await refresh()
        }
    }

    // MARK: Events and shells

    /// loadEvents reads the file's event log again.
    func loadEvents() async {
        if let list: [AuditEvent] = await app.call("events", ["doc": .init(id)]) { events = list }
    }

    /// addEvent shows a new event of the log. One that lost what blocks
    /// set in a shell shows in the status line and counts on the
    /// Console's badge, as in mdsh-gui.
    private func addEvent(_ e: AuditEvent) {
        let last = events.last?.seq ?? 0
        guard e.seq > last else { return }
        if last != 0, e.seq != last + 1 {
            // Events were dropped on the way: read the log again.
            Task { await loadEvents() }
            return
        }
        events.append(e)
        if events.count > 2000 { events.removeFirst(events.count - 2000) }
        if e.lost == true {
            lostNote = e.text
            shellLost = e.text
            status = e.text
            app.unseenProblems += 1
        }
    }

    /// stopWait is how long a stopped block has to stop before the app
    /// asks whether to kill its shell.
    static let stopWait: Duration = .seconds(3)

    /// stop stops the running block: it is interrupted, as ctrl+c would,
    /// and its shell lives on. A block that hasn't stopped stopWait later,
    /// such as a loop that ignores ctrl+c, can have its shell killed, if
    /// the user says so.
    func stop() {
        Task { await app.call("stop", ["doc": .init(id)], as: Empty.self) }
        killTask?.cancel()
        killTask = Task { [weak self] in
            try? await Task.sleep(for: Self.stopWait)
            guard !Task.isCancelled, let self, self.running, self.killAsk == nil else { return }
            if await self.askKill(), self.running {
                _ = await self.app.call("killShell", ["doc": .init(self.id)], as: Bool.self)
            }
        }
    }

    /// askKill asks whether to kill the shell of a block Stop didn't
    /// stop. Cancel is the default, so return doesn't end a shell by
    /// accident.
    func askKill() async -> Bool {
        let alert = Self.shellAlert(
            "The block hasn't stopped",
            "It was interrupted, as ctrl+c would, but it is still running. Kill its shell to end it? The variables, functions and aliases that blocks set in that shell will be gone.",
            "Kill Shell")
        killAsk = alert
        defer { killAsk = nil }
        return await ask(alert)
    }

    /// dismissKill closes the question about killing a shell, as when
    /// the block ends on its own: answered no.
    private func dismissKill() {
        killTask?.cancel()
        killTask = nil
        guard let alert = killAsk else { return }
        if let sheet = alert.window.sheetParent {
            sheet.endSheet(alert.window, returnCode: .cancel)
        } else {
            NSApp.stopModal(withCode: .cancel)
        }
    }

    /// window is the window showing the file, for its questions.
    private var window: NSWindow? {
        WindowRegistry.shared.models.first { $0.doc === self }?.window
    }

    /// ask shows alert as a sheet on the file's window, or on its own if
    /// there is none, and reports whether its first button was pressed.
    func ask(_ alert: NSAlert) async -> Bool {
        guard let w = window else { return alert.runModal() == .alertFirstButtonReturn }
        return await withCheckedContinuation { c in
            alert.beginSheetModal(for: w) { c.resume(returning: $0 == .alertFirstButtonReturn) }
        }
    }

    /// restartShells starts the file's shells again, once the user says
    /// so, and reports whether it did.
    @discardableResult
    func restartShells() async -> Bool {
        let alert = Self.shellAlert(
            "Restart the shells?",
            "This file's shells end, along with any background block, and new ones start with the next block. \(Self.lostWarning)",
            "Restart Shells")
        guard await ask(alert) else { return false }
        return await edit("restart")
    }

    /// lostWarning is what a restart costs, for the questions.
    static let lostWarning = "The variables, functions and aliases that blocks set in them will be gone."

    /// shellAlert is a question before the app ends shells, as
    /// mdsh-gui's: the button that goes ahead is red, and Cancel has the
    /// return key.
    static func shellAlert(_ title: String, _ text: String, _ ok: String) -> NSAlert {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = text
        let go = alert.addButton(withTitle: ok)
        go.hasDestructiveAction = true
        go.keyEquivalent = ""
        let cancel = alert.addButton(withTitle: "Cancel")
        cancel.keyEquivalent = "\r"
        return alert
    }

    func answer(_ reply: String) {
        confirm = nil
        status = nil
        Task { await app.call("answer", ["doc": .init(id), "reply": .init(reply)], as: Empty.self) }
    }

    // MARK: Editing

    /// edit runs an edit of the document and redraws it, saying in the
    /// status line why it failed.
    func edit(_ method: String, _ params: [String: JSONValue] = [:]) async -> Bool {
        var p = params
        p["doc"] = .init(id)
        do {
            try await app.tryCall(method, p)
            await refresh()
            await refreshText()
            return true
        } catch {
            status = error.localizedDescription
            return false
        }
    }

    func setTask(_ task: Int, done: Bool) async { _ = await edit("setTask", ["task": .init(task), "done": .init(done)]) }
    func setTextbox(_ block: Int, _ text: String) async { _ = await edit("setTextbox", ["block": .init(block), "text": .init(text)]) }
    func setChoice(_ block: Int, _ checked: [Bool]) async { _ = await edit("setChoice", ["block": .init(block), "checked": .init(checked)]) }
    func clearOutput(_ block: Int) async { _ = await edit("clearOutput", ["block": .init(block)]) }

    func clearOutputs() async { _ = await edit("clearOutputs") }

    func deleteBlock(_ block: Int) async {
        var p: [String: JSONValue] = ["block": .init(block)]
        p["doc"] = .init(id)
        do {
            let d: Deleted = try await app.tryCall("deleteBlock", p)
            status = "deleted #\(d.name)"
            for w in d.warnings ?? [] { app.log("warn", w, file: file) }
            await refresh()
            await refreshText()
        } catch {
            status = error.localizedDescription
        }
    }

    /// moveBlock moves block up (dir -1) or down (1), past the next
    /// cell, or with byBlock past the next block.
    func moveBlock(_ block: Int, dir: Int, byBlock: Bool) async -> Move? {
        do {
            let m: Move = try await app.tryCall("moveBlock", ["doc": .init(id), "block": .init(block), "dir": .init(dir), "byBlock": .init(byBlock)])
            await refresh()
            await refreshText()
            return m
        } catch {
            status = error.localizedDescription
            return nil
        }
    }

    func moveTo(_ block: Int, at: Int) async -> Move? {
        do {
            let m: Move = try await app.tryCall("moveTo", ["doc": .init(id), "from": .init(block), "at": .init(at)])
            await refresh()
            await refreshText()
            return m
        } catch {
            status = error.localizedDescription
            return nil
        }
    }

    func setFold(block: Int, output: Bool = false, heading: Int = 0, on: Bool) async {
        await setFolds([Fold(block: block, output: output, heading: heading, on: on)])
    }

    func setFolds(_ folds: [Fold]) async {
        guard let f = try? JSONValue(encoding: folds) else { return }
        _ = await edit("setFolds", ["folds": f])
    }

    func setDepends(_ block: Int, _ deps: [Int]) async {
        _ = await edit("setDepends", ["block": .init(block), "deps": .init(deps)])
    }

    func undo() async {
        if let what: String = try? await app.tryCall("undo", ["doc": .init(id)]) {
            status = what.isEmpty ? "nothing to undo" : "undid \(what)"
            await refresh()
            await refreshText()
        }
    }

    func redo() async {
        if let what: String = try? await app.tryCall("redo", ["doc": .init(id)]) {
            status = what.isEmpty ? "nothing to redo" : "redid \(what)"
            await refresh()
            await refreshText()
        }
    }

    /// textChanged takes the source view's text, sending it to the
    /// bridge after a pause in typing.
    func textChanged(_ t: String) {
        text = t
        textTask?.cancel()
        textTask = Task {
            try? await Task.sleep(for: .milliseconds(500))
            if Task.isCancelled { return }
            await sendText()
        }
    }

    func sendText() async {
        let t = text
        guard t != base else { return }
        do {
            let ok: Bool = try await app.tryCall("setText", ["doc": .init(id), "base": .init(base), "text": .init(t)])
            if ok {
                base = t
                await refresh()
            } else {
                // The file changed under the typing, as a run writing its
                // output does; the edit goes again when the run ends and
                // refreshText reads the new text.
                status = "the file changed while you typed; your edit goes in when the run ends"
            }
        } catch {
            status = error.localizedDescription
        }
    }

    // MARK: Saving

    func save() async -> Bool {
        do {
            let i: DocInfo = try await app.tryCall("save", ["doc": .init(id)])
            info = i
            status = "saved"
            return true
        } catch {
            status = error.localizedDescription
            return false
        }
    }

    func saveAs(_ path: String) async -> Bool {
        do {
            let i: DocInfo = try await app.tryCall("saveAs", ["doc": .init(id), "file": .init(path)])
            info = i
            return true
        } catch {
            status = error.localizedDescription
            return false
        }
    }

    func reload() async {
        _ = await edit("reload")
        fileChanged = false
    }

    func setRunOptions(args: String, only: String, step: Bool, stopOnError: Bool) async -> String? {
        do {
            try await app.tryCall("setRunOptions", ["doc": .init(id), "args": .init(args), "only": .init(only), "step": .init(step), "stopOnError": .init(stopOnError)])
            await refresh()
            return nil
        } catch {
            return error.localizedDescription
        }
    }

    // MARK: History

    /// setHistory turns the History pane on or off. Turning it on in
    /// shells started without a terminal, after blocks ran in them, asks
    /// first, as mdsh-gui does, since starting them again loses what the
    /// blocks set.
    func setHistory(_ on: Bool) async {
        do {
            do {
                let _: Bool = try await app.tryCall("setHistory", ["doc": .init(id), "on": .init(on)])
            } catch where on && error.localizedDescription.contains("need to start again") {
                guard await confirmRestart() else {
                    historyTranscript += "(The shells were kept. Commands typed here need shells on a terminal: press Restart Shells, the ↻ in the bar, when you're ready to lose what blocks set.)\n"
                    await refresh()
                    return
                }
                let _: Bool = try await app.tryCall("setHistory", ["doc": .init(id), "on": .init(true), "restart": .init(true)])
                historyTranscript += "(The shells started again, on a terminal, so what blocks set in them is gone.)\n"
            }
            await refresh()
            if on { await loadPrompt() }
        } catch {
            status = error.localizedDescription
        }
    }

    /// confirmRestart asks before History starts the shells again.
    func confirmRestart() async -> Bool {
        await ask(Self.shellAlert(
            "Start the shells again?",
            "Commands typed in the History pane run on a terminal, and this file's shells were started without one. Starting them again, on a terminal, lets the pane run commands. \(Self.lostWarning)",
            "Restart Shells"))
    }

    /// paneSize is the History pane's size in characters, cols by rows,
    /// once it has been drawn, so the prompt is drawn to fit it.
    @ObservationIgnored var paneSize: (cols: Int, rows: Int)?

    /// setPaneSize records the History pane's size and asks for the
    /// prompt again when it changed, as mdsh-gui does on a resize.
    func setPaneSize(cols: Int, rows: Int) {
        guard cols > 0, rows > 0, paneSize.map({ $0.cols != cols || $0.rows != rows }) ?? true else { return }
        paneSize = (cols, rows)
        Task { await loadPrompt() }
    }

    func loadPrompt() async {
        guard info.history else { return }
        var params: [String: JSONValue] = ["doc": .init(id)]
        if let s = paneSize {
            params["cols"] = .init(s.cols)
            params["rows"] = .init(s.rows)
        }
        if let p: Prompt = try? await app.tryCall("historyPrompt", params) {
            prompt = Prompt(ps1: ANSI.strip(p.ps1), ps2: ANSI.strip(p.ps2))
        }
    }

    /// historyIsComplete reports whether input is a whole command, so
    /// enter runs it rather than starting another line.
    func historyIsComplete(_ input: String) async -> Bool {
        (try? await app.tryCall("historyComplete", ["doc": .init(id), "input": .init(input)], as: Bool.self)) ?? true
    }

    func historyEnter(_ input: String) {
        historyTranscript += prompt.ps1 + input.replacingOccurrences(of: "\n", with: "\n" + prompt.ps2) + "\n"
        historyRunning = !isHistoryNote(input)
        historyStarted = historyRunning ? Date() : nil
        Task {
            do {
                try await app.tryCall("historyEnter", ["doc": .init(id), "input": .init(input)])
            } catch {
                historyTranscript += error.localizedDescription + "\n"
            }
            historyRunning = false
            await refresh()
            await refreshText()
        }
    }

    func historyCompletions(_ before: String) async -> Completion? {
        try? await app.tryCall("historyCompletions", ["doc": .init(id), "before": .init(before)])
    }

    func type(_ keys: String) {
        Task { await app.call("type", ["doc": .init(id), "keys": .init(keys)], as: Empty.self) }
    }
}
