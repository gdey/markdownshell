package tui

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdey/markdownshell/library"
)

// start is the start screen: the markdown files under a folder, grouped
// by folder, with the recent ones first, as in mdsh-gui. Typing filters
// them by path and description, and enter opens the one picked.
type start struct {
	dir    string
	home   string
	files  []library.File
	full   bool
	recent []string
	err    error
	status string

	filter textinput.Model
	rows   []startRow // the rows the filter matches
	cursor int        // index into rows
	top    int        // first line shown
	width  int
	height int
}

// startRow is a file on the start screen, under heading when it starts a
// group.
type startRow struct {
	heading string
	path    string
	name    string
	detail  string
	desc    string
}

func newStart(dir string, recent []string) *start {
	s := &start{dir: dir, recent: recent}
	if abs, err := filepath.Abs(dir); err == nil {
		s.dir = abs
	}
	s.home, _ = os.UserHomeDir()
	s.files, s.full, s.err = library.List(s.dir)
	// Files in the folder itself come first, then each folder's together.
	slices.SortStableFunc(s.files, func(a, b library.File) int {
		da, db := path.Dir(a.Rel), path.Dir(b.Rel)
		return cmp.Or(cmp.Compare(btoi(da != "."), btoi(db != ".")), strings.Compare(da, db))
	})
	s.filter = textinput.New()
	s.filter.Prompt = "/ "
	s.filter.Placeholder = "filter by name or description"
	s.filter.Focus()
	s.match()
	return s
}

// match picks the rows the filter matches.
func (s *start) match() {
	q := strings.ToLower(strings.TrimSpace(s.filter.Value()))
	has := func(t string) bool { return strings.Contains(strings.ToLower(t), q) }
	s.rows = nil
	heading := "Recent"
	for _, p := range s.recent {
		if !has(p) {
			continue
		}
		s.rows = append(s.rows, startRow{heading: heading, path: p, name: filepath.Base(p), detail: s.tilde(filepath.Dir(p))})
		heading = ""
	}
	group := "\x00"
	for _, f := range s.files {
		if !has(f.Rel) && !has(f.Description) {
			continue
		}
		dir, name := "", f.Rel
		if i := strings.LastIndex(f.Rel, "/"); i >= 0 {
			dir, name = f.Rel[:i], f.Rel[i+1:]
		}
		r := startRow{path: f.Path, name: name, detail: blocksLabel(f.Blocks) + " · " + ago(f.Mod), desc: f.Description}
		if dir != group {
			group, r.heading = dir, "This folder"
			if dir != "" {
				r.heading = dir + "/"
			}
		}
		s.rows = append(s.rows, r)
	}
	s.cursor = min(s.cursor, max(len(s.rows)-1, 0))
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *start) tilde(path string) string {
	if s.home != "" && (path == s.home || strings.HasPrefix(path, s.home+string(filepath.Separator))) {
		return "~" + path[len(s.home):]
	}
	return path
}

func blocksLabel(n int) string {
	switch n {
	case -1:
		return "can't read"
	case 1:
		return "1 block"
	}
	return fmt.Sprintf("%d blocks", n)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 90*time.Second:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Round(time.Minute)/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Round(time.Hour)/time.Hour))
	case d < 30*24*time.Hour:
		if n := int(d.Round(24*time.Hour) / (24 * time.Hour)); n > 1 {
			return fmt.Sprintf("%d days ago", n)
		}
		return "1 day ago"
	}
	return t.Format("Jan 2, 2006")
}

func (s *start) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		s.filter.Width = max(msg.Width-4, 10)
	case tea.KeyMsg:
		s.status = ""
		switch msg.String() {
		case "ctrl+c":
			return s, tea.Quit
		case "esc":
			if s.filter.Value() == "" {
				return s, tea.Quit
			}
			s.filter.SetValue("")
			s.match()
		case "down", "ctrl+n", "ctrl+j", "tab":
			s.cursor = min(s.cursor+1, max(len(s.rows)-1, 0))
		case "up", "ctrl+p", "ctrl+k", "shift+tab":
			s.cursor = max(s.cursor-1, 0)
		case "pgdown":
			s.cursor = min(s.cursor+max(s.listHeight()/3, 1), max(len(s.rows)-1, 0))
		case "pgup":
			s.cursor = max(s.cursor-max(s.listHeight()/3, 1), 0)
		case "enter":
			if len(s.rows) > 0 {
				file := s.rows[s.cursor].path
				return s, func() tea.Msg { return openMsg{file: file} }
			}
		default:
			var cmd tea.Cmd
			s.filter, cmd = s.filter.Update(msg)
			s.cursor = 0
			s.match()
			return s, cmd
		}
	}
	return s, nil
}

func (s *start) Init() tea.Cmd { return nil }

// header and footer lines around the list.
const startHeader, startFooter = 3, 2

func (s *start) listHeight() int { return max(s.height-startHeader-startFooter, 1) }

var (
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(dimCol)
	pickedStyle  = lipgloss.NewStyle().Bold(true).Foreground(accent)
)

func (s *start) View() string {
	w := max(s.width, 20)
	var b strings.Builder
	where := s.tilde(s.dir)
	if s.full {
		where += fmt.Sprintf(" (the first %d files)", len(s.files))
	}
	b.WriteString(titleStyle.Render("mdsh") + "  " + statusStyle.Render(where) + "\n")
	b.WriteString(s.filter.View() + "\n\n")

	// Draw every row, noting where the picked one is, then show the
	// lines that keep it in view.
	var lines []string
	at, end := 0, 0
	for i, r := range s.rows {
		if r.heading != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, headingStyle.Render(r.heading))
		}
		if i == s.cursor {
			at = len(lines)
		}
		mark, name := "  ", r.name
		if i == s.cursor {
			mark, name = pickedStyle.Render("▸ "), pickedStyle.Render(name)
		}
		gap := max(w-2-lipgloss.Width(r.name)-lipgloss.Width(r.detail)-1, 1)
		lines = append(lines, ansi.Truncate(mark+name+strings.Repeat(" ", gap)+statusStyle.Render(r.detail), w, "…"))
		if r.desc != "" {
			lines = append(lines, "    "+helpStyle.Render(ansi.Truncate(r.desc, w-5, "…")))
		}
		if i == s.cursor {
			end = len(lines)
		}
	}
	h := s.listHeight()
	if at < s.top {
		s.top = at
	}
	if end > s.top+h {
		s.top = end - h
	}
	if s.cursor == 0 {
		s.top = 0
	}
	s.top = max(min(s.top, len(lines)-h), 0)
	shown := lines[s.top:min(s.top+h, len(lines))]
	switch {
	case s.err != nil:
		shown = []string{stderrStyle.Render(s.err.Error())}
	case len(s.rows) == 0 && s.filter.Value() != "":
		shown = []string{helpStyle.Render("No file matches.")}
	case len(s.rows) == 0:
		shown = []string{helpStyle.Render("No markdown files here.")}
	}
	b.WriteString(strings.Join(shown, "\n"))
	for range h - len(shown) {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if s.status != "" {
		b.WriteString(stderrStyle.Render(s.status))
	}
	b.WriteString("\n" + helpStyle.Render("type to filter · ↑/↓ pick · enter open · esc clear, or quit"))
	return b.String()
}
