package termrec

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Text is what rec wrote, as plain text that reads the way the terminal
// showed it: escape sequences are dropped, a carriage return starts its
// line over, a backspace steps back and erasing to the end of the line
// erases. Trailing spaces and blank lines at the end are left off.
func (rec Recording) Text() string {
	var lines []string
	var line []rune
	col := 0
	put := func(r rune) {
		if col < len(line) {
			line[col] = r
		} else {
			for len(line) < col {
				line = append(line, ' ')
			}
			line = append(line, r)
		}
		col++
	}
	data := rec.bytes()
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == 0x1b:
			n, final, params := escape(data[i:])
			if final == 'K' && (params == "" || params == "0") {
				line = line[:min(col, len(line))]
			}
			i += n
			continue
		case c == '\r':
			col = 0
		case c == '\n':
			lines = append(lines, strings.TrimRight(string(line), " "))
			line, col = nil, 0
		case c == '\b':
			col = max(col-1, 0)
		case c == '\t':
			put('\t')
		case c < 0x20 || c == 0x7f:
		default:
			r, n := utf8.DecodeRune(data[i:])
			put(r)
			i += n
			continue
		}
		i++
	}
	lines = append(lines, strings.TrimRight(string(line), " "))
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// FullScreen reports whether rec switched to the terminal's alternate
// screen, as full-screen programs such as vim, less and top do.
func (rec Recording) FullScreen() bool {
	data := rec.bytes()
	for _, s := range []string{"\x1b[?1049h", "\x1b[?1047h", "\x1b[?47h"} {
		if bytes.Contains(data, []byte(s)) {
			return true
		}
	}
	return false
}

func (rec Recording) bytes() []byte {
	var b []byte
	for _, e := range rec.Events {
		b = append(b, e.Data...)
	}
	return b
}

// escape measures the escape sequence data starts with. For a control
// sequence (ESC [) it also returns its final byte and its parameters.
func escape(data []byte) (n int, final byte, params string) {
	if len(data) < 2 {
		return len(data), 0, ""
	}
	switch data[1] {
	case '[':
		for j := 2; j < len(data); j++ {
			if c := data[j]; c >= 0x40 && c <= 0x7e {
				return j + 1, c, string(data[2:j])
			}
		}
		return len(data), 0, ""
	case ']', 'P', '_', '^':
		// Strings, such as a window title, end with BEL or ESC \.
		for j := 2; j < len(data); j++ {
			if data[j] == 0x07 {
				return j + 1, 0, ""
			}
			if data[j] == 0x1b && j+1 < len(data) && data[j+1] == '\\' {
				return j + 2, 0, ""
			}
		}
		return len(data), 0, ""
	case '(', ')', '*', '+', '#', '%':
		return min(3, len(data)), 0, ""
	}
	return 2, 0, ""
}
