package env

import (
	"bytes"
	"strconv"
	"strings"
)

// Shell integration marks (OSC 133, the FinalTerm scheme): ESC ] 133 ;
// kind [; fields] BEL, or ST (ESC \) in place of BEL. mdsh's own carry
// "mdsh" as their last field, and only those count; marks a user's rc
// files print themselves, such as iTerm2's or p10k's, are passed on
// and otherwise ignored.
const (
	markPrompt  = 'A' // prompt starts
	markInput   = 'B' // prompt ends, input starts
	markCommand = 'C' // command output starts
	markDone    = 'D' // command ended; D;status
)

// mark is one OSC 133 mark mdsh printed.
type mark struct {
	kind   byte
	status int // D only; -1 if it had none
}

// markScanner finds mdsh's marks in what the shell writes to its
// terminal. Write takes the stream in pieces as read; a mark split
// across two pieces is held back until it is whole.
type markScanner struct {
	held []byte
	// text is called with each run of bytes between marks, marks of
	// other programs included; mark with each mark of mdsh's.
	text func([]byte)
	mark func(mark)
}

var oscStart = []byte("\x1b]133;")

// maxMark bounds a mark's length, so a stray ESC ] 133 ; with no end
// can't hold the stream back for good.
const maxMark = 64

func (m *markScanner) Write(p []byte) {
	buf := p
	if len(m.held) > 0 {
		buf = append(m.held, p...)
		m.held = nil
	}
	for len(buf) > 0 {
		i := bytes.IndexByte(buf, 0x1b)
		if i < 0 {
			m.text(buf)
			return
		}
		if i > 0 {
			m.text(buf[:i])
			buf = buf[i:]
		}
		// buf starts with ESC.
		if len(buf) < len(oscStart) {
			if bytes.HasPrefix(oscStart, buf) {
				m.held = append([]byte(nil), buf...)
				return
			}
			m.text(buf[:1])
			buf = buf[1:]
			continue
		}
		if !bytes.HasPrefix(buf, oscStart) {
			m.text(buf[:1])
			buf = buf[1:]
			continue
		}
		end, n := oscEnd(buf[len(oscStart):])
		if end < 0 {
			if len(buf) < maxMark {
				m.held = append([]byte(nil), buf...)
				return
			}
			m.text(buf[:1])
			buf = buf[1:]
			continue
		}
		body := string(buf[len(oscStart) : len(oscStart)+end])
		whole := buf[:len(oscStart)+end+n]
		buf = buf[len(whole):]
		if mk, ok := parseMark(body); ok {
			m.mark(mk)
		} else {
			m.text(whole)
		}
	}
}

// oscEnd finds the end of an OSC's body: the index of BEL or ST and its
// length, or -1.
func oscEnd(b []byte) (int, int) {
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case 0x07:
			return i, 1
		case 0x1b:
			if i+1 < len(b) {
				if b[i+1] == '\\' {
					return i, 2
				}
				return -1, 0 // a new escape: not an OSC 133 after all
			}
		}
	}
	return -1, 0
}

// parseMark reads the body of an OSC 133 mark, such as "D;0;mdsh". Only
// mdsh's own marks are taken.
func parseMark(body string) (mark, bool) {
	f := strings.Split(body, ";")
	if len(f) < 2 || f[len(f)-1] != "mdsh" || len(f[0]) != 1 {
		return mark{}, false
	}
	mk := mark{kind: f[0][0], status: -1}
	switch mk.kind {
	case markPrompt, markInput, markCommand:
	case markDone:
		if len(f) == 3 {
			if n, err := strconv.Atoi(f[1]); err == nil {
				mk.status = n
			}
		}
	default:
		return mark{}, false
	}
	return mk, true
}

// stripEsc takes CSI and OSC escape sequences, and other two-byte
// escapes, out of s, leaving its text.
func stripEsc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '[':
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
		case ']':
			j := i + 2
			for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
				j++
			}
			if j < len(s) && s[j] == 0x1b {
				j++
			}
			i = j
		default:
			i++
		}
	}
	return b.String()
}
