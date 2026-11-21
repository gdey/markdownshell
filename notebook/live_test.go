//go:build unix

package notebook

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveHistory: with the shell interactive, what is typed at its own
// prompt in the History pane becomes notes and History blocks as it
// runs.
func TestLiveHistory(t *testing.T) {
	for _, name := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("skip_global_compinit", "1")
			file := write(t, "---\nmdsh:\n  shell: "+path+"\n  env:\n    share: [\"...\"]\n---\n# Log\n")
			var mu sync.Mutex
			var shown strings.Builder
			var kinds []string
			n, err := Open(file, Options{OnEvent: func(e Event) {
				mu.Lock()
				defer mu.Unlock()
				switch e.Kind {
				case "shell":
					shown.Write(e.Bytes)
				case "jobs":
					kinds = append(kinds, "jobs "+e.Data)
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer n.Close()
			n.SetInteractiveShell(true, false)
			if _, err := n.SetHistory(true, true); err != nil {
				t.Fatal(err)
			}
			if err := n.AttachShell(); err != nil {
				t.Fatal(err)
			}
			if !n.Live() {
				t.Fatal("pane isn't live")
			}
			wait := func(what string, ok func(string) bool) {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for !ok(noAt(n.Text())) || n.Running() {
					if time.Now().After(deadline) {
						mu.Lock()
						t.Fatalf("no %s; text:\n%s\nshown: %q", what, n.Text(), shown.String())
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			has := func(s string) func(string) bool {
				return func(text string) bool { return strings.Contains(text, s) }
			}
			n.TypeShell("# a note\r")
			wait("note", has("\na note\n"))
			n.TypeShell("echo hi; (exit 3)\r")
			wait("command", has("exit=3}\nhi\n```"))
			n.TypeShell("printf '\\033[?1049hfull\\033[?1049l'\r")
			wait("full screen", has("history-2.cast"))
			n.SetRecording(false)
			n.TypeShell("echo unrecorded\r")
			time.Sleep(300 * time.Millisecond)
			wait("unrecorded", func(string) bool { return true })
			n.SetRecording(true)
			n.TypeShell("sleep 5\r")
			time.Sleep(300 * time.Millisecond)
			n.TypeShell("\x1a")
			wait("stopped", has("```output {for=history-3 exit="))
			got := noAt(n.Text())
			for _, s := range []string{
				" {#history-1 skip links}\necho hi; (exit 3)\n```",
				" {#history-2 skip links tty}\n",
				" {#history-3 skip links}\nsleep 5\n```",
			} {
				if !strings.Contains(got, s) {
					t.Errorf("no %q in\n%s", s, got)
				}
			}
			if strings.Contains(got, "unrecorded") {
				t.Errorf("unrecorded command added:\n%s", got)
			}
			if name != "sh" {
				deadline := time.Now().Add(5 * time.Second)
				for {
					mu.Lock()
					last := ""
					if len(kinds) > 0 {
						last = kinds[len(kinds)-1]
					}
					mu.Unlock()
					if strings.Contains(last, `"state":"stopped","command":"sleep 5","block":`) {
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("jobs events: %q", kinds)
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
		})
	}
}
