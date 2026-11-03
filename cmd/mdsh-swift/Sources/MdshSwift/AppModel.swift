import AppKit
import MdshKit
import Observation
import SwiftUI

/// AppModel is the app: the bridge, the settings, the console, and the
/// open documents, which windows share by the bridge's document id.
@MainActor
@Observable
final class AppModel {
    static let shared = AppModel()

    private(set) var bridge: Bridge?
    var bridgeError: String?
    var settings = AppSettings()
    var console: [ConsoleEntry] = []
    /// unseenProblems counts warnings and errors logged while the
    /// Console was closed in every window, for its badge.
    var unseenProblems = 0
    var about: About?

    /// documents are the open documents by id. A window holds its
    /// document strongly; this table is only for routing events.
    private var documents: [Int: WeakDocument] = [:]
    /// Files named on the command line, opened once the first window
    /// shows.
    var pendingFiles: [String] = []
    var startFolderArg: String?
    var commandLineRun = CommandLineRun()
    /// commandLineRunFor is the file commandLineRun is for, until its
    /// window takes it.
    var commandLineRunFor: String?

    private init() {
        parseArguments()
        start()
    }

    // MARK: Bridge

    func start() {
        guard let exe = Bridge.locate() else {
            bridgeError = "Can't find mdsh-swift-bridge. Build it with scripts/build-app.sh, or set MDSH_SWIFT_BRIDGE to its path."
            return
        }
        // MDSH_SWIFT_SETTINGS names another settings file, as the
        // screenshot script uses.
        var args: [String] = []
        if let f = ProcessInfo.processInfo.environment["MDSH_SWIFT_SETTINGS"], !f.isEmpty {
            args = ["--settings", f]
        }
        do {
            let b = try Bridge(executable: exe, arguments: args)
            // Events come on the main queue, in order; handling them there
            // keeps them in order with the answers to calls.
            b.onEvent = { ev in MainActor.assumeIsolated { AppModel.shared.handle(ev) } }
            b.onExit = { status in
                Task { @MainActor in
                    AppModel.shared.bridgeError = "The bridge stopped (status \(status)). Quit and open mdsh-swift again."
                }
            }
            b.onStderr = { _ in }
            bridge = b
            Task { await loadSettings() }
        } catch {
            bridgeError = "Can't start the bridge: \(error.localizedDescription)"
        }
    }

    func loadSettings() async {
        guard let bridge else { return }
        if let s: AppSettings = try? await bridge.call("settings") {
            settings = s
        }
    }

    /// call calls the bridge, logging a failure to the console.
    @discardableResult
    func call<R: Decodable>(_ method: String, _ params: [String: JSONValue] = [:], as: R.Type = R.self) async -> R? {
        guard let bridge else {
            log("error", bridgeError ?? "no bridge")
            return nil
        }
        do {
            return try await bridge.call(method, params, as: R.self)
        } catch {
            log("error", "\(method): \(error.localizedDescription)")
            return nil
        }
    }

    /// tryCall calls the bridge and throws its error, for callers that
    /// show it themselves.
    func tryCall<R: Decodable>(_ method: String, _ params: [String: JSONValue] = [:], as: R.Type = R.self) async throws -> R {
        guard let bridge else { throw BridgeError(bridgeError ?? "no bridge") }
        return try await bridge.call(method, params, as: R.self)
    }

    func tryCall(_ method: String, _ params: [String: JSONValue] = [:]) async throws {
        _ = try await tryCall(method, params, as: Empty.self)
    }

    // MARK: Events

    func register(_ d: DocumentModel) { documents[d.id] = WeakDocument(d) }

    func unregister(_ d: DocumentModel) {
        if documents[d.id]?.doc === d { documents[d.id] = nil }
    }

    private func handle(_ ev: BridgeEvent) {
        switch ev.name {
        case "console":
            if let e = try? ev.decode(ConsoleEntry.self) { add(e) }
        case "console-stale":
            // A warning an edit fixed stays, struck through.
            if let w = try? ev.decode(StaleWarning.self) {
                for i in console.indices where console[i].file == w.file && console[i].message == w.message {
                    console[i].stale = true
                }
            }
        case "settings":
            if let s = try? ev.decode(AppSettings.self) { settings = s }
        default:
            documents[ev.doc]?.doc?.handle(ev)
        }
    }

    // MARK: Console

    private var localSeq = 1_000_000

    func log(_ level: String, _ message: String, file: String? = nil, line: Int? = nil) {
        localSeq += 1
        let t = ISO8601DateFormatter().string(from: Date())
        add(ConsoleEntry(seq: localSeq, time: t, level: level, message: message, file: file, line: line))
    }

    private func add(_ e: ConsoleEntry) {
        console.append(e)
        if console.count > 1000 { console.removeFirst(console.count - 1000) }
        if e.level == "warn" || e.level == "error" { unseenProblems += 1 }
    }

    func clearConsole() {
        console.removeAll()
        unseenProblems = 0
    }

    // MARK: Settings

    func saveSettings(_ s: AppSettings) async {
        guard let p = try? JSONValue(encoding: s), case .object(let o) = p else { return }
        if let got: AppSettings = await call("setSettings", o) {
            settings = got
        }
    }

    func pin(_ file: String, _ on: Bool) async {
        await call("pin", ["file": .string(file), "pin": .bool(on)], as: Empty.self)
        await loadSettings()
    }

    // MARK: Command line

    /// CommandLineRun are mdsh-gui's flags that set the Run Options of
    /// the file named on the command line.
    struct CommandLineRun {
        var only = ""
        var step = false
        var stopOnError = false
        var args: [String] = []
    }

    private func parseArguments() {
        var args = Array(CommandLine.arguments.dropFirst())
        // Xcode and LaunchServices add -NSDocumentRevisionsDebugMode and
        // -psn_ arguments; skip them.
        var kept: [String] = []
        var skip = false
        for a in args {
            if skip { skip = false; continue }
            if a.hasPrefix("-psn_") { continue }
            // -NSDocumentRevisionsDebugMode YES and the like come as pairs.
            if a.hasPrefix("-NS") || a.hasPrefix("-Apple") { skip = true; continue }
            kept.append(a)
        }
        args = kept
        var i = 0
        while i < args.count {
            let a = args[i]
            switch a {
            case "--version", "-version":
                print("mdsh-swift \(AppInfo.version)")
                exit(0)
            case "--help", "-h", "-help":
                print(AppInfo.usage)
                exit(0)
            case "--only":
                i += 1
                if i < args.count { commandLineRun.only = args[i] }
            case "--step":
                commandLineRun.step = true
            case "--stop-on-error":
                commandLineRun.stopOnError = true
            default:
                if a.hasPrefix("-") {
                    FileHandle.standardError.write(Data("mdsh-swift: unknown flag \(a)\n\(AppInfo.usage)\n".utf8))
                    exit(2)
                }
                let path = (a as NSString).expandingTildeInPath
                let url = URL(fileURLWithPath: path).standardizedFileURL
                var isDir: ObjCBool = false
                if FileManager.default.fileExists(atPath: url.path, isDirectory: &isDir), isDir.boolValue {
                    startFolderArg = url.path
                } else {
                    pendingFiles.append(url.path)
                    commandLineRunFor = url.path
                    commandLineRun.args = Array(args[(i + 1)...])
                    i = args.count
                }
            }
            i += 1
        }
    }
}

private struct WeakDocument {
    weak var doc: DocumentModel?
    init(_ d: DocumentModel) { doc = d }
}

enum AppInfo {
    static var version: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "(devel)"
    }

    static let usage = """
        usage: mdsh-swift [flags] [file.md [args...] | folder]

        Opens markdown files and runs their code blocks, as mdsh-gui does.
        With no file, the start screen lists the markdown files in folder,
        or in the start folder from Settings.

          --only ID          run only block ID and the blocks it needs
          --step             ask before every block
          --stop-on-error    stop at the first failing block
          --version          print the version
        """
}
