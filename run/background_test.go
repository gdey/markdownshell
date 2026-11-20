//go:build unix

package run_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
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
	if !bg.TimedOut || bg.Duration < 500*time.Millisecond || bg.Duration > 5*time.Second {
		t.Errorf("bg: timed out %v after %s, want true after about 500ms", bg.TimedOut, bg.Duration)
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

// StopBackground kills a background block that outlived the run that
// started it; its result comes with the next Reap, marked stopped.
func TestStopBackground(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#forever background}\necho up; sleep 60\n```\n```sh {#b}\necho b\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := run.NewSession(d, run.Options{Dir: t.TempDir(), Environ: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	r, err := s.RunBlock(ctx, 0)
	if err != nil || !r.Running {
		t.Fatalf("forever: running %v, err %v", r.Running, err)
	}
	if s.StopBackground(d.Blocks[1]) {
		t.Error("StopBackground of a block not in the background = true")
	}
	if !s.StopBackground(d.Blocks[0]) {
		t.Fatal("StopBackground = false, want true")
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		s.Reap()
	}
	if r.Running || r.Err == nil || r.Err.Error() != "stopped" {
		t.Errorf("after StopBackground: running %v, err %v", r.Running, r.Err)
	}
	if s.StopBackground(d.Blocks[0]) {
		t.Error("StopBackground of a block that ended = true")
	}
}

// Processes names a background block's process by its block, and
// KillProcess on it ends the block, which Reap reports.
func TestProcessesNamesBackgroundBlocks(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#server background}\nsleep 60\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := run.NewSession(d, run.Options{Dir: t.TempDir(), Environ: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.RunBlock(context.Background(), 0)
	if err != nil || !r.Running {
		t.Fatalf("server: running %v, err %v", r.Running, err)
	}
	pid := 0
	for _, sh := range s.Processes() {
		for _, p := range sh.Children {
			if p.Block == "server" {
				pid = p.PID
			}
		}
	}
	if pid == 0 {
		t.Fatalf("no process named server: %+v", s.Processes())
	}
	if !s.KillProcess(pid) {
		t.Fatal("KillProcess = false")
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		s.Reap()
	}
	if r.Running || r.Err == nil {
		t.Errorf("after KillProcess: running %v, err %v", r.Running, r.Err)
	}
}

// bgTerminal is a terminal for background tty blocks, as a front end
// gives them, with the keys typed at each.
type bgTerminal struct {
	mu       sync.Mutex
	keys     map[string]chan []byte
	shown    map[string]*strings.Builder
	released map[string]bool
}

func newBGTerminal() *bgTerminal {
	return &bgTerminal{keys: map[string]chan []byte{}, shown: map[string]*strings.Builder{}, released: map[string]bool{}}
}

func (bt *bgTerminal) open(b *doc.Block) (*env.Terminal, func()) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	k := make(chan []byte, 4)
	bt.keys[b.ID] = k
	bt.shown[b.ID] = &strings.Builder{}
	return &env.Terminal{Keys: k, Out: shownWriter{bt, b.ID}, Size: func() (int, int) { return 60, 10 }}, func() {
		bt.mu.Lock()
		bt.released[b.ID] = true
		bt.mu.Unlock()
	}
}

func (bt *bgTerminal) show(id string) string {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if s := bt.shown[id]; s != nil {
		return s.String()
	}
	return ""
}

type shownWriter struct {
	bt *bgTerminal
	id string
}

func (w shownWriter) Write(p []byte) (int, error) {
	w.bt.mu.Lock()
	defer w.bt.mu.Unlock()
	return w.bt.shown[w.id].Write(p)
}

// A background tty block runs on a terminal of its own while other
// blocks run, with the shell's state, takes keys typed there, isn't
// killed at the end of a run, and is recorded once it ends.
func TestBackgroundTTY(t *testing.T) {
	src := "```sh {#a}\nX=1\nf() { echo f$X; }\n```\n" +
		"```sh {#edit background tty}\nf; [ -t 0 ] && echo on-tty; read -r l; echo got $l\n```\n" +
		"```sh {#b}\necho b$X\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	bt := newBGTerminal()
	dir := t.TempDir()
	opts := run.Options{Dir: dir, File: filepath.Join(dir, "doc.md"), Environ: []string{"PATH=" + os.Getenv("PATH")}, BackgroundTerminal: bt.open}
	if _, err := exec.LookPath("bash"); err == nil {
		opts.Shell = "bash"
	}
	s, err := run.NewSession(d, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	start := time.Now()
	if err := s.RunFrom(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > run.BackgroundWait {
		t.Errorf("the run waited %s for the tty block", d)
	}
	r := s.Results()
	wantStdout(t, map[string]*run.Result{"b": r[2]}, "b", "b1\n")
	if !r[1].Running {
		t.Fatalf("edit isn't running after the run: %+v", r[1])
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(bt.show("edit"), "on-tty") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := bt.show("edit"); !strings.Contains(got, "f1\r\non-tty") {
		t.Errorf("edit shows %q", got)
	}
	found := false
	for _, sh := range s.Processes() {
		for _, p := range sh.Children {
			found = found || p.Block == "edit"
		}
	}
	if !found {
		t.Errorf("Processes has no edit: %+v", s.Processes())
	}

	bt.mu.Lock()
	bt.keys["edit"] <- []byte("done\r")
	bt.mu.Unlock()
	for r[1].Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		s.Reap()
	}
	if r[1].Running || r[1].Err != nil || r[1].Status != 0 {
		t.Fatalf("edit: running %v, status %d, err %v", r[1].Running, r[1].Status, r[1].Err)
	}
	if !strings.HasSuffix(r[1].Image, ".cast") {
		t.Errorf("edit's recording = %q, want a cast", r[1].Image)
	}
	bt.mu.Lock()
	released := bt.released["edit"]
	bt.mu.Unlock()
	if !released {
		t.Error("edit's terminal wasn't released")
	}
}

// Without a terminal to give it, a background tty block fails.
func TestBackgroundTTYNoTerminal(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#edit background tty}\nvi\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := run.NewSession(d, run.Options{Dir: t.TempDir(), Environ: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, _ := s.RunBlock(context.Background(), 0)
	if r == nil || !errors.Is(r.Err, run.ErrNoBackgroundTTY) {
		t.Errorf("result %+v, want ErrNoBackgroundTTY", r)
	}
}

// Stopping a background tty block kills it, and closing the session
// kills one still running.
func TestStopBackgroundTTY(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#a background tty}\nsleep 60\n```\n```sh {#b background tty}\nsleep 60\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	bt := newBGTerminal()
	s, err := run.NewSession(d, run.Options{Dir: t.TempDir(), Environ: []string{"PATH=" + os.Getenv("PATH")}, BackgroundTerminal: bt.open})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ra, _ := s.RunBlock(ctx, 0)
	rb, _ := s.RunBlock(ctx, 1)
	if !ra.Running || !rb.Running {
		t.Fatalf("running: a %v, b %v", ra.Running, rb.Running)
	}
	if !s.StopBackground(d.Blocks[0]) {
		t.Fatal("StopBackground = false")
	}
	deadline := time.Now().Add(5 * time.Second)
	for ra.Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		s.Reap()
	}
	if ra.Running || ra.Err == nil || ra.Err.Error() != "stopped" {
		t.Errorf("a after StopBackground: running %v, err %v", ra.Running, ra.Err)
	}
	s.Close()
	if rb.Running || rb.Err == nil {
		t.Errorf("b after Close: running %v, err %v", rb.Running, rb.Err)
	}
}
