package notebook

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHistory(t *testing.T) {
	file := write(t, "# Log\n\n```sh {#setup}\nX=one\n```\n")
	var (
		mu    sync.Mutex
		panes []string
	)
	n, err := Open(file, Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == "terminal" {
			panes = append(panes, e.Data)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.Enter("echo $X"); !errors.Is(err, ErrHistoryOff) {
		t.Fatalf("Enter with the pane off: %v", err)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	restarted, err := n.SetHistory(true)
	if err != nil || !restarted {
		t.Fatalf("SetHistory: restarted %v, err %v", restarted, err)
	}
	if restarted, _ := n.SetHistory(true); restarted {
		t.Error("restarted shells that had terminals")
	}
	if restarted, _ := n.SetHistory(false); restarted {
		t.Error("restarted shells to turn the pane off")
	}
	if err := n.Enter("true"); !errors.Is(err, ErrHistoryOff) {
		t.Fatalf("Enter with the pane off again: %v", err)
	}
	if restarted, _ := n.SetHistory(true); restarted {
		t.Error("restarted shells that had terminals")
	}
	for _, in := range []string{
		"# Setting up\n#  the *shell*.",
		"#again",
		"X=two; cd /",
		"",
		"printf '\\033[1m%s\\033[0m %s\\n' \"$X\" \"$PWD\"",
		"# Now fail:",
		"false",
		"printf '\\033[?1049hfull\\033[?1049l'",
	} {
		if err := n.Enter(in); err != nil {
			t.Fatalf("Enter(%q): %v", in, err)
		}
	}
	want := "# Log\n\n```sh {#setup}\nX=one\n```\n\n" +
		"Setting up\n the *shell*.\nagain\n\n" +
		"```sh {#history-1}\nX=two; cd /\n```\n\n" +
		"```sh {#history-2}\nprintf '\\033[1m%s\\033[0m %s\\n' \"$X\" \"$PWD\"\n```\n\n" +
		"```output {for=history-2 exit=0}\ntwo /\n```\n\n" +
		"Now fail:\n\n" +
		"```sh {#history-3}\nfalse\n```\n\n" +
		"```output {for=history-3 exit=1}\n```\n\n" +
		"```sh {#history-4 tty}\nprintf '\\033[?1049hfull\\033[?1049l'\n```\n"
	got := n.Text()
	if !strings.HasPrefix(got, want) {
		t.Errorf("text =\n%s\nwant it to start\n%s", got, want)
	}
	if !strings.Contains(got[len(want)-1:], "history-4.gif") {
		t.Errorf("no recording after the full-screen command:\n%s", got)
	}
	if !n.Dirty() {
		t.Error("not dirty")
	}
	mu.Lock()
	if got := strings.Join(panes, ","); got != "history,history,history,history" {
		t.Errorf("terminal events said %q", got)
	}
	mu.Unlock()

	// The blocks run again like any other; the setup block gets its own
	// terminal pane, not the History pane.
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	if string(saved) != got {
		t.Error("saved file differs from the text")
	}
}

func TestHistoryNumbers(t *testing.T) {
	n, err := Open(write(t, "```sh {#history-7}\ntrue\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if _, err := n.SetHistory(true); err != nil {
		t.Fatal(err)
	}
	if err := n.Enter("true"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "{#history-8}") {
		t.Errorf("text = %s", n.Text())
	}
}

func TestNoteInOpenFence(t *testing.T) {
	src := "```sh\necho open\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.Enter("# note"); err == nil {
		t.Error("added a note inside an open fence")
	}
	if n.Text() != src {
		t.Errorf("text = %q", n.Text())
	}
}

func TestComplete(t *testing.T) {
	n, err := Open(write(t, "```sh\ntrue\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	for in, want := range map[string]bool{
		"echo hi":                 true,
		"echo )":                  true, // an error, but nothing to wait for
		"# it's a note":           true,
		"":                        true,
		"echo 'abc":               false,
		"if true; then":           false,
		"if true; then\necho\nfi": true,
		"for x in 1 2; do":        false,
		"echo one \\":             false,
		"echo one \\\\":           true,
		"ls |":                    false,
		"true &&":                 false,
		"cat <<EOF\nhi":           false,
		"cat <<'EOF'\nhi\nEOF":    true,
		"f() {":                   false,
	} {
		if got := n.Complete(in); got != want {
			t.Errorf("Complete(%q) = %v", in, got)
		}
	}
}

func TestPrompt(t *testing.T) {
	hash, zhash := "$", "%"
	if os.Geteuid() == 0 {
		hash, zhash = "#", "#"
	}
	for _, tc := range []struct {
		name, src, ps1, ps2 string
	}{
		{"sh", "```sh\ncd /; PS1='$PWD> '; PS2='more> '\n```\n", "/> ", "more> "},
		{"bash", "---\nmdsh:\n  shell: bash\n---\n```sh\ncd /; PS1='\\[\\e[31m\\]\\w\\$\\[\\e[0m\\] '\n```\n", "\x1b[31m/" + hash + "\x1b[0m ", ""},
		{"zsh", "```zsh\ncd /; PS1='%F{red}%~%f %# '; PS2='%_> '\nprecmd() { PS1=\"[hook] $PS1\" }\n```\n", "[hook] \x1b[31m/\x1b[39m " + zhash + " ", "> "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := exec.LookPath(tc.name); err != nil {
				t.Skip(err)
			}
			if tc.name == "bash" {
				// bash expands \ escapes from 4.4; macOS has 3.2.
				v, _ := exec.Command("bash", "-c", `echo $((BASH_VERSINFO[0]*100+BASH_VERSINFO[1]))`).Output()
				if n, _ := strconv.Atoi(strings.TrimSpace(string(v))); n < 404 {
					t.Skipf("bash %s is older than 4.4", strings.TrimSpace(string(v)))
				}
			}
			n, err := Open(write(t, tc.src), Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer n.Close()
			if err := n.RunBlock(0); err != nil {
				t.Fatal(err)
			}
			p, err := n.Prompt()
			if err != nil {
				t.Fatal(err)
			}
			if p.PS1 != tc.ps1 || p.PS2 != tc.ps2 {
				t.Errorf("prompt %q, %q; want %q, %q", p.PS1, p.PS2, tc.ps1, tc.ps2)
			}
			// The hooks ran in a subshell, so asking again is the same.
			if again, _ := n.Prompt(); again != p {
				t.Errorf("again: %q", again)
			}
		})
	}
}

func TestHistoryRC(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, "rc")
	if err := os.WriteFile(rc, []byte("alias hi='echo from rc'\nPS1='rc> '\necho noise; nosuchcommand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Open(write(t, "```sh\necho hi\n```\n"), Options{Run: RunOptions{HistoryRC: rc}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if p, _ := n.Prompt(); p.PS1 == "rc> " {
		t.Fatal("read the rc file with History off")
	}
	if _, err := n.SetHistory(true); err != nil {
		t.Fatal(err)
	}
	if p, _ := n.Prompt(); p.PS1 != "rc> " {
		t.Errorf("PS1 after History on: %q", p.PS1)
	}
}

func TestHistoryBackground(t *testing.T) {
	n, err := Open(write(t, "# Log\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if _, err := n.SetHistory(true); err != nil {
		t.Fatal(err)
	}
	if err := n.Enter("sleep 0.3; echo later &  "); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "```sh {#history-1 background}\nsleep 0.3; echo later\n```\n") {
		t.Fatalf("text = %s", n.Text())
	}
	// The prompt is back while it runs.
	if err := n.Enter("echo now"); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(10 * time.Second); !strings.Contains(n.Text(), "{for=history-1"); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("background output never came: %s", n.Text())
		}
	}
	if !strings.Contains(n.Text(), "{for=history-1 exit=0}\nlater\n") {
		t.Errorf("text = %s", n.Text())
	}
}

func TestBackgrounded(t *testing.T) {
	for in, want := range map[string]bool{
		"sleep 1 &": true, "a && b": false, "echo \\&": false, "cmd >&": false, "a |&": false, "x&": true, "echo hi": false,
	} {
		if _, got := backgrounded(in); got != want {
			t.Errorf("backgrounded(%q) = %v", in, got)
		}
	}
}

func TestRestart(t *testing.T) {
	src := "```sh\nX=set; PS1='$X> '\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if p, _ := n.Prompt(); p.PS1 != "set> " {
		t.Fatalf("prompt %q", p.PS1)
	}
	if err := n.Restart(); err != nil {
		t.Fatal(err)
	}
	if p, _ := n.Prompt(); p.PS1 == "set> " {
		t.Error("the shell kept what the block set")
	}
	if !strings.Contains(n.Text(), "X=set") {
		t.Errorf("text = %s", n.Text())
	}
}
