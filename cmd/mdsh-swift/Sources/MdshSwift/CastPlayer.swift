import AppKit
import MdshKit
import SwiftUI

/// CastPlayer plays an asciinema recording, a tty block's output or a
/// cast fence, as mdsh-gui's cast player does: play and pause, a
/// scrubber, frame by frame stepping, and a button that copies the
/// recording as an animated GIF. The bridge makes the recording into
/// frames by termrec's rule, so frame=N names the same screen here as in
/// mdsh-gui and a PDF. Where the head is left is written to the file as
/// frame=N, through save, once it rests.
struct CastPlayer: View {
    /// src is the recording's link: /doc/<path>.cast or a data: URI,
    /// with #frame=N.
    var src: String
    /// save writes the frame to open at, counting from 1, or 0 for the
    /// last.
    var save: ((Int) -> Void)?
    @Environment(\.theme) private var theme
    @State private var info: CastInfo?
    @State private var failed: String?
    @State private var frame = 0  // the frame shown, from 0
    @State private var images: [Int: NSImage] = [:]
    @State private var shown: NSImage?
    /// paper is the screen's background, from the first frame's corner,
    /// to fill the box past the screen's columns.
    @State private var paper: Color = .black
    @State private var playing = false
    @State private var saved = -1
    @State private var saveTask: Task<Void, Never>?
    @State private var copied = false
    @FocusState private var focused: Bool

    /// castRegex finds a recording linked as an image in output HTML.
    private static let castRegex = try! NSRegularExpression(
        pattern: #"<img[^>]*?src="((?:/doc/[^"]*?\.cast(?:[?#][^"]*)?)|(?:data:application/x-asciicast;base64,[^"]*))""#,
        options: [.caseInsensitive])

    /// source is the first recording html links to, if any.
    static func source(in html: String) -> String? {
        let ns = html as NSString
        guard let m = castRegex.firstMatch(in: html, range: NSRange(location: 0, length: ns.length)) else { return nil }
        return ns.substring(with: m.range(at: 1)).replacingOccurrences(of: "&amp;", with: "&")
    }

    /// body is a data: URI's recording, as the cast fence holds it.
    static func body(of src: String) -> String {
        let s = src.split(separator: "#", maxSplits: 1).first.map(String.init) ?? src
        guard let b64 = s.components(separatedBy: "base64,").last, s.hasPrefix("data:"),
              let d = Data(base64Encoded: b64) else { return "" }
        return String(decoding: d, as: UTF8.self)
    }

    private var key: String { src.split(separator: "#", maxSplits: 1).first.map(String.init) ?? src }
    private var count: Int { info?.delays.count ?? 0 }
    private var last: Int { max(count - 1, 0) }

    var body: some View {
        Group {
            if let failed {
                Text("Couldn't play the recording: \(failed)").foregroundStyle(theme.danger)
            } else if let info {
                // One box holds the screen and its controls, ringed in
                // the accent while the player has the keyboard.
                // The box spans the card, as mdsh-gui's does; the
                // screen keeps its columns at the code font's width.
                VStack(alignment: .leading, spacing: 0) {
                    HStack(spacing: 0) {
                        screen(info).frame(maxWidth: screenWidth(info), alignment: .leading)
                        Spacer(minLength: 0)
                    }
                    .background(paper)
                    controls.padding(.horizontal, 8).padding(.vertical, 5)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(theme.surface)
                .clipShape(RoundedRectangle(cornerRadius: corner))
                .overlay {
                    RoundedRectangle(cornerRadius: corner)
                        .strokeBorder(focused ? theme.accent : theme.border, lineWidth: focused ? 2 : 1)
                }
                .focusable()
                .focusEffectDisabled()
                .focused($focused)
                .onTapGesture { focused = true }
                // mdsh-gui's keys: ← and , step back, → and . step on,
                // space plays and pauses.
                .onKeyPress(.leftArrow) { step(-1); return .handled }
                .onKeyPress(.rightArrow) { step(1); return .handled }
                .onKeyPress(characters: CharacterSet(charactersIn: ",. ")) { k in
                    switch k.characters {
                    case ",": step(-1)
                    case ".": step(1)
                    default: playing ? pause() : play()
                    }
                    return .handled
                }
            } else {
                ProgressView().controlSize(.small)
            }
        }
        .task(id: key) { await load() }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Recording")
    }

    private func screen(_ info: CastInfo) -> some View {
        Group {
            if let shown {
                Image(nsImage: shown)
                    .resizable()
                    .interpolation(.high)
                    .scaledToFit()
            } else {
                Color.black
            }
        }
        // As wide as its columns in the code font, as mdsh-gui's
        // terminal is, however small termrec drew the GIF.
        .aspectRatio(CGFloat(info.width) / CGFloat(max(info.height, 1)), contentMode: .fit)
        .accessibilityLabel("Frame \(frame + 1) of \(count)")
    }

    /// corner is the color at an image's top left pixel. The pixel is
    /// drawn into an sRGB context first, so the color matches how the
    /// image itself is drawn whatever color space the frame came in.
    static func corner(_ img: NSImage) -> Color? {
        guard let cg = img.cgImage(forProposedRect: nil, context: nil, hints: nil),
              let space = CGColorSpace(name: CGColorSpace.sRGB)
        else { return nil }
        var px = [UInt8](repeating: 0, count: 4)
        let ok = px.withUnsafeMutableBytes { buf -> Bool in
            guard let ctx = CGContext(data: buf.baseAddress, width: 1, height: 1, bitsPerComponent: 8,
                                      bytesPerRow: 4, space: space,
                                      bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
            else { return false }
            // Place the image so its top left pixel lands on the context's one pixel.
            ctx.draw(cg, in: CGRect(x: 0, y: 1 - CGFloat(cg.height), width: CGFloat(cg.width), height: CGFloat(cg.height)))
            return true
        }
        guard ok else { return nil }
        return Color(.sRGB, red: Double(px[0]) / 255, green: Double(px[1]) / 255, blue: Double(px[2]) / 255)
    }

    private var corner: CGFloat { theme.look == .gum ? 16 : theme.look == .plain ? 6 : 0 }

    /// clock is a time as mdsh-gui's counter has it: 0:02.1.
    static func clock(_ ms: Int) -> String {
        let tenths = ms / 100
        return String(format: "%d:%02d.%d", tenths / 600, tenths / 10 % 60, tenths % 10)
    }

    /// at is when frame n shows, in milliseconds, as mdsh-gui counts it:
    /// from the recording's start, by the frames' delays (which, as the
    /// GIF's, cap a pause at two seconds).
    private func at(_ n: Int) -> Int { (info?.start ?? 0) + (info?.delays ?? []).prefix(n).reduce(0, +) }

    private func screenWidth(_ info: CastInfo) -> CGFloat {
        let m = ("M" as NSString).size(withAttributes: [.font: theme.nsCodeFont]).width
        return info.cols > 0 && m > 0 ? CGFloat(info.cols) * m + 16 : CGFloat(info.width)
    }

    private var controls: some View {
        HStack(spacing: 8) {
            Button { step(-1) } label: { Image(systemName: "backward.frame.fill") }
                .disabled(frame == 0)
                .help("Previous frame (← or ,)")
                .accessibilityLabel("Previous frame")
            Button { playing ? pause() : play() } label: {
                Image(systemName: playing ? "pause.fill" : "play.fill")
                    .foregroundStyle(playing ? theme.accentText : theme.muted)
                    .frame(width: 22, height: 18)
                    .background(playing ? theme.accent : Color.clear, in: RoundedRectangle(cornerRadius: 4))
            }
            .help(playing ? "Pause (space)" : "Play (space)")
            .accessibilityLabel(playing ? "Pause" : "Play")
            Button { step(1) } label: { Image(systemName: "forward.frame.fill") }
                .disabled(frame >= last)
                .help("Next frame (→ or .)")
                .accessibilityLabel("Next frame")
            Slider(value: Binding(get: { Double(frame) }, set: { v in
                pause()
                show(Int(v.rounded()))
            }), in: 0...Double(max(last, 1)))
            .disabled(count < 2)
            .accessibilityLabel("Frame")
            Text("\(frame + 1) / \(count) · \(Self.clock(at(frame))) / \(Self.clock(at(last)))")
                .fixedSize()
                .font(theme.codeFont(size: theme.codeSize * 0.85))
                .monospacedDigit()
                .foregroundStyle(theme.muted)
                .lineLimit(1)
            if copied {
                Text("Copied a GIF of the recording")
                    .font(theme.uiFont(size: theme.bodySize * 0.8))
                    .foregroundStyle(theme.accent)
            }
            Button { Task { await copyGIF() } } label: { Image(systemName: "doc.on.doc") }
            .help("Copy the recording as an animated GIF")
            .accessibilityLabel(copied ? "Copied" : "Copy as GIF")
        }
        .buttonStyle(.borderless)
        .foregroundStyle(theme.muted)
    }

    // MARK: Playing

    private func load() async {
        playing = false
        images = [:]
        do {
            let i: CastInfo = try await AppModel.shared.tryCall("castLoad", ["src": .init(src)])
            info = i
            failed = nil
            let start = i.frame > 0 ? i.frame - 1 : max(i.delays.count - 1, 0)
            saved = i.frame > 0 ? start : -1
            await showNow(start)
        } catch {
            failed = error.localizedDescription
        }
    }

    private func image(_ n: Int) async -> NSImage? {
        if let img = images[n] { return img }
        guard let id = info?.id,
              let d: Data = try? await AppModel.shared.tryCall("castFrame", ["id": .init(id), "frame": .init(n + 1)]),
              let img = NSImage(data: d)
        else { return nil }
        images[n] = img
        return img
    }

    private func showNow(_ n: Int) async {
        let n = min(max(n, 0), last)
        frame = n
        if let img = await image(n), frame == n {
            if shown == nil, let c = Self.corner(img) { paper = c }
            shown = img
        }
    }

    private func show(_ n: Int) {
        Task { await showNow(n) }
        keep()
    }

    private func step(_ by: Int) {
        pause()
        show(frame + by)
    }

    private func play() {
        guard let info, count > 1 else { return }
        if frame >= last { frame = 0 }
        playing = true
        Task {
            await showNow(frame)
            while playing && frame < last {
                let ms = min(info.delays[frame], 2000)
                try? await Task.sleep(for: .milliseconds(max(ms, 20)))
                guard playing else { return }
                await showNow(frame + 1)
            }
            playing = false
            keep()
        }
    }

    private func pause() {
        playing = false
        keep()
    }

    /// keep writes where the head was left to the file, once it rests.
    private func keep() {
        guard let save else { return }
        saveTask?.cancel()
        saveTask = Task {
            try? await Task.sleep(for: .milliseconds(800))
            guard !Task.isCancelled, !playing, info != nil else { return }
            let at = frame == last ? -1 : frame
            if at == saved { return }
            saved = at
            save(at + 1)
        }
    }

    private func copyGIF() async {
        guard let id = info?.id,
              let d: Data = try? await AppModel.shared.tryCall("castGIF", ["id": .init(id)])
        else { return }
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setData(d, forType: NSPasteboard.PasteboardType("com.compuserve.gif"))
        copied = true
        try? await Task.sleep(for: .seconds(1.5))
        copied = false
    }
}
