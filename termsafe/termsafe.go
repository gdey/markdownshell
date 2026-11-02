// Package termsafe makes text from a document safe to show on a
// terminal by turning the characters a terminal acts on into text.
//
// A block's code is shown so its reader can decide whether to run it.
// Escape sequences and other control characters in it would be acted on
// instead of shown: "\r\x1b[2K" at the end of a line erases what came
// before it, so code could run that its reader never saw. Visible shows
// such characters instead, so what is on the screen is what runs.
package termsafe

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Visible returns s with every control character other than newline and
// tab replaced by a visible stand-in: C0 controls in caret notation (ESC
// as ^[, CR as ^M), DEL as ^?, and as <U+XXXX> the C1 controls, the
// Unicode direction overrides that can reorder how a line reads, and the
// zero-width characters that can hide inside a word. Text without such
// characters is returned as it is. A byte that isn't valid UTF-8 shows
// as <0xXX>, since a lone 0x9b is a CSI to some terminals.
func Visible(s string) string {
	if !HasControl(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&b, "<0x%02X>", s[0])
			s = s[1:]
			continue
		}
		s = s[n:]
		switch {
		case !isControl(r):
			b.WriteRune(r)
		case r < 0x20:
			b.WriteByte('^')
			b.WriteByte(byte(r) + '@')
		case r == 0x7f:
			b.WriteString("^?")
		default:
			fmt.Fprintf(&b, "<U+%04X>", r)
		}
	}
	return b.String()
}

// HasControl reports whether s has a character Visible would replace.
func HasControl(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if isControl(r) {
			return true
		}
	}
	return false
}

func isControl(r rune) bool {
	switch {
	case r == '\n', r == '\t':
		return false
	case r < 0x20, r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		// C1 controls; 0x9b is a one-byte CSI on some terminals.
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		// Bidirectional embeddings, overrides and isolates.
		return true
	case r >= 0x200b && r <= 0x200d, r == 0x2060, r == 0xfeff:
		// Zero-width space, joiners, word joiner and BOM, which make
		// two strings that look the same differ.
		return true
	}
	return false
}
