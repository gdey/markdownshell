import AppKit
import SwiftUI

/// DiagramHTML splits prose HTML around the d2 and svg diagrams Go drew
/// in it (notebook/diagrams.go): a figure with the picture, in a light
/// and a dark theme for d2, then the fenced code, then a note when the
/// picture couldn't be drawn.
enum DiagramHTML {
    enum Part {
        case html(String)
        case diagram(Figure)
    }

    struct Figure {
        var lang: String
        var light: Data?
        var dark: Data?
        /// width is the width Go gave the image, from the SVG's viewBox
        /// when it has no width of its own, as d2's hasn't.
        var width: CGFloat?
        /// code is the fenced block, as HTML.
        var code: String
        /// cast is a cast fence's recording, as a data: URI, for the
        /// cast player.
        var cast: String?
        var note: String

        /// key names the diagram by its source, so a redraw keeps it
        /// showing its code, as mdsh-gui's shownAsCode does.
        var key: String { lang + "\n" + code }
    }

    private static let figure = try! NSRegularExpression(pattern: #"<figure class="diagram" data-lang="([^"]*)">([\s\S]*?)</figure>"#)
    private static let img = try! NSRegularExpression(pattern: #"<img(?: class="(light|dark)")?[^>]*? src="data:image/svg\+xml;base64,([^"]*)""#)
    private static let width = try! NSRegularExpression(pattern: #" width="(\d+)""#)
    private static let pre = try! NSRegularExpression(pattern: #"<pre[\s\S]*?</pre>"#)
    private static let note = try! NSRegularExpression(pattern: #"<p class="diagram-note">([\s\S]*?)</p>"#)

    /// parts is html cut around its diagrams, or nil when it has none.
    static func parts(_ html: String) -> [Part]? {
        let s = html as NSString
        let figures = figure.matches(in: html, range: NSRange(location: 0, length: s.length))
        guard !figures.isEmpty else { return nil }
        var out: [Part] = []
        var at = 0
        for m in figures {
            let before = s.substring(with: NSRange(location: at, length: m.range.location - at))
            if !before.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { out.append(.html(before)) }
            let inner = s.substring(with: m.range(at: 2))
            let ns = inner as NSString
            let all = NSRange(location: 0, length: ns.length)
            var f = Figure(lang: s.substring(with: m.range(at: 1)), code: "", note: "")
            for i in img.matches(in: inner, range: all) {
                let data = Data(base64Encoded: ns.substring(with: i.range(at: 2)))
                let tag = ns.substring(with: i.range) as NSString
                if let w = width.firstMatch(in: tag as String, range: NSRange(location: 0, length: tag.length)),
                   let n = Double(tag.substring(with: w.range(at: 1))) {
                    f.width = CGFloat(n)
                }
                if i.range(at: 1).location != NSNotFound, ns.substring(with: i.range(at: 1)) == "dark" {
                    f.dark = data
                } else {
                    f.light = data
                }
            }
            if let p = pre.firstMatch(in: inner, range: all) { f.code = ns.substring(with: p.range) }
            if f.lang == "cast" { f.cast = CastPlayer.source(in: inner) }
            if let n = note.firstMatch(in: inner, range: all) { f.note = CellView.unescape(ns.substring(with: n.range(at: 1))) }
            out.append(.diagram(f))
            at = NSMaxRange(m.range)
        }
        let rest = s.substring(from: at)
        if !rest.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { out.append(.html(rest)) }
        return out
    }
}

/// DiagramView is a d2 or svg fence drawn as its picture, with mdsh-gui's
/// button in the corner to switch to its code. One Go couldn't draw shows
/// its code and the note saying why.
struct DiagramView: View {
    var figure: DiagramHTML.Figure
    /// saveFrame writes a cast fence's frame=, given its recording.
    var saveFrame: ((String, Int) -> Void)?
    @Environment(\.theme) private var theme
    @State private var showCode = false

    /// shownAsCode holds the diagrams switched to their code.
    @MainActor static var shownAsCode: Set<String> = []

    var body: some View {
        if let src = figure.cast {
            // A cast fence: its player, with the button to switch to its
            // code at the box's top right, as an svg fence has.
            ZStack(alignment: .topTrailing) {
                Group {
                    if showCode {
                        HTMLView(html: figure.code)
                    } else {
                        CastPlayer(src: src, save: saveFrame.map { f in { n in f(CastPlayer.body(of: src), n) } })
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                let title = showCode ? "Show the recording" : "Show the cast source"
                Button {
                    showCode.toggle()
                    if showCode { Self.shownAsCode.insert(figure.key) } else { Self.shownAsCode.remove(figure.key) }
                } label: {
                    Image(systemName: showCode ? "play.rectangle" : "chevron.left.forwardslash.chevron.right")
                }
                .keyButton(theme, primary: false)
                .padding(6)
                .help(title)
                .accessibilityLabel(title)
            }
            .padding(.vertical, 8)
            .onAppear { showCode = Self.shownAsCode.contains(figure.key) }
        } else {
            drawn(figure)
        }
    }

    @ViewBuilder
    private func drawn(_ figure: DiagramHTML.Figure) -> some View {
        let image = picture
        ZStack(alignment: .topTrailing) {
            VStack(alignment: .leading, spacing: 4) {
                if let image, !showCode {
                    Image(nsImage: image)
                        .resizable()
                        .scaledToFit()
                        .frame(maxWidth: figure.width ?? image.size.width, alignment: .leading)
                        .padding(.vertical, 8)
                        .accessibilityLabel("\(figure.lang) diagram")
                } else {
                    HTMLView(html: figure.code)
                }
                if !figure.note.isEmpty {
                    Text(figure.note)
                        .font(theme.uiFont(size: theme.bodySize * 0.9))
                        .foregroundStyle(theme.muted)
                        .textSelection(.enabled)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if image != nil {
                let title = showCode ? "Show the diagram" : "Show the \(figure.lang) source"
                Button {
                    showCode.toggle()
                    if showCode { Self.shownAsCode.insert(figure.key) } else { Self.shownAsCode.remove(figure.key) }
                } label: {
                    Image(systemName: showCode ? "photo" : "chevron.left.forwardslash.chevron.right")
                }
                .keyButton(theme, primary: false)
                .padding(4)
                .help(title)
                .accessibilityLabel(title)
            }
        }
        .padding(.vertical, 4)
        .onAppear { showCode = Self.shownAsCode.contains(figure.key) }
    }

    /// picture is the diagram in the window's theme, as mdsh-gui picks
    /// the light or dark one. It is drawn by NSImage (CoreSVG), which
    /// runs no scripts and loads nothing, so an svg fence stays inert as
    /// it does behind mdsh-gui's data: URL image. Keep it that way if this
    /// ever moves to WebKit: a data: URL image, never inline SVG.
    private var picture: NSImage? {
        let data = theme.dark ? (figure.dark ?? figure.light) : figure.light
        return data.flatMap(NSImage.init(data:))
    }
}
