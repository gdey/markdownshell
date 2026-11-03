import AppKit
import MdshKit
import SwiftUI

/// Look is the window's look, as Settings › Look names it: plain,
/// Teletype ("tty"), Blueprint, Bubble Gum ("gum") or Witchcraft
/// ("witch"), in Settings › Look's order.
enum Look: String, CaseIterable, Identifiable {
    case plain = ""
    case tty
    case blueprint
    case gum
    case witch
    var id: String { rawValue }
    var label: String {
        switch self {
        case .plain: "Plain"
        case .tty: "Teletype"
        case .blueprint: "Blueprint"
        case .gum: "Bubble Gum"
        case .witch: "Witchcraft"
        }
    }

    /// about describes the look under Settings › Look's menu, as its
    /// look.css front matter in mdsh-gui does (bridge/looks_test.go
    /// checks they agree).
    var about: String {
        switch self {
        case .plain: "Plain is the everyday look: the system's own face, white in the light theme and near black in the dark."
        case .tty: "Teletype sets everything in a typewriter face on fan-fold paper, or green phosphor when the theme is dark."
        case .blueprint: "Blueprint draws in white lines on blue paper with a drafting grid, a deeper night blue when the theme is dark."
        case .gum: "Bubble Gum is Hubba Bubba pink with bubbles across the page, puffy cards and bubble-lettered headings, or berry when the theme is dark."
        case .witch: "Witchcraft writes the document on a scroll of speckled kraft paper, framed in a double line with flourishes, against a sky of stars and crescent moons, with carved capitals for headings, or deep plum and gold when the theme is dark."
        }
    }

    /// Texture names a look's texture switches in Settings › Look: the
    /// group's title, and the label of each switch it draws, nil for
    /// one it doesn't.
    struct Texture {
        var label: String
        var sprockets: String?
        var rules: String?
        var shadows: String?
    }

    /// texture is the look's texture switches, nil for a look with none.
    var texture: Texture? {
        switch self {
        case .plain: nil
        case .tty: Texture(label: "Teletype paper", sprockets: "Sprocket strips", rules: "Ruled lines", shadows: "Shadows under cards")
        case .blueprint: Texture(label: "Blueprint sheet", rules: "Grid")
        case .gum: Texture(label: "Bubble gum wrapper", rules: "Bubbles", shadows: "Shadows under cards")
        case .witch: Texture(label: "Grimoire", rules: "Stars and moons", shadows: "Glow around cards")
        }
    }
}

/// Palette is the colors of a look in light or dark, the same values
/// mdsh-gui's style.css, teletype.css, blueprint.css and bubblegum.css
/// use. Bubble Gum's surface is its card fill, --gum-card.
struct Palette: Equatable {
    var background: String
    var surface: String  // cards and bars
    var text: String
    var muted: String
    var border: String
    var accent: String
    var accentText: String  // text on an accent button
    var codeBackground: String
    var danger: String
    var ok: String
    var link: String

    static func of(_ look: Look, dark: Bool) -> Palette {
        switch (look, dark) {
        case (.plain, false):
            Palette(background: "#ffffff", surface: "#f6f8fa", text: "#1f2328", muted: "#59636e", border: "#d1d9e0",
                    accent: "#0969da", accentText: "#ffffff", codeBackground: "#f6f8fa", danger: "#cf222e", ok: "#1a7f37", link: "#0969da")
        case (.plain, true):
            Palette(background: "#0d1117", surface: "#151b23", text: "#e6edf3", muted: "#9198a1", border: "#3d444d",
                    accent: "#2f81f7", accentText: "#0d1117", codeBackground: "#151b23", danger: "#f85149", ok: "#3fb950", link: "#4493f8")
        case (.tty, false):
            Palette(background: "#f3edd9", surface: "#f8f3e3", text: "#2a241c", muted: "#6b6150", border: "#8f8774",
                    accent: "#2a241c", accentText: "#f3edd9", codeBackground: "#efe8d2", danger: "#a3271b", ok: "#3d6b35", link: "#8a2b1f")
        case (.tty, true):
            Palette(background: "#0b0f0b", surface: "#0f170f", text: "#9cff9c", muted: "#649a64", border: "#447544",
                    accent: "#9cff9c", accentText: "#0b0f0b", codeBackground: "#0f170f", danger: "#ff6b4a", ok: "#9cff9c", link: "#ffb000")
        case (.blueprint, false):
            Palette(background: "#1d5ba3", surface: "#1a5296", text: "#f4f8ff", muted: "#dce8f8", border: "#a6c3e6",
                    accent: "#f4f8ff", accentText: "#1d5ba3", codeBackground: "#184d8c", danger: "#ffc9c0", ok: "#8ff0b4", link: "#ffd75e")
        case (.blueprint, true):
            Palette(background: "#0b1e3b", surface: "#0d2244", text: "#d6e8ff", muted: "#7d9cc4", border: "#4f7ab4",
                    accent: "#8fd3ff", accentText: "#0b1e3b", codeBackground: "#0e2547", danger: "#ff8a80", ok: "#7ee2a8", link: "#ffd75e")
        case (.gum, false):
            Palette(background: "#ffc6dc", surface: "#fff0f6", text: "#4a0f33", muted: "#8c3a68", border: "#e879b1",
                    accent: "#d8006f", accentText: "#ffffff", codeBackground: "#ffe1ec", danger: "#b3102f", ok: "#0a6a3e", link: "#b3005c")
        case (.gum, true):
            Palette(background: "#2b0a1f", surface: "#331029", text: "#ffd1e3", muted: "#d690b7", border: "#8a3b6c",
                    accent: "#ff5fae", accentText: "#2b0a1f", codeBackground: "#3a1130", danger: "#ff7f94", ok: "#6fe3a4", link: "#ff8fc6")
        // witchcraft/look.css: the surface is the card's page, --witch-card.
        case (.witch, false):
            Palette(background: "#ece2f6", surface: "#f2e6c9", text: "#2a1240", muted: "#4b3563", border: "#b8a0d4",
                    accent: "#6b21a8", accentText: "#ffffff", codeBackground: "#f2e6c9", danger: "#a3122f", ok: "#1f6b3a", link: "#5a1896")
        case (.witch, true):
            Palette(background: "#160c24", surface: "#241424", text: "#ece1ff", muted: "#b4a2d0", border: "#4e3672",
                    accent: "#b98aff", accentText: "#160c24", codeBackground: "#241424", danger: "#ff7a8a", ok: "#7fe0a0", link: "#d2b0ff")
        }
    }
}

/// Theme is what the views draw with, worked out from the settings and
/// the system's appearance.
struct Theme: Equatable {
    var look: Look
    var dark: Bool
    var palette: Palette
    var textScale: Double
    var uiFont: String
    var codeFont: String
    var lineSpacing: Double
    /// noSprockets and noRules take the paper's holes and its rules or
    /// grid off, as Settings › Look says.
    var noSprockets: Bool
    var noRules: Bool

    init(settings: AppSettings, systemDark: Bool) {
        noSprockets = settings.noSprockets
        noRules = settings.noRules
        look = Look(rawValue: settings.look) ?? .plain
        switch settings.theme {
        case "light": dark = false
        case "dark": dark = true
        default: dark = systemDark
        }
        palette = Palette.of(look, dark: dark)
        if !settings.accent.isEmpty {
            palette.accent = settings.accent
            palette.link = settings.accent
            palette.accentText = Theme.luminance(settings.accent) > 0.5 ? "#000000" : "#ffffff"
        }
        let base = look == .tty ? 115.0 : 100.0
        textScale = (settings.textSize == 0 ? base : Double(settings.textSize)) / 100
        uiFont = settings.uiFont
        codeFont = settings.codeFont
        switch settings.lineSpacing {
        case "compact": lineSpacing = 1.3
        case "relaxed": lineSpacing = 1.8
        default: lineSpacing = 1.5
        }
    }

    var colorScheme: ColorScheme { dark ? .dark : .light }

    func color(_ hex: String) -> Color { Color(nsColor: Theme.nsColor(hex)) }
    var background: Color { color(palette.background) }
    var surface: Color { color(palette.surface) }
    var text: Color { color(palette.text) }
    var muted: Color { color(palette.muted) }
    var border: Color { color(palette.border) }
    var accent: Color { color(palette.accent) }
    var accentText: Color { color(palette.accentText) }
    var codeBackground: Color { color(palette.codeBackground) }
    var danger: Color { color(palette.danger) }
    var ok: Color { color(palette.ok) }
    /// warnBorder and warnBackground are mdsh-gui's --warn-border and
    /// --warn-bg, which the stale badge is drawn in.
    var warnBorder: Color {
        switch (look, dark) {
        case (.plain, false): color("#d4a72c")
        case (.plain, true): color("#9e6a03")
        case (.tty, false): color("#a8862e")
        case (.tty, true): color("#b88a00")
        case (.blueprint, false): color("#ffd75e")
        case (.blueprint, true): color("#c9a63a")
        case (.gum, false): color("#c48f0f")
        case (.gum, true): color("#d9a73a")
        case (.witch, false): color("#b08410")
        case (.witch, true): color("#c9a03a")
        }
    }
    var warnBackground: Color {
        switch (look, dark) {
        case (.plain, false): color("#fff8c5")
        case (.plain, true): color("#2b2611")
        case (.tty, false): color("#ede0b0")
        case (.tty, true): color("#2a2208")
        case (.blueprint, false): color("#4f5a3a")
        case (.blueprint, true): color("#2e2f1c")
        case (.gum, false): color("#fff0b3")
        case (.gum, true): color("#3f2c12")
        case (.witch, false): color("#f8ecc4")
        case (.witch, true): color("#33270f")
        }
    }
    /// side is the strip the paper looks paint their tab bars and
    /// outline in: --tty-side and --bp-side. The plain look has none.
    var side: Color {
        switch look {
        case .tty: color(dark ? "#0f150f" : "#e9e2c9")
        case .blueprint: color(dark ? "#0d2242" : "#1a5596")
        case .gum: color(dark ? "#230818" : "#ffb6d2")
        case .witch: color(dark ? "#100819" : "#e2d4f0")
        case .plain: surface
        }
    }

    /// bubblegum.css's own: the bubbles on the page, the chunky shadow
    /// under cards and buttons, and the pop behind the headings.
    var gumBubble: Color { dark ? color("#ff5fae").opacity(0.09) : Color.white.opacity(0.4) }
    var gumShadow: Color { color(dark ? "#15040e" : "#e879b1") }
    var gumPop: Color { color(dark ? "#b0005a" : "#ff8ac4") }

    /// witchcraft/look.css's own: the gilt of the ornaments, the frame's
    /// hairlines, the glow around cards, the ink of the carved headings,
    /// the scroll the document is written on and its frame's line, and
    /// the confirm slip, --hover.
    var witchGilt: Color { color(dark ? "#e8c36a" : "#8a6410") }
    var witchFrame: Color { dark ? color("#e8c36a").opacity(0.28) : color("#8a6410").opacity(0.35) }
    var witchGlow: Color { dark ? color("#b98aff").opacity(0.22) : color("#5b1896").opacity(0.18) }
    var witchInk: Color { color(dark ? "#e6d4ff" : "#3d1066") }
    var witchSheet: Color { color(dark ? "#2e1a2e" : "#dcc597") }
    var witchLine: Color { color(dark ? "#c9a24a" : "#3a2410") }
    var witchHover: Color { color(dark ? "#2a1946" : "#dfd0ef") }
    /// witchSky is the stars' color and witchMoon the crescents'.
    var witchSky: Color { dark ? color("#efe4ff").opacity(0.3) : color("#7b55a8").opacity(0.22) }
    var witchMoon: Color { dark ? color("#e8c36a").opacity(0.45) : color("#a07d1c").opacity(0.22) }

    var bodySize: CGFloat { 14 * textScale }
    var codeSize: CGFloat { 13 * textScale }

    /// The font stacks are mdsh-gui's (teletype.css, blueprint.css):
    /// the first installed one is used.
    private static let ttyStack = ["Courier Prime", "Courier New", "Courier"]
    private static let blueprintStack = ["Barlow Condensed", "Roboto Condensed", "Arial Narrow", "Avenir Next Condensed", "Helvetica Neue"]
    private static let gumStack = ["Nunito", "Varela Round", "Quicksand", "Arial Rounded MT Bold", "Helvetica Rounded"]
    private static let witchStack = ["Palatino", "Palatino Linotype", "Book Antiqua", "URW Palladio L", "P052", "Georgia"]
    /// carvedStack is Witchcraft's headings' carved capitals.
    private static let carvedStack = ["Cinzel", "Trajan Pro", "Palatino", "Palatino Linotype", "Book Antiqua", "URW Palladio L", "P052", "Georgia"]

    private static let ttyFont = ttyStack.first(where: isFont)
    private static let blueprintFont = blueprintStack.first(where: isFont)
    private static let gumFont = gumStack.first(where: isFont)
    private static let witchFont = witchStack.first(where: isFont)
    static let carvedName = carvedStack.first(where: isFont)

    var codeFontName: String {
        if !codeFont.isEmpty { return codeFont }
        return look == .tty ? (Self.ttyFont ?? "Menlo") : "Menlo"
    }
    var uiFontName: String {
        if !uiFont.isEmpty { return uiFont }
        switch look {
        case .tty: return Self.ttyFont ?? ""
        case .blueprint: return Self.blueprintFont ?? ""
        case .gum: return Self.gumFont ?? ""
        case .witch: return Self.witchFont ?? ""
        case .plain: return ""
        }
    }

    func uiFont(size: CGFloat? = nil, weight: Font.Weight = .regular) -> Font {
        let s = size ?? bodySize
        // A font that isn't installed falls back to the system's.
        if uiFontName.isEmpty || !Self.isFont(uiFontName) { return .system(size: s, weight: weight) }
        return .custom(uiFontName, size: s).weight(weight)
    }

    /// carvedFont is Witchcraft's headings' carved capitals, semibold.
    func carvedFont(size: CGFloat) -> Font {
        guard let name = Self.carvedName else { return .system(size: size, weight: .semibold, design: .serif) }
        return .custom(name, size: size).weight(.semibold)
    }

    func codeFont(size: CGFloat? = nil) -> Font {
        let s = size ?? codeSize
        if !Self.isFont(codeFontName) { return .system(size: s, design: .monospaced) }
        return .custom(codeFontName, size: s)
    }

    private static func isFont(_ name: String) -> Bool {
        NSFont(name: name, size: 12) != nil || NSFontManager.shared.availableMembers(ofFontFamily: name) != nil
    }

    var nsCodeFont: NSFont {
        NSFont(name: codeFontName, size: codeSize) ?? .monospacedSystemFont(ofSize: codeSize, weight: .regular)
    }

    /// css styles the HTML prose and output are drawn from.
    func css(mono: Bool) -> String {
        let p = palette
        let ui = uiFontName.isEmpty ? "-apple-system, 'Helvetica Neue', sans-serif" : "'\(uiFontName)', \(look == .tty ? "monospace" : look == .witch ? "serif" : "sans-serif")"
        let code = "'\(codeFontName)', Menlo, monospace"
        let size = mono ? codeSize : bodySize
        return """
        body { font-family: \(mono ? code : ui); font-size: \(size)px; color: \(p.text); line-height: \(lineSpacing); }
        a { color: \(p.link); }
        code, pre, kbd { font-family: \(code); font-size: \(codeSize)px; }
        code { background-color: \(p.codeBackground); }
        pre { background-color: \(p.codeBackground); padding: 8px; }
        h1, h2, h3, h4, h5, h6 { color: \(p.text); margin: 0.4em 0 0.2em 0; }
        \(look == .blueprint ? "h1, h2, h3, h4, h5, h6 { text-transform: uppercase; letter-spacing: 0.08em; }" : "")
        \(look == .gum ? "h1, h2, h3 { font-weight: 900; text-shadow: 2px 2px 0 \(dark ? "#b0005a" : "#ff8ac4"); } blockquote { border-left: 4px solid \(dark ? "#b0005a" : "#ff8ac4"); } hr { border: 0; border-top: 3px dotted \(p.border); }" : "")
        \(look == .witch ? witchCSS : "")
        h1 { font-size: \(size * 1.9)px; } h2 { font-size: \(size * 1.5)px; } h3 { font-size: \(size * 1.25)px; }
        blockquote { color: \(p.muted); border-left: 3px solid \(p.border); padding-left: 8px; }
        table { border-collapse: collapse; } td, th { border: 1px solid \(p.border); padding: 4px 8px; }
        .mdsh-remote-image, .remote-image { color: \(p.muted); font-style: italic; }
        """
    }

    /// witchCSS is Witchcraft's prose: carved capitals in the ink for
    /// headings, gilt for rules and quotes.
    private var witchCSS: String {
        let gilt = dark ? "#e8c36a" : "#8a6410"
        let carved = Self.carvedName.map { "'\($0)', " } ?? ""
        return "h1, h2, h3 { font-family: \(carved)serif; font-weight: 600; letter-spacing: 0.04em; color: \(dark ? "#e6d4ff" : "#3d1066"); } "
            + "hr { border: 0; border-top: 1px solid \(gilt); } blockquote { border-left: 3px solid \(gilt); font-style: italic; }"
    }

    static func nsColor(_ hex: String) -> NSColor {
        var s = hex.trimmingCharacters(in: .whitespaces)
        if s.hasPrefix("#") { s.removeFirst() }
        guard s.count == 6, let v = UInt32(s, radix: 16) else { return .textColor }
        return NSColor(srgbRed: CGFloat((v >> 16) & 0xff) / 255, green: CGFloat((v >> 8) & 0xff) / 255,
                       blue: CGFloat(v & 0xff) / 255, alpha: 1)
    }

    /// luminance is hex's relative luminance, 0 to 1, for picking text
    /// that reads on it.
    static func luminance(_ hex: String) -> Double {
        let c = nsColor(hex).usingColorSpace(.sRGB) ?? .black
        func lin(_ v: CGFloat) -> Double {
            let d = Double(v)
            return d <= 0.03928 ? d / 12.92 : pow((d + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * lin(c.redComponent) + 0.7152 * lin(c.greenComponent) + 0.0722 * lin(c.blueComponent)
    }
}

/// Paper is what the rendered view is drawn on in the looks drawn on
/// paper, as mdsh-gui paints it behind the cards: a roll of paper with a
/// rule every line and sprocket holes down each edge, a drafting sheet
/// with a fine grid every 20 points and a heavy one every 100, bubbles,
/// or Witchcraft's scroll against the night sky. It scrolls with the
/// document.
struct Paper: View {
    var theme: Theme

    var body: some View {
        Canvas { ctx, size in
            switch theme.look {
            case .tty: tty(ctx, size)
            case .blueprint: if !theme.noRules { grid(ctx, size) }
            case .gum: if !theme.noRules { bubbles(ctx, size) }
            case .witch: witch(ctx, size)
            case .plain: break
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    // teletype.css: --tty-rule, --tty-hole, and --border for the strips.
    private var rule: Color { theme.color(theme.dark ? "#131d13" : "#e7e1ce") }
    private var hole: Color { theme.color(theme.dark ? "#162016" : "#d8cfae") }

    private func tty(_ ctx: GraphicsContext, _ size: CGSize) {
        let step: CGFloat = 39
        if !theme.noRules {
            var y = step - 1
            while y < size.height {
                ctx.fill(Path(CGRect(x: 0, y: y, width: size.width, height: 1)), with: .color(rule))
                y += step
            }
        }
        guard !theme.noSprockets else { return }
        for x in [CGFloat(24), size.width - 25] {
            ctx.fill(Path(CGRect(x: x, y: 0, width: 1, height: size.height)), with: .color(theme.border))
        }
        var y: CGFloat = 12 + step / 2
        while y < size.height {
            for cx in [CGFloat(12), size.width - 12] {
                ctx.fill(Path(ellipseIn: CGRect(x: cx - 5, y: y - 5, width: 10, height: 10)), with: .color(hole))
            }
            y += step
        }
    }

    // bubblegum.css: bubbles big and small blown across the page, a
    // 140-point tile of four.
    private func bubbles(_ ctx: GraphicsContext, _ size: CGSize) {
        let tile: CGFloat = 140
        let spots: [(CGFloat, CGFloat, CGFloat)] = [(34, 38, 14.5), (104, 102, 9.5), (118, 30, 5.5), (60, 110, 4.5)]
        var path = Path()
        var y: CGFloat = 0
        while y < size.height {
            var x: CGFloat = 0
            while x < size.width {
                for (cx, cy, r) in spots {
                    path.addEllipse(in: CGRect(x: x + cx - r, y: y + cy - r, width: r * 2, height: r * 2))
                }
                x += tile
            }
            y += tile
        }
        ctx.fill(path, with: .color(theme.gumBubble))
    }

    // witchcraft/look.css: the night sky, a 220-point tile of stars and
    // crescent moons, and over it the scroll the document is written on,
    // the cards' column and 46 points either side, as mdsh-gui leaves for
    // the frame and the fold buttons: speckled kraft paper, or plum in the
    // dark, ruled round with a double line and a flourish in each corner.
    private func witch(_ ctx: GraphicsContext, _ size: CGSize) {
        if !theme.noRules { sky(ctx, size) }
        // The column is 980 wide with 28 of padding, so 924 of cards.
        let w = min(924 + 2 * 46, size.width)
        let sheet = CGRect(x: (size.width - w) / 2, y: 0, width: w, height: size.height)
        ctx.fill(Path(sheet), with: .color(theme.witchSheet))
        speckles(ctx, sheet)
        let line = theme.witchLine
        // The frame's two lines, between the corners.
        for (inset, gap) in [(CGFloat(10), CGFloat(66)), (16, 72)] {
            var p = Path()
            p.addRect(CGRect(x: sheet.minX + gap, y: inset - 0.5, width: w - 2 * gap, height: 1))
            p.addRect(CGRect(x: sheet.minX + gap, y: size.height - inset - 0.5, width: w - 2 * gap, height: 1))
            p.addRect(CGRect(x: sheet.minX + inset - 0.5, y: gap, width: 1, height: max(0, size.height - 2 * gap)))
            p.addRect(CGRect(x: sheet.maxX - inset - 0.5, y: gap, width: 1, height: max(0, size.height - 2 * gap)))
            ctx.fill(p, with: .color(line))
        }
        // A flourish in each corner, the top left one turned for the rest.
        let corners: [(CGFloat, CGFloat, CGFloat, CGFloat)] = [
            (sheet.minX + 6, 6, 1, 1), (sheet.maxX - 6, 6, -1, 1),
            (sheet.minX + 6, size.height - 6, 1, -1), (sheet.maxX - 6, size.height - 6, -1, -1),
        ]
        for (x, y, sx, sy) in corners {
            let t = CGAffineTransform(translationX: x, y: y).scaledBy(x: sx, y: sy)
            ctx.stroke(Self.flourish.applying(t), with: .color(line), style: StrokeStyle(lineWidth: 1.4, lineCap: .round))
            ctx.fill(Path(ellipseIn: CGRect(x: 20.2, y: 20.2, width: 3.6, height: 3.6)).applying(t), with: .color(line))
        }
    }

    /// flourish is the frame's top left corner, from look.css's SVG.
    private static let flourish: Path = {
        var p = Path()
        p.move(to: CGPoint(x: 4, y: 60)); p.addLine(to: CGPoint(x: 4, y: 12))
        p.addQuadCurve(to: CGPoint(x: 12, y: 4), control: CGPoint(x: 4, y: 4)); p.addLine(to: CGPoint(x: 60, y: 4))
        p.move(to: CGPoint(x: 10, y: 60)); p.addLine(to: CGPoint(x: 10, y: 18))
        p.addQuadCurve(to: CGPoint(x: 18, y: 10), control: CGPoint(x: 10, y: 10)); p.addLine(to: CGPoint(x: 60, y: 10))
        // The curl: c10 0 14 6 10 12, c-3 4 -9 2 -7 -3, c2 -4 8 -2 8 3.
        p.move(to: CGPoint(x: 18, y: 18))
        p.addCurve(to: CGPoint(x: 28, y: 30), control1: CGPoint(x: 28, y: 18), control2: CGPoint(x: 32, y: 24))
        p.addCurve(to: CGPoint(x: 21, y: 27), control1: CGPoint(x: 25, y: 34), control2: CGPoint(x: 19, y: 32))
        p.addCurve(to: CGPoint(x: 29, y: 30), control1: CGPoint(x: 23, y: 23), control2: CGPoint(x: 29, y: 25))
        p.move(to: CGPoint(x: 18, y: 18))
        p.addCurve(to: CGPoint(x: 30, y: 28), control1: CGPoint(x: 18, y: 28), control2: CGPoint(x: 24, y: 32))
        p.move(to: CGPoint(x: 30, y: 14))
        p.addCurve(to: CGPoint(x: 44, y: 14), control1: CGPoint(x: 36, y: 12), control2: CGPoint(x: 40, y: 16))
        p.move(to: CGPoint(x: 14, y: 30))
        p.addCurve(to: CGPoint(x: 14, y: 44), control1: CGPoint(x: 12, y: 36), control2: CGPoint(x: 16, y: 40))
        return p
    }()

    /// speckles is the kraft paper's grain: a 256-point tile of flecks,
    /// the same every time, repeated down the scroll.
    private func speckles(_ ctx: GraphicsContext, _ sheet: CGRect) {
        var c = ctx
        c.clip(to: Path(sheet))
        let tile: CGFloat = 256
        var p = Path()
        var y = sheet.minY
        while y < sheet.maxY {
            var x = sheet.minX
            while x < sheet.maxX {
                for f in Self.flecks { p.addRect(CGRect(x: x + f.x, y: y + f.y, width: f.r, height: f.r)) }
                x += tile
            }
            y += tile
        }
        c.fill(p, with: .color(theme.witchLine.opacity(theme.dark ? 0.35 : 0.28)))
    }

    private static let flecks: [(x: CGFloat, y: CGFloat, r: CGFloat)] = {
        var g = SeededRandom(seed: 4)
        let sizes: [CGFloat] = [1, 1, 1, 1.5, 2]
        var out: [(x: CGFloat, y: CGFloat, r: CGFloat)] = []
        for _ in 0..<170 {
            let x = CGFloat(g.next() % 256)
            let y = CGFloat(g.next() % 256)
            out.append((x: x, y: y, r: sizes[Int(g.next() % 5)]))
        }
        return out
    }()

    // The stars: two four-pointed ones and six dots, and two crescents.
    private func sky(_ ctx: GraphicsContext, _ size: CGSize) {
        let tile: CGFloat = 220
        var stars = Path()
        var moons = Path()
        var y: CGFloat = 0
        while y < size.height {
            var x: CGFloat = 0
            while x < size.width {
                for (cx, cy, r) in [(CGFloat(40), CGFloat(28), CGFloat(10)), (170, 139, 7)] {
                    stars.addPath(Self.star(r: r).applying(CGAffineTransform(translationX: x + cx, y: y + cy)))
                }
                for (cx, cy, r) in [(CGFloat(120), CGFloat(40), CGFloat(1.6)), (196, 70, 1.2), (70, 96, 1.4), (24, 170, 1.8), (132, 196, 1.2), (98, 150, 1)] {
                    stars.addEllipse(in: CGRect(x: x + cx - r, y: y + cy - r, width: 2 * r, height: 2 * r))
                }
                moons.addPath(Self.crescent(r: 13).applying(CGAffineTransform(translationX: x + 192, y: y + 33)))
                moons.addPath(Self.crescent(r: 8).applying(CGAffineTransform(translationX: x + 82, y: y + 183)))
                x += tile
            }
            y += tile
        }
        ctx.fill(stars, with: .color(theme.witchSky))
        ctx.fill(moons, with: .color(theme.witchMoon))
    }

    /// star is a four-pointed star of radius r around the origin.
    private static func star(r: CGFloat) -> Path {
        let k = r * 0.22
        var p = Path()
        p.move(to: CGPoint(x: 0, y: -r))
        p.addLine(to: CGPoint(x: k, y: -k)); p.addLine(to: CGPoint(x: r, y: 0)); p.addLine(to: CGPoint(x: k, y: k))
        p.addLine(to: CGPoint(x: 0, y: r)); p.addLine(to: CGPoint(x: -k, y: k)); p.addLine(to: CGPoint(x: -r, y: 0))
        p.addLine(to: CGPoint(x: -k, y: -k))
        p.closeSubpath()
        return p
    }

    /// crescent is a crescent moon of radius r around the origin: the
    /// disc less one moved toward its upper right.
    private static func crescent(r: CGFloat) -> Path {
        let disc = Path(ellipseIn: CGRect(x: -r, y: -r, width: 2 * r, height: 2 * r))
        let bite = Path(ellipseIn: CGRect(x: -r * 0.55, y: -r * 1.15, width: 2 * r * 0.85, height: 2 * r * 0.85))
        return disc.subtracting(bite)
    }

    // blueprint.css: --bp-minor and --bp-major.
    private func grid(_ ctx: GraphicsContext, _ size: CGSize) {
        let base = theme.dark ? theme.color("#8fd3ff") : Color.white
        let minor = base.opacity(theme.dark ? 0.07 : 0.09)
        let major = base.opacity(theme.dark ? 0.16 : 0.2)
        for (step, color) in [(CGFloat(20), minor), (CGFloat(100), major)] {
            var path = Path()
            var x: CGFloat = 0
            while x < size.width { path.addRect(CGRect(x: x, y: 0, width: 1, height: size.height)); x += step }
            var y: CGFloat = 0
            while y < size.height { path.addRect(CGRect(x: 0, y: y, width: size.width, height: 1)); y += step }
            ctx.fill(path, with: .color(color))
        }
    }
}

private struct ThemeKey: EnvironmentKey {
    static let defaultValue = Theme(settings: AppSettings(), systemDark: false)
}

extension EnvironmentValues {
    var theme: Theme {
        get { self[ThemeKey.self] }
        set { self[ThemeKey.self] = newValue }
    }
}

/// SeededRandom gives the same numbers for the same seed, for textures
/// drawn the same every time: xorshift64*.
private struct SeededRandom {
    var state: UInt64
    init(seed: UInt64) { state = seed &* 0x9E37_79B9_7F4A_7C15 | 1 }
    mutating func next() -> UInt64 {
        state ^= state >> 12
        state ^= state << 25
        state ^= state >> 27
        return state &* 0x2545_F491_4F6C_DD1D
    }
}
