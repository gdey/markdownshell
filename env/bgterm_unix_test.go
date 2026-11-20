//go:build unix

package env

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitShows waits until the terminal shows want.
func waitShows(t *testing.T, out *syncBuf, want string) {
	t.Helper()
	for range 250 {
		if strings.Contains(out.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("terminal shows %q, want %q in it", out.String(), want)
}

// TestBackgroundTerminal runs a job on a terminal of its own, in each
// shell there is, and checks it has the shell's state, its terminal is
// its controlling terminal, keys reach it, and the shell goes on
// running blocks meanwhile.
func TestBackgroundTerminal(t *testing.T) {
	for _, name := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			sh, err := Start(Config{Path: path, Env: []string{"PATH=/usr/bin:/bin", "HOME=" + dir}, Dir: "/", Args: []string{"one", "two words"}})
			if err != nil {
				t.Fatal(err)
			}
			defer sh.Close()
			ctx := context.Background()
			setup := "cd " + quote(dir) + "\nEXP=\"it's\"; export EXP\nPLAIN='a\nb'\nset -- first 'sec ond'\n"
			if name != "sh" {
				setup += "greet() { echo \"hi $1\"; }\n"
			}
			if _, _, err := sh.Run(ctx, setup, io.Discard, io.Discard); err != nil {
				t.Fatal(err)
			}
			keys := make(chan []byte, 4)
			var out, rec syncBuf
			script := `echo "exp=$EXP plain=$PLAIN pwd=$(pwd) args=$# $2"
[ -t 0 ] && echo tty-in
sh -c ': </dev/tty' 2>/dev/null && echo ctty
`
			if name != "sh" {
				script += "greet you\n"
			}
			script += "read -r line; echo \"got $line\"; exit 3\n"
			j, err := sh.BackgroundTerminal(ctx, script, "BG", Terminal{Keys: keys, Out: &out, Size: func() (int, int) { return 100, 30 }}, &rec)
			if err != nil {
				t.Fatal(err)
			}
			want := "exp=it's plain=a\r\nb pwd=" + dir + " args=2 sec ond"
			waitShows(t, &out, "tty-in")
			if !strings.Contains(out.String(), want) {
				t.Errorf("terminal shows %q, want %q", out.String(), want)
			}
			waitShows(t, &out, "ctty")
			if name != "sh" {
				waitShows(t, &out, "hi you")
			}

			// The shell runs blocks while the job waits for a line.
			var o bytes.Buffer
			if _, _, err := sh.Run(ctx, `echo "$BG"`, &o, io.Discard); err != nil || strings.TrimSpace(o.String()) == "" {
				t.Errorf("shell while the job runs: %q, %v", o.String(), err)
			}
			if pids := sh.termJobPIDs(); len(pids) != 1 || pids[0] != j.PID {
				t.Errorf("term jobs %v, want [%d]", pids, j.PID)
			}

			keys <- []byte("hello\r")
			if status, err := waitJob(t, j); status != 3 || err != nil {
				t.Errorf("status %d, err %v; want 3", status, err)
			}
			if !strings.Contains(rec.String(), "got hello") {
				t.Errorf("recording %q has no got hello", rec.String())
			}
			if pids := sh.termJobPIDs(); len(pids) != 0 {
				t.Errorf("term jobs %v after the job ended", pids)
			}
		})
	}
}

// TestBackgroundTerminalOptions checks the shell's set -e and set -u
// carry over to the job.
func TestBackgroundTerminalOptions(t *testing.T) {
	sh, err := Start(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	ctx := context.Background()
	sh.Run(ctx, "set -u", io.Discard, io.Discard)
	var out syncBuf
	j, err := sh.BackgroundTerminal(ctx, "echo before; echo $UNSET_VAR; echo after", "", Terminal{Out: &out}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := waitJob(t, j); status == 0 || err != nil {
		t.Errorf("status %d, err %v; want set -u to fail it", status, err)
	}
	if s := out.String(); !strings.Contains(s, "before") || strings.Contains(s, "after") {
		t.Errorf("terminal shows %q", s)
	}
	// The shell's state, which may hold secrets, isn't left on disk.
	if left, _ := filepath.Glob(filepath.Join(sh.tmp, "bg*", "state.sh")); len(left) > 0 {
		t.Errorf("state left behind: %v", left)
	}
}

// TestBackgroundTerminalKill kills a job, and checks closing the shell
// kills one too, since they are in sessions of their own.
func TestBackgroundTerminalKill(t *testing.T) {
	sh, err := Start(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var out syncBuf
	j, err := sh.BackgroundTerminal(ctx, "echo up; sleep 30", "", Terminal{Out: &out}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	waitShows(t, &out, "up")
	start := time.Now()
	j.Kill()
	if status, err := waitJob(t, j); status != -1 || !errors.Is(err, ErrKilled) {
		t.Errorf("status %d, err %v; want killed", status, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("kill took %s", d)
	}

	j, err = sh.BackgroundTerminal(ctx, "sleep 30", "", Terminal{Out: io.Discard}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	sh.Close()
	select {
	case <-j.Ended():
	case <-time.After(5 * time.Second):
		t.Fatal("closing the shell left the job running")
	}
}

// TestBackgroundTerminalSyntax checks a job with a syntax error doesn't
// start.
func TestBackgroundTerminalSyntax(t *testing.T) {
	sh, err := Start(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	j, err := sh.BackgroundTerminal(context.Background(), "if then", "", Terminal{Out: io.Discard}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if status, done, _ := j.Done(); !done || status != 2 {
		t.Errorf("done %v status %d; want done with 2", done, status)
	}
}
