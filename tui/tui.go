// Package tui is mdsh's interactive mode: a terminal UI that shows a
// document, runs its blocks as they are picked, and saves their output
// back to the file.
package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

// Options configure the TUI.
type Options struct {
	// Run are the options every run uses. Run.File must be set. The TUI
	// sets Confirm and OnEvent itself.
	Run run.Options
	// Yes runs confirm blocks, and every block in step mode, without
	// asking.
	Yes bool
	// Save writes the document's new contents to Run.File.
	Save func([]byte) error
	// AutoSave saves after every block that runs. Otherwise output is
	// saved when w is pressed.
	AutoSave bool
	// Style is the glamour style prose is drawn in, such as "dark",
	// "light" or "notty".
	Style string
}

// Run shows d full screen until the user quits.
func Run(d *doc.Document, opts Options) error {
	m, err := New(d, opts)
	if err != nil {
		return err
	}
	defer m.Close()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// Model is the TUI's state. It is a tea.Model.
type Model struct {
	d    *doc.Document
	opts Options
	sess *run.Session
	// results are the latest result of each block. They are copied from
	// run events, so drawing and saving never read the session while a
	// run is going.
	results []*run.Result
	index   map[*doc.Block]int
	nav     []int // blocks the cursor stops on: runnable and include blocks
	parts   []part

	events chan tea.Msg
	cancel context.CancelFunc
	// active is the block running now, or -1.
	active  int
	running bool
	live    map[int][]chunk
	prompt  *confirmMsg
	// input is the prompt for a new block, while typing is set.
	input  textinput.Model
	typing bool
	// skipSaved is the "only run blocks without saved output" answer.
	skipSaved bool

	cursor int // index into nav
	vp     viewport.Model
	width  int
	ready  bool
	tops   map[int]int // first and last content line of each block
	bots   map[int]int
	// md caches markdown drawn with glamour at the current width.
	md map[string]string

	dirty     bool
	mtime     time.Time
	status    string
	quitArmed bool
	forceArm  bool
}

// part is a piece of the screen: prose between blocks, or a block.
type part struct {
	block    int // index into d.Blocks, or -1 for prose
	from, to int // prose: the range of render.Pieces it is made of
}

type chunk struct {
	stderr bool
	data   string
}

type eventMsg run.Event

type confirmMsg struct {
	block *doc.Block
	reply chan run.Answer
}

type runDoneMsg struct{ err error }

// handOff gives the terminal to a tty block. As a tea.ExecCommand, Bubble
// Tea runs it with the terminal released: it lets the block start, waits
// for it to finish, and then for enter, so the block's last screen can be
// read before the TUI draws over it.
type handOff struct {
	block      *doc.Block
	tty        *os.File
	start, end chan struct{}
}

func (h *handOff) Run() error {
	fmt.Fprintf(h.tty, "\r\n--- mdsh: block %s runs on the terminal ---\r\n", h.block.ID)
	close(h.start)
	<-h.end
	fmt.Fprintf(h.tty, "\r\n--- mdsh: block %s is done; press enter to go back ---", h.block.ID)
	bufio.NewReader(h.tty).ReadString('\n')
	return nil
}

func (h *handOff) SetStdin(io.Reader)  {}
func (h *handOff) SetStdout(io.Writer) {}
func (h *handOff) SetStderr(io.Writer) {}

type ttyBackMsg struct{}

// New sets up the TUI for d. Close the model when done with it.
func New(d *doc.Document, opts Options) (*Model, error) {
	if opts.Run.File == "" {
		return nil, errors.New("interactive mode needs a file")
	}
	if opts.Style == "" {
		opts.Style = "dark"
	}
	m := &Model{
		d:      d,
		opts:   opts,
		index:  map[*doc.Block]int{},
		events: make(chan tea.Msg, 64),
		active: -1,
		live:   map[int][]chunk{},
		md:     map[string]string{},
	}
	if info, err := os.Stat(opts.Run.File); err == nil {
		m.mtime = info.ModTime()
	}
	ro := opts.Run
	ro.OnEvent = func(e run.Event) { m.events <- eventMsg(e) }
	ro.Confirm = m.confirm
	if ro.TTY != nil {
		ro.TakeTTY = m.takeTTY
	}
	sess, err := run.NewSession(d, ro)
	if err != nil {
		return nil, err
	}
	m.sess = sess
	m.results = append([]*run.Result(nil), sess.Results()...)

	m.layout()
	m.input = textinput.New()
	m.input.Prompt = "$ "
	m.input.Placeholder = "a command to add to the document as a new block"
	return m, nil
}

// layout splits the document into prose and the blocks the cursor
// stops on. Prose is drawn from render.Pieces: for block i, piece 3i is
// the prose before it. Blocks the cursor skips are drawn with the prose.
func (m *Model) layout() {
	m.parts, m.nav = nil, nil
	last := 0
	for i, b := range m.d.Blocks {
		m.index[b] = i
		if !run.Runnable(b) && !b.IsInclude() {
			continue
		}
		m.parts = append(m.parts, part{block: -1, from: last, to: 3*i + 1})
		m.parts = append(m.parts, part{block: i})
		m.nav = append(m.nav, i)
		last = 3*i + 3
	}
	m.parts = append(m.parts, part{block: -1, from: last, to: 3*len(m.d.Blocks) + 1})
}

// Close ends the shells blocks ran in.
func (m *Model) Close() { m.sess.Close() }

// confirm asks the user from the run's goroutine and waits for the answer.
func (m *Model) confirm(b *doc.Block) run.Answer {
	if m.opts.Yes {
		return run.Go
	}
	reply := make(chan run.Answer)
	m.events <- confirmMsg{block: b, reply: reply}
	return <-reply
}

// takeTTY hands the terminal to block b, from the run's goroutine, and
// returns once the TUI has stepped aside.
func (m *Model) takeTTY(b *doc.Block) func() {
	h := &handOff{block: b, tty: m.opts.Run.TTY, start: make(chan struct{}), end: make(chan struct{})}
	m.events <- h
	<-h.start
	return func() { close(h.end) }
}

func (m *Model) wait() tea.Msg { return <-m.events }

// Init starts listening for run events.
func (m *Model) Init() tea.Cmd { return m.wait }

// Update handles a key, a window size or a run event.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	case eventMsg:
		m.event(run.Event(msg))
		return m, m.wait
	case confirmMsg:
		i := m.index[msg.block]
		if m.skipSaved && m.hasOutput(i) {
			msg.reply <- run.Skip
		} else {
			m.prompt = &msg
			m.moveTo(i)
		}
		m.refresh()
		return m, m.wait
	case *handOff:
		return m, tea.Batch(m.wait, tea.Exec(msg, func(error) tea.Msg { return ttyBackMsg{} }))
	case ttyBackMsg:
		m.refresh()
		return m, nil
	case runDoneMsg:
		m.running, m.active = false, -1
		m.cancel()
		switch {
		case errors.Is(msg.err, context.Canceled):
			m.status = "stopped"
		case msg.err != nil:
			m.status = msg.err.Error()
		}
		m.refresh()
		return m, m.wait
	}
	return m, nil
}

func (m *Model) event(e run.Event) {
	i := m.index[e.Block]
	switch e.Kind {
	case run.Started:
		m.active = i
		m.live[i] = nil
		m.moveTo(i)
	case run.Stdout, run.Stderr:
		m.live[i] = append(m.live[i], chunk{stderr: e.Kind == run.Stderr, data: string(e.Data)})
	case run.Done:
		delete(m.live, i)
		m.results[i] = e.Result
		if e.Result.Ran {
			m.dirty = true
			if m.opts.AutoSave {
				m.save(false)
			}
		}
	}
	m.refresh()
	if m.active == i && m.cur() == i {
		m.follow(i)
	}
}

func (m *Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Keys typed quickly can arrive together; take them one at a time.
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt})
			cmds = append(cmds, cmd)
		}
		return m, tea.Sequence(cmds...)
	}
	if m.typing {
		return m, m.typed(k)
	}
	if m.prompt != nil {
		m.answer(k.String())
		m.refresh()
		return m, nil
	}
	quitArmed, forceArm := m.quitArmed, m.forceArm
	m.quitArmed, m.forceArm = false, false
	switch k.String() {
	case "ctrl+c":
		if m.running {
			m.cancel()
			return m, nil
		}
		return m, tea.Quit
	case "q":
		switch {
		case m.running:
			m.status = "a block is running; ctrl+c stops it"
		case m.dirty && !quitArmed:
			m.status = "output is not saved; q again quits anyway, w saves"
			m.quitArmed = true
		default:
			return m, tea.Quit
		}
	case "j", "down", "tab":
		m.moveTo(m.navAt(m.cursor + 1))
	case "k", "up", "shift+tab":
		m.moveTo(m.navAt(m.cursor - 1))
	case "g", "home":
		m.moveTo(m.navAt(0))
	case "G", "end":
		m.moveTo(m.navAt(len(m.nav) - 1))
	case "pgdown", "ctrl+f", " ":
		m.vp.PageDown()
	case "pgup", "ctrl+b":
		m.vp.PageUp()
	case "ctrl+d":
		m.vp.HalfPageDown()
	case "ctrl+u":
		m.vp.HalfPageUp()
	case "enter", "r":
		m.start(false)
	case "a":
		m.start(true)
	case "w":
		m.save(forceArm)
	case "i", ":":
		if m.running {
			m.status = "a block is running; ctrl+c stops it"
			break
		}
		m.typing = true
		m.refresh()
		return m, m.input.Focus()
	}
	m.refresh()
	return m, nil
}

// typed handles a key while a new block is being typed. Enter appends
// the command to the document as an sh block and runs it; esc cancels.
func (m *Model) typed(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.typing = false
		m.input.Blur()
		m.input.Reset()
		m.refresh()
		return nil
	case tea.KeyEnter:
		cmd := strings.TrimSpace(m.input.Value())
		m.typing = false
		m.input.Blur()
		m.input.Reset()
		if cmd != "" {
			m.add(cmd)
		}
		m.refresh()
		return nil
	}
	var c tea.Cmd
	m.input, c = m.input.Update(k)
	return c
}

// add appends cmd to the document as an sh block and runs it.
func (m *Model) add(cmd string) {
	b, err := m.d.AppendBlock("sh", cmd)
	if err != nil {
		m.status = "can't add a block: " + err.Error()
		return
	}
	m.sess.Grow()
	m.results = append(m.results, m.sess.Results()[len(m.results):]...)
	m.layout()
	m.dirty = true
	m.moveTo(m.index[b])
	m.start(false)
}

// answer handles a key while a confirm prompt is up. Enter stops, as on
// the command line, since it is the safe choice.
func (m *Model) answer(key string) {
	var a run.Answer
	switch key {
	case "r", "y":
		a = run.Go
	case "k":
		a = run.Skip
	case "o":
		m.skipSaved = true
		a = run.Go
		if m.hasOutput(m.index[m.prompt.block]) {
			a = run.Skip
		}
	case "s", "n", "enter", "esc", "ctrl+c", "q":
		a = run.Stop
	default:
		return
	}
	m.prompt.reply <- a
	m.prompt = nil
}

// start runs the block under the cursor, or with all, it and every
// block after it.
func (m *Model) start(all bool) {
	if m.running {
		m.status = "a block is running; ctrl+c stops it"
		return
	}
	if len(m.nav) == 0 {
		return
	}
	i := m.cur()
	ctx, cancel := context.WithCancel(context.Background())
	m.running, m.cancel, m.status = true, cancel, ""
	m.skipSaved = false
	go func() {
		var err error
		if all {
			err = m.sess.RunFrom(ctx, i)
		} else {
			_, err = m.sess.RunBlock(ctx, i)
		}
		m.events <- runDoneMsg{err}
	}()
}

// save writes the results into the file. It won't overwrite changes made
// to the file since it was opened unless force is set.
func (m *Model) save(force bool) {
	if m.opts.Save == nil {
		return
	}
	file := m.opts.Run.File
	if info, err := os.Stat(file); err == nil && !info.ModTime().Equal(m.mtime) && !force {
		m.status = "the file changed on disk; press w twice to overwrite it"
		m.forceArm = true
		return
	}
	out := render.Markdown(m.d, m.results, render.Options{WriteBack: true, Env: m.opts.Run.Environ})
	if err := m.opts.Save(out); err != nil {
		m.status = "save: " + err.Error()
		return
	}
	if info, err := os.Stat(file); err == nil {
		m.mtime = info.ModTime()
	}
	m.dirty = false
	if !m.opts.AutoSave {
		m.status = "saved"
	}
}

// hasOutput reports whether block i has output saved, or ran this session.
func (m *Model) hasOutput(i int) bool {
	return m.d.Blocks[i].HasOutput() || m.results[i].Ran
}

// cur is the block index under the cursor, or -1 if there are none.
func (m *Model) cur() int {
	if len(m.nav) == 0 {
		return -1
	}
	return m.nav[m.cursor]
}

func (m *Model) navAt(n int) int {
	if len(m.nav) == 0 {
		return -1
	}
	return m.nav[min(max(n, 0), len(m.nav)-1)]
}

// moveTo puts the cursor on block i and scrolls it into view.
func (m *Model) moveTo(i int) {
	for n, b := range m.nav {
		if b == i {
			m.cursor = n
		}
	}
	m.refresh()
	top, bot := m.tops[i], m.bots[i]
	switch {
	case top < m.vp.YOffset:
		m.vp.SetYOffset(top)
	case bot > m.vp.YOffset+m.vp.Height:
		m.vp.SetYOffset(min(top, bot-m.vp.Height))
	}
}

// follow keeps the end of a running block's output in view.
func (m *Model) follow(i int) {
	if bot := m.bots[i]; bot > m.vp.YOffset+m.vp.Height {
		m.vp.SetYOffset(bot - m.vp.Height)
	}
}

func (m *Model) resize(w, h int) {
	if w != m.width {
		m.md = map[string]string{}
	}
	m.width = w
	if !m.ready {
		m.vp = viewport.New(w, h-footerLines)
		m.ready = true
	} else {
		m.vp.Width, m.vp.Height = w, h-footerLines
	}
	m.refresh()
}

// View draws the document above a status line and a key help line.
func (m *Model) View() string {
	if !m.ready {
		return "loading…"
	}
	return m.vp.View() + "\n" + m.footer()
}

func (m *Model) footer() string {
	name := m.opts.Run.File
	state := ""
	if m.dirty {
		state = " " + dirtyStyle.Render("● not saved")
	}
	status := m.status
	if m.running {
		status = "running…"
	}
	line1 := titleStyle.Render(name) + state
	if status != "" {
		line1 += "  " + statusStyle.Render(status)
	}
	var help string
	switch {
	case m.typing:
		help = m.input.View()
	case m.prompt != nil:
		q := m.prompt.block.Attrs["confirm"]
		if q == "" {
			q = "Run this block?"
		}
		help = promptStyle.Render(fmt.Sprintf("%s  [r]un  s[k]ip  [o]nly blocks without saved output  [s]top", q))
	case m.running:
		help = helpStyle.Render("j/k move · ctrl+c stop")
	default:
		help = helpStyle.Render("j/k move · enter run · a run from here · i new block · w save · q quit")
	}
	return line1 + "\n" + help
}

const footerLines = 2
