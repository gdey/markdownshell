//go:build unix

package tui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/run"
)

// drive feeds m what its shell and runs send, as the program would,
// until ok holds.
func drive(t *testing.T, m *Model, what string, ok func() bool) {
	t.Helper()
	lt := m.lterm
	deadline := time.After(10 * time.Second)
	for !ok() {
		lt.mu.Lock()
		var next *liveMsg
		if len(lt.queue) > 0 {
			next = &lt.queue[0]
			lt.queue = lt.queue[1:]
		}
		lt.mu.Unlock()
		if next != nil {
			m.Update(*next)
			continue
		}
		select {
		case msg := <-m.events:
			m.Update(msg)
		case <-lt.wake:
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatalf("no %s; view:\n%s", what, m.View())
		}
	}
}

// TestLiveHistoryTUI: with the interactive shell, the History pane is a
// terminal on the shell; what is typed at its prompt becomes notes and
// blocks as it runs, a stopped job shows on the pane's rule, and ctrl+g
// gives the keys back to the file.
func TestLiveHistoryTUI(t *testing.T) {
	for _, name := range []string{"bash", "zsh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			src := "---\nmdsh:\n  shell: " + path + "\n  env:\n    share: [\"...\"]\n---\n# Log\n\n```sh {#a}\necho played\n```\n"
			m, file := newModel(t, src, Options{AutoSave: true, Run: run.Options{InteractiveShell: true}})
			m.opts.Run.Environ = append(m.opts.Run.Environ, "skip_global_compinit=1", "HOME="+t.TempDir())
			press(m, "h")
			if !m.isLive() || m.lterm.sh == nil {
				t.Fatalf("live %v", m.isLive())
			}
			keys := func(s string) { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: false}) }
			enter := func() { m.Update(tea.KeyMsg{Type: tea.KeyEnter}) }
			has := func(s string) func() bool {
				return func() bool { return strings.Contains(noAt(string(m.d.Src)), s) && !m.lterm.busy }
			}
			keys("# a note")
			enter()
			drive(t, m, "note", has("\na note\n"))
			// As a person would, wait for the prompt before typing on: zsh
			// can take a line typed ahead of its prompt as two.
			time.Sleep(300 * time.Millisecond)
			keys("echo hi")
			enter()
			drive(t, m, "command", func() bool { return strings.Contains(read(t, file), "exit=0}\nhi\n```") && !m.lterm.busy })
			time.Sleep(300 * time.Millisecond)
			if v := m.View(); !strings.Contains(v, "keys go to the shell") {
				t.Errorf("view:\n%s", v)
			}
			keys("sleep 30")
			enter()
			drive(t, m, "sleep", func() bool { return m.lterm.busy })
			time.Sleep(200 * time.Millisecond)
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
			drive(t, m, "stopped job", func() bool { return strings.Contains(m.lterm.jobsText(), "sleep 30 (stopped)") && !m.lterm.busy })
			if v := m.View(); !strings.Contains(v, "jobs: ") {
				t.Errorf("no jobs on the rule:\n%s", v)
			}
			// fg brings it back; a block played meanwhile stops it, runs
			// in the same shell and brings it back again.
			keys("fg")
			enter()
			drive(t, m, "sleep back", func() bool { return m.lterm.busy && m.lterm.jobsText() == "" })
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
			if m.hist.focus {
				t.Error("ctrl+g kept the keys in the pane")
			}
			press(m, "g", "enter")
			drive(t, m, "played block", func() bool { return !m.running && m.results[0] != nil && m.results[0].Ran })
			if got := string(m.results[0].Stdout); !strings.Contains(got, "played") {
				t.Errorf("played block's output: %q", got)
			}
			drive(t, m, "sleep back again", func() bool { return m.lterm.busy })
			press(m, "h")
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			drive(t, m, "sleep ended", func() bool { return !m.lterm.busy })
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
			if text := read(t, file); !strings.Contains(text, " {#history-1 skip links}\necho hi\n```") || !strings.Contains(text, " {#history-2 skip links}\nsleep 30\n```") {
				t.Errorf("file:\n%s", text)
			}
		})
	}
}
