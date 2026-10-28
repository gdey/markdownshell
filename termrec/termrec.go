// Package termrec records what a program writes to a terminal, saves it
// as an asciinema cast, and plays it back as an animated GIF.
package termrec

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"strings"
	"sync"
	"time"

	"github.com/hinshun/vt10x"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Recording is a terminal session: the bytes written to the screen, and
// when.
type Recording struct {
	Cols, Rows int
	Start      time.Time // zero if unknown
	Events     []Event
}

// Event is one write to the screen, At after the recording started.
type Event struct {
	At   time.Duration
	Data []byte
}

// Recorder is an io.Writer that records what is written to it.
type Recorder struct {
	mu    sync.Mutex
	start time.Time
	rec   Recording
}

// NewRecorder starts a recording of a cols by rows terminal.
func NewRecorder(cols, rows int) *Recorder {
	now := time.Now()
	return &Recorder{start: now, rec: Recording{Cols: cols, Rows: rows, Start: now}}
}

func (r *Recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rec.Events = append(r.rec.Events, Event{At: time.Since(r.start), Data: append([]byte(nil), p...)})
	return len(p), nil
}

// Recording returns what has been recorded so far.
func (r *Recorder) Recording() Recording {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rec
	rec.Events = append([]Event(nil), rec.Events...)
	return rec
}

const (
	pad      = 8
	fontSize = 14
	lineH    = 18
	// step is the shortest time between frames; writes closer together
	// than this share a frame.
	step = 100 * time.Millisecond
	// maxDelay caps how long a frame shows, so idle time doesn't drag.
	maxDelay = 2 * time.Second
	// hold is how long the last frame shows before the GIF loops.
	hold = 3 * time.Second
)

// ErrEmpty is returned by GIF for a recording with nothing in it.
var ErrEmpty = errors.New("nothing was written to the terminal")

// GIF plays rec through a terminal emulator and returns the screens it
// shows as a looping animated GIF. A frame is drawn at most every 100ms,
// and only the rows that changed are stored. Rows the session never
// reached are left off.
func GIF(rec Recording) ([]byte, error) {
	if len(rec.Events) == 0 {
		return nil, ErrEmpty
	}
	cols, rows := rec.Cols, rec.Rows
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}
	vt := vt10x.New(vt10x.WithSize(cols, rows))

	// Take the screens first, to find how many rows the session used.
	type shot struct {
		at     time.Duration
		screen [][]cell
	}
	var shots []shot
	used := 1
	for i, e := range rec.Events {
		vt.Write(e.Data)
		if i+1 < len(rec.Events) && len(shots) > 0 && rec.Events[i+1].At-shots[len(shots)-1].at < step {
			continue
		}
		screen := snapshot(vt, cols, rows)
		if n := len(shots); n > 0 {
			if y0, y1 := changed(shots[n-1].screen, screen); y0 > y1 {
				continue
			}
		}
		shots = append(shots, shot{e.At, screen})
		used = max(used, usedRows(screen))
	}

	p, err := newPainter(cols, used)
	if err != nil {
		return nil, err
	}
	defer p.face.Close()
	var (
		out    gif.GIF
		prev   [][]cell
		prevAt time.Duration
	)
	out.Config = image.Config{ColorModel: palette, Width: p.width, Height: p.height}
	for _, sh := range shots {
		cur := sh.screen[:used]
		y0, y1 := changed(prev, cur)
		if y0 > y1 {
			continue
		}
		if n := len(out.Image); n > 0 {
			out.Delay[n-1] = delay(sh.at - prevAt)
		} else {
			y0, y1 = 0, used-1
		}
		out.Image = append(out.Image, p.paint(cur, y0, y1))
		out.Delay = append(out.Delay, delay(hold))
		out.Disposal = append(out.Disposal, gif.DisposalNone)
		prev, prevAt = cur, sh.at
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &out); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// delay converts d to GIF hundredths of a second, capped at maxDelay.
func delay(d time.Duration) int {
	d = min(d, maxDelay)
	return max(int(d/(10*time.Millisecond)), 2)
}

// Frame returns frame n of an animated GIF as it looks on screen,
// counting from 1. n <= 0, or past the end, is the last frame.
func Frame(data []byte, n int) (image.Image, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(g.Image) == 0 {
		return nil, ErrEmpty
	}
	if n <= 0 || n > len(g.Image) {
		n = len(g.Image)
	}
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Empty() {
		bounds = g.Image[0].Bounds()
	}
	img := image.NewRGBA(bounds)
	for _, f := range g.Image[:n] {
		draw.Draw(img, f.Bounds(), f, f.Bounds().Min, draw.Over)
	}
	return img, nil
}

// cell is one character on screen, with palette indexes for its colors.
type cell struct {
	r      rune
	fg, bg uint8
}

func snapshot(vt vt10x.Terminal, cols, rows int) [][]cell {
	vt.Lock()
	defer vt.Unlock()
	cur := vt.Cursor()
	showCursor := vt.CursorVisible()
	screen := make([][]cell, rows)
	for y := range screen {
		screen[y] = make([]cell, cols)
		for x := range screen[y] {
			g := vt.Cell(x, y)
			fg, bg := index(g.FG, defaultFG), index(g.BG, defaultBG)
			if g.Mode&modeBold != 0 && g.FG < 8 {
				fg += 8
			}
			if g.Mode&modeReverse != 0 {
				fg, bg = bg, fg
			}
			if showCursor && x == cur.X && y == cur.Y {
				fg, bg = bg, fg
			}
			screen[y][x] = cell{g.Char, fg, bg}
		}
	}
	return screen
}

// The Mode bits vt10x sets for SGR 7 and SGR 1.
const (
	modeReverse = 1 << 0
	modeBold    = 1 << 2
)

// usedRows is the number of rows down to the last one with text or a
// colored cell on it.
func usedRows(screen [][]cell) int {
	for y := len(screen) - 1; y >= 0; y-- {
		for _, c := range screen[y] {
			if (c.r != 0 && c.r != ' ') || c.bg != defaultBG {
				return y + 1
			}
		}
	}
	return 0
}

// changed returns the first and last rows that differ. y0 > y1 means
// none do.
func changed(prev, cur [][]cell) (y0, y1 int) {
	y0, y1 = len(cur), -1
	for y := range cur {
		if prev != nil && rowEqual(prev[y], cur[y]) {
			continue
		}
		y0, y1 = min(y0, y), max(y1, y)
	}
	return y0, y1
}

func rowEqual(a, b []cell) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// painter draws screens with Go Mono.
type painter struct {
	face          font.Face
	adv, ascent   int
	width, height int
	glyphs        map[rune]*image.Alpha
}

func newPainter(cols, rows int) (*painter, error) {
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: fontSize, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	adv, _ := face.GlyphAdvance('M')
	p := &painter{face: face, adv: adv.Ceil(), ascent: face.Metrics().Ascent.Ceil(), glyphs: map[rune]*image.Alpha{}}
	p.width = cols*p.adv + 2*pad
	p.height = rows*lineH + 2*pad
	return p, nil
}

// paint draws rows y0 to y1 of screen as one frame.
func (p *painter) paint(screen [][]cell, y0, y1 int) *image.Paletted {
	top, bottom := pad+y0*lineH, pad+(y1+1)*lineH
	if y0 == 0 {
		top = 0
	}
	if y1 == len(screen)-1 {
		bottom = p.height
	}
	img := image.NewPaletted(image.Rect(0, top, p.width, bottom), palette)
	draw.Draw(img, img.Bounds(), &image.Uniform{palette[defaultBG]}, image.Point{}, draw.Src)
	for y := y0; y <= y1; y++ {
		for x, c := range screen[y] {
			cx, cy := pad+x*p.adv, pad+y*lineH
			if c.bg != defaultBG {
				draw.Draw(img, image.Rect(cx, cy, cx+p.adv, cy+lineH), &image.Uniform{palette[c.bg]}, image.Point{}, draw.Src)
			}
			if c.r == 0 || c.r == ' ' {
				continue
			}
			mask := p.glyph(c.r)
			if mask == nil {
				continue
			}
			// GIF has no partial transparency, so a glyph is drawn
			// with its mostly covered pixels.
			b := mask.Bounds()
			for my := b.Min.Y; my < b.Max.Y; my++ {
				for mx := b.Min.X; mx < b.Max.X; mx++ {
					if mask.AlphaAt(mx, my).A >= 0x60 {
						img.SetColorIndex(cx+mx, cy+my, c.fg)
					}
				}
			}
		}
	}
	return img
}

// glyph returns r's coverage, offset from the top left of its cell.
func (p *painter) glyph(r rune) *image.Alpha {
	if m, ok := p.glyphs[r]; ok {
		return m
	}
	dr, mask, mp, _, ok := p.face.Glyph(fixed.P(0, p.ascent+(lineH-p.ascent)/2-2), r)
	var m *image.Alpha
	if ok {
		m = image.NewAlpha(dr)
		draw.Draw(m, dr, mask, mp, draw.Src)
	}
	p.glyphs[r] = m
	return m
}

const (
	// defaultBG and defaultFG are the xterm colors nearest mdsh's
	// screenshot colors.
	defaultBG = 234
	defaultFG = 252
)

// palette is the xterm 256-color palette.
var palette = func() color.Palette {
	p := make(color.Palette, 256)
	base := []uint32{
		0x000000, 0xcd0000, 0x00cd00, 0xcdcd00, 0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5,
		0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff,
	}
	for i, c := range base {
		p[i] = rgb(c)
	}
	steps := []uint8{0, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	for i := range 216 {
		p[16+i] = color.RGBA{steps[i/36], steps[i/6%6], steps[i%6], 0xff}
	}
	for i := range 24 {
		v := uint8(8 + 10*i)
		p[232+i] = color.RGBA{v, v, v, 0xff}
	}
	return p
}()

func rgb(c uint32) color.RGBA {
	return color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 0xff}
}

// index maps a vt10x color to the palette: the default colors to def,
// and colors past 255, which are 24-bit, to the nearest entry.
func index(c vt10x.Color, def uint8) uint8 {
	switch {
	case c >= vt10x.DefaultFG:
		return def
	case c < 256:
		return uint8(c)
	}
	return uint8(palette.Index(rgb(uint32(c))))
}

// Screen is a terminal emulator to write a program's output to, for
// reading what it shows.
type Screen struct {
	vt vt10x.Terminal
}

// NewScreen returns a blank cols by rows screen.
func NewScreen(cols, rows int) *Screen {
	return &Screen{vt: vt10x.New(vt10x.WithSize(cols, rows))}
}

func (s *Screen) Write(p []byte) (int, error) { return s.vt.Write(p) }

// Text returns what the screen shows, one line per row, without trailing
// spaces.
func (s *Screen) Text() string {
	s.vt.Lock()
	defer s.vt.Unlock()
	cols, rows := s.vt.Size()
	var b strings.Builder
	for y := range rows {
		var line strings.Builder
		for x := range cols {
			r := s.vt.Cell(x, y).Char
			if r == 0 {
				r = ' '
			}
			line.WriteRune(r)
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	return b.String()
}
