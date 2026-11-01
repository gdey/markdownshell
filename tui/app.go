package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

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

// app is the TUI: a file, or the start screen. It is a tea.Model.
type app struct {
	c     Config
	doc   *Model // nil while the start screen shows
	start *start
	st    state
	size  tea.WindowSizeMsg
	// runOpts are the run options set for each file opened, which it
	// keeps when opened again.
	runOpts map[string]RunOptions
}

// openMsg asks the app to open a file, or the same one again.
type openMsg struct{ file string }

// backMsg asks the app to close the file and show the start screen.
type backMsg struct{}

// savedAsMsg says the file was saved under a new name.
type savedAsMsg struct{ file string }

func newApp(c Config) (*app, error) {
	a := &app{c: c, st: loadState(c.State)}
	if c.File != "" {
		return a, a.open(c.File)
	}
	a.start = newStart(c.Dir, a.st.Recent)
	return a, nil
}

// open opens file in place of whatever shows.
func (a *app) open(file string) error {
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
	m.back = a.c.File == ""
	a.close()
	a.doc, a.start = m, nil
	a.opened(m.opts.Run.File)
	return nil
}

func (a *app) close() {
	if a.doc != nil {
		a.doc.Close()
		a.doc = nil
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
	if a.doc != nil {
		return a.doc.Init()
	}
	return nil
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.size = msg
	case openMsg:
		if err := a.open(msg.file); err != nil {
			if a.doc != nil {
				a.doc.status = err.Error()
				a.doc.refresh()
			} else {
				a.start.status = err.Error()
			}
			return a, nil
		}
		a.doc.Update(a.size)
		return a, a.doc.Init()
	case backMsg:
		a.close()
		a.start = newStart(a.c.Dir, a.st.Recent)
		a.start.Update(a.size)
		return a, nil
	case savedAsMsg:
		a.opened(msg.file)
		return a, nil
	case runOptsMsg:
		if a.runOpts == nil {
			a.runOpts = map[string]RunOptions{}
		}
		a.runOpts[msg.file] = msg.ro
		return a, nil
	case viewMsg:
		a.st.View = View(msg)
		saveState(a.c.State, a.st)
		return a, nil
	}
	if a.doc != nil {
		_, cmd := a.doc.Update(msg)
		return a, cmd
	}
	_, cmd := a.start.Update(msg)
	return a, cmd
}

func (a *app) View() string {
	if a.doc != nil {
		return a.doc.View()
	}
	return a.start.View()
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
