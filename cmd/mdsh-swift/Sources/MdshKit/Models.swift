import Foundation

// The types the bridge sends, as mdsh-gui's Go types encode them. Fields
// Go leaves out when empty (omitempty) are optional here, with a
// non-optional accessor where that reads better.

/// DocInfo is what the window shows about an open document.
public struct DocInfo: Codable, Equatable, Sendable {
    public var id: Int
    public var file: String
    public var dirty: Bool
    public var changed: Bool
    public var running: Bool
    public var openHistory: Bool
    public var run: RunOptions
    public var args: String
    public var history: Bool
    public var hasOutput: Bool
    public var termCols: Int
    public var termRows: Int
    public var envBanner: String
    /// lastView is how the file was last shown, "rendered", "source" or
    /// "both", or empty; lastHistory whether its History pane was open.
    public var lastView: String?
    public var lastHistory: Bool?

    public var name: String { (file as NSString).lastPathComponent }
}

/// RunOptions are the per-file choices mdsh takes as flags.
public struct RunOptions: Codable, Equatable, Sendable {
    public var args: [String]?
    public var only: String
    public var step: Bool
    public var stopOnError: Bool
    public var shell: String

    /// isSet reports whether any option differs from a plain run.
    public var isSet: Bool { !(args ?? []).isEmpty || !only.isEmpty || step || stopOnError }
}

/// Cell is one piece of the document as the window draws it: prose, or
/// a block (block >= 0).
public struct Cell: Codable, Equatable, Identifiable, Sendable {
    public var block: Int
    public var html: String?
    public var tasks: [Int]?
    public var textbox: TextboxCell?
    public var choice: ChoiceCell?
    public var id_: String?
    public var name: String?
    public var lang: String?
    public var body: String?
    public var attrs: String?
    public var depends: [String]?
    public var output: String?
    public var outputTasks: [Int]?
    public var links: Bool?
    public var ran: Bool
    /// stale says the block ran, but a block its depends= names ran
    /// again since, had its run cleared, or is stale itself.
    public var stale: Bool?
    /// nosave says the block's output lives in the session: saving
    /// keeps what the file holds.
    public var nosave: Bool?
    /// secret says the output may hold a secret, and secretCode the code
    /// too: the card masks them until its eye button shows them.
    public var secret: Bool?
    public var secretCode: Bool?
    /// ms is how long the block's last run this session took, timedOut
    /// says its timeout= ended it, and timeout is its timeout=, all in
    /// milliseconds.
    public var ms: Int?
    public var timedOut: Bool?
    public var timeout: Int?
    public var status: Int
    public var failed: Bool
    public var running: Bool
    public var heading: Int?
    public var sectionFrom: Int?
    public var sectionTo: Int?
    public var collapsed: Bool?
    public var collapsedOutput: Bool?
    public var outputLine: Int?
    public var firstLine: Int?
    public var from: Int
    public var to: Int
    public var line: Int
    public var movable: Bool?
    public var lastLine: Int?
    public var hidden: String?
    public var undeletable: String?
    public var history: Bool?
    public var drop: Bool?
    public var dropAfter: Int
    public var dropBefore: Int

    enum CodingKeys: String, CodingKey {
        case block, html, tasks, textbox, choice
        case id_ = "id"
        case name, lang, body, attrs, depends, output, outputTasks, links, ran, stale, nosave, secret, secretCode, ms, timedOut, timeout, status, failed, running
        case heading, sectionFrom, sectionTo, collapsed, collapsedOutput, outputLine, firstLine
        case from, to, line, movable, lastLine, hidden, undeletable, history, drop, dropAfter, dropBefore
    }

    /// id is the cell's place in the file, which is unique.
    public var id: Int { from }
    /// blockID is the block's id, such as "build".
    public var blockID: String { id_ ?? "" }
    public var isBlock: Bool { block >= 0 }
    public var headingLevel: Int { heading ?? 0 }
    public var isHeading: Bool { headingLevel > 0 }
    /// hasSection reports whether a heading's section has blocks to run.
    public var hasSection: Bool { (sectionTo ?? 0) > (sectionFrom ?? 0) }
    /// displayName is the name as a card shows it, underscores as spaces.
    public var displayName: String { (name ?? "").replacingOccurrences(of: "_", with: " ") }
}

/// TextboxCell is a textbox, drawn as a field to type in.
public struct TextboxCell: Codable, Equatable, Sendable {
    public var block: Int
    public var id: String
    public var text: String
    public var lines: Int
    /// fit is lines=fit: any number of lines, in a box as tall as its text.
    public var fit: Bool?
    public var placeholder: String?
    public var problem: String?
    /// nosave says the value lives in the session: saving keeps the file's.
    public var nosave: Bool?
}

/// ChoiceCell is a choice: a dropdown, radio buttons or checkboxes.
public struct ChoiceCell: Codable, Equatable, Sendable {
    public var block: Int
    public var id: String
    public var mode: String
    public var options: [ChoiceOption]
    public var placeholder: String?
    public var problem: String?
    /// nosave says the value lives in the session: saving keeps the file's.
    public var nosave: Bool?
}

public struct ChoiceOption: Codable, Equatable, Sendable {
    public var label: String
    public var checked: Bool
}

/// BlockEvent is what the bridge sends while a block runs. kind is
/// started, stdout, stderr, done, background, terminal, tty or
/// terminal-end.
public struct BlockEvent: Codable, Equatable, Sendable {
    public var kind: String
    public var block: Int
    public var data: String?
    /// bytes are what a tty block showed, for the terminal.
    public var bytes: Data?
}

/// ConfirmQuestion asks whether to run a block, which is marked confirm
/// or, with of set, is named by block of's depends= and hasn't run.
public struct ConfirmQuestion: Codable, Equatable, Sendable {
    public var block: Int
    public var of: Int?
    public var question: String
}

/// LibraryFile is a markdown file the start screen lists.
public struct LibraryFile: Codable, Equatable, Identifiable, Sendable {
    public var path: String
    public var rel: String
    public var blocks: Int
    public var description: String?
    public var tags: [String]?
    public var mod: String
    public var id: String { path }
    public var name: String { (rel as NSString).lastPathComponent }
    /// folder is the folder the file is in, relative to the start
    /// folder; "" for the start folder itself.
    public var folder: String {
        let d = (rel as NSString).deletingLastPathComponent
        return d == "." ? "" : d
    }
}

/// Collection is a folder with a .mdsh.yaml, shown with its own title,
/// icon, color and background.
public struct Collection: Codable, Equatable, Sendable {
    public var rel: String
    public var title: String?
    public var description: String?
    public var icon: String?
    public var color: String?
    public var tags: [String]?
    public var iconURL: String?
    public var backgroundURL: String?
    public var problems: [String]?
    public var error: String?
}

/// StartScreen is what the start screen lists.
public struct StartScreen: Codable, Equatable, Sendable {
    public var dir: String
    public var home: String
    public var files: [LibraryFile]?
    public var collections: [Collection]?
    public var full: Bool
    public var recent: [String]?
    public var pinned: [String]?
    public var lastSession: String
    /// writable are the listed folders, by rel, whose .mdsh.yaml the
    /// folder settings can save.
    public var writable: [String]?
}

/// FolderForm is what the folder settings dialog edits.
public struct FolderForm: Codable, Equatable, Sendable {
    public var path: String
    public var settings: FolderSettings
    public var images: [String]?
    public var writable: Bool
}

/// FolderSettings is a folder's .mdsh.yaml.
public struct FolderSettings: Codable, Equatable, Sendable {
    public var title: String?
    public var description: String?
    public var icon: String?
    public var color: String?
    public var background: String?
    public var tags: [String]?
}

/// AppSettings are the app-wide settings, as mdsh-gui keeps them.
public struct AppSettings: Codable, Equatable, Sendable {
    public var autosave: Bool = false
    public var vim: Bool = false
    public var noSpellCheck: Bool = false
    public var neovim: Bool = false
    public var neovimPath: String = ""
    public var neovimClean: Bool = false
    public var neovimOwnColors: Bool = false
    public var theme: String = ""
    public var look: String = ""
    public var textSize: Int = 0
    public var uiFont: String = ""
    public var codeFont: String = ""
    public var lineSpacing: String = ""
    public var noSprockets: Bool = false
    public var noRules: Bool = false
    public var noShadows: Bool = false
    public var accent: String = ""
    public var historyPane: String = ""
    /// historyFocus is how History's prompt shows it has the keyboard:
    /// "cursor" for a blinking block cursor, empty for the outline.
    public var historyFocus: String = ""
    /// secretShowSeconds is how long a secret stays shown, or on the
    /// clipboard; 0 is a minute.
    public var secretShowSeconds: Int = 0
    public var shell: String = ""
    public var historyRC: String = ""
    public var view: String = ""
    public var startFolder: String = ""
    public var templateFolder: String = ""
    public var sessionFolder: String = ""
    public var sessionTemplate: String = ""
    public var lastFolder: String = ""
    public var recent: [String]? = nil
    public var pinned: [String]? = nil

    public init() {}

    enum CodingKeys: String, CodingKey {
        case autosave, vim, noSpellCheck, neovim, neovimPath, neovimClean, neovimOwnColors
        case theme, look, textSize, uiFont, codeFont, lineSpacing, noSprockets, noRules, noShadows
        case accent, historyPane, historyFocus, secretShowSeconds, shell
        case historyRC
        case view, startFolder, templateFolder, sessionFolder, sessionTemplate, lastFolder, recent, pinned
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func s(_ k: CodingKeys) -> String { (try? c.decodeIfPresent(String.self, forKey: k)) ?? nil ?? "" }
        func b(_ k: CodingKeys) -> Bool { (try? c.decodeIfPresent(Bool.self, forKey: k)) ?? nil ?? false }
        autosave = b(.autosave); vim = b(.vim); noSpellCheck = b(.noSpellCheck)
        neovim = b(.neovim); neovimPath = s(.neovimPath); neovimClean = b(.neovimClean); neovimOwnColors = b(.neovimOwnColors)
        theme = s(.theme); look = s(.look)
        textSize = (try? c.decodeIfPresent(Int.self, forKey: .textSize)) ?? nil ?? 0
        secretShowSeconds = (try? c.decodeIfPresent(Int.self, forKey: .secretShowSeconds)) ?? nil ?? 0
        uiFont = s(.uiFont); codeFont = s(.codeFont); lineSpacing = s(.lineSpacing)
        noSprockets = b(.noSprockets); noRules = b(.noRules); noShadows = b(.noShadows)
        accent = s(.accent); historyPane = s(.historyPane); historyFocus = s(.historyFocus); shell = s(.shell); historyRC = s(.historyRC)
        view = s(.view); startFolder = s(.startFolder); templateFolder = s(.templateFolder)
        sessionFolder = s(.sessionFolder); sessionTemplate = s(.sessionTemplate); lastFolder = s(.lastFolder)
        recent = try? c.decodeIfPresent([String].self, forKey: .recent)
        pinned = try? c.decodeIfPresent([String].self, forKey: .pinned)
    }
}

/// FrontMatter is the front matter form's fields.
public struct FrontMatter: Codable, Equatable, Sendable {
    public var exists: Bool
    public var title: String
    public var description: String
    public var descriptionKey: String
    public var tags: [String]?
    public var shell: String
    public var onError: String
    public var step: Bool
    public var interpolate: String
    public var showStderr: String
    public var cols: Int
    public var rows: Int
    public var shareAll: Bool
    public var share: [String]?
    public var source: [String]?
    public var other: String
}

/// Names are the document's block ids, envs and files, for completion.
public struct Names: Codable, Equatable, Sendable {
    public var ids: [String]?
    public var envs: [String]?
    public var assets: [String]?
    public var files: [String]?
    public var assetsDir: String
}

/// Completion completes the word at the end of what is typed in the
/// History pane.
public struct Completion: Codable, Equatable, Sendable {
    public var word: String
    public var items: [String]?
}

/// Prompt is the shell's PS1 and PS2, expanded.
public struct Prompt: Codable, Equatable, Sendable {
    public var ps1: String
    public var ps2: String
    public init(ps1: String, ps2: String) {
        self.ps1 = ps1
        self.ps2 = ps2
    }
}

/// Move is a block move: the file before (base) and after (text) it.
public struct Move: Codable, Equatable, Sendable {
    public var base: String
    public var text: String
    public var block: Int
    public var line: Int
    public var fromLine: Int
    public var warnings: [String]?
    public var note: String?
}

/// Deleted says which block a delete took out.
public struct Deleted: Codable, Equatable, Sendable {
    public var name: String
    public var warnings: [String]?
}

/// Fold is a fold the rendered view writes to the file.
public struct Fold: Codable, Equatable, Sendable {
    public var block: Int
    public var output: Bool
    public var heading: Int
    public var on: Bool
    public init(block: Int, output: Bool, heading: Int, on: Bool) {
        self.block = block
        self.output = output
        self.heading = heading
        self.on = on
    }
}

/// ReportRequest is what the Create Report dialog asks for.
public struct ReportRequest: Codable, Equatable, Sendable {
    public var format: String
    public var hideCode: Bool
    public var hideOutput: Bool
    public var saved: Bool
    public var ran: Bool
    public var dest: String
    public init(format: String = "pdf", hideCode: Bool = false, hideOutput: Bool = false, saved: Bool = false, ran: Bool = false, dest: String = "") {
        self.format = format
        self.hideCode = hideCode
        self.hideOutput = hideOutput
        self.saved = saved
        self.ran = ran
        self.dest = dest
    }
}

/// ReportStart is what the Create Report dialog shows when it opens.
public struct ReportStart: Codable, Equatable, Sendable {
    public var ran: Int
    public var total: Int
    public var dest: String
    public var home: String
}

/// ConsoleEntry is one line of the Console pane.
public struct ConsoleEntry: Codable, Equatable, Identifiable, Sendable {
    public var seq: Int
    public var time: String
    public var level: String
    public var message: String
    public var file: String?
    public var line: Int?
    /// fix names the fix for a warning about file, for a Fix button.
    public var fix: String?
    /// stale is set when an edit fixed what the entry warned of.
    public var stale: Bool?
    public var id: Int { seq }

    public init(seq: Int, time: String, level: String, message: String, file: String? = nil, line: Int? = nil, fix: String? = nil) {
        self.seq = seq
        self.time = time
        self.level = level
        self.message = message
        self.file = file
        self.line = line
        self.fix = fix
    }
}

/// StaleWarning says a warning about file no longer applies.
public struct StaleWarning: Codable, Equatable, Sendable {
    public var file: String
    public var message: String
}

/// FixInfo says what a warning's fix does, to ask first.
public struct FixInfo: Codable, Equatable, Sendable {
    public var label: String
    public var detail: String
}

/// About is what the About tab shows about the bridge.
public struct About: Codable, Equatable, Sendable {
    public var version: String
    public var commit: String
    public var buildTime: String
    public var modified: Bool
    public var goVersion: String
    public var os: String
    public var arch: String
    public var home: String
    public var settingsFile: String
    public var zshrc: String
    public var tools: [Tool]?
}

public struct Tool: Codable, Equatable, Sendable, Identifiable {
    public var name: String
    public var note: String
    public var path: String
    public var version: String
    public var id: String { name }
}

/// ZshWrapper is whether ~/.zshrc has the mdsh function.
public struct ZshWrapper: Codable, Equatable, Sendable {
    public var path: String
    /// bashPath is the .bashrc the function also goes in, nil or empty
    /// when there is none.
    public var bashPath: String?
    public var home: String
    public var on: Bool
}

/// AttrSpec describes a block attribute, for completion and help.
public struct AttrSpec: Codable, Equatable, Sendable {
    public var name: String
    public var kind: String
    public var bare: Bool?
    public var values: [String]?
    public var `default`: String?
    public var help: String
}

/// SourceBlock is a block in the source text, for the Depends On dialog
/// and the source view's gutter.
public struct SourceBlock: Codable, Equatable, Sendable {
    public var line: Int
    public var id: String
    public var runs: Bool
}

extension AppSettings {
    /// secretShow is how long a secret stays shown after its eye button
    /// shows it, and on the clipboard after it is copied.
    public var secretShow: TimeInterval { secretShowSeconds <= 0 ? 60 : TimeInterval(secretShowSeconds) }
}

/// CastInfo is a recording the cast player loaded: its id, its frames'
/// delays in milliseconds, its size in pixels, and the frame its link
/// names, counting from 1, or 0 for the last.
public struct CastInfo: Codable, Equatable, Sendable {
    public var id: String
    public var cols: Int
    /// start is when the first frame shows, in milliseconds.
    public var start: Int
    public var delays: [Int]
    public var width: Int
    public var height: Int
    public var frame: Int
}

/// AuditEvent is one event of a file's event log, as the Console's
/// Events tab shows it: its shells starting and ending, and why, blocks
/// running, the file changing on disk. Lost is set for one that lost
/// what blocks set in a shell.
public struct AuditEvent: Codable, Equatable, Sendable, Identifiable {
    public var seq: Int
    public var time: String
    public var event: String
    public var text: String
    public var lost: Bool?
    /// json is the event as mdsh --audit writes it, for Copy.
    public var json: String
    public var id: Int { seq }

    public init(seq: Int, time: String, event: String, text: String, lost: Bool? = nil, json: String = "") {
        self.seq = seq
        self.time = time
        self.event = event
        self.text = text
        self.lost = lost
        self.json = json
    }
}
