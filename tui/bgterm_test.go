//go:build unix

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A background tty block runs in its card and takes the keys as it
// starts; ctrl+g gives them back to the file, where another block runs
// while it goes on, and t gives them to it again.
func TestBackgroundTTYInCard(t *testing.T) {
	src := "```sh {#edit background tty}\nprintf 'name? '; read n; echo \"hi $n\" > got.txt; read m\n```\n\n" +
		"```sh {#show}\ncat got.txt\n```\n"
	m, file := newModel(t, src, Options{})
	press(m, "enter")
	screen := func() string {
		if p := m.bgPanes[0]; p != nil {
			return p.screen()
		}
		return ""
	}
	until(t, m, "the question", func() bool { return strings.Contains(screen(), "name?") })
	if m.bgFocus != 0 {
		t.Fatalf("bgFocus = %d, want the block", m.bgFocus)
	}
	if v := m.View(); !strings.Contains(v, "runs in the background, in its card") {
		t.Errorf("the footer doesn't say keys go to the block:\n%s", v)
	}
	// Keys, j and q included, go to the block.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jq")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := filepath.Join(filepath.Dir(file), "got.txt")
	until(t, m, "the answer", func() bool { b, _ := os.ReadFile(got); return string(b) == "hi jq\n" })

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	if m.bgFocus != -1 {
		t.Fatal("ctrl+g left the keys with the block")
	}
	press(m, "j", "enter")
	until(t, m, "show to run", func() bool { return m.results[1] != nil && m.results[1].Ran && !m.running })
	if out := string(m.results[1].Stdout); out != "hi jq\n" {
		t.Errorf("show's output = %q", out)
	}
	if !m.bg[0] || m.bgPanes[0] == nil {
		t.Fatal("the background block stopped when another block ran")
	}

	press(m, "k", "t")
	if m.bgFocus != 0 {
		t.Fatal("t didn't give the keys to the block")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	until(t, m, "the block to end", func() bool { m.sess.Reap(); return !m.bg[0] && m.bgPanes[0] == nil })
	if m.bgFocus != -1 {
		t.Error("the keys stay with a block that ended")
	}
	if r := m.results[0]; r.Status != 0 || !strings.HasSuffix(r.Image, ".cast") {
		t.Errorf("edit: status %d, output %q; want 0 and a cast", r.Status, r.Image)
	}
}
