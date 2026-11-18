package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeLine types s into the History pane and presses enter.
func typeLine(m *Model, s string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestHistoryPane(t *testing.T) {
	src := "# Log\n\n```sh\nX=1; cd /\n```\n"
	m, file := newModel(t, src, Options{AutoSave: true})
	press(m, "enter")
	pump(t, m)
	press(m, "h")
	if !m.hist.open || !m.hist.focus || !m.sess.PTY() {
		t.Fatalf("open %v, focus %v, pty %v", m.hist.open, m.hist.focus, m.sess.PTY())
	}
	if !strings.Contains(strings.Join(m.hist.log, "\n"), "started again") {
		t.Errorf("no word that the shells started again: %q", m.hist.log)
	}
	if h := m.vpHeight(); h >= 40-footerLines-1 {
		t.Errorf("the document keeps its height, %d, with the pane open", h)
	}

	// A command is added as a block and runs on the pane's terminal.
	typeLine(m, "echo hi${X-gone}")
	pump(t, m)
	text := read(t, file)
	if !strings.Contains(text, "```sh {#history-1 skip links}\necho hi${X-gone}\n```\n") || !strings.Contains(text, "higone") {
		t.Errorf("file:\n%s", text)
	}
	if v := m.View(); !strings.Contains(v, "History") || !strings.Contains(v, "higone") {
		t.Errorf("the pane doesn't show the command's output:\n%s", v)
	}

	// A command that goes on gets another line.
	typeLine(m, "if true; then")
	if len(m.hist.lines) != 1 {
		t.Fatalf("lines = %q", m.hist.lines)
	}
	typeLine(m, "echo two")
	typeLine(m, "fi")
	pump(t, m)
	if text := read(t, file); !strings.Contains(text, "```sh {#history-2 skip links}\nif true; then\necho two\nfi\n```\n") {
		t.Errorf("file:\n%s", text)
	}

	// A note is added as prose, carrying on the note before it.
	typeLine(m, "# first note")
	typeLine(m, "# second")
	press(m, "esc", "w", "h")
	if text := read(t, file); !strings.HasSuffix(text, "\n\nfirst note\nsecond\n") {
		t.Errorf("file:\n%s", text)
	}

	// up brings back the command before.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if v := m.hist.input.Value(); v != "fi" || len(m.hist.lines) != 2 {
		t.Errorf("up: %q, %q", m.hist.lines, v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	// esc gives the keys back to the file; h takes them again, and
	// ctrl+d closes the pane.
	press(m, "esc")
	if m.hist.focus || !m.hist.open {
		t.Error("esc didn't leave the pane open without the keys")
	}
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.hist.open {
		t.Error("ctrl+d didn't close the pane")
	}
}

func TestHistoryBlock(t *testing.T) {
	m, _ := newModel(t, "# Log\n", Options{})
	press(m, "h")

	// # ```lang opens a block: the lines after it are kept, not run,
	// under a prompt of their own.
	typeLine(m, "# ```sh {#greet}")
	if !m.inBlock() || m.hist.input.Prompt != blockPromptStyle.Render("```sh> ") {
		t.Fatalf("not in a block: lines %q, prompt %q", m.hist.lines, m.hist.input.Prompt)
	}
	if v := m.View(); !strings.Contains(v, "``` ends it") {
		t.Errorf("no hint that ``` ends the block:\n%s", v)
	}
	typeLine(m, "echo 'not run")
	typeLine(m, "")
	if len(m.hist.lines) != 3 || m.running {
		t.Fatalf("lines %q, running %v", m.hist.lines, m.running)
	}
	typeLine(m, "```")
	if m.inBlock() || len(m.hist.lines) != 0 || m.running {
		t.Fatalf("the block didn't end: lines %q, running %v", m.hist.lines, m.running)
	}
	if want := "# Log\n\n```sh {#greet}\necho 'not run\n\n```\n"; string(m.d.Src) != want {
		t.Errorf("src = %q, want %q", m.d.Src, want)
	}
	if !m.dirty || m.d.Blocks[m.cur()].ID != "greet" {
		t.Errorf("dirty %v, at block %q", m.dirty, m.d.Blocks[m.cur()].ID)
	}
	if log := strings.Join(m.hist.log, "\n"); !strings.Contains(log, "```sh> echo 'not run") || !strings.Contains(log, "hasn't run") {
		t.Errorf("log:\n%s", log)
	}

	// ctrl+c drops a block partway, and the prompt is PS1 again.
	typeLine(m, "# ```sh")
	typeLine(m, "ls")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.inBlock() || m.hist.input.Prompt != lastLine(m.ps1()) || len(m.d.Blocks) != 1 {
		t.Errorf("ctrl+c: lines %q, prompt %q, %d blocks", m.hist.lines, m.hist.input.Prompt, len(m.d.Blocks))
	}

	// An empty block isn't added.
	typeLine(m, "# ```sh")
	typeLine(m, "```")
	if len(m.d.Blocks) != 1 || !strings.Contains(strings.Join(m.hist.log, "\n"), "empty") {
		t.Errorf("%d blocks, log %q", len(m.d.Blocks), m.hist.log)
	}
}

func TestHistoryFullScreen(t *testing.T) {
	m, file := newModel(t, "# Log\n", Options{AutoSave: true})
	press(m, "h")
	typeLine(m, `printf '\033[?1049hfull\n'; sleep 0.2; printf '\033[?1049l'`)
	until(t, m, "the command", func() bool { return strings.Contains(m.pane.screen(), "full") })
	if v := m.View(); !strings.Contains(v, "full") || !strings.Contains(v, "keys go to it") {
		t.Errorf("the pane doesn't show the command:\n%s", v)
	}
	pump(t, m)
	if text := read(t, file); !strings.Contains(text, "{#history-1 skip links tty}") {
		t.Errorf("not kept as a tty block:\n%s", text)
	}
}

func TestHistoryBackgroundAndComplete(t *testing.T) {
	m, _ := newModel(t, "# Log\n", Options{})
	press(m, "h")
	typeLine(m, "true &")
	pump(t, m)
	if text := string(m.d.Src); !strings.Contains(text, "```sh {#history-1 skip background links}\ntrue\n```") {
		t.Errorf("file:\n%s", text)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ech"), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if v := m.hist.input.Value(); v != "echo " {
		t.Errorf("tab: %q", v)
	}
}

func TestHistoryReadlineKeys(t *testing.T) {
	m, _ := newModel(t, "# Log\n", Options{})
	press(m, "h")
	h := &m.hist
	key := func(s string) {
		t.Helper()
		var k tea.KeyMsg
		switch s {
		case "ctrl+w":
			k = tea.KeyMsg{Type: tea.KeyCtrlW}
		case "ctrl+y":
			k = tea.KeyMsg{Type: tea.KeyCtrlY}
		case "ctrl+a":
			k = tea.KeyMsg{Type: tea.KeyCtrlA}
		case "ctrl+e":
			k = tea.KeyMsg{Type: tea.KeyCtrlE}
		case "ctrl+k":
			k = tea.KeyMsg{Type: tea.KeyCtrlK}
		case "ctrl+u":
			k = tea.KeyMsg{Type: tea.KeyCtrlU}
		case "ctrl+d":
			k = tea.KeyMsg{Type: tea.KeyCtrlD}
		case "ctrl+p":
			k = tea.KeyMsg{Type: tea.KeyCtrlP}
		case "ctrl+n":
			k = tea.KeyMsg{Type: tea.KeyCtrlN}
		case "ctrl+l":
			k = tea.KeyMsg{Type: tea.KeyCtrlL}
		case "alt+d":
			k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d"), Alt: true}
		case "alt+b":
			k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}
		default:
			t.Fatalf("no key %q", s)
		}
		if k.String() != s {
			t.Fatalf("key %q is %q", s, k.String())
		}
		m.Update(k)
	}
	want := func(v string, at int) {
		t.Helper()
		if h.input.Value() != v || h.input.Position() != at {
			t.Errorf("input %q at %d, want %q at %d", h.input.Value(), h.input.Position(), v, at)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("echo héllo wörld"), Paste: true})
	key("ctrl+w")
	want("echo héllo ", 11)
	// Kills one after another add up, so ctrl+y brings both back.
	key("ctrl+w")
	want("echo ", 5)
	key("ctrl+y")
	want("echo héllo wörld", 16)
	key("ctrl+a")
	key("alt+d")
	want(" héllo wörld", 0)
	key("ctrl+e")
	key("ctrl+y")
	want(" héllo wörldecho", 16)
	key("ctrl+a")
	key("ctrl+d")
	want("héllo wörldecho", 0)
	if !h.focus || !m.hist.open {
		t.Fatal("ctrl+d with text typed closed the pane")
	}
	key("ctrl+e")
	key("alt+b")
	key("ctrl+k")
	want("héllo ", 6)
	key("ctrl+u")
	want("", 0)
	key("ctrl+y")
	want("héllo wörldecho", 15)

	// ctrl+p and ctrl+n recall commands, as up and down do.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	typeLine(m, "echo one")
	pump(t, m)
	key("ctrl+p")
	want("echo one", 8)
	key("ctrl+n")
	want("", 0)
	key("ctrl+l")
	if len(h.log) != 0 {
		t.Errorf("ctrl+l left %q", h.log)
	}
	key("ctrl+d")
	if m.hist.open {
		t.Error("ctrl+d with nothing typed didn't close the pane")
	}
}

func TestHistorySelectCopy(t *testing.T) {
	var got []string
	old := setClipboard
	setClipboard = func(s string) error { got = append(got, s); return nil }
	t.Cleanup(func() { setClipboard = old })

	m, _ := newModel(t, "# Log\n", Options{})
	press(m, "h")
	typeLine(m, `printf '\033[32mone\033[0m  \ntwo\nthree\n'`)
	pump(t, m)
	n := len(m.hist.log)
	if n < 4 || m.hist.log[n-1] != "three" {
		t.Fatalf("log = %q", m.hist.log)
	}

	// shift+up in the pane selects the last line; up takes in the two
	// before it, and y copies them as text, without colors or the
	// spaces that end them.
	m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.hist.sel == nil {
		t.Fatal("shift+up selected nothing")
	}
	press(m, "k", "k")
	if v := m.View(); !strings.Contains(v, "y copy") {
		t.Errorf("the pane doesn't say how to copy:\n%s", v)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("y gave no command")
	}
	m.Update(cmd())
	if len(got) != 1 || got[0] != "one\ntwo\nthree" {
		t.Errorf("copied %q", got)
	}
	if m.status != "copied 3 lines from History" {
		t.Errorf("status = %q", m.status)
	}
	if m.hist.sel != nil || !m.hist.focus {
		t.Errorf("after y: sel %v, focus %v; want the keys back in the pane", m.hist.sel, m.hist.focus)
	}

	// V in the file selects too; o swaps the ends, so k takes the last
	// line back off, and esc copies nothing.
	press(m, "esc", "V", "k", "k", "o", "k")
	if from, to := m.hist.sel.selRange(); from != n-3 || to != n-2 {
		t.Errorf("selected %d to %d, want %d to %d", from, to, n-3, n-2)
	}
	press(m, "esc")
	if m.hist.sel != nil || m.hist.focus || len(got) != 1 {
		t.Errorf("after esc: sel %v, focus %v, copied %q", m.hist.sel, m.hist.focus, got)
	}

	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	press(m, "V")
	if m.hist.sel != nil || m.status != "History is closed; h opens it" {
		t.Errorf("V with History closed: sel %v, status %q", m.hist.sel, m.status)
	}
}

func TestHistoryFillsScreenAndShowsDir(t *testing.T) {
	src := "# Log\n\n```sh\necho hi\n```\n"
	m, _ := newModel(t, src, Options{})
	press(m, "enter")
	pump(t, m)
	// Z opens the pane over the whole screen, with the keys in it.
	press(m, "Z")
	if !m.hist.open || !m.hist.full || !m.hist.focus {
		t.Fatalf("open %v, full %v, focus %v", m.hist.open, m.hist.full, m.hist.focus)
	}
	if h := m.histHeight(); h != m.height-footerLines-m.bannerLines() {
		t.Errorf("pane height %d of %d", h, m.height)
	}
	v := m.View()
	if strings.Contains(v, "Log") || strings.Contains(v, "echo hi") {
		t.Errorf("the file shows under the full pane:\n%s", v)
	}
	// The rule names the shell's directory, and follows a cd.
	typeLine(m, "cd /")
	pump(t, m)
	if m.hist.prompt.Dir != "/" {
		t.Errorf("dir after cd / = %q", m.hist.prompt.Dir)
	}
	if v := m.View(); !strings.Contains(v, "── History / ──") {
		t.Errorf("the rule doesn't show the directory:\n%s", v)
	}
	// esc gives the file its place back.
	press(m, "esc")
	if m.hist.full || !m.hist.open {
		t.Errorf("after esc: full %v, open %v", m.hist.full, m.hist.open)
	}
	if h := m.vpHeight(); h < 10 {
		t.Errorf("the file has %d lines again", h)
	}
	// Z twice from the file fills the screen and gives it back.
	press(m, "esc", "Z")
	if !m.hist.full {
		t.Error("Z with the pane open didn't fill the screen")
	}
	m.hist.focus = false
	press(m, "Z")
	if m.hist.full {
		t.Error("Z again didn't give the file its place back")
	}
}

// TestHistoryUnrecorded checks that ctrl+o turns recording off, so a
// command runs, with what it sets kept in the shell, but isn't added to
// the file, and that closing the pane turns it on again.
func TestHistoryUnrecorded(t *testing.T) {
	src := "# Log\n"
	m, file := newModel(t, src, Options{AutoSave: true})
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if !m.hist.unrecorded || !strings.Contains(m.View(), "not recording") {
		t.Fatalf("ctrl+o didn't turn recording off:\n%s", m.View())
	}
	typeLine(m, "Y=kept; echo unseen")
	pump(t, m)
	typeLine(m, "false")
	pump(t, m)
	if text := read(t, file); strings.Contains(text, "history-") {
		t.Errorf("an unrecorded command was added:\n%s", text)
	}
	log := strings.Join(m.hist.log, "\n")
	if !strings.Contains(log, "unseen") || !strings.Contains(log, "[exit 1]") {
		t.Errorf("log = %q", log)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	typeLine(m, "echo got$Y")
	pump(t, m)
	if text := read(t, file); !strings.Contains(text, "{#history-1 skip links}") || !strings.Contains(text, "gotkept") {
		t.Errorf("file:\n%s", text)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	press(m, "h")
	if m.hist.unrecorded {
		t.Error("closing the pane didn't turn recording on again")
	}
}
