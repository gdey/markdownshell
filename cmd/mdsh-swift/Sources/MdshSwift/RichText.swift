import AppKit
import SwiftUI

/// HTMLCache turns the HTML the bridge renders, prose and output, into
/// attributed strings, keeping them so a redraw doesn't parse again.
/// AppKit's HTML import must run on the main thread.
@MainActor
enum HTMLCache {
    private static var cache: [String: NSAttributedString] = [:]

    static func attributed(_ html: String, css: String) -> NSAttributedString {
        let key = css + "\u{0}" + html
        if let a = cache[key] { return a }
        if cache.count > 2000 { cache.removeAll() }
        let page = "<html><head><meta charset=\"utf-8\"><style>\(css)</style></head><body>\(localImages(html))</body></html>"
        let a =
            (try? NSAttributedString(
                data: Data(page.utf8),
                options: [.documentType: NSAttributedString.DocumentType.html, .characterEncoding: String.Encoding.utf8.rawValue],
                documentAttributes: nil)) ?? NSAttributedString(string: html)
        let trimmed = trimTrailingNewlines(a)
        cache[key] = trimmed
        return trimmed
    }

    /// localImages turns the notebook's /doc/<path>?v=… image links,
    /// which mdsh-gui's asset server serves, into data: URLs holding
    /// the files, which AppKit's HTML import draws. The notebook emits
    /// /doc/ only for files beside the document; unlike the asset
    /// server, this doesn't check that again.
    static func localImages(_ html: String) -> String {
        guard html.contains("src=\"/doc/") else { return html }
        let re = try! NSRegularExpression(pattern: #"src="/doc(/[^"?]*)(\?[^"]*)?""#)
        let ns = html as NSString
        var out = ""
        var last = 0
        for m in re.matches(in: html, range: NSRange(location: 0, length: ns.length)) {
            out += ns.substring(with: NSRange(location: last, length: m.range.location - last))
            let path = ns.substring(with: m.range(at: 1)).removingPercentEncoding ?? ns.substring(with: m.range(at: 1))
            if let data = FileManager.default.contents(atPath: path) {
                out += "src=\"data:\(mimeType(path));base64,\(data.base64EncodedString())\""
            } else {
                out += "src=\"\""
            }
            last = m.range.location + m.range.length
        }
        out += ns.substring(from: last)
        return out
    }

    private static func mimeType(_ path: String) -> String {
        switch (path as NSString).pathExtension.lowercased() {
        case "svg": "image/svg+xml"
        case "png": "image/png"
        case "jpg", "jpeg": "image/jpeg"
        case "gif": "image/gif"
        case "webp": "image/webp"
        default: "application/octet-stream"
        }
    }

    private static func trimTrailingNewlines(_ a: NSAttributedString) -> NSAttributedString {
        let s = a.string as NSString
        var end = s.length
        while end > 0, let c = Unicode.Scalar(s.character(at: end - 1)), CharacterSet.newlines.contains(c) {
            end -= 1
        }
        return end == s.length ? a : a.attributedSubstring(from: NSRange(location: 0, length: end))
    }
}

/// RichText draws an attributed string in a text view that can be
/// selected and whose links open, sized to fit its width.
struct RichText: NSViewRepresentable {
    var text: NSAttributedString
    var onLink: ((URL) -> Bool)? = nil

    func makeNSView(context: Context) -> NSTextView {
        let v = NSTextView()
        v.isEditable = false
        v.isSelectable = true
        v.drawsBackground = false
        v.textContainerInset = .zero
        v.textContainer?.lineFragmentPadding = 0
        v.textContainer?.widthTracksTextView = true
        v.isVerticallyResizable = false
        v.isHorizontallyResizable = false
        v.delegate = context.coordinator
        v.setAccessibilityRole(.staticText)
        return v
    }

    func updateNSView(_ v: NSTextView, context: Context) {
        context.coordinator.onLink = onLink
        if v.textStorage?.isEqual(to: text) != true {
            v.textStorage?.setAttributedString(text)
        }
    }

    func sizeThatFits(_ proposal: ProposedViewSize, nsView v: NSTextView, context: Context) -> CGSize? {
        let width = proposal.width ?? 600
        guard let container = v.textContainer, let layout = v.layoutManager else { return nil }
        container.containerSize = NSSize(width: width, height: .greatestFiniteMagnitude)
        layout.ensureLayout(for: container)
        let used = layout.usedRect(for: container)
        return CGSize(width: width, height: ceil(used.height))
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    final class Coordinator: NSObject, NSTextViewDelegate {
        var onLink: ((URL) -> Bool)?

        func textView(_ textView: NSTextView, clickedOnLink link: Any, at charIndex: Int) -> Bool {
            let url = (link as? URL) ?? (link as? String).flatMap(URL.init(string:))
            guard let url else { return true }
            if let onLink, onLink(url) { return true }
            // Only web and mail links open, as in mdsh-gui.
            if ["http", "https", "mailto"].contains(url.scheme?.lowercased() ?? "") {
                NSWorkspace.shared.open(url)
            }
            return true
        }

        /// A click on an image opens it full size over the window, as in
        /// mdsh-gui (see ImageViewer).
        func textView(_ textView: NSTextView, clickedOn cell: NSTextAttachmentCellProtocol, in cellFrame: NSRect, at charIndex: Int) {
            guard let storage = textView.textStorage, charIndex < storage.length,
                  let att = storage.attribute(.attachment, at: charIndex, effectiveRange: nil) as? NSTextAttachment
            else { return }
            let image = att.image
                ?? att.fileWrapper?.regularFileContents.flatMap(NSImage.init(data:))
                ?? (cell as? NSTextAttachmentCell)?.image
            guard let image else { return }
            NotificationCenter.default.post(name: .mdshShowImage, object: textView.window, userInfo: ["image": image])
        }
    }
}

extension Notification.Name {
    /// mdshShowImage asks the window (the object) to show userInfo's
    /// "image" full size.
    static let mdshShowImage = Notification.Name("mdshShowImage")
}

/// ImageViewer shows an output image full size over the window, scaled
/// to fit; a click or Escape closes it.
struct ImageViewer: View {
    var image: NSImage
    var close: () -> Void
    @FocusState private var focused: Bool

    var body: some View {
        ZStack {
            Color.black.opacity(0.75)
            Image(nsImage: image)
                .resizable()
                .interpolation(.high)
                .aspectRatio(contentMode: .fit)
                .padding(32)
                .accessibilityLabel("Image, full size")
        }
        .contentShape(Rectangle())
        .onTapGesture(perform: close)
        .focusable()
        .focusEffectDisabled()
        .focused($focused)
        .onKeyPress(.escape) {
            close()
            return .handled
        }
        .onAppear { focused = true }
        .accessibilityAddTraits(.isModal)
        .accessibilityAction(named: "Close", close)
    }
}

/// HTMLView draws HTML from the bridge in the theme's colors and fonts.
struct HTMLView: View {
    var html: String
    var mono = false
    var onLink: ((URL) -> Bool)? = nil
    @Environment(\.theme) private var theme

    var body: some View {
        RichText(text: HTMLCache.attributed(html, css: theme.css(mono: mono)), onLink: onLink)
    }
}

/// ANSI strips terminal escapes from live output, which is drawn as
/// plain text until the block ends and its rendered output comes.
enum ANSI {
    private static let escape = try! NSRegularExpression(pattern: "\u{1B}(\\[[0-9;?]*[ -/]*[@-~]|\\][^\u{07}\u{1B}]*(\u{07}|\u{1B}\\\\)|[@-Z\\\\-_])")

    static func strip(_ s: String) -> String {
        let r = NSRange(s.startIndex..., in: s)
        let out = escape.stringByReplacingMatches(in: s, range: r, withTemplate: "")
        return out.replacingOccurrences(of: "\r\n", with: "\n")
    }
}
