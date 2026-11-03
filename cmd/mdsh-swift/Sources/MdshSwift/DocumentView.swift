import AppKit
import MdshKit
import SwiftUI

/// DocumentView is a window showing a file: the toolbar and banners,
/// the outline, the rendered and source views, and the History and
/// Console panes along the bottom.
struct DocumentView: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(AppModel.self) private var app
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 0) {
            VSplitView {
                VStack(spacing: 0) {
                    banners
                    HSplitView {
                        if model.showOutline {
                            OutlineView(model: model, doc: doc)
                                .frame(minWidth: 180, idealWidth: 240 * theme.textScale, maxWidth: 420)
                        }
                        views
                    }
                }
                .frame(minHeight: 240)
                if model.showConsole {
                    ConsolePane(model: model)
                        .frame(minHeight: 60, idealHeight: 200)
                }
                if model.showHistory {
                    HistoryPane(model: model, doc: doc)
                        .frame(minHeight: 60, idealHeight: 260)
                }
            }
            // The History bar is the window's bottom edge; History opens
            // above it, as in mdsh-gui.
            if !model.showHistory {
                HistoryBar(model: model, doc: doc)
            }
        }
        .foregroundStyle(theme.text)
        .overlay {
            if let img = model.viewedImage {
                ImageViewer(image: img) { model.viewedImage = nil }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .mdshShowImage)) { n in
            guard (n.object as? NSWindow) === model.window, let img = n.userInfo?["image"] as? NSImage else { return }
            model.viewedImage = img
        }
        .toolbar { toolbar }
        .sheet(isPresented: $model.runOptionsOpen) { RunOptionsSheet(doc: doc) }
        .sheet(isPresented: $model.reportOpen) { ReportSheet(doc: doc) }
        .sheet(isPresented: $model.dependsOpen) { DependsSheet(model: model, doc: doc) }
        .sheet(isPresented: $model.clearAllAsk) {
            AskSheet(title: "Clear all outputs?",
                     text: "Every block's output, output copy and stored value goes from the document. The shells keep the variables, functions and aliases blocks set; Restart Shells clears those. Nothing is saved until you save.",
                     action: "Clear", safeDefault: true) { Task { await doc.clearOutputs() } }
        }
        .sheet(item: $model.deleteAsk) { c in
            AskSheet(title: "Delete \(c.blockID.isEmpty ? "the block at line \(c.line)" : "#" + c.blockID)?",
                     text: "The block goes from the file, with its output and its tape. Edit › Undo puts it back.",
                     action: "Delete") { Task { await doc.deleteBlock(c.block) } }
        }
        .onChange(of: doc.status) { _, st in
            if let st { NSAccessibility.post(element: NSApp as Any, notification: .announcementRequested, userInfo: [.announcement: st]) }
        }
        .onChange(of: doc.confirm) { _, q in
            // A question brings the rendered view back, to show it.
            if q != nil, !model.showRendered { model.showRendered = true }
        }
    }

    /// views are the source and rendered views: the source on the
    /// left, as in mdsh-gui, split evenly until the grip moves.
    @ViewBuilder
    private var views: some View {
        let rendered = RenderedView(model: model, doc: doc)
        let source = SourceView(model: model, doc: doc)
        if model.showSource && model.showRendered {
            SplitPair(fraction: $model.splitFraction, minLeading: 280, minTrailing: 320) {
                source
            } trailing: {
                rendered
            }
        } else if model.showSource {
            source
        } else {
            rendered.frame(minWidth: 320)
        }
    }

    // MARK: Toolbar

    @ToolbarContentBuilder
    private var toolbar: some ToolbarContent {
        ToolbarItemGroup(placement: .navigation) {
            Button { model.showOutline.toggle() } label: { Image(systemName: "sidebar.left") }
                .help("Outline (⇧⌘O)")
                .accessibilityLabel("Outline")
                .accessibilityAddTraits(model.showOutline ? .isSelected : [])
        }
        ToolbarItemGroup(placement: .principal) {
            if doc.info.run.isSet {
                Button { model.runOptionsOpen = true } label: { Text(runOptionsChip).lineLimit(1) }
                    .help("Run options")
            }
            // Chips for the running block, the one waiting on a question
            // and the background ones: a dashed chip with the mark and the
            // name, which jumps to the block, as in mdsh-gui.
            if let b = doc.current ?? doc.confirm?.block, let c = doc.cell(block: b) {
                RunChip(mark: doc.confirm != nil ? "?" : "▶", name: c.displayName,
                        help: doc.confirm != nil ? "Waiting for you" : "Running") { model.selected = c.from }
            }
            ForEach(Array(doc.background).sorted(), id: \.self) { b in
                if let c = doc.cell(block: b) {
                    RunChip(mark: "◐", name: c.displayName, help: "Running in the background") { model.selected = c.from }
                }
            }
        }
        ToolbarItemGroup(placement: .primaryAction) {
            if let st = doc.status {
                Text(st)
                    .font(theme.look == .plain ? theme.uiFont(size: theme.bodySize * 0.85) : theme.codeFont(size: 12))
                    .foregroundStyle(theme.muted)
                    .lineLimit(1)
                    // Room between it and the run chips before it.
                    .padding(.leading, 10)
                    .accessibilityAddTraits(.updatesFrequently)
            }
            ViewSwitch(model: model)
            Button { model.toggleConsole() } label: {
                Image(systemName: "terminal")
                    .overlay(alignment: .topTrailing) {
                        if app.unseenProblems > 0 && !model.showConsole {
                            Text("\(app.unseenProblems)")
                                .font(.system(size: 9, weight: .bold))
                                .padding(.horizontal, 3)
                                .background(theme.danger, in: Capsule())
                                .foregroundStyle(.white)
                                .offset(x: 8, y: -6)
                        }
                    }
            }
            .help("Console (⇧⌘Y)")
            .accessibilityLabel(app.unseenProblems > 0 ? "Console, \(app.unseenProblems) new problems" : "Console")
            Button { model.clearAllAsk = true } label: { Image(systemName: "eraser") }
                .help("Clear all outputs (⌥⌘K)")
                .disabled(!doc.info.hasOutput)
            // Run all is the window's main button, and turns into Stop
            // while a run goes, drawn in the look as mdsh-gui draws it.
            Button {
                if doc.running { doc.stop() } else { doc.runAll() }
            } label: {
                Text(doc.running ? "■ Stop" : "▶ Run all")
            }
            .buttonStyle(PrimaryButtonStyle(theme: theme, stop: doc.running))
            .help(doc.running ? "Stop (⌘.)" : "Run all (⌘R)")
        }
    }

    private var runOptionsChip: String {
        let r = doc.info.run
        var parts: [String] = []
        if !doc.info.args.isEmpty { parts.append(doc.info.args) }
        if !r.only.isEmpty { parts.append("--only " + r.only) }
        if r.step { parts.append("--step") }
        if r.stopOnError { parts.append("--stop-on-error") }
        return parts.joined(separator: " ")
    }

    // MARK: Banners

    @ViewBuilder
    private var banners: some View {
        if doc.fileChanged {
            Banner(text: "\(doc.name) changed on disk.") {
                Button("Reload") { Task { await doc.reload() } }
                Button("Keep mine") { doc.fileChanged = false }
            }
        }
        if !doc.info.envBanner.isEmpty {
            Banner(text: doc.info.envBanner) {}
        }
        if !model.moveBanner.isEmpty {
            Banner(text: model.moveBanner.joined(separator: " ")) {
                Button("Undo") {
                    model.moveBanner = []
                    Task { await doc.undo() }
                }
                Button { model.moveBanner = [] } label: { Image(systemName: "xmark") }
                    .buttonStyle(.borderless)
                    .accessibilityLabel("Dismiss")
            }
        }
    }

}

/// ViewSwitch is the joined Source and Rendered pair, the active halves
/// filled, as mdsh-gui's toolbar has it.
struct ViewSwitch: View {
    @Bindable var model: WindowModel
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 0) {
            half("chevron.left.forwardslash.chevron.right", "Source view", key: "⌘\\", on: model.showSource) { model.toggleSource() }
            Rectangle().fill(Color.secondary.opacity(0.6)).frame(width: 1, height: 18)
            half("doc.richtext", "Rendered view", key: "⇧⌘\\", on: model.showRendered) { model.toggleRendered() }
        }
        .overlay(RoundedRectangle(cornerRadius: theme.look == .plain ? 6 : 0).stroke(Color.secondary.opacity(0.6)))
        .clipShape(RoundedRectangle(cornerRadius: theme.look == .plain ? 6 : 0))
    }

    private func half(_ symbol: String, _ name: String, key: String, on: Bool, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .frame(width: 30, height: 20)
                // The toolbar keeps the system's colors in every look, so
                // the switch uses them too, like the buttons beside it.
                .foregroundStyle(on ? Color.primary : Color.secondary)
                .background(on ? theme.accent.opacity(0.18) : .clear)
        }
        .buttonStyle(.plain)
        .help("\(name) (\(key))")
        .accessibilityLabel(name)
        .accessibilityAddTraits(on ? .isSelected : [])
    }
}

/// PrimaryButtonStyle is Run all in each look: Plain's filled accent
/// button, Teletype's uppercase keycap with a hard shadow that sinks
/// when pressed, and Blueprint's lettered filled box, with Stop drawn in
/// the danger color.
struct PrimaryButtonStyle: ButtonStyle {
    var theme: Theme
    var stop: Bool

    func makeBody(configuration: Configuration) -> some View {
        let pressed = configuration.isPressed
        let label = configuration.label
            .textCase(theme.look == .plain || theme.look == .gum || theme.look == .witch ? nil : .uppercase)
            .kerning(theme.look == .blueprint ? 0.6 : 0)
            .font(theme.look == .tty ? theme.codeFont(size: 13).weight(.bold) : theme.uiFont(size: 13, weight: .semibold))
            .padding(.horizontal, 10)
            .padding(.vertical, 4)
        switch theme.look {
        case .plain:
            label
                .foregroundStyle(stop ? theme.danger : theme.accentText)
                .background(stop ? theme.danger.opacity(0.12) : theme.accent, in: RoundedRectangle(cornerRadius: 6))
                .opacity(pressed ? 0.8 : 1)
        case .tty:
            let ink = stop ? theme.danger : theme.text
            label
                .foregroundStyle(ink)
                .background(theme.background)
                .overlay(Rectangle().stroke(ink))
                .background(Rectangle().fill(ink).offset(x: pressed ? 0 : 2, y: pressed ? 0 : 2))
                .offset(x: pressed ? 2 : 0, y: pressed ? 2 : 0)
        case .blueprint:
            label
                .foregroundStyle(stop ? theme.danger : theme.accentText)
                .background(stop ? Color.clear : theme.accent)
                .overlay(Rectangle().stroke(stop ? theme.danger : theme.accent))
                .opacity(pressed ? 0.8 : 1)
        case .witch:
            // Amethyst edged in gilt; Stop in the danger red.
            label
                .fontWeight(.bold)
                .kerning(0.3)
                .foregroundStyle(stop ? theme.color(theme.dark ? "#160c24" : "#ffffff") : theme.accentText)
                .background(stop ? theme.danger : theme.accent, in: RoundedRectangle(cornerRadius: 4))
                .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(stop ? theme.danger : theme.witchGilt))
                .opacity(pressed ? 0.8 : 1)
        case .gum:
            // A gumball: a pill on a chunky shadow it sinks into when
            // pressed.
            let fill = stop ? theme.danger : theme.accent
            let noShadows = AppModel.shared.settings.noShadows
            label
                .fontWeight(.heavy)
                .foregroundStyle(stop ? theme.color(theme.dark ? "#2b0a1f" : "#ffffff") : theme.accentText)
                .background(fill, in: Capsule())
                .background {
                    if !noShadows && !pressed { Capsule().fill(theme.gumShadow).offset(y: 3) }
                }
                .offset(y: pressed && !noShadows ? 2 : 0)
        }
    }
}

/// AskSheet is an in-window question with a destructive answer, as
/// mdsh-gui asks before Clear All Outputs and Delete.
struct AskSheet: View {
    var title: String
    var text: String
    var action: String
    /// safeDefault makes Cancel the button Return presses, for a question
    /// whose answer can't be undone.
    var safeDefault = false
    var perform: () -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(.title2.weight(.semibold))
            Text(text).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            HStack {
                Spacer()
                if safeDefault {
                    Button("Cancel") { dismiss() }.keyboardShortcut(.defaultAction)
                    danger
                } else {
                    Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                    danger.keyboardShortcut(.defaultAction)
                }
            }
        }
        .padding(20)
        .frame(width: 420)
    }

    /// danger is the answer, in red, as mdsh-gui's danger button.
    private var danger: some View {
        Button(action, role: .destructive) {
            perform()
            dismiss()
        }
        .buttonStyle(.borderedProminent)
        .tint(.red)
    }
}

/// KeyButtonStyle is an in-page button in the paper looks, as mdsh-gui
/// draws them: Teletype's uppercase keycap with a hard shadow, and
/// Blueprint's lettered outlined box, filled for the primary one, and
/// Bubble Gum's pill, bold, the primary one a gumball on a shadow it
/// sinks into when pressed.
struct KeyButtonStyle: ButtonStyle {
    var theme: Theme
    var primary = false

    func makeBody(configuration: Configuration) -> some View {
        let pressed = configuration.isPressed
        if theme.look == .gum {
            gum(configuration.label, pressed: pressed)
        } else if theme.look == .witch {
            witch(configuration.label, pressed: pressed)
        } else {
            paper(configuration.label, pressed: pressed)
        }
    }

    private func gum(_ label: Configuration.Label, pressed: Bool) -> some View {
        let noShadows = AppModel.shared.settings.noShadows
        return label
            .font(theme.uiFont(size: 12, weight: primary ? .heavy : .bold))
            .padding(.horizontal, 11)
            .padding(.vertical, 3)
            .foregroundStyle(primary ? theme.accentText : theme.text)
            .background(primary ? theme.accent : Color.clear, in: Capsule())
            .overlay(Capsule().strokeBorder(primary ? theme.accent : theme.border, lineWidth: 2))
            .background {
                if primary && !noShadows && !pressed { Capsule().fill(theme.gumShadow).offset(y: 3) }
            }
            .offset(y: primary && pressed && !noShadows ? 2 : 0)
    }

    /// witch is Witchcraft's: the primary one amethyst edged in gilt,
    /// the rest quiet, edged in the look's border.
    private func witch(_ label: Configuration.Label, pressed: Bool) -> some View {
        label
            .font(theme.uiFont(size: 12, weight: primary ? .bold : .semibold))
            .padding(.horizontal, 10)
            .padding(.vertical, 3)
            .foregroundStyle(primary ? theme.accentText : theme.text)
            .background(primary ? theme.accent : Color.clear, in: RoundedRectangle(cornerRadius: 4))
            .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(primary ? theme.witchGilt : theme.border))
            .opacity(pressed ? 0.8 : 1)
    }

    @ViewBuilder
    private func paper(_ label: Configuration.Label, pressed: Bool) -> some View {
        let label = label
            .textCase(.uppercase)
            .kerning(theme.look == .blueprint ? 0.5 : 0)
            .font(theme.look == .tty ? theme.codeFont(size: 12).weight(.bold) : theme.uiFont(size: 12, weight: .semibold))
            .padding(.horizontal, 9)
            .padding(.vertical, 3)
        if theme.look == .tty {
            label
                .foregroundStyle(theme.text)
                .background(theme.background)
                .overlay(Rectangle().stroke(theme.text))
                .background(Rectangle().fill(theme.text).offset(x: pressed ? 0 : 2, y: pressed ? 0 : 2))
                .offset(x: pressed ? 2 : 0, y: pressed ? 2 : 0)
        } else {
            label
                .foregroundStyle(primary ? theme.accentText : theme.text)
                .background(primary ? theme.accent : Color.clear)
                .overlay(Rectangle().stroke(primary ? theme.accent : theme.text))
                .opacity(pressed ? 0.8 : 1)
        }
    }
}

extension View {
    /// keyButton draws the button in the look's own style in Teletype and
    /// Blueprint, and leaves the system's in Plain.
    @ViewBuilder
    func keyButton(_ theme: Theme, primary: Bool = false) -> some View {
        if theme.look == .plain {
            if primary { self.buttonStyle(.borderedProminent) } else { self }
        } else {
            self.buttonStyle(KeyButtonStyle(theme: theme, primary: primary))
        }
    }
}

/// RunChip is a toolbar chip naming a block that runs or waits.
struct RunChip: View {
    var mark: String
    var name: String
    var help: String
    var action: () -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        Button(action: action) {
            Text("\(mark) \(name)")
                .font(theme.codeFont(size: 12))
                .lineLimit(1)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .overlay {
                    // Bubble Gum's chip is a dotted pill.
                    if theme.look == .gum {
                        Capsule().stroke(theme.border, style: StrokeStyle(lineWidth: 2, dash: [2, 2]))
                    } else {
                        RoundedRectangle(cornerRadius: 5).stroke(theme.muted, style: StrokeStyle(lineWidth: 1, dash: [3, 2]))
                    }
                }
        }
        .buttonStyle(.plain)
        .help(help)
        .accessibilityLabel("\(name), \(help.lowercased())")
    }
}

/// Banner is a strip across the top of the document with a message and
/// buttons.
struct Banner<Buttons: View>: View {
    var text: String
    @ViewBuilder var buttons: Buttons
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "exclamationmark.triangle").foregroundStyle(theme.muted)
            Text(text).frame(maxWidth: .infinity, alignment: .leading)
            if theme.look == .plain {
                buttons
            } else {
                buttons.buttonStyle(KeyButtonStyle(theme: theme))
            }
        }
        .font(theme.uiFont(size: theme.bodySize * 0.9))
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(theme.dark ? Color(nsColor: Theme.nsColor("#2b2611")) : Color(nsColor: Theme.nsColor("#fff8c5")))
        .overlay(alignment: .bottom) { Rectangle().fill(Color(nsColor: Theme.nsColor(theme.dark ? "#9e6a03" : "#d4a72c"))).frame(height: 1) }
        .accessibilityElement(children: .contain)
    }
}
