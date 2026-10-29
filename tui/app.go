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
	// Recent is the file that keeps the files opened most recently, which
	// the start screen lists first. Empty keeps none.
	Recent string
}

// Run shows the file, or the start screen, full screen until the user
// quits.
func Run(c Config) error {
	a, err := newApp(c)
	if err != nil {
		return err
	}
	defer a.close()
	_, err = tea.NewProgram(a, tea.WithAltScreen()).Run()
	return err
}

// MaxRecent is how many recent files are kept.
const MaxRecent = 10

// app is the TUI: a file, or the start screen. It is a tea.Model.
type app struct {
	c      Config
	doc    *Model // nil while the start screen shows
	start  *start
	recent []string
	size   tea.WindowSizeMsg
}

// openMsg asks the app to open a file, or the same one again.
type openMsg struct{ file string }

// backMsg asks the app to close the file and show the start screen.
type backMsg struct{}

// savedAsMsg says the file was saved under a new name.
type savedAsMsg struct{ file string }

func newApp(c Config) (*app, error) {
	a := &app{c: c, recent: loadRecent(c.Recent)}
	if c.File != "" {
		return a, a.open(c.File)
	}
	a.start = newStart(c.Dir, a.recent)
	return a, nil
}

// open opens file in place of whatever shows.
func (a *app) open(file string) error {
	d, opts, err := a.c.Open(file)
	if err != nil {
		return err
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
	a.recent = slices.DeleteFunc(a.recent, func(f string) bool { return f == file })
	a.recent = append([]string{file}, a.recent...)
	if len(a.recent) > MaxRecent {
		a.recent = a.recent[:MaxRecent]
	}
	saveRecent(a.c.Recent, a.recent)
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
		a.start = newStart(a.c.Dir, a.recent)
		a.start.Update(a.size)
		return a, nil
	case savedAsMsg:
		a.opened(msg.file)
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

// loadRecent reads the recent files kept in file, leaving out those gone.
func loadRecent(file string) []string {
	var r struct{ Recent []string }
	if file == "" {
		return nil
	}
	if b, err := os.ReadFile(file); err != nil || json.Unmarshal(b, &r) != nil {
		return nil
	}
	return slices.DeleteFunc(r.Recent, func(f string) bool {
		_, err := os.Stat(f)
		return err != nil
	})
}

// saveRecent keeps the recent files in file. It is best effort: the
// start screen just won't list them.
func saveRecent(file string, recent []string) {
	if file == "" {
		return
	}
	b, err := json.MarshalIndent(struct {
		Recent []string `json:"recent"`
	}{recent}, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, append(b, '\n'), 0o644)
}
