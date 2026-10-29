//go:build unix

package run_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// A background block's output stays under it, even what it prints
// while later blocks run.
func TestBackgroundOutputKeptApart(t *testing.T) {
	src := "```sh {#bg background}\necho start\nsleep 0.3\necho late\necho oops >&2\n```\n" +
		"```sh {#next}\nsleep 1; echo next\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	wantStdout(t, r, "bg", "start\nlate\n")
	wantStdout(t, r, "next", "next\n")
	if got := string(r["bg"].Stderr); got != "oops\n" {
		t.Errorf("bg stderr = %q", got)
	}
	if r["bg"].Running || r["bg"].Status != 0 || r["bg"].Err != nil {
		t.Errorf("bg: running %v, status %d, err %v", r["bg"].Running, r["bg"].Status, r["bg"].Err)
	}
}

// Later blocks wait for a background block through its PID variable,
// and see its exit status, even from exit.
func TestBackgroundWait(t *testing.T) {
	src := "```sh {#bg-1 background}\nsleep 0.2; exit 3\n```\n" +
		"```sh {#w}\nwait \"$MDSH_BG_bg_1\"; echo \"rc=$?\"\n```\n" +
		"```sh {#after run={{#bg-1/?}}==3}\necho after\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	wantStdout(t, r, "w", "rc=3\n")
	if r["bg-1"].Status != 3 {
		t.Errorf("bg-1 status = %d, want 3", r["bg-1"].Status)
	}
	wantStdout(t, r, "after", "after\n")
}

func TestPIDVar(t *testing.T) {
	if got := run.PIDVar("still-open.2"); got != "MDSH_BG_still_open_2" {
		t.Errorf("PIDVar = %q", got)
	}
}

// A background block runs in a subshell: it sees the shell's variables
// and functions, but what it sets stays its own.
func TestBackgroundSubshell(t *testing.T) {
	src := "```sh {#a}\nX=1\nf() { echo f$X; }\n```\n" +
		"```sh {#bg background}\nf\nX=2\n```\n" +
		"```sh {#b}\nwait \"$MDSH_BG_bg\"; echo $X\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	wantStdout(t, r, "bg", "f1\n")
	wantStdout(t, r, "b", "1\n")
}

// A syntax error doesn't start the block, and doesn't end the shell.
func TestBackgroundSyntaxError(t *testing.T) {
	src := "```sh {#bg background}\nif then\n```\n```sh {#b}\necho b\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	if r["bg"].Status != 2 || len(r["bg"].Stderr) == 0 {
		t.Errorf("bg: status %d, stderr %q; want a syntax error", r["bg"].Status, r["bg"].Stderr)
	}
	wantStdout(t, r, "b", "b\n")
}

// timeout= kills a background block and what it started, not its
// shell, and the run goes on. A killed child may stay a zombie where
// nothing reaps orphans, so ps tells whether it still runs.
func TestBackgroundTimeout(t *testing.T) {
	src := "```sh {#bg background timeout=500ms}\nsleep 30 &\necho $! > child.pid\nsleep 30\n```\n" +
		"```sh {#b}\nsleep 1; echo b\n```\n" +
		"```sh {#c}\ncase $(ps -o stat= -p $(cat child.pid)) in ''|Z*) echo gone ;; *) echo alive ;; esac\n```\n"
	start := time.Now()
	r := runFIFO(t, src, 10*time.Second)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("run took %s", d)
	}
	bg := r["bg"]
	if bg.Exited || bg.Status != -1 || bg.Err == nil || !strings.Contains(bg.Err.Error(), "timed out after 500ms") {
		t.Errorf("bg: exited %v, status %d, err %v", bg.Exited, bg.Status, bg.Err)
	}
	wantStdout(t, r, "b", "b\n")
	wantStdout(t, r, "c", "gone\n")
}

// After the last block, a background block with no timeout= gets
// BackgroundWait to end, and is killed after that.
func TestBackgroundKilledAtEnd(t *testing.T) {
	src := "```sh {#quick background}\nsleep 0.5; echo quick\n```\n" +
		"```sh {#slow background}\necho slow; sleep 60\n```\n" +
		"```sh {#b}\necho b\n```\n"
	start := time.Now()
	r := runFIFO(t, src, 20*time.Second)
	d := time.Since(start)
	if d < run.BackgroundWait || d > run.BackgroundWait+5*time.Second {
		t.Errorf("run took %s, want about %s", d, run.BackgroundWait)
	}
	wantStdout(t, r, "quick", "quick\n")
	if r["quick"].Err != nil {
		t.Errorf("quick: %v", r["quick"].Err)
	}
	wantStdout(t, r, "slow", "slow\n")
	if err := r["slow"].Err; err == nil || !strings.Contains(err.Error(), "after the last block") {
		t.Errorf("slow: err %v", err)
	}
}

// A background block's Done comes when it ends, after later blocks
// started; closing a session kills one still running.
func TestBackgroundSessionEvents(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#bg background}\nsleep 0.3\n```\n```sh {#b}\nsleep 1\n```\n```sh {#forever background}\nsleep 60\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	opts := run.Options{Dir: t.TempDir(), Environ: []string{"PATH=" + os.Getenv("PATH")}}
	opts.OnEvent = func(e run.Event) {
		if e.Kind == run.Started || e.Kind == run.Done {
			events = append(events, map[run.EventKind]string{run.Started: "start ", run.Done: "done "}[e.Kind]+e.Block.ID)
		}
	}
	s, err := run.NewSession(d, opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := range 2 {
		if _, err := s.RunBlock(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.RunBlock(ctx, 2)
	if err != nil || !r.Running {
		t.Fatalf("forever: running %v, err %v", r.Running, err)
	}
	s.Close()
	want := "start bg,start b,done b,done bg,start forever,done forever"
	if got := strings.Join(events, ","); got != want {
		t.Errorf("events = %s, want %s", got, want)
	}
	if r.Running || r.Err == nil || !strings.Contains(r.Err.Error(), "killed") {
		t.Errorf("forever after Close: running %v, err %v", r.Running, r.Err)
	}
}
