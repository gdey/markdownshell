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
