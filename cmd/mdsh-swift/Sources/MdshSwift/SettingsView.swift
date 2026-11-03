import AppKit
import MdshKit
import SwiftUI

/// SettingsTab names the Settings window's tabs.
enum SettingsTab: String, Hashable {
    case general, look, editing, shell, files, about
    /// requested is the tab the next Settings window opens on, as About
    /// mdsh-swift asks for About.
    @MainActor static var requested: SettingsTab?
}

/// SettingsView is the Settings window. Changes save at once and every
/// window follows them.
struct SettingsView: View {
    @Environment(AppModel.self) private var app
    @State private var s = AppSettings()
    @State private var loaded = false
    @State private var tab: SettingsTab = .general
    @Environment(\.colorScheme) private var systemScheme

    private static let tabs: [(SettingsTab, String, String)] = [
        (.general, "General", "gearshape"), (.look, "Look", "paintpalette"), (.editing, "Editing", "pencil"),
        (.shell, "Shell", "terminal"), (.files, "Files", "folder"), (.about, "About", "info.circle"),
    ]

    var body: some View {
        // The window is drawn in the look it sets, as mdsh-gui's is, and
        // follows a change at once.
        let theme = Theme(settings: loaded ? s : app.settings, systemDark: systemScheme == .dark)
        Group {
            if theme.look == .plain {
                TabView(selection: $tab) {
                    ForEach(Self.tabs, id: \.0) { t, title, icon in
                        content(t).tabItem { Label(title, systemImage: icon) }.tag(t)
                    }
                }
                .padding(20)
            } else {
                VStack(spacing: 0) {
                    tabBar(theme)
                    content(tab)
                        .scrollContentBackground(.hidden)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
                .background(theme.background)
                .foregroundStyle(theme.text)
                .tint(theme.look == .tty ? theme.text : theme.accent)
                .font(theme.uiFont())
            }
        }
        .environment(\.theme, theme)
        .autocorrectionDisabled()
        .preferredColorScheme(s.theme == "" ? nil : theme.colorScheme)
        .onAppear {
            s = app.settings
            loaded = true
            if let t = SettingsTab.requested {
                tab = t
                SettingsTab.requested = nil
            }
        }
        .onChange(of: app.settings) { _, new in if new != s { s = new } }
        .onChange(of: s) { _, new in
            guard loaded, new != app.settings else { return }
            Task { await app.saveSettings(new) }
        }
    }

    /// tabBar is mdsh-gui's tab row in the paper looks: the names on the
    /// look's side strip, the open one bold and underlined in ink.
    private func tabBar(_ theme: Theme) -> some View {
        HStack(spacing: 2) {
            ForEach(Self.tabs, id: \.0) { t, title, _ in
                let on = tab == t
                Button { tab = t } label: {
                    Text(theme.look == .blueprint ? title.uppercased() : title)
                        .font(theme.uiFont(weight: on ? .bold : .regular))
                        .foregroundStyle(on ? theme.text : theme.muted)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .overlay(alignment: .bottom) { Rectangle().fill(on ? theme.text : .clear).frame(height: 2) }
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(on ? [.isSelected] : [])
            }
            Spacer()
        }
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .background(theme.side)
        .overlay(alignment: .bottom) { Rectangle().fill(theme.border).frame(height: 1) }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Settings")
    }

    @ViewBuilder private func content(_ t: SettingsTab) -> some View {
        switch t {
        case .general: general
        case .look: look
        case .editing: editing
        case .shell: shell
        case .files: files
        case .about: AboutTab()
        }
    }

    // MARK: Tabs

    private var general: some View {
        Form {
            Picker("Open files in", selection: $s.view) {
                Text("The view the last tab had").tag("")
                Text("The rendered view").tag("rendered")
                Text("The source view").tag("source")
                Text("Both, side by side").tag("both")
            }
            .settingHelp("The view a file opens in. Each tab can switch views from its toolbar.")
            Picker("Start folder", selection: Binding(get: { s.startFolder.isEmpty ? "last" : "fixed" }, set: { v in
                if v == "last" { s.startFolder = "" } else if s.startFolder.isEmpty { chooseFolder { s.startFolder = $0 } }
            })) {
                Text("The last folder used").tag("last")
                Text("A fixed folder").tag("fixed")
            }
            .settingHelp("What the start screen lists when mdsh isn't started from a terminal. From a terminal, it lists the current folder.")
            if !s.startFolder.isEmpty {
                LabeledContent("") {
                    HStack {
                        Text(s.startFolder).textSelection(.enabled)
                        Button("Choose…") { chooseFolder { s.startFolder = $0 } }
                    }
                }
            }
        }
        .formStyle(.grouped)
    }

    private var look: some View {
        Form {
            Picker("Theme", selection: $s.theme) {
                Text("Match the system").tag("")
                Text("Light").tag("light")
                Text("Dark").tag("dark")
            }
            .settingHelp("The editors' colors follow it. Match the system follows the OS.")
            Picker("Look", selection: $s.look) {
                ForEach(Look.allCases) { Text($0.label).tag($0.rawValue) }
            }
            .settingHelp("Teletype sets everything in a typewriter face on fan-fold paper, or green phosphor when the theme is dark. Blueprint draws in white lines on blue paper with a drafting grid, a deeper night blue when the theme is dark. Bubble Gum is Hubba Bubba pink with bubbles across the page, puffy cards and bubble-lettered headings, or berry when the theme is dark.")
            LabeledContent("Text size") {
                HStack {
                    Slider(value: Binding(get: { Double(s.textSize == 0 ? (s.look == "tty" ? 115 : 100) : s.textSize) },
                                          set: { s.textSize = Int($0.rounded() / 5) * 5 }),
                           in: 80...150, step: 5)
                    Text("\(s.textSize == 0 ? (s.look == "tty" ? 115 : 100) : s.textSize)%").monospacedDigit().frame(width: 44)
                    Button("Default") { s.textSize = 0 }.disabled(s.textSize == 0)
                }
            }
            .settingHelp("Scales all the text, code and terminals too.")
            Picker("Interface font", selection: $s.uiFont) {
                Text("The look's own").tag("")
                ForEach(fonts, id: \.self) { Text($0).tag($0) }
            }
            .settingHelp("The face for menus, prose and Settings. Fonts listed are the ones found on this computer.")
            Picker("Code font", selection: $s.codeFont) {
                Text("The look's own").tag("")
                ForEach(monoFonts, id: \.self) { Text($0).tag($0) }
            }
            .settingHelp("The face for code, output and the terminals.")
            Picker("Line spacing", selection: $s.lineSpacing) {
                Text("Compact").tag("compact")
                Text("Normal").tag("")
                Text("Relaxed").tag("relaxed")
            }
            .settingHelp("The space between lines of prose and code.")
            LabeledContent("Accent color") {
                HStack {
                    Picker("", selection: Binding(get: { accents.contains { $0.1 == s.accent } ? s.accent : "custom" }, set: { v in
                        if v != "custom" { s.accent = v } else if s.accent.isEmpty { s.accent = "#0969da" }
                    })) {
                        ForEach(accents, id: \.1) { Text($0.0).tag($0.1) }
                        Text("Custom…").tag("custom")
                    }
                    .labelsHidden()
                    if !accents.contains(where: { $0.1 == s.accent }) {
                        ColorPicker("", selection: Binding(get: { Color(nsColor: Theme.nsColor(s.accent)) }, set: { s.accent = hex($0) }), supportsOpacity: false)
                            .labelsHidden()
                    }
                }
            }
            .settingHelp("Links, run buttons and what is selected.")
            Picker("History pane", selection: $s.historyPane) {
                Text("The look's own").tag("")
                Text("Match the window").tag("match")
                Text("Inverted").tag("invert")
                Text("Always dark").tag("dark")
            }
            .settingHelp("Inverted is light under a dark window and dark under a light one. The look's own is always dark for Plain and the window's for Teletype, Blueprint and Bubble Gum.")
            Picker("History prompt focus", selection: $s.historyFocus) {
                Text("Outline").tag("")
                Text("Blinking cursor").tag("cursor")
            }
            .settingHelp("How the History pane's prompt shows it has the keyboard: an outline around it, or a blinking block cursor where you type.")
            // The paper's group shows only for the looks drawn on paper.
            if s.look != "" {
                Section {
                    if s.look == "tty" {
                        Toggle("Sprocket strips", isOn: Binding(get: { !s.noSprockets }, set: { s.noSprockets = !$0 }))
                    }
                    Toggle(s.look == "blueprint" ? "Drafting grid" : s.look == "gum" ? "Bubbles" : "Ruled lines", isOn: Binding(get: { !s.noRules }, set: { s.noRules = !$0 }))
                    if s.look == "tty" || s.look == "gum" {
                        Toggle("Shadows under cards", isOn: Binding(get: { !s.noShadows }, set: { s.noShadows = !$0 }))
                    }
                } header: {
                    Text(s.look == "tty" ? "Teletype paper" : s.look == "gum" ? "Bubble gum wrapper" : "Blueprint sheet")
                } footer: {
                    Text("What the paper is drawn with.").font(.caption).foregroundStyle(.secondary)
                }
            }
        }
        .formStyle(.grouped)
    }

    private var editing: some View {
        Form {
            Toggle("Save the file after each run", isOn: $s.autosave)
                .settingHelp("Off by default: the window shows • while output isn't saved.")
            Picker("Show secrets for", selection: $s.secretShowSeconds) {
                Text("15 seconds").tag(15)
                Text("30 seconds").tag(30)
                Text("1 minute").tag(0)
                Text("2 minutes").tag(120)
                Text("5 minutes").tag(300)
            }
            .settingHelp("How long a block marked secret stays shown after you press its eye button, and how long a secret you copy stays on the clipboard.")
            Toggle("Vim key bindings in the code editor", isOn: $s.vim)
                .settingHelp("Normal, insert and visual modes; :w saves the file and :q closes the editor. Not in mdsh-swift yet.")
            // mdsh-swift has no Neovim pane yet; the rows are kept so the
            // settings read as mdsh-gui's, and stay off.
            Toggle(isOn: $s.neovim) {
                HStack(spacing: 6) {
                    Text("Use Neovim for the source pane")
                    Badge(text: "Experimental", color: .orange)
                }
            }
            .disabled(true)
            .settingHelp("Still has rough edges. Needs Vim key bindings on. Not in mdsh-swift yet.")
            Toggle("Start Neovim without my config", isOn: $s.neovimClean)
                .disabled(true)
                .settingHelp("Runs it with --clean, skipping your init.lua or init.vim and plugins.")
            Toggle("Use the app's colors in Neovim", isOn: Binding(get: { !s.neovimOwnColors }, set: { s.neovimOwnColors = !$0 }))
                .disabled(true)
                .settingHelp("Neovim follows the Theme, as the built-in editor does. Off keeps the colorscheme from your config.")
            TextField("Neovim to run", text: $s.neovimPath, prompt: Text("nvim on PATH"))
                .disabled(true)
            Toggle("Check spelling in the code editor", isOn: Binding(get: { !s.noSpellCheck }, set: { s.noSpellCheck = !$0 }))
                .settingHelp("Underlines misspelled words in the prose, not in code, links or HTML.")
        }
        .formStyle(.grouped)
    }

    private var shell: some View {
        Form {
            TextField("Shell for every file", text: $s.shell, prompt: Text("From the file's front matter (default)"))
                .settingHelp("Overrides the shell from each file's front matter. Empty uses the file's: zsh when it has zsh blocks, else sh.")
            HStack {
                TextField("File the History pane's shell reads", text: $s.historyRC, prompt: Text("None, or a file such as ~/.zshrc"))
                Button("Choose…") { chooseFile { s.historyRC = $0 } }
            }
            .settingHelp("Read as History is turned on, as an interactive shell reads its rc file, for your aliases, functions and prompt. Blocks get them too while History is on. A file named for one shell, such as .zshrc or .bashrc, is read only when blocks run in that shell.")
            ZshWrapperToggle()
        }
        .formStyle(.grouped)
    }

    private var files: some View {
        Form {
            folderRow("Templates folder", $s.templateFolder, clear: "Clear")
                .settingHelp("New File… offers the markdown files in this folder to start from, as well as a blank file.")
            folderRow("Sessions folder", $s.sessionFolder, clear: "Default")
                .settingHelp("New Session on the start screen opens the day's file here, such as 2026-11-02.md, making it the first time. Last Session opens the file here changed last. Both open with History.")
            LabeledContent("Session template") {
                HStack {
                    Text(s.sessionTemplate.isEmpty ? "None" : s.sessionTemplate).foregroundStyle(.secondary).textSelection(.enabled)
                    Button("Choose…") { chooseFile { s.sessionTemplate = $0 } }
                    Button("Clear") { s.sessionTemplate = "" }.disabled(s.sessionTemplate.isEmpty)
                }
            }
            .settingHelp("A new session starts as a copy of this markdown file.")
        }
        .formStyle(.grouped)
    }

    private func folderRow(_ label: String, _ value: Binding<String>, clear: String) -> some View {
        LabeledContent(label) {
            HStack {
                Text(value.wrappedValue.isEmpty ? (clear == "Default" ? "~/.mdsh/sessions" : "None") : value.wrappedValue)
                    .foregroundStyle(.secondary).textSelection(.enabled)
                Button("Choose…") { chooseFolder { value.wrappedValue = $0 } }
                Button(clear) { value.wrappedValue = "" }.disabled(value.wrappedValue.isEmpty)
            }
        }
    }

    // MARK: Helpers

    private let accents = [("The look's own", ""), ("Green", "#3fb950"), ("Amber", "#d4a017"), ("Blue", "#2f81f7"), ("Red", "#d1495b"), ("Purple", "#8250df")]

    private var fonts: [String] { NSFontManager.shared.availableFontFamilies }

    private var monoFonts: [String] {
        NSFontManager.shared.availableFontFamilies.filter { fam in
            NSFont(name: fam, size: 12).map { $0.isFixedPitch || $0.fontDescriptor.symbolicTraits.contains(.monoSpace) } ?? false
        }
    }

    private func hex(_ c: Color) -> String {
        let n = NSColor(c).usingColorSpace(.sRGB) ?? .black
        return String(format: "#%02x%02x%02x", Int(n.redComponent * 255), Int(n.greenComponent * 255), Int(n.blueComponent * 255))
    }

    private func chooseFolder(_ then: (String) -> Void) {
        let p = NSOpenPanel()
        p.canChooseDirectories = true
        p.canChooseFiles = false
        p.canCreateDirectories = true
        if p.runModal() == .OK, let u = p.url { then(u.path) }
    }

    private func chooseFile(_ then: (String) -> Void) {
        let p = NSOpenPanel()
        p.canChooseFiles = true
        p.showsHiddenFiles = true
        if p.runModal() == .OK, let u = p.url { then(u.path) }
    }
}

/// ZshWrapperToggle adds the mdsh function to ~/.zshrc, or takes it out.
struct ZshWrapperToggle: View {
    @Environment(AppModel.self) private var app
    @State private var state: ZshWrapper?
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading) {
            Toggle("Give blocks your zsh aliases and functions", isOn: Binding(get: { state?.on ?? false }, set: { on in
                Task {
                    do { state = try await app.tryCall("setZshWrapper", ["on": .init(on)]) } catch { self.error = error.localizedDescription }
                }
            }))
            .disabled(state == nil)
            Text("Adds an mdsh function to \(state.map { tilde($0.path, $0.home) } ?? "~/.zshrc") that passes your aliases and functions to mdsh.")
                .font(.caption).foregroundStyle(.secondary)
            if let error { Text(error).foregroundStyle(.red).font(.caption) }
        }
        .task { state = try? await app.tryCall("zshWrapper") }
    }
}

/// AboutTab shows the version, build, paths, tools and links.
struct AboutTab: View {
    @Environment(AppModel.self) private var app
    @State private var about: About?

    private let links = [
        ("Documentation", "https://gdey.github.io/markdownshell/"),
        ("Source code", "https://github.com/gdey/markdownshell"),
        ("Releases", "https://github.com/gdey/markdownshell/releases"),
        ("Changelog", "https://github.com/gdey/markdownshell/blob/main/CHANGELOG.md"),
        ("Report a problem", "https://github.com/gdey/markdownshell/issues/new"),
    ]

    var body: some View {
        Form {
            Section {
                HStack(spacing: 12) {
                    Image(nsImage: NSApp.applicationIconImage).resizable().frame(width: 56, height: 56)
                        .accessibilityHidden(true)
                    Text("mdsh-swift").font(.largeTitle.weight(.semibold))
                    Spacer()
                    Button("Copy") { copy() }
                        .help("Copy the version, build and tools, for a bug report")
                }
                LabeledContent("Version", value: AppInfo.version)
                if let a = about {
                    LabeledContent("Bridge", value: a.version)
                    if !a.commit.isEmpty {
                        LabeledContent("Build", value: "\(a.commit.prefix(12))\(a.modified ? " modified" : "") \(a.buildTime)")
                    }
                    LabeledContent("Go", value: a.goVersion)
                    LabeledContent("Platform", value: "\(a.os)/\(a.arch)")
                }
            }
            if let a = about {
                Section("Paths") {
                    pathRow("Settings", a.settingsFile, a.home)
                    pathRow(".zshrc", a.zshrc, a.home)
                    let start = app.settings.startFolder.isEmpty ? app.settings.lastFolder : app.settings.startFolder
                    if !start.isEmpty { pathRow("Start folder", start, a.home) }
                    if !app.settings.templateFolder.isEmpty {
                        pathRow("Templates folder", app.settings.templateFolder, a.home)
                    }
                }
                Section {
                    ForEach(a.tools ?? []) { t in
                        LabeledContent {
                            Text(t.path.isEmpty ? "not found" : [tilde(t.path, a.home), t.version].filter { !$0.isEmpty }.joined(separator: " · "))
                                .foregroundStyle(t.path.isEmpty ? .red : .secondary)
                                .textSelection(.enabled)
                        } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(t.name)
                                if !t.note.isEmpty { Text(t.note).font(.caption).foregroundStyle(.secondary) }
                            }
                        }
                    }
                } header: {
                    Text("Tools")
                } footer: {
                    Text("Programs mdsh-swift runs, as found on this app's PATH.").font(.caption).foregroundStyle(.secondary)
                }
            }
            Section("Links") {
                ForEach(links, id: \.1) { l in Link(l.0, destination: URL(string: l.1)!) }
            }
        }
        .formStyle(.grouped)
        .task { about = try? await app.tryCall("about") }
    }

    private func pathRow(_ label: String, _ path: String, _ home: String) -> some View {
        LabeledContent(label) {
            HStack {
                Text(tilde(path, home)).textSelection(.enabled)
                Button("Show") { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)]) }
            }
        }
    }

    private func copy() {
        var lines = ["mdsh-swift \(AppInfo.version)"]
        if let a = about {
            lines += ["bridge \(a.version) \(a.commit) \(a.buildTime)", "\(a.goVersion) \(a.os)/\(a.arch)"]
            for t in a.tools ?? [] { lines.append("\(t.name): \(t.path.isEmpty ? "not found" : t.path) \(t.version)") }
        }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(lines.joined(separator: "\n"), forType: .string)
    }
}

extension View {
    /// settingHelp puts mdsh-gui's muted line of help under a setting.
    func settingHelp(_ text: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            self
            Text(text)
                .font(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}
