import AppKit
import MdshKit
import SwiftUI

/// WindowRoot is one window: the start screen until a file opens, then
/// the document.
struct WindowRoot: View {
    let target: WindowTarget
    @State private var model = WindowModel()
    @Environment(AppModel.self) private var app
    @Environment(\.openWindow) private var openWindow
    @Environment(\.colorScheme) private var systemScheme

    var body: some View {
        let theme = Theme(settings: app.settings, systemDark: systemScheme == .dark)
        Group {
            if let err = app.bridgeError {
                ContentUnavailableView("mdsh-swift can't start", systemImage: "exclamationmark.triangle", description: Text(err))
            } else if let doc = model.doc {
                DocumentView(model: model, doc: doc)
            } else {
                StartView(model: model)
            }
        }
        .environment(\.theme, theme)
        // The fields take code; see AppDelegate.
        .autocorrectionDisabled()
        .preferredColorScheme(app.settings.theme == "" ? nil : theme.colorScheme)
        .background(theme.background)
        .focusedSceneValue(\.windowModel, model)
        .navigationTitle(model.title)
        // The close button's dot too, as other Mac apps show unsaved
        // changes.
        .onChange(of: model.doc?.info.dirty ?? false, initial: true) { _, dirty in
            model.window?.isDocumentEdited = dirty
        }
        .background(WindowAccessor { w in attach(w) })
        .onDrop(of: [.fileURL], isTargeted: nil) { providers in drop(providers) }
        .task {
            WindowOpener.action = openWindow
            WindowRegistry.shared.add(model)
            await model.load(target)
            openPendingFiles()
        }
        .onDisappear {
            WindowRegistry.shared.remove(model)
            Task { await model.closeDocument() }
        }
        .alert("Error", isPresented: Binding(get: { model.error != nil && model.doc != nil }, set: { if !$0 { model.error = nil } })) {
            Button("OK") { model.error = nil }
        } message: {
            Text(model.error ?? "")
        }
    }

    /// attach sets the window up once it exists: it joins the window
    /// that opened it as a tab, and asks about unsaved output before
    /// closing.
    private func attach(_ w: NSWindow) {
        // SwiftUI may set its own delegate after the first look, so the
        // guard is put back whenever the view updates.
        CloseGuard.install(on: w, model: model)
        guard model.window !== w else { return }
        model.window = w
        w.isDocumentEdited = model.doc?.info.dirty ?? false
        w.tabbingIdentifier = "mdsh"
        w.tabbingMode = .preferred
        if let parent = WindowOpener.tabParent, parent !== w, parent.tabbingIdentifier == "mdsh" {
            WindowOpener.tabParent = nil
            parent.addTabbedWindow(w, ordered: .above)
            w.makeKeyAndOrderFront(nil)
        }
    }

    /// The first window opens the files after the first one named on the
    /// command line in tabs of their own.
    private func openPendingFiles() {
        guard app.pendingFiles.count > 1 else {
            app.pendingFiles = []
            return
        }
        let rest = app.pendingFiles.dropFirst()
        app.pendingFiles = []
        for f in rest { WindowOpener.open(.open(f)) }
    }

    /// drop opens dropped markdown files: the first here if this window
    /// shows the start screen, the rest in new tabs.
    private func drop(_ providers: [NSItemProvider]) -> Bool {
        var handled = false
        for p in providers where p.canLoadObject(ofClass: URL.self) {
            handled = true
            _ = p.loadObject(ofClass: URL.self) { url, _ in
                guard let url, ["md", "markdown"].contains(url.pathExtension.lowercased()) else { return }
                Task { @MainActor in
                    if model.doc == nil {
                        await model.open(url.path)
                    } else if let w = WindowRegistry.shared.window(showing: url.path) {
                        w.window?.makeKeyAndOrderFront(nil)
                    } else {
                        WindowOpener.open(.open(url.path))
                    }
                }
            }
        }
        return handled
    }
}

/// WindowAccessor hands over the NSWindow a view is in.
struct WindowAccessor: NSViewRepresentable {
    var onWindow: (NSWindow) -> Void

    func makeNSView(context: Context) -> NSView {
        let v = AccessorView()
        v.onWindow = onWindow
        return v
    }

    func updateNSView(_ v: NSView, context: Context) {
        if let w = v.window { onWindow(w) }
    }

    final class AccessorView: NSView {
        var onWindow: ((NSWindow) -> Void)?
        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            if let w = window { onWindow?(w) }
        }
    }
}

/// CloseGuard stands between a window and SwiftUI's delegate, so the
/// window can ask about unsaved output before it closes. Everything
/// else goes on to SwiftUI's delegate.
final class CloseGuard: NSObject, NSWindowDelegate {
    private weak var original: NSWindowDelegate?
    fileprivate weak var model: WindowModel?
    private static var guards: [ObjectIdentifier: CloseGuard] = [:]

    @MainActor
    static func install(on w: NSWindow, model: WindowModel) {
        if let g = w.delegate as? CloseGuard {
            g.model = model
            return
        }
        let g = CloseGuard()
        g.original = w.delegate
        g.model = model
        guards[ObjectIdentifier(w)] = g
        w.delegate = g
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        guard let model else { return original?.windowShouldClose?(sender) ?? true }
        let choice = MainActor.assumeIsolated { model.closeChoice() }
        switch choice {
        case .cancel:
            return false
        case .close:
            return original?.windowShouldClose?(sender) ?? true
        case .save:
            // Save first, then close; close() doesn't ask again.
            Task { @MainActor in
                if await model.saveForClose() { sender.close() }
            }
            return false
        }
    }

    func windowWillClose(_ notification: Notification) {
        original?.windowWillClose?(notification)
        if let w = notification.object as? NSWindow {
            CloseGuard.guards[ObjectIdentifier(w)] = nil
        }
    }

    override func responds(to aSelector: Selector!) -> Bool {
        super.responds(to: aSelector) || (original?.responds(to: aSelector) ?? false)
    }

    override func forwardingTarget(for aSelector: Selector!) -> Any? {
        if let o = original, o.responds(to: aSelector) { return o }
        return super.forwardingTarget(for: aSelector)
    }
}

// MARK: Focused values

private struct WindowModelKey: FocusedValueKey {
    typealias Value = WindowModel
}

extension FocusedValues {
    var windowModel: WindowModel? {
        get { self[WindowModelKey.self] }
        set { self[WindowModelKey.self] = newValue }
    }
}
