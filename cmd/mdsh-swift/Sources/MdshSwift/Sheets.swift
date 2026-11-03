import AppKit
import MdshKit
import SwiftUI
import UniformTypeIdentifiers

/// RunOptionsSheet sets the file's arguments, --only, step mode and
/// stop on error.
struct RunOptionsSheet: View {
    var doc: DocumentModel
    @Environment(\.dismiss) private var dismiss
    @Environment(\.theme) private var theme
    @State private var args = ""
    @State private var only = ""
    @State private var step = false
    @State private var stopOnError = false
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Run options").font(.title2.weight(.semibold))
            Text("For this file, until it closes. The shells start again when these change.").foregroundStyle(.secondary)
            Form {
                TextField("Arguments ($1 and on)", text: $args, prompt: Text("e.g. prod 'two words'"))
                Picker("Only run", selection: $only) {
                    Text("Every block").tag("")
                    ForEach(doc.blocks.filter { !$0.blockID.isEmpty && $0.textbox == nil && $0.choice == nil }, id: \.from) { c in
                        Text(c.displayName).tag(c.blockID)
                    }
                }
                Toggle("Ask before every block (step)", isOn: $step)
                Toggle("Stop at the first failing block", isOn: $stopOnError)
            }
            if let error { Text(error).foregroundStyle(theme.danger) }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Apply") { apply() }.keyboardShortcut(.defaultAction)
            }
        }
        .padding(20)
        .frame(width: 480)
        .onAppear {
            args = doc.info.args
            only = doc.info.run.only
            step = doc.info.run.step
            stopOnError = doc.info.run.stopOnError
        }
    }

    private func apply() {
        Task {
            if let err = await doc.setRunOptions(args: args, only: only, step: step, stopOnError: stopOnError) {
                error = err
            } else {
                dismiss()
            }
        }
    }
}

/// ReportSheet writes a PDF, HTML or markdown copy of the file, and
/// shows the mdsh command that makes the same one.
struct ReportSheet: View {
    var doc: DocumentModel
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @Environment(\.theme) private var theme
    @State private var format = "pdf"
    @State private var code = true
    @State private var output = true
    @State private var from = "window"
    @State private var dest = ""
    @State private var command = ""
    @State private var start: ReportStart?
    @State private var error: String?
    @State private var done: String?
    @State private var working = false

    private static let exts = ["pdf": ".pdf", "html": ".html", "markdown": ".md"]

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Create report").font(.title2.weight(.semibold))
            Form {
                Picker("Format", selection: $format) {
                    Text("PDF").tag("pdf")
                    Text("HTML").tag("html")
                    Text("Markdown").tag("markdown")
                }
                .pickerStyle(.segmented)
                LabeledContent("Show") {
                    VStack(alignment: .leading) {
                        Toggle("Code blocks", isOn: $code)
                        Toggle("Output", isOn: $output)
                    }
                }
                Picker("Output from", selection: $from) {
                    Text(windowLabel).tag("window")
                    Text("Run all blocks first").tag("run")
                    Text("Saved output only (like --no-run)").tag("saved")
                }
                .pickerStyle(.radioGroup)
                LabeledContent("Save to") {
                    HStack {
                        TextField("", text: $dest).labelsHidden()
                        Button("Choose…") { choose() }
                    }
                }
            }
            if !command.isEmpty {
                HStack(alignment: .firstTextBaseline) {
                    Text(doc.info.dirty ? "Once saved, same as" : "Same as").foregroundStyle(.secondary)
                    Text(command).font(theme.codeFont()).textSelection(.enabled)
                }
            }
            if let done { Text(done) }
            if let error { Text(error).foregroundStyle(theme.danger) }
            HStack {
                Spacer()
                if done != nil {
                    Button("Show in Folder") { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: fullDest)]) }
                    Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
                } else {
                    Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                    Button("Create report") { create() }.keyboardShortcut(.defaultAction).disabled(working || dest.isEmpty)
                }
            }
        }
        .padding(20)
        .frame(width: 560)
        .task {
            start = try? await app.tryCall("reportStart", ["doc": .init(doc.id)])
            dest = start?.dest ?? ""
            await updateCommand()
        }
        .onChange(of: format) { _, _ in Task { await updateCommand() } }
        .onChange(of: code) { _, _ in Task { await updateCommand() } }
        .onChange(of: output) { _, _ in Task { await updateCommand() } }
        .onChange(of: from) { _, _ in Task { await updateCommand() } }
        .onChange(of: dest) { _, _ in Task { await updateCommand() } }
    }

    private var windowLabel: String {
        guard let s = start else { return "This tab's run" }
        return "This tab's run (\(s.ran) of \(s.total) blocks ran)"
    }

    /// fullDest is dest with the format's extension.
    private var fullDest: String {
        let ext = Self.exts[format] ?? ""
        return dest.hasSuffix(ext) ? dest : dest + ext
    }

    private var request: ReportRequest {
        ReportRequest(format: format, hideCode: !code, hideOutput: !output, saved: from == "saved", ran: from == "run", dest: fullDest)
    }

    private func updateCommand() async {
        guard let r = try? JSONValue(encoding: request) else { return }
        command = (try? await app.tryCall("reportCommand", ["doc": .init(doc.id), "report": r], as: String.self)) ?? ""
    }

    private func choose() {
        let panel = NSSavePanel()
        panel.message = "Save the report"
        panel.directoryURL = URL(fileURLWithPath: (fullDest as NSString).deletingLastPathComponent)
        panel.nameFieldStringValue = (fullDest as NSString).lastPathComponent
        if let t = UTType(filenameExtension: String((Self.exts[format] ?? ".md").dropFirst())) { panel.allowedContentTypes = [t] }
        if panel.runModal() == .OK, let url = panel.url { dest = url.path }
    }

    private func create() {
        working = true
        error = nil
        Task {
            defer { working = false }
            do {
                if from == "run" {
                    try await app.tryCall("runAll", ["doc": .init(doc.id)])
                }
                guard let r = try? JSONValue(encoding: request) else { return }
                try await app.tryCall("createReport", ["doc": .init(doc.id), "report": r])
                done = "Saved \(tilde(fullDest, start?.home ?? ""))"
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}

/// DependsSheet picks the blocks above the selected one that it runs
/// after, in the order checked.
struct DependsSheet: View {
    var model: WindowModel
    var doc: DocumentModel
    @Environment(\.dismiss) private var dismiss
    @Environment(\.theme) private var theme
    @State private var order: [Int] = []

    private var cell: Cell? { model.selectedCell }

    private var above: [Cell] {
        guard let c = cell else { return [] }
        return doc.blocks.filter { $0.from < c.from && $0.textbox == nil && $0.choice == nil }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Depends on").font(.title2.weight(.semibold))
            Text("\(cell?.displayName ?? "This block") runs after the blocks checked here, in the order checked. One that hasn't run is asked about first.")
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if above.isEmpty {
                Text("No block above it can be depended on.").foregroundStyle(.secondary)
            } else {
                ScrollView {
                    VStack(alignment: .leading, spacing: 4) {
                        ForEach(above, id: \.from) { b in
                            Toggle(isOn: Binding(get: { order.contains(b.block) }, set: { on in
                                if on { order.append(b.block) } else { order.removeAll { $0 == b.block } }
                            })) {
                                // Numbered in the order checked, as mdsh-gui shows it.
                                if let i = order.firstIndex(of: b.block) {
                                    Text("\(i + 1). \(b.displayName)")
                                } else {
                                    Text(b.displayName)
                                }
                            }
                            .toggleStyle(.checkbox)
                        }
                    }
                }
                .frame(maxHeight: 320)
            }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Apply") {
                    if let c = cell { Task { await doc.setDepends(c.block, order) } }
                    dismiss()
                }
                .keyboardShortcut(.defaultAction)
                .disabled(cell == nil)
            }
        }
        .padding(20)
        .frame(width: 440)
        .onAppear {
            let ids = cell?.depends ?? []
            order = ids.compactMap { id in doc.blocks.first { $0.blockID == id }?.block }
        }
    }
}

/// FrontMatterForm is the form that slides down over the page for the
/// file's front matter. Each change is written as it is made.
struct FrontMatterForm: View {
    @Bindable var model: WindowModel
    var doc: DocumentModel
    @Environment(AppModel.self) private var app
    @Environment(\.theme) private var theme
    @State private var fm: FrontMatter?
    @State private var tags = ""
    @State private var share = ""
    @State private var source = ""
    @State private var cols = ""
    @FocusState private var otherFocused: Bool
    @State private var rows = ""
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Front matter").font(theme.uiFont(size: theme.bodySize * 1.3, weight: .semibold))
                Text("Each change is written to the file as you make it; Undo takes it back.")
                    .foregroundStyle(theme.muted)
                    .font(theme.uiFont(size: theme.bodySize * 0.9))
            }
            if let f = fm {
                if !f.exists && f.title.isEmpty {
                    Text("This file has no front matter yet. Fill in a field to add it.").foregroundStyle(theme.muted)
                }
                HStack(alignment: .top, spacing: 28) {
                    VStack(alignment: .leading, spacing: 18) { about; environment }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    VStack(alignment: .leading, spacing: 18) { running; otherKeys }
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .textFieldStyle(.roundedBorder)
            } else {
                ProgressView()
            }
            if let error { Text(error).foregroundStyle(theme.danger) }
            HStack {
                Spacer()
                Button("Done") { withAnimation { model.frontMatterOpen = false } }
                    .keyboardShortcut(.cancelAction)
            }
        }
        .padding(16)
        .background(theme.surface)
        .overlay(alignment: .bottom) { Rectangle().fill(theme.border).frame(height: 1) }
        .task { await load() }
    }

    // MARK: Groups, as mdsh-gui lays them out

    private func heading(_ text: String, _ key: String? = nil) -> some View {
        HStack(spacing: 4) {
            Text(text.uppercased()).kerning(0.6)
            if let key { Text(key).font(theme.codeFont(size: theme.codeSize * 0.85)) }
        }
        .font(theme.uiFont(size: theme.bodySize * 0.75, weight: .semibold))
        .foregroundStyle(theme.muted)
        .accessibilityAddTraits(.isHeader)
    }

    private func labelled<V: View>(_ label: String, @ViewBuilder _ content: () -> V) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(label).font(theme.uiFont(size: theme.bodySize * 0.9))
            content()
        }
    }

    private var about: some View {
        VStack(alignment: .leading, spacing: 8) {
            heading("About the file")
            labelled("Title") { TextField("", text: field(\.title)).onSubmit(save) }
            labelled(fm?.descriptionKey.capitalized ?? "Description") {
                TextField("Shown under the file's name on the start screen", text: field(\.description), axis: .vertical)
                    .lineLimit(2...3).onSubmit(save)
            }
            labelled("Tags") { TextField("ops, deploy", text: $tags).onSubmit(save) }
        }
    }

    private var environment: some View {
        VStack(alignment: .leading, spacing: 8) {
            heading("Environment", "env:")
            labelled("Variables shared from the caller") {
                Picker("", selection: Binding(get: { fm?.shareAll ?? false }, set: { fm?.shareAll = $0; save() })) {
                    Text("All of them").tag(true)
                    Text("Only these").tag(false)
                }
                .pickerStyle(.radioGroup)
                .labelsHidden()
                TextField("HOME PATH (empty for none)", text: $share)
                    .onSubmit(save)
                    .disabled(fm?.shareAll ?? false)
                    .accessibilityLabel("Variables to share, separated by spaces")
            }
            labelled("Files sourced into every shell, one a line") {
                TextField("~/.aliases", text: $source, axis: .vertical).lineLimit(2...4).onSubmit(save)
            }
        }
    }

    private var running: some View {
        VStack(alignment: .leading, spacing: 8) {
            heading("Running", "mdsh:")
            labelled("Shell") { TextField("zsh with zsh blocks, else /bin/sh", text: field(\.shell)).onSubmit(save) }
            labelled("When a block fails") {
                Picker("", selection: pick(\.onError)) {
                    Text("Go on (default)").tag(""); Text("Go on").tag("continue"); Text("Stop the run").tag("stop")
                }.labelsHidden()
            }
            labelled("Placeholders") {
                Picker("", selection: pick(\.interpolate)) {
                    Text("Filled in (default)").tag(""); Text("Filled in").tag("on"); Text("Left as written").tag("off")
                }.labelsHidden()
            }
            labelled("Stderr") {
                Picker("", selection: pick(\.showStderr)) {
                    Text("Shown for failed blocks (default)").tag(""); Text("Always shown").tag("on"); Text("Never shown").tag("off")
                }.labelsHidden()
            }
            Toggle("Ask before every block (step)", isOn: Binding(get: { fm?.step ?? false }, set: { fm?.step = $0; save() }))
            HStack(spacing: 12) {
                labelled("Terminal columns") { TextField("fit", text: $cols).onSubmit(save) }
                labelled("Rows") { TextField("24", text: $rows).onSubmit(save) }
            }
        }
    }

    private var otherKeys: some View {
        VStack(alignment: .leading, spacing: 8) {
            heading("Other keys")
            labelled("YAML, kept as written") {
                // Applied when it loses focus, as mdsh-gui applies it on
                // blur; a problem shows in the line under the form.
                TextEditor(text: Binding(get: { fm?.other ?? "" }, set: { fm?.other = $0 }))
                    .font(theme.codeFont())
                    .frame(height: 80)
                    .overlay(RoundedRectangle(cornerRadius: 4).stroke(theme.border))
                    .focused($otherFocused)
                    .onChange(of: otherFocused) { _, on in if !on { save() } }
                    // Closing the form with the editor still focused saves too.
                    .onDisappear { if otherFocused { save() } }
            }
        }
    }

    private func field(_ k: WritableKeyPath<FrontMatter, String>) -> Binding<String> {
        Binding(get: { fm?[keyPath: k] ?? "" }, set: { fm?[keyPath: k] = $0 })
    }

    private func pick(_ k: WritableKeyPath<FrontMatter, String>) -> Binding<String> {
        Binding(get: { fm?[keyPath: k] ?? "" }, set: { fm?[keyPath: k] = $0; save() })
    }

    private func load() async {
        do {
            let f: FrontMatter = try await app.tryCall("frontMatter", ["doc": .init(doc.id)])
            fm = f
            tags = (f.tags ?? []).joined(separator: ", ")
            share = (f.share ?? []).joined(separator: " ")
            source = (f.source ?? []).joined(separator: "\n")
            cols = f.cols > 0 ? String(f.cols) : ""
            rows = f.rows > 0 ? String(f.rows) : ""
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func save() {
        guard var f = fm else { return }
        f.tags = tags.split(whereSeparator: { $0 == "," || $0 == " " }).map(String.init)
        f.share = share.split(separator: " ").map(String.init)
        f.source = source.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        // A size that isn't a number is reported, not sent, as mdsh-gui
        // does, so the key isn't dropped.
        func size(_ text: String, _ name: String) -> Int? {
            let t = text.trimmingCharacters(in: .whitespaces)
            if t.isEmpty { return 0 }
            guard let n = Int(t), n >= 0 else {
                error = "\(name) must be a number"
                return nil
            }
            return n
        }
        guard let c = size(cols, "Terminal columns"), let r = size(rows, "Rows") else { return }
        f.cols = c
        f.rows = r
        Task {
            do {
                let v = try JSONValue(encoding: f)
                try await app.tryCall("setFrontMatter", ["doc": .init(doc.id), "frontMatter": v])
                error = nil
                await doc.refresh()
                await doc.refreshText()
                await load()
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}
