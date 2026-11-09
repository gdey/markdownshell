package tui

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/history"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termsafe"
)

// The History pane, as in mdsh-gui, is a terminal along the bottom of the
// screen, like a paper tape: a command typed there runs in the file's
// shell, on a terminal, and is added to the end of the file as a block,
// #history-N, with its output. A full-screen command, such as vi, is kept
// as a tty block with its recording. A line starting with # is added as
// prose.

// hist is the History pane's state.
type hist struct {
	open bool
	// focus is set while keys go to the pane.
	focus bool
	input textinput.Model
	// lines are the lines typed so far of a command that goes on.
	lines []string
	// log is what the pane has shown, oldest first.
	log    []string
	prompt history.Prompt
	// recall are the commands that up and down bring back, and recallAt
	// the one shown, len(recall) for none.
	recall   []string
	recallAt int
	// running is set while a command typed in the pane runs on the
	// terminal, which the pane then shows.
	running bool
	// noteEnd is where the last note added ended the file, so the next
	// note carries on its paragraph; -1 for none.
	noteEnd int
	// killed is what the last of ctrl+w, ctrl+u, ctrl+k and alt+d took,
	// for ctrl+y; killing is set when the last key killed, so the next
	// kill adds to it, as in readline.
	killed  string
	killing bool
	// sel, when set, is the lines selected in log, to copy.
	sel *histSel
}

// maxLog is the most lines the pane keeps.
const maxLog = 1000

// histDone is how a command typed in the pane ended.
type histDone struct {
	block      *doc.Block
	result     *run.Result
	fullScreen bool
}

// histHeight is the height of the History pane, its rule included, or 0
// while it is closed.
func (m *Model) histHeight() int {
	if !m.hist.open {
		return 0
	}
	return min(max(m.height/3, minPaneRows+3), max(m.height-footerLines-3, 2))
}

// histRows is how many rows of the pane show the terminal.
func (m *Model) histRows() int { return max(m.histHeight()-1, 1) }

// shellPath is the shell blocks run in.
func (m *Model) shellPath() string { return history.Shell(m.d, m.opts.Run.Shell) }

func (m *Model) zsh() bool { return history.Lang(m.shellPath()) == "zsh" }

// historyEnv is the environment the shells start with while the pane is
// open. Their terminal is the pane, which draws as an xterm does.
func historyEnv(environ []string) []string {
	if environ == nil {
		environ = os.Environ()
	}
	environ = slices.DeleteFunc(slices.Clone(environ), func(kv string) bool { return strings.HasPrefix(kv, "TERM=") })
	return append(environ, "TERM=xterm-256color")
}

// openHistory opens the History pane, with the keys going to it.
// Commands typed there run on a terminal, so the shells start again,
// with terminals of their own, unless they have them already.
func (m *Model) openHistory() tea.Cmd {
	if m.hist.open {
		m.hist.focus = true
		return m.hist.input.Focus()
	}
	if m.running {
		m.status = "a block is running; ctrl+c stops it"
		return nil
	}
	// Opening History starts the shell, to ask for its prompt.
	if m.askSources(m.openHistory) {
		return nil
	}
	m.hist.open = true
	if !m.sess.PTY() {
		sess, err := m.newSession(m.opts.Run)
		if err != nil {
			m.hist.open = false
			m.status = "can't open History: " + err.Error()
			return nil
		}
		m.sess.Close()
		m.sess = sess
		for i, r := range m.results {
			if r.Ran && run.Runnable(m.d.Blocks[i]) {
				m.histLog(helpStyle.Render("(The shells started again, on a terminal, so variables that blocks set are gone.)"))
				break
			}
		}
	}
	m.hist.focus = true
	m.hist.noteEnd = -1
	m.hist.recall = nil
	for _, b := range m.d.Blocks {
		if strings.HasPrefix(b.ID, "history-") {
			m.hist.recall = append(m.hist.recall, strings.TrimSuffix(b.Body, "\n"))
		}
	}
	m.hist.recallAt = len(m.hist.recall)
	m.askPrompt()
	if m.ready {
		m.resize(m.width, m.height)
	}
	return m.hist.input.Focus()
}

// closeHistory closes the pane. The shells keep their terminals.
func (m *Model) closeHistory() {
	m.hist.sel = nil
	m.hist.open, m.hist.focus = false, false
	m.hist.lines = nil
	m.hist.input.Reset()
	m.hist.input.Blur()
	if m.ready {
		m.resize(m.width, m.height)
	}
}

// askPrompt asks the shell for its prompts.
func (m *Model) askPrompt() {
	m.hist.prompt = history.Prompt{}
	if m.running {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if p, err := history.AskPrompt(ctx, m.sess, m.zsh()); err == nil {
		m.hist.prompt = p
	}
	m.setPrompt()
}

// ps1 and ps2 are the shell's prompts, or $ and > for a shell with none.
// A block can export PS1, so only its color escapes reach the terminal;
// the rest, such as shell-integration marks, are dropped.
func (m *Model) ps1() string {
	if m.hist.prompt.PS1 == "" {
		return "$ "
	}
	return termsafe.SGROnly(m.hist.prompt.PS1)
}

func (m *Model) ps2() string {
	if m.hist.prompt.PS2 == "" {
		return "> "
	}
	return termsafe.SGROnly(m.hist.prompt.PS2)
}

// setPrompt puts the prompt before what is typed: PS1, or PS2 on a line
// that carries on a command. Only the last line of a prompt of several
// lines goes there; the others show above it.
func (m *Model) setPrompt() {
	p := m.ps1()
	if len(m.hist.lines) > 0 {
		p = m.ps2()
	}
	m.hist.input.Prompt = lastLine(p)
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// histLog adds lines to what the pane shows.
func (m *Model) histLog(lines ...string) {
	for _, l := range lines {
		m.hist.log = append(m.hist.log, strings.Split(strings.ReplaceAll(l, "\t", "    "), "\n")...)
	}
	if n := len(m.hist.log) - maxLog; n > 0 {
		m.hist.log = slices.Delete(m.hist.log, 0, n)
		if s := m.hist.sel; s != nil {
			s.anchor, s.cur = max(s.anchor-n, 0), max(s.cur-n, 0)
		}
	}
}

// histKey handles a key while the pane has the keys.
func (m *Model) histKey(k tea.KeyMsg) tea.Cmd {
	h := &m.hist
	appending := h.killing
	h.killing = false
	switch k.String() {
	case "esc":
		h.focus = false
		h.input.Blur()
		return nil
	case "ctrl+d":
		// With nothing typed it closes the pane, as it ends a shell;
		// otherwise it deletes the character under the cursor.
		if h.input.Value() == "" && len(h.lines) == 0 {
			m.closeHistory()
			return nil
		}
	case "ctrl+y":
		if h.killed != "" {
			v, at := []rune(h.input.Value()), h.input.Position()
			h.input.SetValue(string(v[:at]) + h.killed + string(v[at:]))
			h.input.SetCursor(at + len([]rune(h.killed)))
		}
		return nil
	case "ctrl+l":
		// It clears the pane, as it clears a terminal.
		h.log = nil
		return nil
	case "ctrl+c":
		if h.input.Value() != "" || len(h.lines) > 0 {
			m.echo(h.input.Value() + "^C")
			h.lines = nil
			h.input.Reset()
			m.setPrompt()
		}
		return nil
	case "up", "down", "ctrl+p", "ctrl+n":
		by := -1
		if k.String() == "down" || k.String() == "ctrl+n" {
			by = 1
		}
		if at := h.recallAt + by; at >= 0 && at <= len(h.recall) {
			h.recallAt = at
			v := ""
			if at < len(h.recall) {
				v = h.recall[at]
			}
			// A command of several lines comes back with its last line
			// to edit.
			lines := strings.Split(v, "\n")
			h.lines = lines[:len(lines)-1]
			m.setPrompt()
			h.input.SetValue(lines[len(lines)-1])
			h.input.CursorEnd()
		}
		return nil
	case "tab":
		m.complete()
		return nil
	case "shift+up":
		if why := m.selectHistory(); why != "" {
			m.status = why
		}
		return nil
	case "enter":
		m.entered()
		return nil
	}
	before, at := []rune(h.input.Value()), h.input.Position()
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(k)
	switch k.String() {
	case "ctrl+w", "alt+backspace", "ctrl+u", "ctrl+k", "alt+d", "alt+delete":
		h.kill(before, at, appending)
	}
	return cmd
}

// kill keeps what the key just took out of the input, which held before
// with the cursor at at, for ctrl+y. A kill after a kill adds to what
// it took: at the end for a kill forward, at the start for one back.
func (h *hist) kill(before []rune, at int, appending bool) {
	gone := len(before) - len([]rune(h.input.Value()))
	if gone <= 0 {
		h.killing = appending
		return
	}
	var text string
	backward := h.input.Position() < at
	if backward {
		text = string(before[at-gone : at])
	} else {
		text = string(before[at : at+gone])
	}
	switch {
	case !appending:
		h.killed = text
	case backward:
		h.killed = text + h.killed
	default:
		h.killed += text
	}
	h.killing = true
}

// echo adds what is typed, the lines before line and line, to the log,
// after their prompts.
func (m *Model) echo(line string) {
	p := m.ps1()
	for _, l := range append(slices.Clone(m.hist.lines), line) {
		m.histLog(p + l)
		p = m.ps2()
	}
}

// entered takes enter: a whole command runs, and one that goes on gets
// another line.
func (m *Model) entered() {
	h := &m.hist
	line := h.input.Value()
	text := strings.Join(append(slices.Clone(h.lines), line), "\n")
	if !history.Whole(m.shellPath(), text) {
		h.lines = append(h.lines, line)
		h.input.Reset()
		m.setPrompt()
		return
	}
	if m.running {
		m.status = "a block is running; ctrl+c stops it"
		return
	}
	m.echo(line)
	h.lines = nil
	h.input.Reset()
	m.setPrompt()
	if strings.TrimSpace(text) == "" {
		return
	}
	if note, ok := history.Note(text); ok {
		m.addNote(note)
		return
	}
	h.recall = append(h.recall, text)
	h.recallAt = len(h.recall)
	m.runTyped(text)
}

// addNote adds a note to the end of the file as prose.
func (m *Model) addNote(text string) {
	carry := m.hist.noteEnd == len(m.d.Src)
	if err := history.AddNote(m.d, text, carry); err != nil {
		m.histLog(stderrStyle.Render(err.Error()))
		return
	}
	m.sess.Sync()
	m.hist.noteEnd = len(m.d.Src)
	m.layout()
	m.dirty = true
	m.md = map[string]string{}
	m.refresh()
	m.vp.GotoBottom()
}

// runTyped adds text to the end of the file as a #history-N block and
// runs it on the terminal the pane shows, or, ending in a lone &, in the
// background.
func (m *Model) runTyped(text string) {
	if m.askBeforeRun(func() { m.runTyped(text) }) || m.askSources(func() tea.Cmd { m.runTyped(text); return nil }) {
		return
	}
	body, bg := history.Background(text)
	info := history.Info(history.Lang(m.shellPath()), history.NextID(m.d), bg)
	b, err := m.d.AppendBlock(info, body)
	if err == nil {
		err = m.sess.Sync()
	}
	if err != nil {
		m.histLog(stderrStyle.Render("can't add the command to the file: " + err.Error()))
		return
	}
	m.hist.noteEnd = -1
	m.results = append(m.results, m.sess.Results()[len(m.results):]...)
	m.layout()
	m.dirty = true
	i := m.index[b]
	m.moveTo(i)
	ctx, cancel := context.WithCancel(context.Background())
	m.running, m.cancel, m.status = true, cancel, ""
	m.skipSaved = false
	if bg {
		go func() {
			_, err := m.sess.RunBlock(ctx, i)
			m.events <- runDoneMsg{err: err}
		}()
		return
	}
	m.hist.running = true
	m.fit()
	go func() {
		r, full, err := m.sess.RunTyped(ctx, i)
		m.events <- runDoneMsg{err: err, hist: &histDone{block: b, result: r, fullScreen: full}}
	}()
}

// typedDone ends a command typed in the pane: one that took the screen
// is marked tty, so its output is a recording, and what it showed goes
// in the pane's log.
func (m *Model) typedDone(d *histDone) {
	m.hist.running = false
	m.fit()
	b, r := d.block, d.result
	if d.fullScreen {
		text := b.Fence + strings.TrimSuffix(b.Info, "}") + " tty}\n" + b.Body + b.Fence + "\n"
		if err := m.d.SetBlock(b, text); err != nil {
			m.histLog(stderrStyle.Render(err.Error()))
		}
		m.sess.Sync()
		m.layout()
		m.dirty = true
		if m.opts.AutoSave {
			m.save(false)
		}
		m.histLog(helpStyle.Render("(full screen: kept as a recording)"))
	} else if r != nil {
		if out := strings.TrimRight(clean(string(r.Stdout)), "\n"); out != "" {
			m.histLog(links(b, out))
		}
	}
	if r != nil && r.Err != nil {
		m.histLog(stderrStyle.Render(r.Err.Error()))
	}
}

// complete completes the word before the cursor, asking the shell. One
// match takes the word's place; more fill in what they share, or show
// below.
func (m *Model) complete() {
	if m.running {
		return
	}
	h := &m.hist
	line := h.input.Value()
	pos := min(h.input.Position(), len(line))
	before := strings.Join(append(slices.Clone(h.lines), line[:pos]), "\n")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c := history.Complete(ctx, m.sess, before, m.zsh())
	if len(c.Items) == 0 || len(c.Word) > pos {
		return
	}
	put := ""
	switch {
	case len(c.Items) == 1:
		put = c.Items[0]
		if !strings.HasSuffix(put, "/") {
			put += " "
		}
	default:
		put = shared(c.Items)
		if len(put) <= len(c.Word) {
			m.echo(line)
			m.histLog(columns(c.Items, max(m.width, 20)))
			return
		}
	}
	start := pos - len(c.Word)
	h.input.SetValue(line[:start] + put + line[pos:])
	h.input.SetCursor(start + len(put))
}

// shared is the start all of items share.
func shared(items []string) string {
	p := items[0]
	for _, s := range items[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

// columns lays items out in rows w wide.
func columns(items []string, w int) string {
	wide := 0
	for _, s := range items {
		wide = max(wide, lipgloss.Width(s))
	}
	wide += 2
	per := max(w/wide, 1)
	var out strings.Builder
	for i, s := range items {
		if i > 0 && i%per == 0 {
			out.WriteByte('\n')
		}
		out.WriteString(s)
		if i%per != per-1 && i != len(items)-1 {
			out.WriteString(strings.Repeat(" ", wide-lipgloss.Width(s)))
		}
	}
	return out.String()
}

var histRule = lipgloss.NewStyle().Foreground(dimCol)

// histView draws the pane: a rule, then the terminal while a command
// typed there runs, else the end of the log and the prompt.
func (m *Model) histView() string {
	rows := m.histRows()
	title := " History "
	hint := " esc back to the file · ctrl+d close "
	if !m.hist.focus {
		hint = " h type here "
	}
	if m.hist.running {
		hint = " keys go to the command · ctrl+] stops it "
	}
	if m.hist.sel != nil {
		hint = " ↑/↓ select · y copy · esc cancel "
	}
	fill := max(m.width-lipgloss.Width(title)-lipgloss.Width(hint)-2, 1)
	rule := histRule.Render("──"+title+strings.Repeat("─", fill)) + helpStyle.Render(hint)
	var lines []string
	if m.hist.sel != nil {
		lines = m.selView(rows)
	} else if m.hist.running {
		lines = strings.Split(m.pane.screen(), "\n")
	} else {
		lines = slices.Clone(m.hist.log)
		// The prompt's lines before its last show above what is typed.
		p := m.ps1()
		for _, l := range m.hist.lines {
			lines = append(lines, strings.Split(p+l, "\n")...)
			p = m.ps2()
		}
		if i := strings.LastIndexByte(p, '\n'); i >= 0 {
			lines = append(lines, strings.Split(p[:i], "\n")...)
		}
		lines = append(lines, m.hist.input.View())
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	for i, l := range lines {
		lines[i] = truncate(l, m.width)
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return truncate(rule, m.width) + "\n" + strings.Join(lines, "\n")
}
