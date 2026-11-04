// Package weblink finds web addresses in a block's output, for blocks
// with the links attribute, so the viewers can make them clickable.
//
// Only http:, https: and mailto: addresses count, so following one opens
// the browser or the mail app and never runs or opens a local file.
package weblink

import (
	"regexp"
	"strings"
)

// Attr is the block attribute that turns links on in its output.
const Attr = "links"

// address matches a web address: http:// or https:// and a host, or
// mailto: and an address. It stops at space, control characters, quotes,
// angle brackets and backslashes, which can't be in one unescaped and
// would end an HTML attribute, a CSS string or a terminal's OSC 8.
var address = regexp.MustCompile(`(?i)(?:https?://[^\s\x00-\x1f\x7f"'<>\\` + "`" + `]+|mailto:[^\s\x00-\x1f\x7f"'<>\\` + "`" + `@]+@[^\s\x00-\x1f\x7f"'<>\\` + "`" + `]+)`)

// Find returns the start and end of each web address in s.
func Find(s string) [][2]int {
	var spans [][2]int
	for _, m := range address.FindAllStringIndex(s, -1) {
		end := m[0] + trim(s[m[0]:m[1]])
		if end > m[0] && host(s[m[0]:end]) && starts(s[:m[0]]) {
			spans = append(spans, [2]int{m[0], end})
		}
	}
	return spans
}

// starts reports whether an address may start after before: not in
// the middle of a word, as in nohttps://, but right after a color
// escape.
func starts(before string) bool {
	if before == "" || csiEnd.MatchString(before) {
		return true
	}
	c := before[len(before)-1]
	return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c >= 0x80)
}

// csiEnd matches a CSI escape sequence at the end of the text.
var csiEnd = regexp.MustCompile(`\x1b\[[0-9;:?]*[ -/]*[@-~]$`)

// host reports whether an http(s) address has a host after its //.
func host(u string) bool {
	_, rest, ok := strings.Cut(u, "://")
	return !ok || rest != "" && rest[0] != '/'
}

// trim is how much of u is the address: punctuation that ends a sentence
// is left off, and so is a closing bracket without its opening one, as
// in (see https://example.com).
func trim(u string) int {
	for len(u) > 0 {
		last := u[len(u)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?':
			u = u[:len(u)-1]
			continue
		case ')', ']', '}':
			open := map[byte]byte{')': '(', ']': '[', '}': '{'}[last]
			if strings.Count(u, string(open)) < strings.Count(u, string(last)) {
				u = u[:len(u)-1]
				continue
			}
		}
		break
	}
	return len(u)
}

// OSC8 wraps each web address in s in an OSC 8 hyperlink, which a
// terminal that knows them makes clickable and others ignore. Text that
// already has OSC sequences, which may be links of its own, is left as
// it is.
func OSC8(s string) string {
	if strings.Contains(s, "\x1b]") {
		return s
	}
	spans := Find(s)
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		u := s[sp[0]:sp[1]]
		b.WriteString(s[at:sp[0]])
		b.WriteString("\x1b]8;;" + u + "\x1b\\" + u + "\x1b]8;;\x1b\\")
		at = sp[1]
	}
	b.WriteString(s[at:])
	return b.String()
}
