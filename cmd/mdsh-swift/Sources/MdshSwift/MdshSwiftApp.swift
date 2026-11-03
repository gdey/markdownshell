import AppKit
import MdshKit
import SwiftUI

@main
struct MdshSwiftApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var app = AppModel.shared

    var body: some Scene {
        WindowGroup("mdsh", id: "doc", for: WindowTarget.self) { $target in
            WindowRoot(target: target)
                .environment(app)
        } defaultValue: {
            AppModel.shared.pendingFiles.first.map { WindowTarget.open($0) } ?? .start()
        }
        .defaultSize(width: 1280, height: 900)
        .commands { AppCommands(app: app) }
    }
}

/// AppDelegate does what SwiftUI's App doesn't: making the app a
/// regular one when started from a terminal, opening files from Finder,
/// and asking about unsaved output before quitting.
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationWillFinishLaunching(_ notification: Notification) {
        // A write to a bridge that has died fails the call rather than
        // killing the app.
        signal(SIGPIPE, SIG_IGN)
        NSApp.setActivationPolicy(.regular)
        NSWindow.allowsAutomaticWindowTabbing = true
        // Nearly every field here takes code: YAML, shell, paths, names.
        // macOS's corrections (capitals, smart quotes and dashes, text
        // replacement) would rewrite it, so they are off in this app, as
        // mdsh-gui marks its code fields spellcheck="false". Set in the
        // app's own domain, which wins over the system-wide setting.
        for key in ["NSAutomaticCapitalizationEnabled", "NSAutomaticSpellingCorrectionEnabled",
                    "NSAutomaticQuoteSubstitutionEnabled", "NSAutomaticDashSubstitutionEnabled",
                    "NSAutomaticPeriodSubstitutionEnabled", "NSAutomaticTextReplacementEnabled",
                    "NSAutomaticTextCompletionEnabled"] {
            UserDefaults.standard.set(false, forKey: key)
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.activate(ignoringOtherApps: true)
        Snapshot.scheduleIfAsked()
    }

    /// application(_:open:) takes the files Finder sends: Open With, a
    /// double click on a file mdsh-swift owns, or a drop on the Dock icon.
    /// While the app starts they wait for the first window, which opens
    /// them; later each opens as WindowOpener.openFile says.
    func application(_ application: NSApplication, open urls: [URL]) {
        Task { @MainActor in
            let files = urls.filter(\.isFileURL).map(\.standardizedFileURL.path)
            guard WindowOpener.action != nil else {
                AppModel.shared.pendingFiles += files.filter { !AppModel.shared.pendingFiles.contains($0) }
                return
            }
            for f in files { WindowOpener.openFile(f) }
        }
    }

    /// applicationShouldTerminate asks about each window's unsaved
    /// output, saves what the user chose to, and only then stops the
    /// bridge, so the saves reach it.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        Task { @MainActor in
            for w in WindowRegistry.shared.models where w.doc?.info.dirty == true {
                w.window?.makeKeyAndOrderFront(nil)
                switch w.closeChoice() {
                case .cancel:
                    sender.reply(toApplicationShouldTerminate: false)
                    return
                case .save:
                    if !(await w.saveForClose()) {
                        sender.reply(toApplicationShouldTerminate: false)
                        return
                    }
                case .close:
                    break
                }
            }
            AppModel.shared.bridge?.stop()
            sender.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
}

/// WindowOpener opens windows from outside a view, as the app
/// delegate needs, with the openWindow action the first window lends.
@MainActor
enum WindowOpener {
    static var action: OpenWindowAction?
    /// tabParent is the window a window opening now joins as a tab;
    /// nil opens a window of its own.
    static var tabParent: NSWindow?

    static func open(_ target: WindowTarget, asTab: Bool = true) {
        tabParent = asTab ? NSApp.keyWindow : nil
        action?(id: "doc", value: target)
    }

    /// openFile opens file as mdsh-gui opens a file Finder sends: a file
    /// open already comes forward, the window in front takes it if it
    /// shows the start screen, and otherwise it opens in a new tab there.
    static func openFile(_ file: String) {
        let reg = WindowRegistry.shared
        if let w = reg.window(showing: file) {
            w.window?.makeKeyAndOrderFront(nil)
        } else if let w = reg.models.first(where: { $0.window === (NSApp.keyWindow ?? NSApp.mainWindow) }), w.isStart {
            Task { await w.open(file) }
        } else {
            open(.open(file))
        }
        NSApp.activate(ignoringOtherApps: true)
    }

    /// openSettings shows Settings at tab as mdsh-gui does: its tab
    /// comes forward if one is open; otherwise one opens after the tab
    /// in front of the document window in front, or in a window of its
    /// own when there is none.
    static func openSettings(_ tab: SettingsTab = .general) {
        AppModel.shared.settingsTab = tab
        let reg = WindowRegistry.shared
        if let w = reg.models.first(where: \.isSettings)?.window {
            w.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            return
        }
        let docs = reg.models.compactMap(\.window).filter { $0.tabbingIdentifier == "mdsh" && $0.isVisible }
        let front = [NSApp.keyWindow, NSApp.mainWindow].compactMap { $0 }.first { w in docs.contains { $0 === w } }
        tabParent = front ?? docs.first
        action?(id: "doc", value: WindowTarget.settings())
        NSApp.activate(ignoringOtherApps: true)
    }
}

/// WindowRegistry keeps the open windows' models, for Quit and menus.
@MainActor
final class WindowRegistry {
    static let shared = WindowRegistry()
    private var list: [WeakModel] = []

    var models: [WindowModel] { list.compactMap(\.model) }

    func add(_ m: WindowModel) {
        list.removeAll { $0.model == nil }
        list.append(WeakModel(model: m))
    }

    func remove(_ m: WindowModel) { list.removeAll { $0.model == nil || $0.model === m } }

    /// window is the model showing file, if any.
    func window(showing file: String) -> WindowModel? { models.first { $0.doc?.file == file } }

    private struct WeakModel { weak var model: WindowModel? }
}

/// Snapshot draws the app's front window into a PNG and quits, when
/// MDSH_SWIFT_SNAPSHOT names the file, after MDSH_SWIFT_SNAPSHOT_DELAY
/// seconds (default 6). The screenshot script uses it: the app draws
/// itself, so it needs no screen recording permission.
@MainActor
enum Snapshot {
    /// fallback is the window Snapshot makes itself when SwiftUI shows
    /// none, as on a CI runner with no login session.
    private static var fallback: NSWindow?

    static func scheduleIfAsked() {
        let env = ProcessInfo.processInfo.environment
        guard let path = env["MDSH_SWIFT_SNAPSHOT"], !path.isEmpty else { return }
        let delay = Double(env["MDSH_SWIFT_SNAPSHOT_DELAY"] ?? "") ?? 6
        // MDSH_SWIFT_SHOW=settings:TAB draws the Settings page alone on
        // TAB, and settingstab:TAB opens it as a tab of the document
        // window, as Settings… and About do.
        let show = (env["MDSH_SWIFT_SHOW"] ?? "").split(separator: ",")
        let settingsTab = show.first { $0.hasPrefix("settings:") }
            .flatMap { SettingsTab(rawValue: String($0.dropFirst("settings:".count))) }
        let settingsAsTab = show.first { $0.hasPrefix("settingstab:") }
            .flatMap { SettingsTab(rawValue: String($0.dropFirst("settingstab:".count))) }
        if let settingsAsTab {
            DispatchQueue.main.asyncAfter(deadline: .now() + 3) { WindowOpener.openSettings(settingsAsTab) }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
            if let settingsTab {
                AppModel.shared.settingsTab = settingsTab
                let w = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 720, height: 720),
                                 styleMask: [.titled, .closable], backing: .buffered, defer: false)
                w.title = "Settings"
                w.contentView = NSHostingView(rootView: SettingsView().environment(AppModel.shared).frame(width: 720, height: 720))
                w.makeKeyAndOrderFront(nil)
                fallback = w
                return
            }
            guard NSApp.windows.filter({ $0.frame.width > 200 }).isEmpty else { return }
            FileHandle.standardError.write(Data("mdsh-swift: snapshot: SwiftUI showed no window; drawing one\n".utf8))
            let target = AppModel.shared.pendingFiles.first.map { WindowTarget.open($0) } ?? .start()
            let w = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1280, height: 900),
                             styleMask: [.titled, .closable, .resizable], backing: .buffered, defer: false)
            w.contentView = NSHostingView(rootView: WindowRoot(target: target).environment(AppModel.shared))
            w.makeKeyAndOrderFront(nil)
            fallback = w
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + max(delay, 4)) {
            // What was selected, so a scene can be checked from its log.
            // The app was started in the background, so there may be no
            // main window: take the frontmost visible one, as take does.
            let shown = fallback ?? NSApp.mainWindow ?? NSApp.orderedWindows.first { $0.isVisible && $0.frame.width > 200 }
            for m in WindowRegistry.shared.models where m.window === shown {
                let sel = m.selected.flatMap { f in m.doc?.cells.first { $0.from == f } }
                FileHandle.standardError.write(Data("mdsh-swift: snapshot: selected \(sel.map { $0.displayName } ?? "nothing")\n".utf8))
            }
            do {
                try take(to: URL(fileURLWithPath: path))
            } catch {
                FileHandle.standardError.write(Data("mdsh-swift: snapshot: \(error)\n".utf8))
            }
            // Skip asking about unsaved output.
            AppModel.shared.bridge?.stop()
            exit(0)
        }
    }

    static func take(to url: URL) throws {
        let windows = NSApp.windows.filter { $0.isVisible && $0.contentView != nil && $0.frame.width > 200 }
        // An open sheet is drawn instead of its window.
        let front = fallback ?? NSApp.mainWindow ?? windows.first ?? NSApp.keyWindow
        guard let w = front?.attachedSheet ?? front,
              let view = w.contentView?.superview ?? w.contentView
        else {
            throw BridgeError("no window to draw (\(NSApp.windows.count) windows)")
        }
        guard let rep = view.bitmapImageRepForCachingDisplay(in: view.bounds) else {
            throw BridgeError("can't make a bitmap")
        }
        view.cacheDisplay(in: view.bounds, to: rep)
        guard let png = rep.representation(using: .png, properties: [:]) else { throw BridgeError("can't make a PNG") }
        try png.write(to: url)
    }
}
