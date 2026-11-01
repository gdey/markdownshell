package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMoveBlockKeys(t *testing.T) {
	src := "# T\n\n```sh {#a}\necho a\n```\n\nMiddle.\n\n```sh {#b}\necho b\n```\n"
	m, file := newModel(t, src, Options{})
	press(m, "j") // on b
	if got := m.d.Blocks[m.cur()].ID; got != "b" {
		t.Fatalf("cursor on %s", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	want := "# T\n\n```sh {#a}\necho a\n```\n\n```sh {#b}\necho b\n```\n\nMiddle.\n"
	if got := string(m.d.Src); got != want {
		t.Fatalf("after alt+up:\n%s", got)
	}
	if got := m.d.Blocks[m.cur()].ID; got != "b" {
		t.Errorf("cursor left the moved block for %s", got)
	}
	press(m, "K")
	if !strings.HasPrefix(string(m.d.Src), "# T\n\n```sh {#b}") {
		t.Fatalf("after K:\n%s", m.d.Src)
	}
	if !m.dirty || !strings.Contains(m.status, "moved") {
		t.Errorf("dirty %v, status %q", m.dirty, m.status)
	}
	press(m, "K") // past the heading
	if !strings.HasPrefix(string(m.d.Src), "```sh {#b}") {
		t.Fatalf("after K again:\n%s", m.d.Src)
	}
	press(m, "K")
	if !strings.Contains(m.status, "can't move further") {
		t.Errorf("status at the top %q", m.status)
	}
	press(m, "u", "u", "u")
	if got := string(m.d.Src); got != src {
		t.Fatalf("after undoing all three:\n%s", got)
	}
	press(m, "u")
	if m.status != "nothing to undo" {
		t.Errorf("status %q", m.status)
	}
	press(m, "k", "J", "w")
	if got := read(t, file); got != "# T\n\nMiddle.\n\n```sh {#a}\necho a\n```\n\n```sh {#b}\necho b\n```\n" {
		t.Errorf("saved:\n%s", got)
	}
}

func TestMoveKeepsOutputAndWarns(t *testing.T) {
	src := "```sh {#a}\necho hi\n```\n\n```sh {#b run={{#a/ran?}}}\necho b\n```\n"
	m, file := newModel(t, src, Options{})
	press(m, "enter")
	pump(t, m)
	press(m, "J")
	if !strings.Contains(m.notice, "old order") {
		t.Errorf("no note in %q", m.notice)
	}
	if !strings.Contains(m.notice, "comes after it") {
		t.Errorf("no order warning in %q", m.notice)
	}
	if !strings.Contains(m.footer(), "comes after it") {
		t.Errorf("footer without the warning:\n%s", m.footer())
	}
	press(m, "w")
	if m.notice != "" {
		t.Errorf("notice kept after a key: %q", m.notice)
	}
	got := read(t, file)
	if !strings.HasPrefix(got, "```sh {#b") || !strings.Contains(got, "```sh {#a}\necho hi\n```\n\n```output {for=a exit=0}\nhi\n```") {
		t.Errorf("saved:\n%s", got)
	}
}

func TestMoveUndoAfterEdit(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\nX.\n"
	m, _ := newModel(t, src, Options{})
	press(m, "J")
	m.editedAll("```sh {#a}\necho changed\n```\n\nX.\n", string(m.d.Src))
	press(m, "u")
	if !strings.Contains(m.status, "can't be undone") {
		t.Errorf("status %q", m.status)
	}
}
