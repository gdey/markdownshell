import AppKit
import MdshKit
import SwiftUI
import UniformTypeIdentifiers

/// AppCommands are the menu bar's commands, as mdsh-gui's menu.go has
/// them, acting on the window in front.
struct AppCommands: Commands {
    var app: AppModel
    @FocusedValue(\.windowModel) private var window
    @Environment(\.openWindow) private var openWindow
    @Environment(\.openSettings) private var openSettings

    private var doc: DocumentModel? { window?.doc }

    var body: some Commands {
        CommandGroup(replacing: .appInfo) {
            Button("About mdsh-swift") {
                SettingsTab.requested = .about
                openSettings()
            }
        }

        CommandGroup(replacing: .newItem) {
            Button("New File…") { window?.newFileOpen = true }
                .keyboardShortcut("n")
            Button("New Tab") { open(.start(), asTab: true) }
                .keyboardShortcut("t")
            Button("New Window") { open(.start(), asTab: false) }
                .keyboardShortcut("n", modifiers: [.command, .shift])
            Button("Open…") { openPanel() }
                .keyboardShortcut("o")
            Menu("Open Recent") {
                let recent = app.settings.recent ?? []
                if recent.isEmpty {
                    Text("No recent files")
                }
                ForEach(recent, id: \.self) { f in
                    Button((f as NSString).lastPathComponent) { openFile(f) }
                        .help(f)
                }
                Divider()
                Button("Clear Menu") {
                    var s = app.settings
                    s.recent = []
                    Task { await app.saveSettings(s) }
                }
                .disabled(recent.isEmpty)
            }
        }

        CommandGroup(replacing: .saveItem) {
            Button("Save") { window?.save() }
                .keyboardShortcut("s")
                .disabled(doc == nil)
            Button("Save As…") { window?.saveAs() }
                .keyboardShortcut("s", modifiers: [.command, .shift])
                .disabled(doc == nil)
            Divider()
            Button("Create Report…") { window?.reportOpen = true }
                .disabled(doc == nil)
            Button("Reload") { if let d = doc { Task { await d.reload() } } }
                .disabled(doc == nil)
            Divider()
            Button("Close Tab") { window?.window?.performClose(nil) }
                .keyboardShortcut("w")
            Button("Close Window") { closeWindow() }
                .keyboardShortcut("w", modifiers: [.command, .shift])
        }

        CommandGroup(replacing: .undoRedo) {
            Button("Undo") { undo(redo: false) }
                .keyboardShortcut("z")
            Button("Redo") { undo(redo: true) }
                .keyboardShortcut("z", modifiers: [.command, .shift])
        }

        CommandGroup(after: .pasteboard) {
            Divider()
            Button("Move Block Up") { window?.moveSelected(dir: -1, byBlock: false) }
                .keyboardShortcut(.upArrow, modifiers: .option)
                .disabled(window?.currentBlock == nil)
            Button("Move Block Down") { window?.moveSelected(dir: 1, byBlock: false) }
                .keyboardShortcut(.downArrow, modifiers: .option)
                .disabled(window?.currentBlock == nil)
            Button("Move Block Above Previous Block") { window?.moveSelected(dir: -1, byBlock: true) }
                .keyboardShortcut(.upArrow, modifiers: [.option, .shift])
                .disabled(window?.currentBlock == nil)
            Button("Move Block Below Next Block") { window?.moveSelected(dir: 1, byBlock: true) }
                .keyboardShortcut(.downArrow, modifiers: [.option, .shift])
                .disabled(window?.currentBlock == nil)
            Button("Depends On…") { window?.dependsOpen = true }
                .disabled(window?.currentBlock == nil)
            Divider()
            Button("Front Matter") { window?.frontMatterOpen.toggle() }
                .keyboardShortcut("m", modifiers: [.command, .shift])
                .disabled(doc == nil)
        }

        CommandMenu("Run") {
            Button("Run Current Block") { window?.runCurrent() }
                .keyboardShortcut(.return)
                .disabled(window?.currentBlock == nil)
            Button("Run Section") { window?.runSection() }
                .keyboardShortcut(.return, modifiers: [.command, .shift])
                .disabled(window?.selectedCell == nil)
            Button("Run All") { doc?.runAll() }
                .keyboardShortcut("r")
                .disabled(doc == nil || doc?.running == true)
            Button("Stop") { doc?.stop() }
                .keyboardShortcut(".")
                .disabled(doc?.running != true)
            Divider()
            Button("Clear All Outputs…") { window?.clearAllAsk = true }
                .keyboardShortcut("k", modifiers: [.command, .option])
                .disabled(doc?.info.hasOutput != true)
            Button("Run Options…") { window?.runOptionsOpen = true }
                .disabled(doc == nil)
        }

        CommandGroup(after: .toolbar) {
            Toggle("Source View", isOn: binding(\.showSource, toggle: { $0.toggleSource() }))
                .keyboardShortcut("\\")
                .disabled(doc == nil)
            Toggle("Rendered View", isOn: binding(\.showRendered, toggle: { $0.toggleRendered() }))
                .keyboardShortcut("\\", modifiers: [.command, .shift])
                .disabled(doc == nil)
            Toggle("Outline", isOn: binding(\.showOutline, toggle: { $0.showOutline.toggle() }))
                .keyboardShortcut("o", modifiers: [.command, .shift])
                .disabled(doc == nil)
            Toggle("History", isOn: binding(\.showHistory, toggle: { $0.toggleHistory() }))
                .keyboardShortcut("h", modifiers: [.command, .shift])
                .disabled(doc == nil)
            Toggle("Console", isOn: binding(\.showConsole, toggle: { $0.toggleConsole() }))
                .keyboardShortcut("y", modifiers: [.command, .shift])
            Divider()
            Toggle("Show Code", isOn: binding(\.showCode, toggle: { $0.showCode.toggle() }))
                .disabled(doc == nil)
            Toggle("Show Output", isOn: binding(\.showOutput, toggle: { $0.showOutput.toggle() }))
                .disabled(doc == nil)
            Menu("Folding") {
                Button("Collapse All Blocks") { foldAll(blocks: true) }
                Button("Collapse All Sections") { foldAll(blocks: false) }
                Button("Expand All") {
                    window?.folded = []
                    window?.foldedOutput = []
                }
                Divider()
                Button("Copy Folds to Source View") { window?.foldsToSource() }
                Button("Copy Folds to Rendered View") { window?.foldsToRendered() }
            }
            .disabled(doc == nil)
            Divider()
            Button("Zoom In") { zoom(5) }
                .keyboardShortcut("+")
            Button("Zoom Out") { zoom(-5) }
                .keyboardShortcut("-")
            Button("Actual Size") { zoom(0) }
                .keyboardShortcut("0")
            Divider()
            Button("Show Next Tab") { NSApp.keyWindow?.selectNextTab(nil) }
                .keyboardShortcut("]", modifiers: [.command, .shift])
            Button("Show Previous Tab") { NSApp.keyWindow?.selectPreviousTab(nil) }
                .keyboardShortcut("[", modifiers: [.command, .shift])
        }
    }

    // MARK: Helpers

    private func binding(_ key: ReferenceWritableKeyPath<WindowModel, Bool>, toggle: @escaping (WindowModel) -> Void) -> Binding<Bool> {
        Binding(get: { window?[keyPath: key] ?? false }, set: { _ in if let w = window { toggle(w) } })
    }

    private func open(_ target: WindowTarget, asTab: Bool) {
        WindowOpener.tabParent = asTab ? NSApp.keyWindow : nil
        openWindow(id: "doc", value: target)
    }

    /// openFile opens file: in the window in front if it shows the start
    /// screen, else in a new tab. A file open already comes forward.
    func openFile(_ file: String) {
        if let w = WindowRegistry.shared.window(showing: file) {
            w.window?.makeKeyAndOrderFront(nil)
        } else if let w = window, w.doc == nil {
            Task { await w.open(file) }
        } else {
            open(.open(file), asTab: true)
        }
    }

    private func openPanel() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = false
        panel.allowedContentTypes = [UTType(filenameExtension: "md") ?? .plainText, UTType(filenameExtension: "markdown") ?? .plainText]
        if let dir = window?.start?.dir { panel.directoryURL = URL(fileURLWithPath: dir) }
        guard panel.runModal() == .OK else { return }
        for url in panel.urls { openFile(url.path) }
    }

    private func closeWindow() {
        guard let w = NSApp.keyWindow else { return }
        for t in w.tabbedWindows ?? [w] { t.performClose(nil) }
    }

    /// undo goes to a text field being edited, else to the rendered
    /// view's edits.
    private func undo(redo: Bool) {
        if let r = NSApp.keyWindow?.firstResponder as? NSTextView, r.isEditable, let um = r.undoManager {
            redo ? um.redo() : um.undo()
            return
        }
        guard let d = doc else { return }
        Task { redo ? await d.redo() : await d.undo() }
    }

    private func foldAll(blocks: Bool) {
        guard let w = window, let d = doc else { return }
        let cells = d.cells.filter { blocks ? $0.isBlock : $0.isHeading }
        w.folded.formUnion(cells.map(\.from))
    }

    private func zoom(_ by: Int) {
        var s = app.settings
        let base = s.look == "tty" ? 115 : 100
        s.textSize = by == 0 ? 0 : min(max((s.textSize == 0 ? base : s.textSize) + by, 80), 150)
        Task { await app.saveSettings(s) }
    }
}
