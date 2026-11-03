import AppKit
import SwiftUI

/// CodeView draws a block's code with line numbers, highlighted.
struct CodeView: View {
    var code: String
    var lang: String
    var firstLine = 1
    @Environment(\.theme) private var theme

    var body: some View {
        let lines = code.split(separator: "\n", omittingEmptySubsequences: false)
        let shown = lines.last == "" ? Array(lines.dropLast()) : lines
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(alignment: .top, spacing: 10) {
                VStack(alignment: .trailing, spacing: 0) {
                    ForEach(shown.indices, id: \.self) { i in
                        Text("\(firstLine + i)")
                    }
                }
                .foregroundStyle(theme.muted.opacity(0.7))
                .accessibilityHidden(true)
                Text(Highlighter.highlight(shown.joined(separator: "\n"), lang: lang, theme: theme))
                    .textSelection(.enabled)
                    .fixedSize(horizontal: true, vertical: false)
            }
            .font(theme.codeFont())
            .lineSpacing(2)
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
        }
        .background(theme.codeBackground)
        .accessibilityLabel("Code")
        .accessibilityValue(code)
    }
}

/// Highlighter colors code a little, in the theme's syntax colors:
/// comments, strings, numbers, keywords and $variables. It is no parser;
/// it covers the common cases in the languages blocks are written in.
enum Highlighter {
    private static let shellKeywords: Set<String> = [
        "if", "then", "else", "elif", "fi", "for", "in", "do", "done", "while", "until", "case", "esac",
        "function", "return", "export", "local", "set", "unset", "echo", "cd", "exit", "source", "readonly",
    ]
    private static let cKeywords: Set<String> = [
        "func", "package", "import", "return", "if", "else", "for", "range", "var", "const", "type", "struct",
        "def", "class", "from", "with", "as", "let", "fn", "pub", "use", "mod", "impl", "while", "switch",
        "case", "default", "break", "continue", "true", "false", "nil", "None", "null", "self", "new", "try",
        "catch", "throw", "async", "await", "interface", "enum", "match", "in", "not", "and", "or", "lambda",
        "SELECT", "FROM", "WHERE", "INSERT", "UPDATE", "DELETE", "JOIN", "GROUP", "ORDER", "BY", "select", "from", "where",
    ]

    static func highlight(_ code: String, lang: String, theme: Theme) -> AttributedString {
        var out = AttributedString(code)
        out.foregroundColor = theme.text
        for (r, c) in spans(code, lang: lang, palette: SyntaxPalette(look: theme.look, dark: theme.dark)) {
            guard let sr = Range(r, in: code), let ar = Range(sr, in: out) else { continue }
            out[ar].foregroundColor = c
        }
        return out
    }

    /// spans are the colored ranges of code, in order: a later one wins
    /// over an earlier one it overlaps. The source view colors fenced
    /// code with them too.
    static func spans(_ code: String, lang: String, palette: SyntaxPalette) -> [(NSRange, Color)] {
        let shell = ["sh", "bash", "zsh", "shell", "console", ""].contains(lang.lowercased())
        let hashComments = shell || ["python", "py", "ruby", "rb", "perl", "yaml", "yml", "toml", "dockerfile", "ini"].contains(lang.lowercased())
        let keywords = shell ? shellKeywords : cKeywords
        let all = NSRange(location: 0, length: (code as NSString).length)
        var out: [(NSRange, Color)] = []

        func color(_ pattern: String, _ c: Color, options: NSRegularExpression.Options = []) {
            guard let re = try? NSRegularExpression(pattern: pattern, options: options) else { return }
            for m in re.matches(in: code, range: all) { out.append((m.range, c)) }
        }
        // Later passes win, so comments and strings come last.
        let words = keywords.map { NSRegularExpression.escapedPattern(for: $0) }.joined(separator: "|")
        color("\\b(\(words))\\b", palette.keyword)
        color("\\b\\d+(\\.\\d+)?\\b", palette.number)
        if shell { color("\\$\\{?[A-Za-z_][A-Za-z0-9_]*\\}?|\\$[0-9@#?*$!-]", palette.variable) }
        color("\"(\\\\.|[^\"\\\\])*\"|'[^'\\n]*'", palette.string)
        if hashComments {
            color("(^|\\s)#[^\\n]*", palette.comment, options: [.anchorsMatchLines])
        } else {
            color("//[^\\n]*|/\\*[\\s\\S]*?\\*/", palette.comment)
            if ["sql", "lua"].contains(lang.lowercased()) { color("--[^\\n]*", palette.comment) }
        }
        return out
    }

    /// SyntaxPalette is mdsh-gui's --syn-* colors.
    struct SyntaxPalette {
        var keyword, string, number, comment, variable: Color
        /// heading, link and punct color the source view's markdown.
        var heading, link, punct: Color
        init(look: Look, dark: Bool) {
            func c(_ h: String) -> Color { Color(nsColor: Theme.nsColor(h)) }
            let v: [String]  // keyword, string, number, comment, variable, heading, link, punct
            switch (look, dark) {
            case (.plain, true): v = ["#ff7b72", "#a5d6ff", "#79c0ff", "#9198a1", "#ffa657", "#79c0ff", "#a5d6ff", "#d1d7e0"]
            case (.plain, false): v = ["#cf222e", "#0a3069", "#0550ae", "#6e7781", "#953800", "#0550ae", "#0969da", "#424a53"]
            case (.tty, false): v = ["#8a2b1f", "#3d6b35", "#2f4f7a", "#8e887a", "#6b4a1c", "#2a241c", "#8a2b1f", "#4a4234"]
            case (.tty, true): v = ["#ffb000", "#c8ffc8", "#7fd4ff", "#538753", "#d6ff9c", "#9cff9c", "#ffb000", "#7fbf7f"]
            case (.blueprint, false): v = ["#ffd75e", "#b8f5c9", "#ffc49b", "#a9c3e6", "#e6f0ff", "#ffffff", "#ffd75e", "#d4e3f7"]
            case (.blueprint, true): v = ["#ffd75e", "#9fe8b6", "#ffb98a", "#6d8cb4", "#d6e8ff", "#ffffff", "#ffd75e", "#a9c3e6"]
            case (.gum, false): v = ["#c21d6a", "#1f5e9a", "#8a3d00", "#86587a", "#7d2d00", "#b8005f", "#b8005f", "#4a3340"]
            case (.gum, true): v = ["#ff7fbf", "#9fd3ff", "#ffb57f", "#b57a9c", "#ffc29a", "#ffffff", "#ff8fc6", "#e3b9cf"]
            }
            keyword = c(v[0]); string = c(v[1]); number = c(v[2]); comment = c(v[3]); variable = c(v[4])
            heading = c(v[5]); link = c(v[6]); punct = c(v[7])
        }
    }
}
