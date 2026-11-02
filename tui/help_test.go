package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pickHelp picks the help item with keys.
func pickHelp(t *testing.T, m *Model, keys string) {
	t.Helper()
	for n, it := range helpItems {
		if it.keys == keys {
			m.helpAt = n
			return
		}
	}
	t.Fatalf("no help item %q", keys)
}

func TestHelpPanel(t *testing.T) {
	m, _ := newModel(t, twoBlocks, Options{})
	press(m, "?")
	if !m.helping {
		t.Fatal("? didn't open the help panel")
	}
	v := m.View()
	for _, s := range []string{"Keys", "run the block", "undo the last move", "(nothing to undo)"} {
		if !strings.Contains(v, s) {
			t.Errorf("help panel lacks %q:\n%s", s, v)
		}
	}
	// The tabs are at the end, scrolled to.
	press(m, "G")
	if v := m.View(); !strings.Contains(v, "(there's one tab)") || strings.Contains(v, "Keys") {
		t.Errorf("G didn't scroll to the tabs:\n%s", v)
	}
	press(m, "esc")
	if m.helping {
		t.Fatal("esc didn't close the help panel")
	}

	// Picking an item does what its key does.
	press(m, "?")
	pickHelp(t, m, "J/K alt+↓/↑")
	press(m, "enter")
	if m.helping || len(m.undo) != 1 {
		t.Fatalf("picking J didn't move the block: helping %v, %d undos", m.helping, len(m.undo))
	}
	press(m, "?")
	if strings.Contains(m.View(), "(nothing to undo)") {
		t.Error("u is greyed after a move")
	}
	press(m, "j")
	if m.helpAt != 1 {
		t.Errorf("j in the panel picked %d, want 1", m.helpAt)
	}

	// A greyed item says why and leaves the panel up.
	pickHelp(t, m, "M")
	press(m, "enter")
	if !m.helping || !strings.Contains(m.status, "hasn't changed") {
		t.Errorf("picking a greyed item: helping %v, status %q", m.helping, m.status)
	}

	// ctrl+t goes to the app.
	pickHelp(t, m, "ctrl+t")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || cmd().(tea.KeyMsg).String() != "ctrl+t" {
		t.Error("picking ctrl+t didn't ask the app for a tab")
	}
}

func TestHelpPanelWhileRunning(t *testing.T) {
	src := "# T\n\n```sh {#a confirm}\necho hi\n```\n"
	m, _ := newModel(t, src, Options{})
	press(m, "enter")
	press(m, "?")
	if v := m.View(); !strings.Contains(v, "stop the run") || !strings.Contains(v, "(a block is running)") {
		t.Errorf("help while running:\n%s", v)
	}
	// The block's question closes the panel.
	pump(t, m)
	if m.prompt == nil || m.helping {
		t.Fatalf("the question didn't take over: prompt %v, helping %v", m.prompt != nil, m.helping)
	}
	press(m, "s")
	pump(t, m)
}

func TestHelpPanelScrolls(t *testing.T) {
	m, _ := newModel(t, twoBlocks, Options{})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	press(m, "?", "G")
	if v := m.View(); !strings.Contains(v, "new tab") {
		t.Errorf("the last key doesn't show:\n%s", v)
	}
}

// TestHelpCoversKeys checks every key the TUI takes is in the panel.
func TestHelpCoversKeys(t *testing.T) {
	var all strings.Builder
	for _, it := range helpItems {
		all.WriteString(" " + it.keys + " " + it.what(&Model{}))
	}
	for _, k := range []string{"j", "k", "g", "G", "pgup", "ctrl+b", "ctrl+d", "ctrl+u", "enter", "r", "a", "s", "i", ":", "ctrl+c", "ctrl+]", ",", "e", "E", "J", "K", "u", "w", "ctrl+s", "W", "R", "M", "p", "q", "ctrl+w", "v", "o", "C", "O", "?", "]", "[", "ctrl+t"} {
		if !strings.Contains(all.String(), k) {
			t.Errorf("%s isn't in the help panel", k)
		}
	}
}
