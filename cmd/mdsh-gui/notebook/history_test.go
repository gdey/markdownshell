package notebook

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
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
