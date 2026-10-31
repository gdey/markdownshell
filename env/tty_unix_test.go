//go:build unix

package env

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestRunTTY runs a script on a shell's own pseudo-terminal. RunTTY needs
// a controlling terminal to pass keys from, so the test binary is run
// again as a child whose controlling terminal is another pty, and the
// test types at that.
func TestRunTTY(t *testing.T) {
	if os.Getenv("MDSH_TTY_CHILD") == "1" {
		ttyChild()
		return
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer ptm.Close()
	var report bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunTTY$")
	cmd.Env = append(os.Environ(), "MDSH_TTY_CHILD=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pts, pts, &report
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pts.Close()

	var (
		mu    sync.Mutex
		shown strings.Builder
		read  = make(chan struct{})
	)
	go func() {
		defer close(read)
		buf := make([]byte, 1024)
		for {
			n, err := ptm.Read(buf)
			mu.Lock()
			shown.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	// Wait for what the script shows before typing, rather than for a
	// fixed time, which a loaded machine can outlast.
	waitFor := func(s string) {
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			ok := strings.Contains(shown.String(), s)
			mu.Unlock()
			if ok {
				return
			}
		}
		cmd.Process.Kill()
		t.Fatalf("terminal never showed %q", s)
	}
	waitFor("terminal ok")
	ptm.Write([]byte("typed\r"))
	waitFor("sleeping")
	ptm.Write([]byte{3}) // ctrl+c, during the sleep
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v\n%s", err, report.String())
	}
	ptm.Close()
	<-read
	out := shown.String()

	for _, want := range []string{"terminal ok", "got typed", "sleep ended 130"} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal shows %q, want %q in it", out, want)
		}
	}
	got := report.String()
	for _, want := range []string{"status=0 X=set\n", "recorded: terminal ok", "recorded: got typed"} {
		if !strings.Contains(got, want) {
			t.Errorf("child reported %q, want %q in it", got, want)
		}
	}
}

// ttyChild runs a block on its shell's terminal, then checks the shell
// survived ctrl+c and kept the block's variables, and reports what was
// recorded.
func ttyChild() {
	fail := func(err error) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fail(err)
	}
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		fail(err)
	}
	defer sh.Close()
	ctx := context.Background()
	// The sleep says so from its own process, which already ends on
	// ctrl+c, so the test's ctrl+c can't come between commands, where
	// the shell ignores it.
	script := "test -t 0 && test -t 1 && test -t 2 && echo terminal ok\nread -r l; echo \"got $l\"\nsh -c 'echo sleeping; exec sleep 5'; echo \"sleep ended $?\"\nX=set\n"
	var rec bytes.Buffer
	if _, _, err := sh.RunTTY(ctx, script, tty, &rec); err != nil {
		fail(err)
	}
	var out bytes.Buffer
	status, _, err := sh.Run(ctx, `echo "X=$X"`, &out, os.Stderr)
	if err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "status=%d %s", status, out.String())
	for _, l := range strings.Split(rec.String(), "\n") {
		fmt.Fprintf(os.Stderr, "recorded: %s\n", strings.TrimSpace(l))
	}
}

// A block that writes to /dev/tty outside RunTTY must not fill the
// terminal up and hang.
func TestTTYShellDrainsTerminal(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer sh.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	status, _, err := sh.Run(ctx, "head -c 200000 /dev/zero | tr '\\0' x >/dev/tty; echo done", &out, io.Discard)
	if err != nil || status != 0 || out.String() != "done\n" {
		t.Errorf("status %d, err %v, out %q", status, err, out.String())
	}
}

func TestRunScripted(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer sh.Close()
	ctx := context.Background()
	var out bytes.Buffer
	var mu sync.Mutex
	shown := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(p)
	})
	script := "test -t 0 && echo terminal ok\nstty size\nread -r l; X=$l\n"
	status, _, err := sh.RunScripted(ctx, script, 100, 30, shown, func(ctx context.Context, keys io.Writer) error {
		_, err := io.WriteString(keys, "typed\r")
		return err
	})
	if err != nil || status != 0 {
		t.Fatalf("status %d, err %v", status, err)
	}
	mu.Lock()
	got := out.String()
	mu.Unlock()
	for _, want := range []string{"terminal ok", "30 100", "typed"} {
		if !strings.Contains(got, want) {
			t.Errorf("screen %q, want %q in it", got, want)
		}
	}
	var x bytes.Buffer
	sh.Run(ctx, `printf %s "$X"`, &x, io.Discard)
	if x.String() != "typed" {
		t.Errorf("X = %q", x.String())
	}
}

func TestRunScriptedInputEnds(t *testing.T) {
	defer func(g time.Duration) { tapeGrace = g }(tapeGrace)
	tapeGrace = 200 * time.Millisecond
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer sh.Close()
	_, _, err = sh.RunScripted(context.Background(), "read -r l\n", 80, 24, io.Discard, func(context.Context, io.Writer) error { return nil })
	if !errors.Is(err, ErrTapeEnded) {
		t.Errorf("err = %v, want ErrTapeEnded", err)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// What a block writes to its terminal after a quiet spell, right as it
// ends, must not be cut off.
func TestRunScriptedShowsLastOutput(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer sh.Close()
	for i := range 20 {
		var out bytes.Buffer
		var mu sync.Mutex
		shown := writerFunc(func(p []byte) (int, error) {
			mu.Lock()
			defer mu.Unlock()
			return out.Write(p)
		})
		_, _, err := sh.RunScripted(context.Background(), "sleep 0.05; printf end", 80, 24, shown, func(context.Context, io.Writer) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		got := out.String()
		mu.Unlock()
		if got != "end" {
			t.Fatalf("run %d: screen %q, want %q", i, got, "end")
		}
	}
}

// TestRunTerminal runs a script on a terminal mdsh draws itself: keys
// come on a channel, and the size can change while it runs.
func TestRunTerminal(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer sh.Close()
	var (
		mu    sync.Mutex
		out   bytes.Buffer
		cols  = 100
		keys  = make(chan []byte, 1)
		sized = make(chan struct{}, 1)
	)
	shown := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(p)
	})
	term := Terminal{
		Keys: keys,
		Out:  shown,
		Size: func() (int, int) {
			mu.Lock()
			defer mu.Unlock()
			return cols, 30
		},
		Resized: sized,
	}
	go func() {
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			ready := strings.Contains(out.String(), "ready")
			if ready {
				cols = 120
			}
			mu.Unlock()
			if ready {
				sized <- struct{}{}
				keys <- []byte("typed\r")
				return
			}
		}
	}()
	var rec bytes.Buffer
	script := "test -t 0 && echo terminal ok\nstty size\necho ready\nread -r l; X=$l\nstty size\n"
	status, _, err := sh.RunTerminal(context.Background(), script, term, &rec)
	if err != nil || status != 0 {
		t.Fatalf("status %d, err %v", status, err)
	}
	mu.Lock()
	got := out.String()
	mu.Unlock()
	for _, want := range []string{"terminal ok", "30 100", "typed", "30 120"} {
		if !strings.Contains(got, want) {
			t.Errorf("screen %q, want %q in it", got, want)
		}
	}
	if !strings.Contains(rec.String(), "terminal ok") {
		t.Errorf("recorded %q", rec.String())
	}
	var x bytes.Buffer
	if _, _, err := sh.Run(context.Background(), `echo "$X"`, &x, io.Discard); err != nil || x.String() != "typed\n" {
		t.Errorf("X = %q, err %v", x.String(), err)
	}
}

// TestRunTerminalSuspend: ctrl+z, and a program that stops itself as
// vim does on ctrl+z, have no shell with job control to come back to, so
// the stop signals are ignored rather than stopping the block for good.
// (Linux already drops them for the shell's orphaned process group;
// macOS stops a program that sends one to itself.)
func TestRunTerminalSuspend(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer sh.Close()
	var (
		mu   sync.Mutex
		out  bytes.Buffer
		keys = make(chan []byte, 2)
	)
	shown := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(p)
	})
	go func() {
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			ready := strings.Contains(out.String(), "ready")
			mu.Unlock()
			if ready {
				keys <- []byte("\x1a")
				time.Sleep(200 * time.Millisecond)
				keys <- []byte("on\r")
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	script := "sh -c 'kill -TSTP 0; echo ready; read -r l; echo carried $l'\n"
	status, _, err := sh.RunTerminal(ctx, script, Terminal{Keys: keys, Out: shown}, io.Discard)
	mu.Lock()
	screen := out.String()
	mu.Unlock()
	if err != nil || status != 0 {
		t.Fatalf("status %d, err %v; screen %q", status, err, screen)
	}
	if !strings.Contains(screen, "carried on") {
		t.Errorf("screen %q", screen)
	}
	// The shell is still good for the next block, and stops as usual.
	var o bytes.Buffer
	if _, _, err := sh.Run(context.Background(), "trap; echo next", &o, io.Discard); err != nil || !strings.HasSuffix(o.String(), "next\n") || strings.Contains(o.String(), "TSTP") {
		t.Errorf("next block: %q, %v", o.String(), err)
	}
}

// TestRunTerminalJobs: with job control, ctrl+z stops the command and
// the block ends, and fg in the next block brings it back.
func TestRunTerminalJobs(t *testing.T) {
	for _, path := range []string{"/bin/sh", "bash", "zsh"} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := exec.LookPath(path); err != nil {
				t.Skip(err)
			}
			if path == "zsh" {
				t.Skip("zsh has no job control unless interactive; TestRunTerminalSuspend covers ctrl+z being ignored")
			}
			sh, err := Start(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
			if err != nil {
				t.Skip("no pseudo-terminals:", err)
			}
			defer sh.Close()
			var (
				mu  sync.Mutex
				out bytes.Buffer
			)
			shown := writerFunc(func(p []byte) (int, error) {
				mu.Lock()
				defer mu.Unlock()
				return out.Write(p)
			})
			// type sends keys once the screen shows want.
			typeAfter := func(want string, keys ...string) chan []byte {
				ch := make(chan []byte, len(keys))
				go func() {
					for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
						mu.Lock()
						ok := strings.Contains(out.String(), want)
						mu.Unlock()
						if ok {
							for _, k := range keys {
								ch <- []byte(k)
								time.Sleep(100 * time.Millisecond)
							}
							return
						}
					}
				}()
				return ch
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			term := Terminal{Out: shown, JobControl: true}
			// Some shells can't do job control without being
			// interactive, such as bash; there ctrl+z
			// is ignored, as TestRunTerminalSuspend checks.
			var o bytes.Buffer
			if _, _, err := sh.RunTerminal(ctx, ":\n", term, io.Discard); err != nil {
				t.Fatal(err)
			}
			if _, _, err := sh.Run(ctx, "echo $__mdsh_jobs", &o, io.Discard); err != nil || o.String() != "yes\n" {
				t.Skipf("no job control here: %q, %v", o.String(), err)
			}
			term.Keys = typeAfter("ready", "\x1a")
			status, _, err := sh.RunTerminal(ctx, "sh -c 'echo ready; read -r l; echo carried $l'\n", term, io.Discard)
			if err != nil || status == 0 {
				t.Fatalf("stopping: status %d, err %v; screen %q", status, err, out.String())
			}
			mu.Lock()
			out.Reset()
			mu.Unlock()
			term.Keys = typeAfter("", "on\r")
			status, _, err = sh.RunTerminal(ctx, "fg >/dev/null\n", term, io.Discard)
			mu.Lock()
			screen := out.String()
			mu.Unlock()
			if err != nil || status != 0 || !strings.Contains(screen, "carried on") {
				t.Fatalf("fg: status %d, err %v; screen %q", status, err, screen)
			}
			// Job control is off again for other blocks.
			o.Reset()
			if _, _, err := sh.Run(context.Background(), "case $- in *m*) echo on;; *) echo off;; esac", &o, io.Discard); err != nil || o.String() != "off\n" {
				t.Errorf("after: %q, %v", o.String(), err)
			}
		})
	}
}
