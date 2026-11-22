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
			var live []string // live and changed events
			n, err := Open(file, Options{OnEvent: func(e Event) {
				mu.Lock()
				defer mu.Unlock()
				switch e.Kind {
				case "shell":
					shown.Write(e.Bytes)
				case "jobs":
					kinds = append(kinds, "jobs "+e.Data)
				case "live", "changed":
					live = append(live, strings.TrimSpace(e.Kind+" "+e.Data))
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
				// As a person would, let the next prompt draw before
				// typing on: sh echoes a line typed ahead of its prompt
				// before the prompt, where it isn't read as the line.
				time.Sleep(300 * time.Millisecond)
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
			wait("unrecorded", func(string) bool {
				mu.Lock()
				defer mu.Unlock()
				return strings.Count(shown.String(), "unrecorded") >= 2
			})
			n.SetRecording(true)
			n.TypeShell("sleep 5\r")
			for !n.Running() {
				time.Sleep(20 * time.Millisecond)
			}
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
			// The window hears when a typed command starts and ends,
			// and when the document changes, as the note did.
			mu.Lock()
			said := strings.Join(live, "\n")
			mu.Unlock()
			for _, s := range []string{"changed\nlive running\nchanged\nlive\nchanged", "live running"} {
				if !strings.Contains(said, s) {
					t.Errorf("no %q in live events:\n%s", s, said)
				}
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
				// Brought back, it is the command that runs, and no
				// longer a stopped job.
				n.TypeShell("fg\r")
				deadline = time.Now().Add(5 * time.Second)
				for {
					mu.Lock()
					last := kinds[len(kinds)-1]
					mu.Unlock()
					if last == "jobs []" {
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("jobs events after fg: %q", kinds)
					}
					time.Sleep(20 * time.Millisecond)
				}
				n.Stop()
			}
		})
	}
}

// TestLivePlayStops: a block played while a command typed at the live
// pane's prompt holds the shell stops it, runs in the same shell, and
// brings it back.
func TestLivePlayStops(t *testing.T) {
	for _, name := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("skip_global_compinit", "1")
			file := write(t, "---\nmdsh:\n  shell: "+path+"\n  env:\n    share: [\"...\"]\n---\n"+
				"```sh {#a}\nexport Z=from-a; echo played\n```\n\n```sh {#b}\necho \"$Z\"\n```\n")
			n, err := Open(file, Options{})
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
			n.TypeShell("sleep 30\r")
			deadline := time.Now().Add(10 * time.Second)
			for !n.Running() {
				if time.Now().After(deadline) {
					t.Fatal("sleep never ran")
				}
				time.Sleep(20 * time.Millisecond)
			}
			time.Sleep(200 * time.Millisecond)
			if err := n.RunBlock(0); err != nil {
				t.Fatal(err)
			}
			// Once sleep is back it holds the shell again, so b stops it
			// too.
			if err := n.RunBlock(1); err != nil {
				t.Fatal(err)
			}
			for !n.Running() {
				if time.Now().After(deadline) {
					t.Fatal("sleep isn't back")
				}
				time.Sleep(20 * time.Millisecond)
			}
			n.Stop()
			for n.Running() {
				if time.Now().After(deadline) {
					t.Fatal("sleep never ended")
				}
				time.Sleep(20 * time.Millisecond)
			}
			got := noAt(n.Text())
			for _, s := range []string{
				"{for=a exit=0}\nplayed\n```",
				"{for=b exit=0}\nfrom-a\n```",
				" {#history-1 skip links}\nsleep 30\n```",
				"{for=history-1 exit=",
			} {
				if !strings.Contains(got, s) {
					t.Errorf("no %q in\n%s", s, got)
				}
			}
		})
	}
}

// TestLiveSide: a block run in a side shell while a typed command holds
// the live pane's shell runs from the shell's state, and leaves the
// command running.
func TestLiveSide(t *testing.T) {
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	file := write(t, "---\nmdsh:\n  shell: "+path+"\n  env:\n    share: [\"...\"]\n---\n"+
		"```sh {#a}\necho \"$Y $PWD\"; Z=side\n```\n")
	n, err := Open(file, Options{})
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
	n.TypeShell("export Y=from-shell; cd " + dir + "\r")
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(n.Text(), "{#history-1") || n.Running() {
		if time.Now().After(deadline) {
			t.Fatal("the first command never ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
	n.TypeShell("sleep 30\r")
	for !strings.Contains(n.Text(), "sleep 30") || !n.Running() {
		if time.Now().After(deadline) {
			t.Fatal("sleep never ran")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := n.RunBlockSide(0); err != nil {
		t.Fatal(err)
	}
	got := noAt(n.Text())
	if !strings.Contains(got, "{for=a exit=0}\nfrom-shell "+dir+"\n```") {
		t.Errorf("side run:\n%s", got)
	}
	if strings.Contains(got, "{for=history-") || !n.Running() {
		t.Errorf("sleep didn't go on: running %v\n%s", n.Running(), got)
	}
	n.Stop()
	for n.Running() {
		if time.Now().After(deadline) {
			t.Fatal("sleep never ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := noAt(n.Text()); !strings.Contains(got, "{for=history-2 exit=130}") {
		t.Errorf("after ctrl+c:\n%s", got)
	}
}

// TestLiveSubEnv: the pane moves to a sub-env, whose scripted shell it
// asks to end first; commands typed there become History blocks of that
// env, and moving back shows the top level's screen again.
func TestLiveSubEnv(t *testing.T) {
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	t.Setenv("HOME", t.TempDir())
	file := write(t, "---\nmdsh:\n  shell: "+path+"\n  env:\n    share: [\"...\"]\n---\n"+
		"```sh {#a env=build share=Y}\necho \"in build: ${Y-}\"\n```\n")
	var mu sync.Mutex
	var shown strings.Builder
	var moved []string
	n, err := Open(file, Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		switch e.Kind {
		case "shell":
			shown.Write(e.Bytes)
		case "shell-env":
			moved = append(moved, e.Data+": "+string(e.Bytes))
			shown.Reset()
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
	deadline := time.Now().Add(10 * time.Second)
	until := func(what string, ok func() bool) {
		t.Helper()
		for !ok() {
			if time.Now().After(deadline) {
				mu.Lock()
				t.Fatalf("no %s; text:\n%s\nshown: %q", what, n.Text(), shown.String())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	n.TypeShell("export Y=from-top; echo top-marker\r")
	until("top command", func() bool { return strings.Contains(n.Text(), "{#history-1 skip links}") && !n.Running() })
	// A block starts build scripted.
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if got := noAt(n.Text()); !strings.Contains(got, "in build: from-top") {
		t.Fatalf("build's block:\n%s", got)
	}
	envs := n.HistoryEnvs()
	if len(envs) != 2 || !envs[0].Current || envs[1].Name != "build" || !envs[1].Scripted || envs[1].Live {
		t.Fatalf("envs: %+v", envs)
	}
	if err := n.SetHistoryEnv("build", false); err != ErrEnvScripted {
		t.Fatalf("SetHistoryEnv: %v", err)
	}
	if err := n.SetHistoryEnv("build", true); err != nil {
		t.Fatal(err)
	}
	if n.HistoryEnvName() != "build" {
		t.Fatalf("on %q", n.HistoryEnvName())
	}
	n.TypeShell("echo \"typed in build: $Y\"\r")
	until("build command", func() bool {
		return strings.Contains(noAt(n.Text()), "{#history-2 env=build skip links}\necho \"typed in build: $Y\"\n```") && !n.Running()
	})
	if got := noAt(n.Text()); !strings.Contains(got, "typed in build: from-top") {
		t.Errorf("build's typed command:\n%s", got)
	}
	envs = n.HistoryEnvs()
	if envs[0].Current || !envs[1].Current || !envs[1].Live || envs[1].Scripted {
		t.Errorf("envs: %+v", envs)
	}
	if err := n.SetHistoryEnv("", false); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	last := moved[len(moved)-1]
	mu.Unlock()
	if !strings.HasPrefix(last, ": ") || !strings.Contains(last, "top-marker") || strings.Contains(last, "typed in build") {
		t.Errorf("moving back showed %q", last)
	}
	if err := n.SetHistoryEnv("nowhere", false); err == nil {
		t.Error("moved to an env no block names")
	}
}
