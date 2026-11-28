// Package copyable finds the text a block's output offers to copy.
//
// A program can offer text for the clipboard two ways. iTerm2's
// OSC 1337 CopyToClipboard and EndCopy bracket text it prints: what
// comes between them is shown as usual and is also the text to copy.
// OSC 52 carries the text in the sequence, base64 encoded, and nothing
// is shown. A terminal writes the clipboard on either. mdsh never
// writes the clipboard from output, which is saved in the file and run
// again by whoever opens it, so it offers each such text with a copy
// button, or a key in the TUI, instead.
package copyable

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

// MaxText caps the text one OSC 52 sequence can offer, as a guard
// against output made to be huge.
const MaxText = 1 << 20

// A Part is a piece of output: text as it was printed, escapes and all,
// or a sequence that starts or ends a copyable region or offers text.
type Part struct {
	Kind Kind
	// Text is the output, for Plain, and the text offered, for Hidden.
	Text string
	// N numbers the copyable texts in the output from 1, for Open,
	// Close and Hidden.
	N int
}

// Kind is what a Part is.
type Kind int

const (
	Plain  Kind = iota
	Open        // OSC 1337 CopyToClipboard: the shown text after it, to Close, is copyable
	Close       // OSC 1337 EndCopy, or the end of the output with a region open
	Hidden      // OSC 52: Text is offered but isn't shown
)

// Split cuts s, a block's output, into parts. Its other escape
// sequences stay in the Plain parts, for the caller to treat as it
// does any output. Output with no copyable text is one Plain part.
func Split(s string) []Part {
	var parts []Part
	n, open := 0, 0
	plain := func(t string) {
		if t == "" {
			return
		}
		if k := len(parts) - 1; k >= 0 && parts[k].Kind == Plain {
			parts[k].Text += t
			return
		}
		parts = append(parts, Part{Kind: Plain, Text: t})
	}
	for {
		i := strings.Index(s, "\x1b]")
		if i < 0 {
			plain(s)
			break
		}
		body, end, ok := osc(s[i:])
		if !ok {
			// Unterminated: the rest is output as it is.
			plain(s)
			break
		}
		plain(s[:i])
		switch {
		case strings.HasPrefix(body, "1337;CopyToClipboard"):
			if open != 0 {
				parts = append(parts, Part{Kind: Close, N: open})
			}
			n++
			open = n
			parts = append(parts, Part{Kind: Open, N: n})
		case body == "1337;EndCopy":
			if open != 0 {
				parts = append(parts, Part{Kind: Close, N: open})
				open = 0
			}
		case strings.HasPrefix(body, "52;"):
			if t, ok := clip(body); ok {
				n++
				parts = append(parts, Part{Kind: Hidden, Text: t, N: n})
			}
		default:
			plain(s[i : i+end])
		}
		s = s[i+end:]
	}
	if open != 0 {
		parts = append(parts, Part{Kind: Close, N: open})
	}
	return parts
}

// Texts is the copyable texts in s, in order: a region's text as shown,
// without its escape sequences and control characters, and each OSC 52
// text.
func Texts(s string) []string {
	var out []string
	var cur *strings.Builder
	for _, p := range Split(s) {
		switch p.Kind {
		case Open:
			cur = &strings.Builder{}
		case Plain:
			if cur != nil {
				cur.WriteString(p.Text)
			}
		case Close:
			if cur != nil {
				out = append(out, Shown(cur.String()))
				cur = nil
			}
		case Hidden:
			out = append(out, p.Text)
		}
	}
	return out
}

// Shown is s as a terminal shows it, near enough for copying: without
// escape sequences and control characters but newlines and tabs, and
// without a carriage return before a newline.
func Shown(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		c := s[0]
		switch {
		case c == 0x1b:
			s = s[seqLen(s):]
			continue
		case c == '\n', c == '\t', c >= 0x20 && c != 0x7f:
			b.WriteByte(c)
		}
		s = s[1:]
	}
	return b.String()
}

// seqLen is the length of the escape sequence s starts with (s[0] is
// ESC): at least 1.
func seqLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return len(s)
	case ']', 'P', '_', '^', 'X':
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	}
	return 2
}

// Has reports whether s may offer copyable text, cheaply.
func Has(s string) bool {
	return strings.Contains(s, "\x1b]1337;CopyToClipboard") || strings.Contains(s, "\x1b]52;")
}

// osc reads the OSC sequence s starts with: its body, between ESC ] and
// BEL or ST, and its length.
func osc(s string) (body string, n int, ok bool) {
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case 0x07:
			return s[2:i], i + 1, true
		case 0x1b:
			if i+1 < len(s) && s[i+1] == '\\' {
				return s[2:i], i + 2, true
			}
			return "", 0, false
		}
	}
	return "", 0, false
}

// clip is the text an OSC 52 body ("52;c;base64") sets the clipboard
// to. A query (?), an empty text, which clears the clipboard, and text
// that isn't base64 of UTF-8 offer nothing.
func clip(body string) (string, bool) {
	parts := strings.SplitN(body, ";", 3)
	if len(parts) != 3 || parts[2] == "" || parts[2] == "?" || len(parts[2]) > base64.StdEncoding.EncodedLen(MaxText) {
		return "", false
	}
	data := strings.TrimRight(parts[2], "=")
	b, err := base64.RawStdEncoding.DecodeString(data)
	if err != nil || len(b) == 0 || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}
