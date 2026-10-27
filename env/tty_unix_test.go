//go:build unix

package env

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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

	screen := make(chan string)
	go func() {
		b, _ := io.ReadAll(ptm)
		screen <- string(b)
	}()
	time.Sleep(time.Second)
	ptm.Write([]byte("typed\r"))
	time.Sleep(500 * time.Millisecond)
	ptm.Write([]byte{3}) // ctrl+c, during the sleep
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v\n%s", err, report.String())
	}
	ptm.Close()
	out := <-screen

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
	script := "test -t 0 && test -t 1 && test -t 2 && echo terminal ok\nread -r l; echo \"got $l\"\nsleep 5; echo \"sleep ended $?\"\nX=set\n"
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
