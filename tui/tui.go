// Package tui is mdsh's interactive mode: a terminal UI that shows a
// document, runs its blocks as they are picked, and saves their output
// back to the file.
package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termimg"
)

// Options configure the TUI.
type Options struct {
	// Run are the options every run uses. Run.File must be set. The TUI
	// sets Confirm and OnEvent itself.
	Run run.Options
	// Yes runs confirm blocks, every block in step mode, and the
	// blocks depends= names, without
	// asking.
	Yes bool
	// Save writes the document's new contents to file: Run.File, or
	// the file picked by save as.
	Save func(file string, out []byte) error
	// AutoSave saves after every block that runs. Otherwise output is
	// saved when w or ctrl+s is pressed.
	AutoSave bool
	// Images turns on image previews, drawn with colored half blocks.
	Images bool
	// Protocol is how v shows an image at full size. With termimg.None
	// it is drawn with half blocks, as large as the terminal allows.
	Protocol termimg.Protocol
	// Style is the glamour style prose is drawn in, such as "dark",
	// "light" or "notty".
	Style string
	// View is how the file is shown at first.
	View View
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
	// bg are the background blocks still running. While there are any,
	// a tick checks for those that ended.
	bg      map[int]bool
	ticking bool
	prompt  *confirmMsg
	// input is the prompt for a new block, while typing is set.
	input  textinput.Model
	typing bool
	// skipSaved is the "only run blocks without saved output" answer.
	skipSaved bool

	cursor int // index into nav
	vp     viewport.Model
	width  int
	height int
	ready  bool
	tops   map[int]int // first and last content line of each block
	bots   map[int]int
	// md caches markdown drawn with glamour at the current width.
	md map[string]string
	// images caches image previews; see preview.
	images map[string]string

	dirty     bool
	mtime     time.Time
	status    string
	quitArmed bool
	forceArm  bool
	// reloadArmed is set after R with unsaved output: R again reloads.
	reloadArmed bool
	// changed is set when the file changed on disk since it was opened
	// or saved, until it is reloaded or kept.
	changed bool
	// saveAs is the prompt for the file to save as, while naming is set.
	saveAs    textinput.Model
	naming    bool
	overwrite string // the existing file enter again overwrites

	// shown is how the file is shown: the outline, code and output.
	shown View

	// form is the report or run options form, while one is up;
	// formKind says which: "report" or "run".
	form     *form
	formKind string
	// report is the report to write when the run going on ends.
	report *pendingReport
	// optsChanged is set when the run options changed, until the app is
	// told.
	optsChanged bool

	// back is set when the TUI came from the start screen: q goes back
	// to it rather than quitting.
	back bool
	// done is closed by Close, so nothing waits on events after it.
	done chan struct{}
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
	// of, for a dependency's question, is the block that depends on
	// block.
	of    *doc.Block
	reply chan run.Answer
}

type runDoneMsg struct{ err error }

// reapMsg is the tick that checks for background blocks that ended.
// Ticks name their model, so a model that replaced it ignores them.
type reapMsg struct{ m *Model }

// reapEvery is how often the TUI checks on background blocks while
// nothing else runs.
const reapEvery = 250 * time.Millisecond

// handOff gives the terminal to a tty block. As a tea.ExecCommand, Bubble
// Tea runs it with the terminal released: it lets the block start, waits
// for it to finish, and then for enter, so the block's last screen can be
// read before the TUI draws over it.
type handOff struct {
	label      string // the block, as doc.Document.Label names it
	tty        *os.File
	start, end chan struct{}
}

func (h *handOff) Run() error {
	fmt.Fprintf(h.tty, "\r\n--- mdsh: block %s runs on the terminal ---\r\n", h.label)
	close(h.start)
	<-h.end
	fmt.Fprintf(h.tty, "\r\n--- mdsh: block %s is done; press enter to go back ---", h.label)
	bufio.NewReader(h.tty).ReadString('\n')
	return nil
}

func (h *handOff) SetStdin(io.Reader)  {}
func (h *handOff) SetStdout(io.Writer) {}
func (h *handOff) SetStderr(io.Writer) {}

type ttyBackMsg struct{}

// viewer shows an image full screen, with the terminal released, until
// enter is pressed.
type viewer struct {
	img      image.Image
	protocol termimg.Protocol
	in       io.Reader
	out      io.Writer
}

func (v *viewer) Run() error {
	cols, rows := 80, 24
	if f, ok := v.out.(*os.File); ok {
		if w, h, err := term.GetSize(int(f.Fd())); err == nil {
			cols, rows = w, h
		}
	}
	io.WriteString(v.out, "\x1b[2J\x1b[H")
	if v.protocol == termimg.None || termimg.Show(v.out, v.protocol, v.img, cols) != nil {
		io.WriteString(v.out, strings.ReplaceAll(termimg.HalfBlocks(v.img, cols, rows-2), "\n", "\r\n")+"\r\n")
	}
	io.WriteString(v.out, "press enter to go back")
	bufio.NewReader(v.in).ReadString('\n')
	if v.protocol == termimg.Kitty {
		io.WriteString(v.out, "\x1b_Ga=d\x1b\\")
	}
	io.WriteString(v.out, "\x1b[2J\x1b[H")
	return nil
}

func (v *viewer) SetStdin(r io.Reader)  { v.in = r }
func (v *viewer) SetStdout(w io.Writer) { v.out = w }
func (v *viewer) SetStderr(io.Writer)   {}

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
		done:   make(chan struct{}),
		shown:  opts.View,
		active: -1,
		live:   map[int][]chunk{},
		bg:     map[int]bool{},
		md:     map[string]string{},
		images: map[string]string{},
	}
	if info, err := os.Stat(opts.Run.File); err == nil {
		m.mtime = info.ModTime()
	}
	sess, err := m.newSession(opts.Run)
	if err != nil {
		return nil, err
	}
	m.sess = sess
	m.results = append([]*run.Result(nil), sess.Results()...)

	m.layout()
	m.input = textinput.New()
	m.input.Prompt = "$ "
	m.input.Placeholder = "a command to add to the document as a new block"
	m.saveAs = textinput.New()
	m.saveAs.Prompt = "save as: "
	return m, nil
}

// newSession starts a session that runs d with ro, reporting to m.
func (m *Model) newSession(ro run.Options) (*run.Session, error) {
	ro.OnEvent = func(e run.Event) { m.send(eventMsg(e)) }
	ro.Confirm = m.confirm
	ro.Depend = m.depend
	if ro.TTY != nil {
		ro.TakeTTY = m.takeTTY
	}
	return run.NewSession(m.d, ro)
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
func (m *Model) Close() {
	m.sess.Close()
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}

// send passes msg to Update, unless the model is closed.
func (m *Model) send(msg tea.Msg) bool {
	select {
	case m.events <- msg:
		return true
	case <-m.done:
		return false
	}
}

// confirm asks the user from the run's goroutine and waits for the answer.
func (m *Model) confirm(b *doc.Block) run.Answer {
	if m.opts.Yes {
		return run.Go
	}
	reply := make(chan run.Answer)
	if !m.send(confirmMsg{block: b, reply: reply}) {
		return run.Stop
	}
	select {
	case a := <-reply:
		return a
	case <-m.done:
		return run.Stop
	}
}

// depend asks whether to run dep, which b depends on, first.
func (m *Model) depend(dep, b *doc.Block) run.Answer {
	if m.opts.Yes {
		return run.Go
	}
	reply := make(chan run.Answer)
	m.events <- confirmMsg{block: dep, of: b, reply: reply}
	return <-reply
}

// takeTTY hands the terminal to block b, from the run's goroutine, and
// returns once the TUI has stepped aside.
func (m *Model) takeTTY(b *doc.Block) func() {
	h := &handOff{label: m.d.Label(b), tty: m.opts.Run.TTY, start: make(chan struct{}), end: make(chan struct{})}
	if !m.send(h) {
		return func() {}
	}
	<-h.start
	return func() { close(h.end) }
}

func (m *Model) wait() tea.Msg {
	select {
	case msg := <-m.events:
		return msg
	case <-m.done:
		return nil
	}
}

// checkMsg is the tick that checks whether the file changed on disk.
type checkMsg struct{ m *Model }

// checkEvery is how often the TUI looks at the file's modification time.
const checkEvery = time.Second

func (m *Model) check() tea.Cmd {
	return tea.Tick(checkEvery, func(time.Time) tea.Msg { return checkMsg{m} })
}

// diskChanged reports whether the file changed on disk since it was
// opened or saved.
func (m *Model) diskChanged() bool {
	info, err := os.Stat(m.opts.Run.File)
	return err == nil && !info.ModTime().Equal(m.mtime)
}

// tick schedules a check on background blocks, if any are running and
// none is scheduled.
func (m *Model) tick() tea.Cmd {
	if len(m.bg) == 0 || m.ticking {
		return nil
	}
	m.ticking = true
	return tea.Tick(reapEvery, func(time.Time) tea.Msg { return reapMsg{m} })
}

// Init starts listening for run events, and watching the file.
func (m *Model) Init() tea.Cmd { return tea.Batch(m.wait, m.check()) }

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
		return m, tea.Batch(m.wait, m.tick())
	case checkMsg:
		if msg.m != m {
			return m, nil
		}
		if !m.changed && m.diskChanged() {
			m.changed = true
			m.refresh()
		}
		return m, m.check()
	case reapMsg:
		if msg.m != m {
			return m, nil
		}
		m.ticking = false
		// A run checks on them itself, before each block; the session
		// runs one thing at a time.
		if !m.running {
			m.sess.Reap()
		}
		return m, m.tick()
	case confirmMsg:
		i := m.index[msg.block]
		if m.skipSaved && m.hasOutput(i) && msg.of == nil {
			msg.reply <- run.Skip
		} else {
			m.prompt = &msg
			m.moveTo(i)
		}
		m.refresh()
		return m, m.wait
	case *handOff:
		return m, tea.Batch(m.wait, tea.Exec(msg, func(error) tea.Msg { return ttyBackMsg{} }))
	case editedMsg:
		m.edited(msg)
		m.refresh()
		return m, nil
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
		if r := m.report; r != nil {
			m.report = nil
			if errors.Is(msg.err, context.Canceled) {
				m.status = "stopped; the report wasn't saved"
			} else {
				was := m.status
				m.writeReport(*r)
				if was != "" {
					m.status = was + " · " + m.status
				}
			}
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
		if e.Block.Attrs.Has("background") {
			// The run goes straight on; Done comes when the block ends.
			m.bg[i] = true
			delete(m.live, i)
			break
		}
		m.active = i
		m.live[i] = nil
		m.moveTo(i)
	case run.Stdout, run.Stderr:
		m.live[i] = append(m.live[i], chunk{stderr: e.Kind == run.Stderr, data: string(e.Data)})
	case run.Done:
		delete(m.live, i)
		delete(m.bg, i)
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
	if m.naming {
		return m, m.named(k)
	}
	if m.form != nil {
		return m, m.formed(k)
	}
	if m.prompt != nil {
		m.answer(k.String())
		m.refresh()
		return m, nil
	}
	quitArmed, forceArm, reloadArmed := m.quitArmed, m.forceArm, m.reloadArmed
	m.quitArmed, m.forceArm, m.reloadArmed = false, false, false
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
			if m.back {
				m.status = "output is not saved; q again closes it anyway, w saves"
			}
			m.quitArmed = true
		case m.back:
			return m, func() tea.Msg { return backMsg{} }
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
	case "e":
		if cmd := m.edit(); cmd != nil {
			return m, cmd
		}
	case "E":
		if cmd := m.editAll(); cmd != nil {
			return m, cmd
		}
	case "v":
		if cmd := m.view(); cmd != nil {
			return m, cmd
		}
	case "w", "ctrl+s":
		m.save(forceArm)
	case "W":
		if m.running {
			m.status = "a block is running; ctrl+c stops it"
			break
		}
		m.naming, m.overwrite = true, ""
		m.saveAs.SetValue(m.opts.Run.File)
		m.saveAs.CursorEnd()
		m.refresh()
		return m, m.saveAs.Focus()
	case "R":
		switch {
		case m.running:
			m.status = "a block is running; ctrl+c stops it"
		case m.dirty && !reloadArmed:
			m.status = "output is not saved; R again reloads anyway"
			m.reloadArmed = true
		default:
			file := m.opts.Run.File
			return m, func() tea.Msg { return openMsg{file: file} }
		}
	case "M":
		if m.changed {
			m.keepMine()
		}
	case "p":
		if cmd := m.reportForm(); cmd != nil {
			return m, cmd
		}
	case ",":
		if cmd := m.runOptionsForm(); cmd != nil {
			return m, cmd
		}
	case "o":
		return m, m.toggle(&m.shown.Outline)
	case "C":
		return m, m.toggle(&m.shown.HideCode)
	case "O":
		return m, m.toggle(&m.shown.HideOutput)
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

// editedMsg reports that the editor opened by edit or editAll has
// exited.
type editedMsg struct {
	block int // -1 for the whole file
	file  string
	// text is what the editor was given, for the whole file.
	text string
	err  error
}

// edit opens the block under the cursor, fences and all, in $VISUAL,
// $EDITOR or vi.
func (m *Model) edit() tea.Cmd {
	i := m.cur()
	switch {
	case i < 0:
		return nil
	case m.running:
		m.status = "a block is running; ctrl+c stops it"
		return nil
	case m.d.Blocks[i].IsInclude():
		m.status = "an include block's lines come from its file; edit that instead"
		return nil
	}
	f, err := os.CreateTemp("", "mdsh-*.md")
	if err == nil {
		_, err = f.WriteString(m.d.BlockText(m.d.Blocks[i]))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		m.status = "edit: " + err.Error()
		return nil
	}
	editor := strings.Fields(m.editor())
	cmd := exec.Command(editor[0], append(editor[1:], f.Name())...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editedMsg{block: i, file: f.Name(), err: err} })
}

// editor is the command that edits a file: $VISUAL, $EDITOR or vi.
func (m *Model) editor() string {
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if e := strings.TrimSpace(m.getenv(v)); e != "" {
			return e
		}
	}
	return "vi"
}

// getenv looks name up in the run's environment.
func (m *Model) getenv(name string) string {
	for _, kv := range m.opts.Run.Environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	if m.opts.Run.Environ == nil {
		return os.Getenv(name)
	}
	return ""
}

// editAll opens the whole file in $VISUAL, $EDITOR or vi, as it would
// be saved, so output not saved yet is in it, with the cursor on the
// block under the TUI's cursor where the editor takes a +line argument.
// This is the TUI's source view: mdsh-gui edits the whole file too.
func (m *Model) editAll() tea.Cmd {
	switch {
	case m.running:
		m.status = "a block is running; ctrl+c stops it"
		return nil
	case len(m.bg) > 0:
		m.status = "a block is running in the background; wait for it to end"
		return nil
	}
	text, line := m.fileText()
	f, err := os.CreateTemp("", "mdsh-*.md")
	if err == nil {
		_, err = f.WriteString(text)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		m.status = "edit: " + err.Error()
		return nil
	}
	editor := strings.Fields(m.editor())
	args := editor[1:]
	if takesLine(editor[0]) {
		args = append(args, fmt.Sprintf("+%d", line))
	}
	cmd := exec.Command(editor[0], append(args, f.Name())...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editedMsg{block: -1, file: f.Name(), text: text, err: err} })
}

// fileText is the file as it would be saved, and the line the block
// under the cursor starts on in it.
func (m *Model) fileText() (string, int) {
	text := string(render.Markdown(m.d, m.results, render.Options{WriteBack: true, Env: m.opts.Run.Environ}))
	line := 1
	if d, err := doc.Parse([]byte(text)); err == nil && m.cur() >= 0 && len(d.Blocks) == len(m.d.Blocks) {
		line = 1 + strings.Count(text[:d.Blocks[m.cur()].Span.Start], "\n")
	}
	return text, line
}

// takesLine reports whether editor opens a file at a line given as +N.
func takesLine(editor string) bool {
	switch filepath.Base(editor) {
	case "vi", "vim", "nvim", "gvim", "view", "nano", "pico", "emacs", "emacsclient", "micro", "kak", "joe", "ne", "mg", "jed", "mcedit":
		return true
	}
	return false
}

// editedAll puts the edited file into the document. Blocks whose code
// didn't change keep their output, wherever they moved; edited and new
// blocks start with none, as in mdsh-gui.
func (m *Model) editedAll(text string, was string) {
	if text == was {
		return
	}
	var b *doc.Block // the block under the cursor
	if i := m.cur(); i >= 0 {
		b = m.d.Blocks[i]
	}
	if err := m.d.SetSource([]byte(text)); err != nil {
		m.status = "edit: " + err.Error()
		return
	}
	if err := m.sess.Sync(); err != nil {
		m.status = "edited; " + err.Error()
	} else {
		m.status = "edited"
	}
	m.results = append([]*run.Result(nil), m.sess.Results()...)
	m.index = map[*doc.Block]int{}
	m.layout()
	m.cursor = min(m.cursor, max(len(m.nav)-1, 0))
	// The cursor stays on its block, or one with its id if it was edited.
	if i, ok := m.index[b]; ok && b != nil {
		m.moveTo(i)
	} else if b != nil {
		for _, i := range m.nav {
			if m.d.Blocks[i].ID == b.ID {
				m.moveTo(i)
				break
			}
		}
	}
	m.md = map[string]string{}
	m.dirty = true
	if m.opts.AutoSave {
		m.save(false)
	}
}

// edited puts the edited block, or file, into the document.
func (m *Model) edited(msg editedMsg) {
	defer os.Remove(msg.file)
	if msg.err != nil {
		m.status = "editor: " + msg.err.Error()
		return
	}
	text, err := os.ReadFile(msg.file)
	if err != nil {
		m.status = "edit: " + err.Error()
		return
	}
	if msg.block < 0 {
		m.editedAll(string(text), msg.text)
		return
	}
	b := m.d.Blocks[msg.block]
	if strings.TrimRight(string(text), "\n") == strings.TrimRight(m.d.BlockText(b), "\n") {
		return
	}
	if err := m.d.SetBlock(b, string(text)); err != nil {
		m.status = "edit: " + err.Error()
		return
	}
	m.md = map[string]string{}
	m.dirty = true
	m.status = "edited; enter runs it"
	if err := m.sess.Sync(); err != nil {
		m.status = "edited; " + err.Error()
	}
	if m.opts.AutoSave {
		m.save(false)
	}
}

// view shows the image of the block under the cursor full screen.
func (m *Model) view() tea.Cmd {
	i := m.cur()
	if i < 0 {
		return nil
	}
	link := m.results[i].Image
	if !m.results[i].Ran && m.d.Blocks[i].HasOutput() {
		b := m.d.Blocks[i]
		link = imageLink(saved(m.d.Src[b.Output.Start:b.Output.End]))
	}
	if link == "" {
		m.status = "this block has no image"
		return nil
	}
	img, err := termimg.Load(m.imagePath(link))
	if err != nil {
		m.status = "can't show " + link + ": " + err.Error()
		return nil
	}
	return tea.Exec(&viewer{img: img, protocol: m.opts.Protocol}, func(error) tea.Msg { return ttyBackMsg{} })
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
	m.sess.Sync()
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
		if m.prompt.of != nil {
			return
		}
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
	if m.diskChanged() && !force {
		m.changed = true
		m.status = "the file changed on disk; press w twice to overwrite it"
		m.forceArm = true
		return
	}
	// Saving the file itself names its unnamed blocks.
	renames, err := run.NameBlocks(m.d, m.results, m.opts.Run)
	if err == nil && len(renames) > 0 {
		err = m.sess.Sync()
	}
	if err == nil {
		err = m.write(m.opts.Run.File)
	}
	if err != nil {
		m.status = "save: " + err.Error()
		return
	}
	if !m.opts.AutoSave {
		m.status = "saved"
	}
}

// write saves the document with its results to file, which then is the
// file the TUI watches.
func (m *Model) write(file string) error {
	out := render.Markdown(m.d, m.results, render.Options{WriteBack: true, Env: m.opts.Run.Environ})
	if err := m.opts.Save(file, out); err != nil {
		return err
	}
	if info, err := os.Stat(file); err == nil {
		m.mtime = info.ModTime()
	}
	m.dirty, m.changed = false, false
	return nil
}

// keepMine answers the file-changed notice by keeping what is shown: the
// next save overwrites the file.
func (m *Model) keepMine() {
	if info, err := os.Stat(m.opts.Run.File); err == nil {
		m.mtime = info.ModTime()
	}
	m.changed = false
	m.dirty = true
	m.status = "kept yours; saving overwrites the file"
}

// named handles a key while the save as prompt is up. Enter saves to the
// file typed, asking again if it is another file that exists, and the
// TUI then follows the new file; esc cancels.
func (m *Model) named(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.naming = false
		m.saveAs.Blur()
		m.refresh()
		return nil
	case tea.KeyEnter:
		name := strings.TrimSpace(m.saveAs.Value())
		if name == "" {
			return nil
		}
		name = m.path(name)
		if _, err := os.Stat(name); err == nil && name != m.opts.Run.File && name != m.overwrite {
			m.overwrite = name
			m.status = filepath.Base(name) + " exists; enter again overwrites it"
			m.refresh()
			return nil
		}
		m.naming = false
		m.saveAs.Blur()
		return m.saveTo(name)
	}
	m.overwrite = ""
	var c tea.Cmd
	m.saveAs, c = m.saveAs.Update(k)
	return c
}

// saveTo saves to file and follows it. In another folder, blocks run
// there, so the file is opened again with fresh shells, as mdsh-gui does.
func (m *Model) saveTo(file string) tea.Cmd {
	defer m.refresh()
	if err := m.write(file); err != nil {
		m.status = "save: " + err.Error()
		return nil
	}
	moved := filepath.Dir(file) != filepath.Dir(m.opts.Run.File)
	m.opts.Run.File = file
	m.status = "saved as " + file
	if moved {
		return func() tea.Msg { return openMsg{file: file} }
	}
	return func() tea.Msg { return savedAsMsg{file: file} }
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
	m.width, m.height = w, h
	if !m.ready {
		m.vp = viewport.New(m.docWidth(), m.vpHeight())
		m.ready = true
	} else {
		m.vp.Width, m.vp.Height = m.docWidth(), m.vpHeight()
	}
	m.refresh()
}

// vpHeight is the height left for the document, under the banner, if
// one shows, and over the footer.
func (m *Model) vpHeight() int {
	h := m.height - footerLines
	if m.changed {
		h--
	}
	return max(h, 1)
}

// View draws the document above a status line and a key help line.
func (m *Model) View() string {
	if !m.ready {
		return "loading…"
	}
	view := m.vp.View()
	if m.form != nil {
		view = m.form.view(m.docWidth(), m.vp.Height)
	}
	if m.showOutline() {
		sep := lipgloss.NewStyle().Foreground(dimCol).Render(strings.TrimSuffix(strings.Repeat("│\n", m.vp.Height), "\n"))
		view = lipgloss.JoinHorizontal(lipgloss.Top, m.outline(m.vp.Height), sep, view)
	}
	view += "\n" + m.footer()
	if m.changed {
		view = bannerStyle.Width(m.width).Render("The file changed on disk.  R reload it · M keep yours") + "\n" + view
	}
	return view
}

func (m *Model) footer() string {
	name := m.tilde(m.opts.Run.File)
	state := ""
	if m.dirty {
		state = " " + dirtyStyle.Render("• not saved")
	}
	status := m.status
	if m.running {
		status = "running…"
	}
	if n := len(m.bg); n > 0 {
		if status != "" {
			status += " · "
		}
		status += fmt.Sprintf("%d in background", n)
	}
	rest := state
	if status != "" {
		rest += "  " + statusStyle.Render(status)
	}
	// A long path gives way to the status: it loses its start.
	if room := max(m.width-lipgloss.Width(rest), 12); len([]rune(name)) > room {
		r := []rune(name)
		name = "…" + string(r[len(r)-room+1:])
	}
	line1 := titleStyle.Render(name) + rest
	var help string
	switch {
	case m.typing:
		help = m.input.View()
	case m.naming:
		help = m.saveAs.View()
	case m.form != nil && m.formKind == "report":
		help = helpStyle.Render(formHelp + "save · esc cancel")
	case m.form != nil:
		help = helpStyle.Render(formHelp + "apply · esc cancel")
	case m.prompt != nil && m.prompt.of != nil:
		help = promptStyle.Render(fmt.Sprintf("%s depends on %s, which hasn't run. Run it first?  [r]un  s[k]ip  [s]top", m.d.Label(m.prompt.of), m.d.Label(m.prompt.block)))
	case m.prompt != nil:
		help = promptStyle.Render(m.name(m.prompt.block)+" is waiting for an answer:") + helpStyle.Render(" r run · k skip · o only blocks without saved output · s stop")
	case m.running:
		help = helpStyle.Render("j/k move · ctrl+c stop")
	default:
		help = helpStyle.Render("j/k move · enter run · a from here · i new · e edit · E edit file · v view · p report · , run options · o outline · C code · O output · w save · W save as · q " + m.quitWord())
	}
	return truncate(line1, m.width) + "\n" + truncate(help, m.width)
}

const footerLines = 2

// quitWord is what q does.
func (m *Model) quitWord() string {
	if m.back {
		return "close"
	}
	return "quit"
}
