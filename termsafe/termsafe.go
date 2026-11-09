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

// Styled is s for printing to a terminal as document text or output:
// SGR sequences (ESC [ params m, which only set colours and the like)
// are kept, so coloured output stays coloured, and every other control
// character is shown as Visible shows it, so a saved output or a line
// of prose can't write the clipboard (OSC 52), set the window title,
// make a link whose target isn't its text (OSC 8), or move the cursor
// to hide text. A newline, a tab and a carriage return just before a
// newline (CRLF line ends) pass as they are. Unlike Visible it leaves
// zero-width and bidi characters alone: they don't drive the terminal,
// and prose and output use them (emoji joiners, right-to-left text).
func Styled(s string) string {
	if !hasC0C1(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for len(s) > 0 {
		if n := sgrLen(s); n > 0 {
			b.WriteString(s[:n])
			s = s[n:]
			continue
		}
		r, n := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, "<0x%02X>", s[0])
		case r == '\r' && strings.HasPrefix(s[1:], "\n"), !isC0C1(r):
			b.WriteString(s[:n])
		default:
			b.WriteString(Visible(s[:n]))
		}
		s = s[n:]
	}
	return b.String()
}

// sgrLen is the length of the SGR sequence s starts with, or 0.
func sgrLen(s string) int {
	if !strings.HasPrefix(s, "\x1b[") {
		return 0
	}
	for i := 2; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9', c == ';', c == ':':
		case c == 'm':
			return i + 1
		default:
			return 0
		}
	}
	return 0
}

// hasC0C1 reports whether s holds a character Styled changes.
func hasC0C1(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if isC0C1(r) {
			return true
		}
	}
	return false
}

// isC0C1 reports whether r is a control character that drives a
// terminal: C0 but newline and tab, DEL, and C1.
func isC0C1(r rune) bool {
	return r != '\n' && r != '\t' && (r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f)
}

// SGROnly is s with every control character and escape sequence taken
// out but SGR sequences (colours), newlines and tabs: for text a
// terminal should draw rather than obey, where showing the sequences
// would only be noise, such as a shell prompt, which commonly carries
// shell-integration marks (OSC 133) and title sets. A CSI sequence goes
// up to its final byte and an OSC, DCS, APC, PM or SOS string up to BEL
// or ST; an unterminated one runs to the end of s.
func SGROnly(s string) string {
	if !hasC0C1(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		if n := sgrLen(s); n > 0 {
			b.WriteString(s[:n])
			s = s[n:]
			continue
		}
		if s[0] == 0x1b {
			s = s[seqLen(s):]
			continue
		}
		r, n := utf8.DecodeRuneInString(s)
		if !(r == utf8.RuneError && n == 1) && !isC0C1(r) {
			b.WriteString(s[:n])
		}
		s = s[n:]
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
