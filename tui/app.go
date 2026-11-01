package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
)

// Opener reads file and returns its document and the options to show it
// with. The TUI calls it to open a file from the start screen, to reload
// one and to follow one saved into another folder.
type Opener func(file string) (*doc.Document, Options, error)

// Config is what Run shows.
type Config struct {
	Open Opener
	// File is the file to open. Without one, the start screen lists the
	// markdown files under Dir, and q in a file goes back to it.
	File string
	Dir  string
	// State is the file that keeps the files opened most recently, which
	// the start screen lists first, and the view the last file was left
	// in. Empty keeps neither.
	State string
}

// Run shows the file, or the start screen, full screen until the user
// quits.
func Run(c Config) error {
	a, err := newApp(c)
	if err != nil {
		return err
	}
	defer a.close()
	var replies replyFilter
	_, err = tea.NewProgram(a, tea.WithAltScreen(), tea.WithFilter(replies.filter)).Run()
	return err
}

// MaxRecent is how many recent files are kept.
const MaxRecent = 10

// app is the TUI: tabs, as in mdsh-gui, each showing a file or the start
// screen. It is a tea.Model.
type app struct {
	c    Config
	tabs []*tab
	at   int // the tab in front
	st   state
	size tea.WindowSizeMsg
	// runOpts are the run options set for each file opened, which it
	// keeps when opened again.
	runOpts map[string]RunOptions
	// quitArmed is set after a quit with output not saved in another
	// tab: quitting again quits anyway.
	quitArmed bool
}

// tab shows a file, or the start screen.
type tab struct {
	doc   *Model // nil while the start screen shows
	start *start
	// back is set when q in the file goes back to the start screen.
	back bool
}

// tabMsg is a message from a tab's command, so it reaches that tab even
// when another is in front.
type tabMsg struct {
	t   *tab
	msg tea.Msg
}

// openMsg asks the app to open a file, or the same one again, in the
// tab it came from.
type openMsg struct{ file string }

// backMsg asks the app to close the tab's file and show the start
// screen.
type backMsg struct{}

// savedAsMsg says the file was saved under a new name.
type savedAsMsg struct{ file string }

// closeTabMsg asks the app to close the tab. The last tab quits.
type closeTabMsg struct{}

// nextTabMsg asks for the tab by, the next one (1) or the one before
// (-1).
type nextTabMsg struct{ by int }

// quitAllMsg is a tab's quit, which the app checks against the other
// tabs' unsaved output.
type quitAllMsg struct{}

func newApp(c Config) (*app, error) {
	a := &app{c: c, st: loadState(c.State)}
	t := &tab{back: c.File == ""}
	a.tabs = []*tab{t}
	if c.File != "" {
		return a, a.open(t, c.File)
	}
	t.start = newStart(c.Dir, a.st.Recent)
	return a, nil
}

// tab is the tab in front.
func (a *app) tab() *tab { return a.tabs[a.at] }

// open opens file in tab t in place of whatever it shows.
func (a *app) open(t *tab, file string) error {
	d, opts, err := a.c.Open(file)
	if err != nil {
		return err
	}
	opts.View = a.st.View
	if ro, ok := a.runOpts[opts.Run.File]; ok {
		opts.Run.Args, opts.Run.Only, opts.Run.Step, opts.Run.StopOnError = ro.Args, ro.Only, ro.Step, ro.StopOnError
	}
	m, err := New(d, opts)
	if err != nil {
		return err
	}
	m.back = t.back
	if t.doc != nil {
		t.doc.Close()
	}
	t.doc, t.start = m, nil
	a.opened(m.opts.Run.File)
	return nil
}

// close closes every tab's file.
func (a *app) close() {
	for _, t := range a.tabs {
		if t.doc != nil {
			t.doc.Close()
			t.doc = nil
		}
	}
}

// opened puts file first in the recent files.
func (a *app) opened(file string) {
	a.st.Recent = slices.DeleteFunc(a.st.Recent, func(f string) bool { return f == file })
	a.st.Recent = append([]string{file}, a.st.Recent...)
	if len(a.st.Recent) > MaxRecent {
		a.st.Recent = a.st.Recent[:MaxRecent]
	}
	saveState(a.c.State, a.st)
}

func (a *app) Init() tea.Cmd {
	if t := a.tab(); t.doc != nil {
		return a.wrap(t, t.doc.Init())
	}
	return nil
}

// wrap marks what cmd sends as coming from tab t. Bubble Tea's own
// messages, such as the one that runs an editor, pass as they are.
func (a *app) wrap(t *tab, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		switch msg := msg.(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			out := make(tea.BatchMsg, len(msg))
			for i, c := range msg {
				out[i] = a.wrap(t, c)
			}
			return out
		case tea.QuitMsg:
			return tabMsg{t, quitAllMsg{}}
		}
		if typ := reflect.TypeOf(msg); typ.PkgPath() == reflect.TypeOf(tea.QuitMsg{}).PkgPath() {
			return msg
		}
		return tabMsg{t, msg}
	}
}

// barHeight is the height of the tab bar: shown with more than one tab.
func (a *app) barHeight() int {
	if len(a.tabs) > 1 {
		return 1
	}
	return 0
}

// resize tells every tab the size it has under the tab bar.
func (a *app) resize() tea.Cmd {
	size := a.size
	size.Height = max(size.Height-a.barHeight(), 1)
	var cmds []tea.Cmd
	for _, t := range a.tabs {
		if t.doc != nil {
			_, cmd := t.doc.Update(size)
			cmds = append(cmds, a.wrap(t, cmd))
		} else {
			t.start.Update(size)
		}
	}
	return tea.Batch(cmds...)
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	t := a.tab()
	if m, ok := msg.(tabMsg); ok {
		t, msg = m.t, m.msg
		if !slices.Contains(a.tabs, t) {
			return a, nil // a tab closed since
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.size = msg
		return a, a.resize()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+t":
			return a, a.newTab()
		case "ctrl+c", "q", "ctrl+w", "esc":
		default:
			a.quitArmed = false
		}
	case openMsg:
		// A file open in another tab is shown there.
		for i, u := range a.tabs {
			if u != t && u.doc != nil && u.doc.opts.Run.File == msg.file {
				a.at = i
				return a, nil
			}
		}
		if err := a.open(t, msg.file); err != nil {
			if t.doc != nil {
				t.doc.status = err.Error()
				t.doc.refresh()
			} else {
				t.start.status = err.Error()
			}
			return a, nil
		}
		size := a.size
		size.Height = max(size.Height-a.barHeight(), 1)
		t.doc.Update(size)
		return a, a.wrap(t, t.doc.Init())
	case backMsg:
		if t.doc != nil {
			t.doc.Close()
		}
		t.doc, t.start = nil, newStart(a.c.Dir, a.st.Recent)
		return a, a.resize()
	case savedAsMsg:
		a.opened(msg.file)
		return a, nil
	case viewMsg:
		a.st.View = View(msg)
		saveState(a.c.State, a.st)
		return a, nil
	case runOptsMsg:
		if a.runOpts == nil {
			a.runOpts = map[string]RunOptions{}
		}
		a.runOpts[msg.file] = msg.ro
		return a, nil
	case closeTabMsg:
		return a, a.closeTab(t)
	case nextTabMsg:
		a.at = (a.at + msg.by + len(a.tabs)) % len(a.tabs)
		return a, nil
	case quitAllMsg:
		return a, a.quit(t)
	}
	if t.doc != nil {
		_, cmd := t.doc.Update(msg)
		return a, a.wrap(t, cmd)
	}
	_, cmd := t.start.Update(msg)
	return a, a.wrap(t, cmd)
}

// newTab opens a tab with the start screen in front.
func (a *app) newTab() tea.Cmd {
	t := &tab{start: newStart(a.c.Dir, a.st.Recent), back: true}
	a.tabs = slices.Insert(a.tabs, a.at+1, t)
	a.at++
	return a.resize()
}

// closeTab closes t. Closing the last tab quits.
func (a *app) closeTab(t *tab) tea.Cmd {
	i := slices.Index(a.tabs, t)
	if i < 0 {
		return nil
	}
	if len(a.tabs) == 1 {
		return a.quit(t)
	}
	if t.doc != nil {
		t.doc.Close()
	}
	a.tabs = slices.Delete(a.tabs, i, i+1)
	if a.at >= i && a.at > 0 {
		a.at--
	}
	return a.resize()
}

// quit quits, unless another tab than t has output not saved: then it
// shows that tab, and quitting again quits anyway.
func (a *app) quit(t *tab) tea.Cmd {
	if !a.quitArmed {
		for i, u := range a.tabs {
			if u != t && u.doc != nil && (u.doc.dirty || u.doc.running) {
				a.at, a.quitArmed = i, true
				u.doc.status = "output is not saved here; quit again to quit anyway"
				if u.doc.running {
					u.doc.status = "a block is running here; quit again to stop it and quit"
				}
				u.doc.refresh()
				return nil
			}
		}
	}
	return tea.Quit
}

var (
	tabStyle   = lipgloss.NewStyle().Foreground(dimCol).Padding(0, 1)
	frontStyle = lipgloss.NewStyle().Bold(true).Foreground(accent).Reverse(true).Padding(0, 1)
)

// bar draws the tab bar: each tab's file, with • when output isn't
// saved, … while a block runs and ? while one waits for an answer.
func (a *app) bar() string {
	var parts []string
	for i, t := range a.tabs {
		name := "Start"
		if m := t.doc; m != nil {
			name = filepath.Base(m.opts.Run.File)
			switch {
			case m.prompt != nil:
				name += " ?"
			case m.running || len(m.bg) > 0:
				name += " …"
			}
			if m.dirty {
				name += " •"
			}
		}
		style := tabStyle
		if i == a.at {
			style = frontStyle
		}
		parts = append(parts, style.Render(name))
	}
	return truncate(strings.Join(parts, ""), max(a.size.Width, 1))
}

func (a *app) View() string {
	t := a.tab()
	view := ""
	if t.doc != nil {
		view = t.doc.View()
	} else {
		view = t.start.View()
	}
	if len(a.tabs) > 1 {
		view = a.bar() + "\n" + view
	}
	return view
}

// state is what the TUI keeps between runs: the files opened most
// recently, newest first, and the view the last file was left in.
type state struct {
	Recent []string `json:"recent"`
	View   View     `json:"view"`
}

// loadState reads the state kept in file, leaving out recent files that
// are gone.
func loadState(file string) state {
	var st state
	if file == "" {
		return st
	}
	if b, err := os.ReadFile(file); err != nil || json.Unmarshal(b, &st) != nil {
		return state{}
	}
	st.Recent = slices.DeleteFunc(st.Recent, func(f string) bool {
		_, err := os.Stat(f)
		return err != nil
	})
	return st
}

// saveState keeps st in file. It is best effort: without it the start
// screen just won't list recent files, and files open in the usual view.
func saveState(file string, st state) {
	if file == "" {
		return
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, append(b, '\n'), 0o644)
}
