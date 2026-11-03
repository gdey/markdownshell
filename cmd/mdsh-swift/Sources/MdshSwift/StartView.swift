import AppKit
import MdshKit
import SwiftUI

/// StartView is the start screen: the markdown files under the start
/// folder to pick from, with Pinned and Recent above them.
struct StartView: View {
    @Bindable var model: WindowModel
    @Environment(AppModel.self) private var app
    @Environment(\.theme) private var theme
    @Environment(\.openWindow) private var openWindow
    @FocusState private var filterFocused: Bool

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                Text("Open a runbook")
                    .font(theme.uiFont(size: theme.bodySize * 1.9, weight: .semibold))
                    .accessibilityAddTraits(.isHeader)
                bar
                if let s = model.start {
                    Text(tilde(s.dir, s.home))
                        .font(theme.uiFont(size: theme.bodySize * 0.9))
                        .foregroundStyle(theme.muted)
                        .textSelection(.enabled)
                    content(s)
                } else if let err = model.error {
                    Text(err).foregroundStyle(theme.danger)
                } else {
                    ProgressView()
                }
            }
            .padding(.horizontal, 32)
            .padding(.vertical, 24)
            .frame(maxWidth: 900, alignment: .leading)
            .frame(maxWidth: .infinity)
        }
        .foregroundStyle(theme.text)
        .onAppear { filterFocused = true }
        .sheet(isPresented: $model.newFileOpen) { NewFileSheet(model: model) }
        .sheet(item: Binding(get: { model.folderSettings.map(FolderFormBox.init) }, set: { model.folderSettings = $0?.form })) { box in
            FolderSettingsSheet(model: model, form: box.form)
        }
    }

    private var bar: some View {
        HStack(spacing: 8) {
            TextField("Filter files, #tag", text: $model.startFilter)
                .textFieldStyle(.roundedBorder)
                .focused($filterFocused)
                .accessibilityLabel("Filter files, or #tag")
                .onSubmit { if let f = firstMatch { open(f) } }
            Button("New File…") { model.newFileOpen = true }
            Button("Open…") { openPanel() }
            Button("New Session") { session(new: true) }
                .help("Open today's session, a file named for the date in the sessions folder, with History open")
            Button("Last Session") { session(new: false) }
                .disabled(model.start?.lastSession.isEmpty ?? true)
        }
        .font(theme.uiFont())
    }

    // MARK: Listing

    @ViewBuilder
    private func content(_ s: StartScreen) -> some View {
        let f = Filter(model.startFilter)
        let collections = Dictionary((s.collections ?? []).filter { $0.error == nil }.map { ($0.rel, $0) }, uniquingKeysWith: { a, _ in a })
        let broken = Dictionary((s.collections ?? []).compactMap { c in c.error.map { (c.rel, $0) } }, uniquingKeysWith: { a, _ in a })
        let files = s.files ?? []
        let byPath = Dictionary(files.map { ($0.path, $0) }, uniquingKeysWith: { a, _ in a })
        let matches: (LibraryFile) -> Bool = { file in
            let ft = file.tags ?? []
            guard f.tags.allSatisfy(ft.contains) else { return false }
            let c = collections[file.folder]
            return f.has(file.rel) || f.has(file.description) || ft.contains(where: { f.has($0) }) || f.has(c?.title) || f.has(c?.description)
        }
        let shown: (String) -> Bool = { p in
            if f.tags.isEmpty { return f.has(p) }
            guard let file = byPath[p] else { return false }
            return matches(file)
        }
        let pinned = (s.pinned ?? []).filter(shown)
        let recent = (s.recent ?? []).filter { !(s.pinned ?? []).contains($0) && shown($0) }
        let groups = group(files.filter(matches), collections: collections)

        tagBar(files, active: f.tags)
        if !pinned.isEmpty {
            section("Pinned") { ForEach(pinned, id: \.self) { pathRow($0, s) } }
        }
        if !recent.isEmpty {
            section("Recent") { ForEach(recent, id: \.self) { pathRow($0, s) } }
        }
        ForEach(groups, id: \.dir) { g in
            if let c = collections[g.dir] {
                CollectionSection(collection: c, dir: g.dir, startDir: s.dir, active: f.tags,
                                  writable: (s.writable ?? []).contains(g.dir), onTag: toggleTag, onGear: { gear(g.dir) }) {
                    ForEach(g.files) { file in libraryRow(file, dir: g.dir, collection: c, s) }
                }
            } else {
                HStack {
                    Text(g.dir.isEmpty ? "This folder" : g.dir + "/")
                        .font(theme.uiFont(size: theme.bodySize * 1.15, weight: .semibold))
                        .help(broken[g.dir] ?? "")
                    if broken[g.dir] != nil { Text("⚠").help(broken[g.dir] ?? "") }
                    if broken[g.dir] == nil, (s.writable ?? []).contains(g.dir) {
                        gearButton(g.dir)
                    }
                }
                .accessibilityAddTraits(.isHeader)
                VStack(spacing: 0) {
                    ForEach(g.files) { file in libraryRow(file, dir: g.dir, collection: nil, s) }
                }
            }
        }
        if groups.isEmpty && pinned.isEmpty && recent.isEmpty {
            Text("No markdown files here. Use New File…, Open… or drop a file on the window.")
                .foregroundStyle(theme.muted)
        }
        if s.full {
            Text("More files than are listed here; filter to find the rest, or open a folder further down.")
                .font(theme.uiFont(size: theme.bodySize * 0.9))
                .foregroundStyle(theme.muted)
        }
    }

    private func section<C: View>(_ title: String, @ViewBuilder rows: () -> C) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(theme.uiFont(size: theme.bodySize * 1.15, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            VStack(spacing: 0) { rows() }
        }
    }

    private func tagBar(_ files: [LibraryFile], active: [String]) -> some View {
        var counts: [String: Int] = [:]
        for f in files { for t in f.tags ?? [] { counts[t, default: 0] += 1 } }
        return Group {
            if !counts.isEmpty {
                FlowLayout(spacing: 6) {
                    Text("Tags").foregroundStyle(theme.muted)
                    ForEach(counts.keys.sorted(), id: \.self) { t in
                        TagButton(tag: t, on: active.contains(t), count: counts[t]) { toggleTag(t) }
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityLabel("Tags")
            }
        }
    }

    private func pathRow(_ p: String, _ s: StartScreen) -> some View {
        FileRow(path: p, label: (p as NSString).lastPathComponent,
                detail: tilde((p as NSString).deletingLastPathComponent, s.home),
                pinned: (s.pinned ?? []).contains(p), onOpen: { open(p) }, onPin: { pin(p, $0) })
    }

    private func libraryRow(_ f: LibraryFile, dir: String, collection: Collection?, _ s: StartScreen) -> some View {
        let own = (f.tags ?? []).filter { !(collection?.tags ?? []).contains($0) }
        let label = dir.isEmpty ? f.rel : String(f.rel.dropFirst(dir.count + 1))
        return FileRow(path: f.path, label: label, detail: blocksLabel(f.blocks) + " · " + ago(f.mod),
                       description: f.description, tags: own, pinned: (s.pinned ?? []).contains(f.path),
                       onOpen: { open(f.path) }, onPin: { pin(f.path, $0) })
    }

    private func gearButton(_ dir: String) -> some View {
        Button { gear(dir) } label: { Image(systemName: "gearshape") }
            .buttonStyle(.borderless)
            .help("Folder settings")
            .accessibilityLabel("Folder settings for \(dir.isEmpty ? "this folder" : dir)")
    }

    // MARK: Actions

    private var firstMatch: String? {
        guard let s = model.start else { return nil }
        let f = Filter(model.startFilter)
        if let p = (s.pinned ?? []).first(where: { f.tags.isEmpty && f.has($0) }) { return p }
        if let p = (s.recent ?? []).first(where: { f.tags.isEmpty && f.has($0) }) { return p }
        return (s.files ?? []).first { file in
            f.tags.allSatisfy((file.tags ?? []).contains) && (f.has(file.rel) || f.has(file.description))
        }?.path
    }

    private func open(_ path: String) {
        if let w = WindowRegistry.shared.window(showing: path), w !== model {
            w.window?.makeKeyAndOrderFront(nil)
            return
        }
        Task { await model.open(path) }
    }

    private func pin(_ path: String, _ on: Bool) {
        Task {
            await app.pin(path, on)
            await model.loadStart()
        }
    }

    private func toggleTag(_ t: String) {
        var words = model.startFilter.split(separator: " ").map(String.init)
        if let i = words.firstIndex(where: { $0.lowercased() == "#" + t }) {
            words.remove(at: i)
        } else {
            words.append("#" + t)
        }
        model.startFilter = words.joined(separator: " ")
    }

    private func gear(_ dir: String) {
        Task {
            do {
                model.folderSettings = try await app.tryCall("folderSettings", ["rel": .init(dir)])
            } catch {
                model.error = error.localizedDescription
            }
        }
    }

    private func session(new: Bool) {
        Task {
            do {
                let file: String = try await app.tryCall(new ? "newSession" : "lastSession")
                await model.open(file, session: true)
            } catch {
                model.error = error.localizedDescription
            }
        }
    }

    private func openPanel() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        panel.allowedContentTypes = [.init(filenameExtension: "md")!, .init(filenameExtension: "markdown")!]
        if let dir = model.start?.dir { panel.directoryURL = URL(fileURLWithPath: dir) }
        guard panel.runModal() == .OK else { return }
        for (i, url) in panel.urls.enumerated() {
            if i == 0 {
                open(url.path)
            } else {
                WindowOpener.open(.open(url.path))
            }
        }
    }

    // MARK: Grouping

    struct Group_: Identifiable {
        var dir: String
        var files: [LibraryFile]
        var id: String { dir }
    }

    /// group puts files under their folders, in path order with each
    /// folder before its subfolders. A collection whose files are all in
    /// subfolders still gets its header.
    private func group(_ files: [LibraryFile], collections: [String: Collection]) -> [Group_] {
        var groups: [String: [LibraryFile]] = [:]
        for f in files {
            groups[f.folder, default: []].append(f)
            var up = f.folder
            while !up.isEmpty {
                up = (up as NSString).deletingLastPathComponent
                if collections[up] != nil, groups[up] == nil { groups[up] = [] }
            }
        }
        let key = { (d: String) in d.replacingOccurrences(of: "/", with: "\u{0}") }
        return groups.keys.sorted { key($0) < key($1) }.map { Group_(dir: $0, files: groups[$0]!) }
    }
}

/// FolderFormBox makes a FolderForm identifiable for .sheet(item:).
struct FolderFormBox: Identifiable {
    var form: FolderForm
    var id: String { form.path }
}

/// Filter is the start screen's filter box: #tags a file must all have,
/// and words that must appear in its name, description or tags.
struct Filter {
    var tags: [String]
    var text: String

    init(_ s: String) {
        let words = s.lowercased().split(whereSeparator: \.isWhitespace).map(String.init)
        let isTag = { (w: String) in w.count > 1 && w.hasPrefix("#") }
        tags = words.filter(isTag).map { String($0.dropFirst()) }
        text = words.filter { !isTag($0) }.joined(separator: " ")
    }

    func has(_ s: String?) -> Bool { text.isEmpty || (s ?? "").lowercased().contains(text) }
}

/// FileRow is one file on the start screen: its name, description and
/// tags, a detail on the right, and a pin.
struct FileRow: View {
    var path: String
    var label: String
    var detail: String
    var description: String? = nil
    var tags: [String] = []
    var pinned: Bool
    var onOpen: () -> Void
    var onPin: (Bool) -> Void
    @Environment(\.theme) private var theme
    @State private var hover = false

    var body: some View {
        HStack(spacing: 8) {
            Button(action: onOpen) {
                HStack(alignment: .firstTextBaseline) {
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 6) {
                            Text(label).font(theme.uiFont(weight: .medium)).foregroundStyle(theme.color(theme.palette.link))
                            if !tags.isEmpty {
                                Text(tags.map { "#" + $0 }.joined(separator: " "))
                                    .font(theme.uiFont(size: theme.bodySize * 0.85))
                                    .foregroundStyle(theme.muted)
                            }
                        }
                        if let d = description, !d.isEmpty {
                            Text(d)
                                .font(theme.uiFont(size: theme.bodySize * 0.9))
                                .foregroundStyle(theme.muted)
                                .lineLimit(2)
                                .help(d)
                        }
                    }
                    Spacer(minLength: 12)
                    Text(detail)
                        .font(theme.uiFont(size: theme.bodySize * 0.9))
                        .foregroundStyle(theme.muted)
                        .lineLimit(1)
                }
                .contentShape(Rectangle())
                .padding(.vertical, 6)
                .padding(.horizontal, 8)
            }
            .buttonStyle(.plain)
            .help(path)
            Button { onPin(!pinned) } label: {
                Image(systemName: pinned ? "pin.fill" : "pin")
                    .foregroundStyle(pinned ? theme.accent : theme.muted)
            }
            .buttonStyle(.borderless)
            .opacity(pinned || hover ? 1 : 0.35)
            .help(pinned ? "Unpin" : "Pin to the top")
            .accessibilityLabel((pinned ? "Unpin " : "Pin ") + label)
            .accessibilityAddTraits(pinned ? .isSelected : [])
        }
        .background(hover ? theme.surface : .clear, in: RoundedRectangle(cornerRadius: 6))
        .onHover { hover = $0 }
    }
}

/// TagButton is a #tag that filters the start screen.
struct TagButton: View {
    var tag: String
    var on: Bool
    var count: Int? = nil
    var action: () -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        Button(action: action) {
            HStack(spacing: 4) {
                Text("#" + tag)
                if let count { Text("\(count)").foregroundStyle(on ? theme.accentText.opacity(0.8) : theme.muted) }
            }
            .font(theme.uiFont(size: theme.bodySize * 0.85))
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(on ? theme.accent : theme.surface, in: Capsule())
            .foregroundStyle(on ? theme.accentText : theme.text)
            .overlay(Capsule().stroke(theme.border))
        }
        .buttonStyle(.plain)
        .help((on ? "Stop filtering on #" : "Show files tagged #") + tag)
        .accessibilityLabel(count.map { "\(tag), \($0) \($0 == 1 ? "file" : "files")" } ?? tag)
        .accessibilityAddTraits(on ? .isSelected : [])
    }
}

/// CollectionSection is a collection's files under its header: icon,
/// title, description and tags, in its color over its background.
struct CollectionSection<Rows: View>: View {
    var collection: Collection
    var dir: String
    var startDir: String
    var active: [String]
    var writable: Bool
    var onTag: (String) -> Void
    var onGear: () -> Void
    @ViewBuilder var rows: Rows
    @Environment(\.theme) private var theme

    var body: some View {
        let color = collection.color.map { Theme.nsColor($0) }.map { Color(nsColor: $0) } ?? theme.accent
        let title = collection.title ?? (dir.isEmpty ? (startDir as NSString).lastPathComponent : (dir as NSString).lastPathComponent)
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .top, spacing: 12) {
                icon(title: title, color: color)
                VStack(alignment: .leading, spacing: 4) {
                    HStack(alignment: .firstTextBaseline) {
                        Text(title).font(theme.uiFont(size: theme.bodySize * 1.25, weight: .semibold))
                        Text(dir.isEmpty ? "this folder" : dir + "/").foregroundStyle(theme.muted)
                    }
                    if let d = collection.description, !d.isEmpty {
                        Text(d).foregroundStyle(theme.muted)
                    }
                    if let tags = collection.tags, !tags.isEmpty {
                        FlowLayout(spacing: 6) {
                            ForEach(tags, id: \.self) { t in TagButton(tag: t, on: active.contains(t)) { onTag(t) } }
                        }
                    }
                }
                Spacer()
                if writable {
                    Button(action: onGear) { Image(systemName: "gearshape") }
                        .buttonStyle(.borderless)
                        .help("Folder settings")
                        .accessibilityLabel("Folder settings for \(title)")
                }
            }
            .padding(12)
            .background {
                ZStack {
                    color.opacity(0.12)
                    if let bg = collection.backgroundURL, let url = URL(string: bg), let img = NSImage(contentsOf: url) {
                        Image(nsImage: img).resizable().scaledToFill().opacity(0.25)
                    }
                }
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
            .overlay(alignment: .leading) { Rectangle().fill(color).frame(width: 4).clipShape(RoundedRectangle(cornerRadius: 2)) }
            .help((collection.problems ?? []).isEmpty ? "" : ".mdsh.yaml: " + (collection.problems ?? []).joined(separator: "\n"))
            VStack(spacing: 0) { rows }
        }
    }

    @ViewBuilder
    private func icon(title: String, color: Color) -> some View {
        Group {
            if let u = collection.iconURL, let url = URL(string: u), let img = NSImage(contentsOf: url) {
                Image(nsImage: img).resizable().scaledToFit()
            } else {
                Text(collection.icon ?? String(title.prefix(1)).uppercased())
                    .font(.system(size: 22))
            }
        }
        .frame(width: 40, height: 40)
        .background(color.opacity(0.18), in: RoundedRectangle(cornerRadius: 8))
        .accessibilityHidden(true)
    }
}

/// FlowLayout lays its views out in rows, wrapping as needed.
struct FlowLayout: Layout {
    var spacing: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? 600
        var x: CGFloat = 0, y: CGFloat = 0, row: CGFloat = 0, maxX: CGFloat = 0
        for s in subviews {
            let sz = s.sizeThatFits(.unspecified)
            if x > 0, x + sz.width > width {
                x = 0
                y += row + spacing
                row = 0
            }
            x += sz.width + spacing
            maxX = max(maxX, x)
            row = max(row, sz.height)
        }
        return CGSize(width: min(maxX, width), height: y + row)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX, y = bounds.minY, row: CGFloat = 0
        for s in subviews {
            let sz = s.sizeThatFits(.unspecified)
            if x > bounds.minX, x + sz.width > bounds.maxX {
                x = bounds.minX
                y += row + spacing
                row = 0
            }
            s.place(at: CGPoint(x: x, y: y), proposal: .unspecified)
            x += sz.width + spacing
            row = max(row, sz.height)
        }
    }
}

// MARK: Formatting

/// tilde shows a path under home as ~/….
func tilde(_ path: String, _ home: String) -> String {
    guard !home.isEmpty else { return path }
    if path == home { return "~" }
    if path.hasPrefix(home + "/") { return "~" + path.dropFirst(home.count) }
    return path
}

func blocksLabel(_ n: Int) -> String {
    if n < 0 { return "can't read" }
    return n == 1 ? "1 block" : "\(n) blocks"
}

/// ago says how long ago an RFC 3339 time was, as mdsh-gui does.
func ago(_ when: String) -> String {
    let f = ISO8601DateFormatter()
    f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    let date = f.date(from: when) ?? ISO8601DateFormatter().date(from: when)
    guard let date else { return "" }
    let s = Date().timeIntervalSince(date)
    if s < 90 { return "just now" }
    if s < 3600 { return "\(Int((s / 60).rounded())) min ago" }
    if s < 86400 { return "\(Int((s / 3600).rounded())) h ago" }
    let days = Int((s / 86400).rounded())
    if days < 30 { return days == 1 ? "1 day ago" : "\(days) days ago" }
    return date.formatted(date: .numeric, time: .omitted)
}
