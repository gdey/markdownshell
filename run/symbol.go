package run

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/gdey/markdownshell/doc"
)

// defKeywords are the keywords that start a definition in the languages
// whose lexers don't mark the name being defined, as JavaScript's and
// shell's don't: the name after one of them is a definition.
var defKeywords = []string{
	"class", "const", "def", "enum", "fn", "func", "function",
	"interface", "let", "module", "struct", "sub", "trait", "type",
	"union", "var",
}

// outerKeywords also start the definition A in A.b: Rust's methods are
// in an impl block.
var outerKeywords = append([]string{"impl"}, defKeywords...)

// indentLangs are the lexers, by name, of languages that nest by
// indentation rather than braces.
var indentLangs = []string{"python", "python 2", "yaml", "coffeescript", "nim", "haskell"}

// closers are the lines that close a block in languages that end one
// with a word, kept with the block when they are as indented as its
// first line.
var closers = []string{"end", "}", "};", "fi", "done", "esac", "endfunction", "endif"}

// plainLang says lang names no language, so the file's name is the
// better guide.
func plainLang(lang string) bool {
	switch strings.ToLower(lang) {
	case "", "text", "txt", "plain", "plaintext", "output":
		return true
	}
	return false
}

// symLexer is the lexer for an include in lang of file, or nil.
func symLexer(lang, file string) chroma.Lexer {
	if !plainLang(lang) {
		if l := lexers.Get(lang); l != nil && l != lexers.Fallback {
			return l
		}
	}
	return lexers.Match(filepath.Base(file))
}

// tok is a token with the line it starts on, counting from 1.
type tok struct {
	chroma.Token
	line int
}

// symbolTokens is symbolLines for a language read with a Chroma lexer,
// best effort. The start is the first line where a token naming the
// symbol is a definition: one the lexer marks as a function's or a
// class's name, one after a keyword such as function or class, or, in
// shell, name() at the start of a line. The end is the brace that
// closes the first { after it, or for a language that nests by
// indentation the last line indented deeper than the start. A doc
// comment right above the start is kept unless comment=false, as are
// decorators and attributes.
//
// A.b is b defined inside A's definition.
func symbolTokens(data []byte, inc doc.Include, lang string) (start, end int, warn string) {
	l := symLexer(lang, inc.File)
	if l == nil {
		return 0, 0, noSymbol(inc) + " that symbol= can find: it has no lexer for " + inc.File
	}
	it, err := l.Tokenise(nil, string(data))
	if err != nil {
		return 0, 0, noSymbol(inc)
	}
	var toks []tok
	line := 1
	for _, t := range it.Tokens() {
		toks = append(toks, tok{t, line})
		line += strings.Count(t.Value, "\n")
	}
	lines := strings.Split(string(data), "\n")
	indent := slices.Contains(indentLangs, strings.ToLower(l.Config().Name))

	names := strings.Split(inc.Symbol, ".")
	// Each name is looked for inside each definition of the one before.
	defs := defsOf(toks, 0, len(toks), names[0], len(names) > 1)
	for k, name := range names[1:] {
		var inner []int
		for _, d := range defs {
			last := blockEnd(toks, d, lines, indent)
			to := d + 1
			for to < len(toks) && toks[to].line <= last {
				to++
			}
			inner = append(inner, defsOf(toks, d+1, to, name, k+2 < len(names))...)
		}
		defs = inner
	}
	if len(defs) == 0 {
		return 0, 0, noSymbol(inc)
	}
	if len(defs) > 1 {
		warn = manySymbols(inc, len(defs))
	}
	start = toks[defs[0]].line
	end = blockEnd(toks, defs[0], lines, indent)
	return above(toks, start, inc.NoComment), end, warn
}

// defsOf is the index in ts, from to to, of each token that defines
// name. outer accepts the keywords of outerKeywords too.
func defsOf(ts []tok, from, to int, name string, outer bool) []int {
	keywords := defKeywords
	if outer {
		keywords = outerKeywords
	}
	var defs []int
	for i := from; i < to; i++ {
		t := ts[i]
		if t.Value != name {
			continue
		}
		switch {
		case t.Type == chroma.NameFunction, t.Type == chroma.NameClass:
		case t.Type == chroma.NameTag && lineStart(ts, i):
		case afterKeyword(ts, i, keywords):
		case lineStart(ts, i) && shellFunc(ts, i):
		default:
			continue
		}
		defs = append(defs, i)
	}
	return defs
}

// space says t is only white space.
func space(t tok) bool {
	return strings.TrimSpace(t.Value) == "" && (t.Type.InCategory(chroma.Text) || t.Type == chroma.TextWhitespace)
}

// prev is the index of the token before i that isn't white space on
// the same line, or -1.
func prev(ts []tok, i int) int {
	for j := i - 1; j >= 0; j-- {
		if ts[j].line != ts[i].line && strings.Contains(ts[j].Value, "\n") {
			return -1
		}
		if !space(ts[j]) {
			return j
		}
	}
	return -1
}

func lineStart(ts []tok, i int) bool { return prev(ts, i) < 0 }

func afterKeyword(ts []tok, i int, keywords []string) bool {
	j := prev(ts, i)
	return j >= 0 && ts[j].Type.InCategory(chroma.Keyword) && slices.Contains(keywords, ts[j].Value)
}

// shellFunc says ts[i] is followed by ( ) and {, as a shell function's
// name is.
func shellFunc(ts []tok, i int) bool {
	var rest strings.Builder
	for _, t := range ts[i+1:] {
		rest.WriteString(t.Value)
		if rest.Len() > 16 {
			break
		}
	}
	s := strings.Join(strings.Fields(rest.String()), "")
	return strings.HasPrefix(s, "(){")
}

// punct says t can open or close a block: strings and comments can't.
func punct(t tok) bool {
	return t.Type.InCategory(chroma.Punctuation) || t.Type.InCategory(chroma.Operator) ||
		(t.Type == chroma.Text && strings.TrimSpace(t.Value) != "")
}

// blockEnd is the last line of the definition at ts[i].
func blockEnd(ts []tok, i int, lines []string, indent bool) int {
	if indent {
		return indentEnd(lines, ts[i].line, headEnd(ts, i))
	}
	parens := 0
	for j := i; j < len(ts); j++ {
		t := ts[j]
		for _, c := range t.Value {
			if !punct(t) {
				break
			}
			switch c {
			case '(', '[':
				parens++
			case ')', ']':
				parens--
			case ';':
				if parens <= 0 {
					return t.line
				}
			case '{':
				if parens <= 0 {
					return braceEnd(ts, j)
				}
			}
		}
		// A body starts on the definition's line or, Allman style, the
		// line after its head; past that there's no brace to find, and
		// the block is what is indented under it.
		if parens <= 0 && strings.Contains(t.Value, "\n") {
			if text := firstText(ts, j+1); !strings.HasPrefix(text, "{") {
				return indentEnd(lines, ts[i].line, ts[i].line)
			}
		}
	}
	return ts[i].line
}

// firstText is the first non-blank text from ts[j] on.
func firstText(ts []tok, j int) string {
	for ; j < len(ts); j++ {
		if s := strings.TrimSpace(ts[j].Value); s != "" {
			return s
		}
	}
	return ""
}

// braceEnd is the line of the brace that closes the { in ts[j].
func braceEnd(ts []tok, j int) int {
	depth := 0
	for ; j < len(ts); j++ {
		if !punct(ts[j]) {
			continue
		}
		line := ts[j].line
		for _, c := range ts[j].Value {
			switch c {
			case '\n':
				line++
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return line
				}
			}
		}
	}
	return ts[len(ts)-1].line
}

// headEnd is the line a definition's head ends on: where the brackets
// opened from ts[i] on are closed again.
func headEnd(ts []tok, i int) int {
	depth := 0
	for j := i; j < len(ts); j++ {
		t := ts[j]
		if punct(t) {
			for _, c := range t.Value {
				switch c {
				case '(', '[', '{':
					depth++
				case ')', ']', '}':
					depth--
				}
			}
		}
		if depth <= 0 && strings.Contains(t.Value, "\n") {
			return t.line
		}
	}
	return ts[len(ts)-1].line
}

// indentOf is the width of line's leading white space, and false for a
// blank line.
func indentOf(line string) (int, bool) {
	t := strings.TrimLeft(line, " \t")
	if strings.TrimSpace(t) == "" {
		return 0, false
	}
	return len(line) - len(t), true
}

// indentEnd is the last line after head, counting from 1, indented
// deeper than line start, past blank lines; with a closing word such as
// end as indented as start kept.
func indentEnd(lines []string, start, head int) int {
	base, _ := indentOf(lines[start-1])
	end := head
	for n := head + 1; n <= len(lines); n++ {
		w, ok := indentOf(lines[n-1])
		if !ok {
			continue
		}
		if w <= base {
			if w == base && slices.Contains(closers, strings.TrimSpace(lines[n-1])) {
				end = n
			}
			break
		}
		end = n
	}
	return end
}

// above is the first line of the definition starting at line start,
// with the comments, decorators and attributes right above it; comments
// left out when noComment is set.
func above(ts []tok, start int, noComment bool) int {
	byLine := map[int][]tok{}
	for _, t := range ts {
		if t.line >= start || space(t) {
			continue
		}
		// A block comment is on each line it spans.
		for n := t.line; n <= t.line+strings.Count(strings.TrimRight(t.Value, "\n"), "\n") && n < start; n++ {
			byLine[n] = append(byLine[n], t)
		}
	}
	first := start
	for n := start - 1; n >= 1; n-- {
		lt := byLine[n]
		if len(lt) == 0 {
			break
		}
		t := lt[0].Type
		switch {
		case t.InSubCategory(chroma.NameDecorator), t == chroma.CommentPreproc, t == chroma.NameAttribute && lt[0].Value != "":
		case allComment(lt):
			if noComment {
				return first
			}
		default:
			return first
		}
		first = n
	}
	return first
}

func allComment(ts []tok) bool {
	for _, t := range ts {
		if !t.Type.InCategory(chroma.Comment) && t.Type != chroma.LiteralStringDoc {
			return false
		}
	}
	return true
}
