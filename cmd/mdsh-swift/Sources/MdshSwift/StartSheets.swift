import AppKit
import MdshKit
import SwiftUI
import UniformTypeIdentifiers

/// NewFileSheet offers the templates in the templates folder, then asks
/// where to save the new file and opens it.
struct NewFileSheet: View {
    var model: WindowModel
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @Environment(\.theme) private var theme
    @State private var templates: [LibraryFile]?
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("New file").font(.title2.weight(.semibold))
            Text("Start from a template, then choose where to save it.").foregroundStyle(.secondary)
            ScrollView {
                VStack(spacing: 0) {
                    FileRow(path: "", label: "Blank", detail: "a heading named for the file", pinned: false,
                            onOpen: { create("") }, onPin: { _ in })
                    ForEach(templates ?? []) { t in
                        FileRow(path: t.path, label: t.rel.replacingOccurrences(of: ".md", with: ""), detail: blocksLabel(t.blocks),
                                description: t.description, pinned: false, onOpen: { create(t.path) }, onPin: { _ in })
                    }
                }
            }
            .frame(minHeight: 120, maxHeight: 360)
            if let error { Text(error).foregroundStyle(theme.danger) }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
            }
        }
        .padding(20)
        .frame(width: 520)
        .task {
            do {
                templates = try await app.tryCall("templates")
                // With no templates there is nothing to pick.
                if templates?.isEmpty ?? true { create("") }
            } catch {
                self.error = "Templates folder: " + error.localizedDescription
                templates = []
            }
        }
    }

    private func create(_ template: String) {
        let panel = NSSavePanel()
        panel.message = "Create a new markdown file"
        panel.prompt = "Create"
        panel.nameFieldStringValue = "Untitled.md"
        panel.canCreateDirectories = true
        panel.allowedContentTypes = [UTType(filenameExtension: "md") ?? .plainText]
        if let dir = model.start?.dir { panel.directoryURL = URL(fileURLWithPath: dir) }
        dismiss()
        guard panel.runModal() == .OK, let url = panel.url else { return }
        Task {
            do {
                let file: String = try await app.tryCall("createFile", ["file": .init(url.path), "template": .init(template)])
                if model.doc == nil {
                    await model.open(file)
                } else {
                    WindowOpener.open(.open(file))
                }
            } catch {
                model.error = error.localizedDescription
            }
        }
    }
}

/// FolderSettingsSheet edits a folder's .mdsh.yaml: what the start
/// screen shows for it.
struct FolderSettingsSheet: View {
    var model: WindowModel
    @State var form: FolderForm
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @Environment(\.theme) private var theme
    @State private var error: String?
    @State private var tags = ""
    @State private var color = Color.accentColor
    /// picked is set once the color well shows the saved color, so only
    /// a pick rewrites it: a saved "red" stays "red".
    @State private var picked = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // mdsh-gui's dialog: titled by the folder, saying where it
            // saves, each field labelled above with its hint below.
            Text("Settings for \(folderName)").font(.title3.weight(.semibold))
            Text("Saved in \(form.path)/.mdsh.yaml. Every field is optional; other keys in the file are kept.")
                .font(.callout)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
            field("Title") { TextField("", text: bind(\.title)).labelsHidden() }
            field("Description") { TextField("", text: bind(\.description)).labelsHidden() }
            field("Icon", hint: "One emoji, or an image in this folder.") { withImages(bind(\.icon)) }
            field("Color", hint: "A #hex, rgb(), hsl() or CSS color name.") {
                HStack {
                    TextField("#3b82f6", text: bind(\.color)).labelsHidden()
                    ColorPicker("Pick a color", selection: $color, supportsOpacity: false)
                        .labelsHidden()
                        .help("Pick a color")
                        .onChange(of: color) { _, c in if picked { form.settings.color = hex(c) } }
                }
            }
            field("Background image", hint: "An image in this folder, shown behind its header.") { withImages(bind(\.background)) }
            field("Tags", hint: "Split by commas or spaces. Every file in the folder, and below it, gets them.") {
                TextField("ops, prod", text: $tags).labelsHidden()
            }
            if let error { Text(error).foregroundStyle(theme.danger) }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Save") { save() }.keyboardShortcut(.defaultAction).disabled(!form.writable)
            }
            .padding(.top, 6)
        }
        .padding(20)
        .frame(width: 440)
        .onAppear {
            tags = (form.settings.tags ?? []).joined(separator: " ")
            if let c = form.settings.color, !c.isEmpty { color = Color(nsColor: Theme.nsColor(c)) }
            DispatchQueue.main.async { picked = true }
        }
    }

    /// folderName is the folder as the start screen names it.
    private var folderName: String {
        if let t = form.settings.title, !t.isEmpty { return t }
        return (form.path as NSString).lastPathComponent
    }

    private func field<Content: View>(_ label: String, hint: String? = nil, @ViewBuilder _ content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(label).font(.callout)
            content()
            if let hint { Text(hint).font(.caption).foregroundStyle(.secondary) }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(label)
    }

    /// withImages is a text field offering the folder's images, as
    /// mdsh-gui's datalist does.
    private func withImages(_ text: Binding<String>) -> some View {
        HStack(spacing: 4) {
            TextField("", text: text).labelsHidden()
            if let images = form.images, !images.isEmpty {
                Menu {
                    ForEach(images, id: \.self) { i in Button(i) { text.wrappedValue = i } }
                } label: {
                    Image(systemName: "photo")
                }
                .menuStyle(.borderlessButton)
                .fixedSize()
                .help("The images in this folder")
                .accessibilityLabel("Images in this folder")
            }
        }
    }

    private func bind(_ k: WritableKeyPath<FolderSettings, String?>) -> Binding<String> {
        Binding(get: { form.settings[keyPath: k] ?? "" }, set: { form.settings[keyPath: k] = $0 })
    }

    private func hex(_ c: Color) -> String {
        let n = NSColor(c).usingColorSpace(.sRGB) ?? .black
        return String(format: "#%02x%02x%02x", Int(n.redComponent * 255), Int(n.greenComponent * 255), Int(n.blueComponent * 255))
    }

    private func save() {
        form.settings.tags = tags.split(whereSeparator: { $0 == " " || $0 == "," }).map(String.init)
        let rel: String = {
            guard let dir = model.start?.dir else { return "" }
            if form.path == dir { return "" }
            return String(form.path.dropFirst(dir.count + 1))
        }()
        Task {
            do {
                guard case .object(let s)? = try? JSONValue(encoding: form.settings) else { return }
                try await app.tryCall("setFolderSettings", ["rel": .init(rel), "settings": .object(s)])
                await model.loadStart()
                dismiss()
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}
