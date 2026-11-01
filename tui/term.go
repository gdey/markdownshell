package tui

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
)

// pane is the terminal a tty block runs in, drawn in the block's card
// as in mdsh-gui: what the block shows goes to a terminal emulator, and
// keys go to the block while it runs.
type pane struct {
	keys    chan []byte
	resized chan struct{}
	// drawn is signalled when the screen changed, so the TUI redraws.
	drawn chan struct{}

	mu         sync.Mutex
	cols, rows int
	block      int // the block running in it, or -1
	vt         vt10x.Terminal
}

func newPane() *pane {
	p := &pane{
		keys:    make(chan []byte, 64),
		resized: make(chan struct{}, 1),
		drawn:   make(chan struct{}, 1),
		cols:    80,
		rows:    24,
		block:   -1,
	}
	return p
}

// env is the pane as run sees it.
func (p *pane) env() *env.Terminal {
	return &env.Terminal{Keys: p.keys, Out: paneOut{p}, Size: p.size, Resized: p.resized}
}

func (p *pane) size() (cols, rows int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cols, p.rows
}

// setSize sizes the pane. A block running in it follows.
func (p *pane) setSize(cols, rows int) {
	p.mu.Lock()
	changed := p.cols != cols || p.rows != rows
	p.cols, p.rows = cols, rows
	if changed && p.vt != nil {
		p.vt.Resize(cols, rows)
	}
	p.mu.Unlock()
	if changed {
		select {
		case p.resized <- struct{}{}:
		default:
		}
	}
}

type paneOut struct{ p *pane }

func (o paneOut) Write(b []byte) (int, error) {
	o.p.mu.Lock()
	if o.p.vt != nil {
		o.p.vt.Write(b)
	}
	o.p.mu.Unlock()
	select {
	case o.p.drawn <- struct{}{}:
	default:
	}
	return len(b), nil
}

// keyWriter sends what the emulator answers, such as the cursor's
// position, back to the block as typed.
type keyWriter struct{ p *pane }

func (w keyWriter) Write(b []byte) (int, error) {
	select {
	case w.p.keys <- append([]byte(nil), b...):
	default:
	}
	return len(b), nil
}

// take gives the pane to block i, with a blank screen.
func (p *pane) take(i int) {
	p.mu.Lock()
	p.block = i
	p.vt = vt10x.New(vt10x.WithSize(p.cols, p.rows), vt10x.WithWriter(keyWriter{p}))
	p.mu.Unlock()
	// Keys typed before the block started aren't for it.
	for len(p.keys) > 0 {
		<-p.keys
	}
}

// release ends block i's use of the pane.
func (p *pane) release() {
	p.mu.Lock()
	p.block, p.vt = -1, nil
	p.mu.Unlock()
}

// running is the block running in the pane, or -1.
func (p *pane) running() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.block
}

// typeKey sends k to the block running in the pane.
func (p *pane) typeKey(k tea.KeyMsg) {
	b := keyBytes(k)
	if len(b) == 0 || p.running() < 0 {
		return
	}
	select {
	case p.keys <- b:
	default:
	}
}

// keyBytes is what a terminal sends for k.
func keyBytes(k tea.KeyMsg) []byte {
	var b []byte
	switch k.Type {
	case tea.KeyRunes:
		b = []byte(string(k.Runes))
		if k.Paste {
			b = append(append([]byte("\x1b[200~"), b...), "\x1b[201~"...)
		}
	case tea.KeySpace:
		b = []byte(" ")
	case tea.KeyUp:
		b = []byte("\x1b[A")
	case tea.KeyDown:
		b = []byte("\x1b[B")
	case tea.KeyRight:
		b = []byte("\x1b[C")
	case tea.KeyLeft:
		b = []byte("\x1b[D")
	case tea.KeyHome:
		b = []byte("\x1b[H")
	case tea.KeyEnd:
		b = []byte("\x1b[F")
	case tea.KeyPgUp:
		b = []byte("\x1b[5~")
	case tea.KeyPgDown:
		b = []byte("\x1b[6~")
	case tea.KeyDelete:
		b = []byte("\x1b[3~")
	case tea.KeyInsert:
		b = []byte("\x1b[2~")
	case tea.KeyShiftTab:
		b = []byte("\x1b[Z")
	case tea.KeyF1:
		b = []byte("\x1bOP")
	case tea.KeyF2:
		b = []byte("\x1bOQ")
	case tea.KeyF3:
		b = []byte("\x1bOR")
	case tea.KeyF4:
		b = []byte("\x1bOS")
	default:
		// Control keys are their own byte: ctrl+a is 1, enter 13,
		// backspace 127.
		if k.Type >= 0 && (k.Type < 32 || k.Type == 127) {
			b = []byte{byte(k.Type)}
		}
	}
	if k.Alt && len(b) > 0 {
		b = append([]byte{0x1b}, b...)
	}
	return b
}

// screen draws the pane's screen, with its cursor.
func (p *pane) screen() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.vt == nil {
		return ""
	}
	p.vt.Lock()
	defer p.vt.Unlock()
	cols, rows := p.vt.Size()
	cur := p.vt.Cursor()
	showCursor := p.vt.CursorVisible()
	var out strings.Builder
	for y := range rows {
		if y > 0 {
			out.WriteByte('\n')
		}
		last := ""
		for x := range cols {
			g := p.vt.Cell(x, y)
			sgr := sgrOf(g, showCursor && x == cur.X && y == cur.Y)
			if sgr != last {
				out.WriteString("\x1b[0m" + sgr)
				last = sgr
			}
			r := g.Char
			if r == 0 || !utf8.ValidRune(r) || r < ' ' {
				r = ' '
			}
			out.WriteRune(r)
		}
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// The Mode bits vt10x sets.
const (
	vtReverse   = 1 << 0
	vtUnderline = 1 << 1
	vtBold      = 1 << 2
	vtItalic    = 1 << 4
)

// sgrOf is the escape that draws g, or "" for a plain cell.
func sgrOf(g vt10x.Glyph, cursor bool) string {
	var codes []string
	if g.Mode&vtBold != 0 {
		codes = append(codes, "1")
	}
	if g.Mode&vtItalic != 0 {
		codes = append(codes, "3")
	}
	if g.Mode&vtUnderline != 0 {
		codes = append(codes, "4")
	}
	if (g.Mode&vtReverse != 0) != cursor {
		codes = append(codes, "7")
	}
	codes = append(codes, sgrColor(g.FG, 30)...)
	codes = append(codes, sgrColor(g.BG, 40)...)
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// sgrColor is the SGR codes for c as a foreground (base 30) or background
// (base 40) color.
func sgrColor(c vt10x.Color, base int) []string {
	switch {
	case c >= vt10x.DefaultFG:
		return nil
	case c < 8:
		return []string{fmt.Sprint(base + int(c))}
	case c < 16:
		return []string{fmt.Sprint(base + 60 + int(c) - 8)}
	case c < 256:
		return []string{fmt.Sprint(base + 8), "5", fmt.Sprint(int(c))}
	}
	return []string{fmt.Sprint(base + 8), "2", fmt.Sprint(int(c>>16) & 0xff), fmt.Sprint(int(c>>8) & 0xff), fmt.Sprint(int(c) & 0xff)}
}

// The most rows a block's terminal gets, as in mdsh-gui, and the
// fewest.
const paneRows, minPaneRows = 24, 5

// fit sizes the pane to the card it is drawn in: the card's width, and
// up to 24 rows, less when the window is short.
func (m *Model) fit() {
	cols := max(m.docWidth()-4-4-2, 20)
	rows := min(paneRows, max(m.vpHeight()-6, minPaneRows))
	m.pane.setSize(cols, rows)
}

// paneMsg says what the block in the pane shows changed.
type paneMsg struct{ m *Model }

// paneTurnMsg says block starts using the pane, or with end, stops.
type paneTurnMsg struct {
	block *doc.Block
	end   bool
	reply chan struct{}
}

// watchPane waits for the pane's screen to change.
func (m *Model) watchPane() tea.Msg {
	select {
	case <-m.pane.drawn:
		return paneMsg{m}
	case <-m.done:
		return nil
	}
}

// takeTTY is run's TakeTTY: the block runs in its card. It returns once
// the TUI shows the pane.
func (m *Model) takeTTY(b *doc.Block) func() {
	reply := make(chan struct{})
	if m.send(paneTurnMsg{block: b, reply: reply}) {
		select {
		case <-reply:
		case <-m.done:
		}
	}
	return func() {
		m.pane.release()
		m.send(paneTurnMsg{block: b, end: true})
	}
}

// paneView draws the pane in its card.
func (m *Model) paneView() string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(runningCol).
		Render(m.pane.screen())
}
