package env

import (
	"strconv"
	"strings"
)

// A tty block run by RunTTY writes to the user's own terminal, so it can
// retitle it or repaint its colours, and those last after the block. As
// tmux does for a pane, RunTTY puts them back: the title is saved on
// xterm's title stack before the block runs and restored after it, and
// each colour the block set is reset to the terminal's default.

const (
	// titlePush saves the window and icon titles on xterm's title stack
	// (CSI 22;0 t), and titlePop restores them (CSI 23;0 t). Terminals
	// without the stack ignore both.
	titlePush = "\x1b[22;0t"
	titlePop  = "\x1b[23;0t"
)

// maxOSC is the most of an OSC's text colourWatch keeps: enough for any
// colour, which is all it reads.
const maxOSC = 512

// colourWatch watches what a block writes to the terminal for OSCs that
// set a colour: OSC 4 and 5 (palette and special colours) and OSC 10 to
// 19 (foreground, background, cursor and the rest). Queries ("?") change
// nothing, so they don't count: programs ask for the background to pick
// light or dark styles, and resetting a colour the user set on purpose
// would undo it for nothing.
type colourWatch struct {
	state int
	buf   []byte
	// set are the OSC numbers of the colours set, in the order first set.
	set []int
}

const (
	cwGround = iota
	cwEsc    // after ESC
	cwOSC    // in an OSC's text
	cwOSCEsc // after ESC in an OSC, which ESC \ ends
)

func (w *colourWatch) Write(b []byte) (int, error) {
	for _, c := range b {
		switch w.state {
		case cwGround:
			if c == 0x1b {
				w.state = cwEsc
			}
		case cwEsc:
			switch c {
			case ']':
				w.state, w.buf = cwOSC, w.buf[:0]
			case 0x1b:
			default:
				w.state = cwGround
			}
		case cwOSC:
			switch {
			case c == 0x07:
				w.osc()
				w.state = cwGround
			case c == 0x1b:
				w.state = cwOSCEsc
			case c == 0x18 || c == 0x1a:
				// CAN and SUB cancel it.
				w.state = cwGround
			case len(w.buf) < maxOSC:
				w.buf = append(w.buf, c)
			}
		case cwOSCEsc:
			// ESC \ ends the OSC; any other ESC cancels it and starts
			// a new escape.
			if c == '\\' {
				w.osc()
				w.state = cwGround
				break
			}
			switch c {
			case ']':
				w.state, w.buf = cwOSC, w.buf[:0]
			case 0x1b:
				w.state = cwEsc
			default:
				w.state = cwGround
			}
		}
	}
	return len(b), nil
}

// osc notes the colours the OSC in buf sets.
func (w *colourWatch) osc() {
	f := strings.Split(string(w.buf), ";")
	ps, err := strconv.Atoi(f[0])
	if err != nil {
		return
	}
	switch {
	case ps == 4 || ps == 5:
		// Pairs of entry and colour: OSC 4 ; 1 ; red ; 2 ; ? sets
		// entry 1 and asks about entry 2.
		for i := 2; i < len(f); i += 2 {
			if f[i] != "?" {
				w.note(ps)
				return
			}
		}
	case ps >= 10 && ps <= 19:
		// Each value is the next colour on: OSC 10 ; ? ; blue asks
		// for the foreground and sets the background.
		for i, v := range f[1:] {
			if v != "?" && ps+i <= 19 {
				w.note(ps + i)
			}
		}
	}
}

func (w *colourWatch) note(ps int) {
	for _, s := range w.set {
		if s == ps {
			return
		}
	}
	w.set = append(w.set, ps)
}

// reset is what puts back the colours the block set: OSC 104 for the
// palette, 105 for the special colours, and 110 to 119 for the rest.
func (w *colourWatch) reset() string {
	var b strings.Builder
	for _, ps := range w.set {
		b.WriteString("\x1b]" + strconv.Itoa(100+ps) + "\x1b\\")
	}
	return b.String()
}
