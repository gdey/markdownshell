package tui

import (
	"slices"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termsafe"
)

// srcPrompt is the question, on screen, whether to read the files the
// front matter's env.source names into the shells.
type srcPrompt struct {
	paths []string
	// then is what was asked for, done once the answer lets it go on.
	then func() tea.Cmd
}

// srcAnswer is the answer given about the env.source files, for the
// session's ApproveSources, which runs on the run's goroutine.
type srcAnswer struct {
	paths  []string
	answer run.Answer
}

// approveSources is the session's ApproveSources: the answer the TUI got
// for paths before it let anything start a shell (see askSources). The
// question can't be asked here, since a shell also starts from Update,
// as History asks for its prompt.
func (m *Model) approveSources(paths []string) run.Answer {
	m.srcMu.Lock()
	defer m.srcMu.Unlock()
	if m.src != nil && slices.Equal(m.src.paths, paths) {
		return m.src.answer
	}
	return run.Stop
}

func (m *Model) setSources(paths []string, a run.Answer) {
	m.srcMu.Lock()
	m.src = &srcAnswer{paths: paths, answer: a}
	m.srcMu.Unlock()
}

// askSources asks, before something would start a shell, whether to
// read in the files env.source names, unless that is answered already:
// in this session, by --yes, or for this file before (see package
// trust). It reports whether it asked; then runs once the answer is
// yes or no, and not at all on stop.
func (m *Model) askSources(then func() tea.Cmd) bool {
	paths, err := run.SourcePaths(m.d, m.opts.Run)
	if err != nil || len(paths) == 0 {
		// A bad path is the session's error to report.
		return false
	}
	m.srcMu.Lock()
	known := m.src != nil && slices.Equal(m.src.paths, paths)
	m.srcMu.Unlock()
	switch {
	case known:
		return false
	case m.opts.Yes:
		m.setSources(paths, run.Go)
		return false
	}
	if allow, ok := m.opts.Trust.Lookup(m.opts.Run.File, paths); ok {
		m.setSources(paths, map[bool]run.Answer{true: run.Go, false: run.Skip}[allow])
		return false
	}
	m.srcAsk = &srcPrompt{paths: paths, then: then}
	m.helping = false
	m.status = ""
	if m.ready {
		m.resize(m.width, m.height)
	}
	return true
}

// answerSources handles a key while the env.source question is up.
// Enter stops, as for a confirm, since it is the safe choice.
func (m *Model) answerSources(key string) tea.Cmd {
	var a run.Answer
	switch key {
	case "y", "r":
		a = run.Go
	case "n", "k":
		a = run.Skip
	case "s", "enter", "esc", "ctrl+c", "q":
		a = run.Stop
	default:
		return nil
	}
	q := m.srcAsk
	m.srcAsk = nil
	if m.ready {
		m.resize(m.width, m.height)
	}
	if a == run.Stop {
		m.status = "not run: the env.source files weren't approved"
		return nil
	}
	m.setSources(q.paths, a)
	_ = m.opts.Trust.Remember(m.opts.Run.File, q.paths, a == run.Go)
	return q.then()
}

// srcBanner is the lines over the document while the env.source
// question is up: the files, one a line, as many as fit.
func (m *Model) srcBanner() []string {
	if m.srcAsk == nil {
		return nil
	}
	lines := []string{"This file reads these files into its shell before anything runs:"}
	limit := max(m.height/3, 2)
	for i, p := range m.srcAsk.paths {
		if i == limit-1 && len(m.srcAsk.paths) > limit {
			lines = append(lines, "  … and "+strconv.Itoa(len(m.srcAsk.paths)-i)+" more, in the front matter")
			break
		}
		lines = append(lines, "  "+keepEnd(termsafe.Visible(m.tilde(p)), m.width-4))
	}
	return lines
}

// keepEnd cuts s to n columns from the left, so a long path keeps its
// file name, which matters most, and loses its leading folders.
func keepEnd(s string, n int) string {
	if n < 2 || ansi.StringWidth(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && ansi.StringWidth(string(r))+1 > n {
		r = r[1:]
	}
	return "…" + string(r)
}
