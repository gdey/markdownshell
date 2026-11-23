package env

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// typedLine is the command line a line editor showed, from what it
// drew: prompt is the prompt's last line, as the shell drew it, and
// echo what came after it until the line ran. Both are drawn on a
// terminal cols wide, so that a line that wrapped past the edge, which
// bash's readline draws by writing the character after the edge, a
// carriage return and that character again, and one edited with
// backspace or the cursor keys read as the line itself. What the prompt
// covers is left out.
func typedLine(prompt, echo string, cols int) string {
	if cols <= 0 {
		cols = 80
	}
	var t lineTerm
	t.cols = cols
	t.draw(prompt)
	py, px := t.y, t.x
	if t.pending {
		py, px = py+1, 0
	}
	t.draw(echo)
	var b strings.Builder
	for y := py; y < len(t.rows); y++ {
		row := t.rows[y]
		if y > py && !row.soft {
			b.WriteByte('\n')
		}
		cells, width := row.cells, cols
		if y == py {
			cells, width = cells[min(px, len(cells)):], cols-px
		}
		if y+1 < len(t.rows) && t.rows[y+1].soft {
			// The line went on past the edge: the cells up to it
			// are part of it, blanks too.
			for len(cells) < width {
				cells = append(cells, ' ')
			}
		}
		for _, r := range cells {
			if r != 0 {
				b.WriteRune(r)
			}
		}
	}
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// lineTerm is as much of a terminal as a line editor's drawing of one
// line needs: text, autowrap, carriage return, backspace, and the
// cursor and erase sequences readline, zle and libedit use.
type lineTerm struct {
	cols    int
	rows    []lineRow
	x, y    int
	pending bool // the cursor is past the last column, as xterm's is
}

type lineRow struct {
	cells []rune // 0 is the second half of a wide character
	soft  bool   // the row above wrapped onto it
}

func (t *lineTerm) row() *lineRow {
	for len(t.rows) <= t.y {
		t.rows = append(t.rows, lineRow{})
	}
	return &t.rows[t.y]
}

func (t *lineTerm) set(x int, r rune) {
	row := t.row()
	for len(row.cells) <= x {
		row.cells = append(row.cells, ' ')
	}
	row.cells[x] = r
}

func (t *lineTerm) put(r rune) {
	w := runewidth.RuneWidth(r)
	if w == 0 {
		return
	}
	if t.pending || t.x+w > t.cols {
		t.y++
		t.x = 0
		t.pending = false
		t.row().soft = true
	}
	t.set(t.x, r)
	if w == 2 {
		t.set(t.x+1, 0)
	}
	t.x += w
	if t.x >= t.cols {
		t.x = t.cols - 1
		t.pending = true
	}
}

func (t *lineTerm) draw(s string) {
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b:
			i = t.escape(s, i)
			continue
		case c == '\r':
			t.x, t.pending = 0, false
		case c == '\n':
			t.y++
			t.pending = false
			t.row()
		case c == '\b':
			if t.pending {
				t.pending = false
			} else if t.x > 0 {
				t.x--
			}
		case c == '\t':
			t.x = min((t.x/8+1)*8, t.cols-1)
		case c < 0x20 || c == 0x7f:
		default:
			r, n := utf8.DecodeRuneInString(s[i:])
			t.put(r)
			i += n
			continue
		}
		i++
	}
}

// escape follows the escape sequence at s[i] and returns where the
// text after it starts.
func (t *lineTerm) escape(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j >= len(s) {
			return len(s)
		}
		t.csi(s[i+2:j], s[j])
		return j + 1
	case ']':
		j := i + 2
		for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
			j++
		}
		if j < len(s) && s[j] == 0x1b {
			j++
		}
		return j + 1
	}
	return i + 2
}

func (t *lineTerm) csi(params string, final byte) {
	if strings.HasPrefix(params, "?") {
		return // modes, such as bracketed paste
	}
	n, _ := strconv.Atoi(strings.SplitN(params, ";", 2)[0])
	count := max(n, 1)
	switch final {
	case 'C':
		t.x = min(t.x+count, t.cols-1)
		t.pending = false
	case 'D':
		if t.pending {
			count--
		}
		t.x = max(t.x-count, 0)
		t.pending = false
	case 'A':
		t.y = max(t.y-count, 0)
		t.pending = false
	case 'B':
		t.y += count
		t.row()
		t.pending = false
	case 'G':
		t.x = min(count-1, t.cols-1)
		t.pending = false
	case 'K':
		row := t.row()
		switch n {
		case 0:
			if t.x < len(row.cells) {
				row.cells = row.cells[:t.x]
			}
		case 1:
			for x := 0; x <= t.x && x < len(row.cells); x++ {
				row.cells[x] = ' '
			}
		case 2:
			row.cells = nil
		}
	case 'J':
		if n == 0 {
			row := t.row()
			if t.x < len(row.cells) {
				row.cells = row.cells[:t.x]
			}
			t.rows = t.rows[:t.y+1]
		}
	case 'P':
		row := t.row()
		if t.x < len(row.cells) {
			row.cells = append(row.cells[:t.x], row.cells[min(t.x+count, len(row.cells)):]...)
		}
	case '@':
		row := t.row()
		if t.x < len(row.cells) {
			ins := []rune(strings.Repeat(" ", count))
			row.cells = append(row.cells[:t.x], append(ins, row.cells[t.x:]...)...)
			if len(row.cells) > t.cols {
				row.cells = row.cells[:t.cols]
			}
		}
	}
}
