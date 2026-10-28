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
