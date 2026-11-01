package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/report"
	"github.com/gdey/markdownshell/run"
)

// form is a small form drawn in place of the document, as mdsh-gui's
// Report dialog and Run menu are: text fields, switches and choices.
// up and down move between fields, space and left/right change a
// switch or choice, enter submits and esc cancels.
type form struct {
	title  string
	fields []*field
	at     int
	// note is drawn under the fields, such as the matching command.
	note   func() string
	status string
	// overwrite is the existing file enter again overwrites.
	overwrite string
}

type fieldKind int

const (
	textField fieldKind = iota
	switchField
	choiceField
)

type field struct {
	label   string
	kind    fieldKind
	input   textinput.Model // a text field
	on      bool            // a switch
	choices []string        // a choice
	pick    int
	help    string
}

func newText(label, value, placeholder string) *field {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	in.SetValue(value)
	return &field{label: label, kind: textField, input: in}
}

// text is a text field's value, trimmed.
func (f *field) text() string { return strings.TrimSpace(f.input.Value()) }

// focus puts the cursor on field i.
func (f *form) focus(i int) tea.Cmd {
	f.at = min(max(i, 0), len(f.fields)-1)
	var cmd tea.Cmd
	for n, fl := range f.fields {
		if fl.kind != textField {
			continue
		}
		if n == f.at {
			cmd = fl.input.Focus()
			fl.input.CursorEnd()
		} else {
			fl.input.Blur()
		}
	}
	return cmd
}

// formKey is what a key did to a form.
type formKey int

const (
	formChanged formKey = iota
	formSubmit
	formCancel
)

// key handles a key. A key that changes a field also clears the status.
func (f *form) key(k tea.KeyMsg) (formKey, tea.Cmd) {
	fl := f.fields[f.at]
	switch k.String() {
	case "esc", "ctrl+c":
		return formCancel, nil
	case "enter":
		return formSubmit, nil
	case "down", "tab", "ctrl+n":
		return formChanged, f.focus(f.at + 1)
	case "up", "shift+tab", "ctrl+p":
		return formChanged, f.focus(f.at - 1)
	}
	f.status, f.overwrite = "", ""
	switch fl.kind {
	case textField:
		var cmd tea.Cmd
		fl.input, cmd = fl.input.Update(k)
		return formChanged, cmd
	case switchField:
		switch k.String() {
		case " ", "x", "left", "right", "h", "l":
			fl.on = !fl.on
		}
	case choiceField:
		switch k.String() {
		case " ", "right", "l":
			fl.pick = (fl.pick + 1) % len(fl.choices)
		case "left", "h":
			fl.pick = (fl.pick + len(fl.choices) - 1) % len(fl.choices)
		}
	}
	return formChanged, nil
}

var formLabelStyle = lipgloss.NewStyle().Foreground(dimCol)

// view draws the form w wide and h high.
func (f *form) view(w, h int) string {
	labelW := 0
	for _, fl := range f.fields {
		labelW = max(labelW, lipgloss.Width(fl.label))
	}
	lines := []string{titleStyle.Render(f.title), ""}
	for n, fl := range f.fields {
		mark, label := "  ", formLabelStyle.Render(fl.label)
		if n == f.at {
			mark, label = pickedStyle.Render("▸ "), pickedStyle.Render(fl.label)
		}
		label += strings.Repeat(" ", labelW-lipgloss.Width(fl.label)) + "  "
		var value string
		switch fl.kind {
		case textField:
			fl.input.Width = max(w-labelW-6, 10)
			value = fl.input.View()
		case switchField:
			value = "[ ]"
			if fl.on {
				value = "[x]"
			}
		case choiceField:
			var parts []string
			for i, c := range fl.choices {
				if i == fl.pick {
					c = pickedStyle.Render("(•) " + c)
				} else {
					c = helpStyle.Render("( ) " + c)
				}
				parts = append(parts, c)
			}
			value = strings.Join(parts, "  ")
		}
		lines = append(lines, truncate(mark+label+value, w))
		if fl.help != "" && n == f.at {
			lines = append(lines, truncate(strings.Repeat(" ", labelW+4)+helpStyle.Render(fl.help), w))
		}
	}
	if f.note != nil {
		lines = append(lines, "")
		for _, l := range strings.Split(f.note(), "\n") {
			lines = append(lines, truncate(l, w))
		}
	}
	if f.status != "" {
		lines = append(lines, "", truncate(stderrStyle.Render(f.status), w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
}

// formHelp is the key help while a form is up.
const formHelp = "↑/↓ field · space or ←/→ change · enter "

// The report form's fields, in order.
const (
	repFormat = iota
	repCode
	repOutput
	repFrom
	repDest
)

// Where a report's output comes from.
const (
	fromRun   = iota // this run, as shown
	fromAll          // run every block first
	fromSaved        // the file as saved, like mdsh -n
)

// reportForm opens the report form, as mdsh-gui's File › Create Report:
// PDF, HTML or markdown, with or without code and output, of this run,
// a run of every block or the file as saved. It writes beside the file
// by default.
func (m *Model) reportForm() tea.Cmd {
	if m.running {
		m.status = "a block is running; ctrl+c stops it"
		return nil
	}
	format := &field{label: "Format", kind: choiceField, choices: []string{"PDF", "HTML", "Markdown"}}
	code := &field{label: "Code", kind: switchField, on: !m.shown.HideCode, help: "show the code of runnable blocks"}
	output := &field{label: "Output", kind: switchField, on: !m.shown.HideOutput, help: "show the output of blocks"}
	ran, total := m.ran()
	this := fmt.Sprintf("this run (%d of %d ran)", ran, total)
	from := &field{label: "Output from", kind: choiceField, choices: []string{this, "run all first", "the saved file"}}
	dest := newText("Save to", m.tilde(m.reportDest(report.PDF)), "")
	f := &form{title: "Create report", fields: []*field{format, code, output, from, dest}}
	f.note = func() string {
		return helpStyle.Render("Same as: ") + m.reportCommand().String()
	}
	m.form, m.formKind = f, "report"
	m.refresh()
	return f.focus(0)
}

// reportFormat is the format picked in the report form.
func (m *Model) reportFormat() report.Format {
	return report.Formats[m.form.fields[repFormat].pick]
}

// reportDest is the report file beside the document, for format. A
// markdown report is name-report.md, since name.md is the document.
func (m *Model) reportDest(format report.Format) string {
	file := m.opts.Run.File
	base := strings.TrimSuffix(file, filepath.Ext(file))
	if format == report.Markdown {
		base += "-report"
	}
	return base + format.Ext()
}

// ran counts the runnable blocks that ran here, and all of them.
func (m *Model) ran() (ran, total int) {
	for i, b := range m.d.Blocks {
		if !run.Runnable(b) {
			continue
		}
		total++
		if m.results[i].Ran {
			ran++
		}
	}
	return ran, total
}

// reportCommand is the mdsh command for the report the form describes.
func (m *Model) reportCommand() report.Command {
	f := m.form.fields
	home, _ := os.UserHomeDir()
	ro := m.opts.Run
	return report.Command{
		File:        ro.File,
		Dest:        m.path(f[repDest].text()),
		Format:      m.reportFormat(),
		HideCode:    !f[repCode].on,
		HideOutput:  !f[repOutput].on,
		Run:         f[repFrom].pick == fromAll,
		Args:        ro.Args,
		Only:        ro.Only,
		Step:        ro.Step,
		StopOnError: ro.StopOnError,
		Home:        home,
	}
}

// reportChanged keeps the file's extension in step with the format,
// while it is the file beside the document.
func (m *Model) reportChanged(was report.Format) {
	dest := m.form.fields[repDest]
	if now := m.reportFormat(); now != was && m.path(dest.text()) == m.reportDest(was) {
		dest.input.SetValue(m.tilde(m.reportDest(now)))
		dest.input.CursorEnd()
	}
}

// submitReport writes the report, or runs every block first and writes
// it when they are done. It reports whether the form is done.
func (m *Model) submitReport() bool {
	f := m.form
	c := m.reportCommand()
	switch {
	case f.fields[repDest].text() == "":
		f.status = "name a file to save the report to"
		return false
	case c.Dest == c.File:
		f.status = "that is the document itself; pick another file"
		return false
	}
	if _, err := os.Stat(c.Dest); err == nil && c.Dest != f.overwrite {
		f.overwrite = c.Dest
		f.status = filepath.Base(c.Dest) + " exists; enter again overwrites it"
		return false
	}
	r := pendingReport{Command: c, saved: f.fields[repFrom].pick == fromSaved}
	if f.fields[repFrom].pick == fromAll {
		if len(m.nav) == 0 {
			m.writeReport(r)
			return true
		}
		m.moveTo(m.nav[0])
		m.report = &r
		m.start(true)
		return true
	}
	m.writeReport(r)
	return true
}

// pendingReport is a report to write, once a run of every block ends.
type pendingReport struct {
	report.Command
	saved bool // of the file as saved
}

// writeReport renders the report and writes it.
func (m *Model) writeReport(r pendingReport) {
	opts := report.Options{
		Format:     r.Format,
		HideCode:   r.HideCode,
		HideOutput: r.HideOutput,
		// The report's conditions see $MDSH_MODE report, so
		// show={{$MDSH_MODE}}!=report leaves a block out of it.
		Env:   run.WithMode(m.environ(), run.ModeReport),
		Title: report.Title(r.File),
		Dir:   filepath.Dir(r.File),
	}
	out, err := func() ([]byte, error) {
		if !r.saved {
			return report.Render(m.d, m.results, opts)
		}
		src, err := os.ReadFile(r.File)
		if err != nil {
			return nil, err
		}
		d, err := doc.Parse(src)
		if err != nil {
			return nil, err
		}
		return report.Render(d, run.Includes(d, run.Options{File: r.File}), opts)
	}()
	if err == nil {
		err = os.WriteFile(r.Dest, out, 0o644)
	}
	if err != nil {
		m.status = "report: " + err.Error()
		return
	}
	m.status = "report saved to " + m.tilde(r.Dest)
}

// environ is the environment blocks run in.
func (m *Model) environ() []string {
	if m.opts.Run.Environ == nil {
		return os.Environ()
	}
	return m.opts.Run.Environ
}

// The run options form's fields, in order.
const (
	optArgs = iota
	optOnly
	optStep
	optStop
)

// runOptionsForm opens the form for this file's run options, as
// mdsh-gui's Run menu and Arguments field: the arguments $1 and on,
// --only, --step and --stop-on-error.
func (m *Model) runOptionsForm() tea.Cmd {
	switch {
	case m.running:
		m.status = "a block is running; ctrl+c stops it"
		return nil
	case len(m.bg) > 0:
		m.status = "a block is running in the background; wait for it to end"
		return nil
	}
	ro := m.opts.Run
	args := newText("Arguments", run.JoinArgs(ro.Args), "$1 and on, quoted as in a shell")
	only := newText("Only", ro.Only, "a block's id: run it and the blocks before it in its env chain")
	step := &field{label: "Step", kind: switchField, on: ro.Step, help: "ask before every block"}
	stop := &field{label: "Stop on error", kind: switchField, on: ro.StopOnError, help: "stop at the first block that fails"}
	f := &form{title: "Run options", fields: []*field{args, only, step, stop}}
	f.note = func() string {
		return helpStyle.Render("Changing them starts new shells, as reopening the file would.")
	}
	m.form, m.formKind = f, "run"
	m.refresh()
	return f.focus(0)
}

// submitRunOptions applies the run options. It reports whether the form
// is done.
func (m *Model) submitRunOptions() bool {
	f := m.form.fields
	args, err := run.SplitArgs(f[optArgs].input.Value())
	if err != nil {
		m.form.status = "arguments: " + err.Error()
		return false
	}
	ro := m.opts.Run
	ro.Args, ro.Only, ro.Step, ro.StopOnError = args, f[optOnly].text(), f[optStep].on, f[optStop].on
	if err := m.setRunOptions(ro); err != nil {
		m.form.status = err.Error()
		return false
	}
	m.status = "run options set"
	return true
}

// setRunOptions starts a session with ro in place of the one there is.
// Output shown stays, but blocks run again in new shells.
func (m *Model) setRunOptions(ro run.Options) error {
	if m.running || len(m.bg) > 0 {
		return errors.New("a block is running; wait for it to end")
	}
	if ro.Only != "" {
		found := false
		for _, b := range m.d.Blocks {
			found = found || b.ID == ro.Only
		}
		if !found {
			return errors.New("no block has the id " + ro.Only)
		}
	}
	sess, err := m.newSession(ro)
	if err != nil {
		return err
	}
	m.sess.Close()
	m.sess, m.opts.Run = sess, ro
	m.optsChanged = true
	return nil
}

// formed handles a key while a form is up.
func (m *Model) formed(k tea.KeyMsg) tea.Cmd {
	var was report.Format
	if m.formKind == "report" {
		was = m.reportFormat()
	}
	what, cmd := m.form.key(k)
	done := false
	switch what {
	case formCancel:
		done = true
	case formSubmit:
		if m.formKind == "report" {
			done = m.submitReport()
		} else {
			done = m.submitRunOptions()
		}
	case formChanged:
		if m.formKind == "report" {
			m.reportChanged(was)
		}
	}
	if done {
		m.form = nil
		cmd = m.optsCmd()
	}
	m.refresh()
	return cmd
}

// optsCmd tells the app the run options changed, so the file keeps them
// when it is opened again.
func (m *Model) optsCmd() tea.Cmd {
	if !m.optsChanged {
		return nil
	}
	m.optsChanged = false
	o := runOptsMsg{file: m.opts.Run.File, ro: RunOptions{Args: m.opts.Run.Args, Only: m.opts.Run.Only, Step: m.opts.Run.Step, StopOnError: m.opts.Run.StopOnError}}
	return func() tea.Msg { return o }
}

// RunOptions are the run options the run options form sets.
type RunOptions struct {
	Args        []string
	Only        string
	Step        bool
	StopOnError bool
}

// runOptsMsg says file's run options changed.
type runOptsMsg struct {
	file string
	ro   RunOptions
}

// path resolves a file typed in a prompt: ~/ is the home folder, and
// relative paths are in the document's folder.
func (m *Model) path(name string) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			name = filepath.Join(home, name[2:])
		}
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(filepath.Dir(m.opts.Run.File), name)
	}
	return filepath.Clean(name)
}

// tilde writes a path under the home folder as ~/….
func (m *Model) tilde(name string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(name, home+string(filepath.Separator)) {
		return "~" + name[len(home):]
	}
	return name
}
