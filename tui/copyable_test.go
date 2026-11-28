package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCopyOffered(t *testing.T) {
	var got []string
	old := setClipboard
	setClipboard = func(s string) error { got = append(got, s); return nil }
	t.Cleanup(func() { setClipboard = old })

	// Saved output, as a run writes it: a region a program bracketed
	// with OSC 1337, and an OSC 52 text ("hidden" in base64).
	src := "# T\n\n```sh {#tok}\nmake-token\n```\n\n```output {for=tok exit=0}\n" +
		"token: \x1b]1337;CopyToClipboard=\a\x1b[1mabc123\x1b[m\x1b]1337;EndCopy\a\n\x1b]52;c;aGlkZGVu\a" +
		"\n```\n\n```sh {#plain}\necho hi\n```\n"
	m, _ := newModel(t, src, Options{})
	v := m.View()
	if !strings.Contains(v, "⧉1") || !strings.Contains(v, "abc123") || !strings.Contains(v, "⧉2") || !strings.Contains(v, "text to copy, 6 chars (c)") {
		t.Errorf("the output doesn't mark what it offers to copy:\n%s", v)
	}
	if strings.Contains(v, "1337") || strings.Contains(v, "aGlkZGVu") || strings.Contains(v, "^[") {
		t.Errorf("the copy sequences show:\n%s", v)
	}

	copy := func() {
		t.Helper()
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
		if cmd == nil {
			t.Fatalf("c gave no command; status %q", m.status)
		}
		m.Update(cmd())
	}
	copy()
	if got[len(got)-1] != "abc123" || m.status != "copied text ⧉1 1 of 2 from tok's output" {
		t.Errorf("copied %q, status %q", got[len(got)-1], m.status)
	}
	copy()
	if got[len(got)-1] != "hidden" || m.status != "copied text ⧉2 2 of 2 from tok's output" {
		t.Errorf("copied %q, status %q", got[len(got)-1], m.status)
	}
	copy()
	if got[len(got)-1] != "abc123" {
		t.Errorf("c doesn't start again after the last: copied %q", got[len(got)-1])
	}

	press(m, "j")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}); cmd != nil || m.status != "the block's output offers nothing to copy" {
		t.Errorf("c on a block with nothing to copy: status %q", m.status)
	}
}
