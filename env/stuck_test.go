//go:build unix

package env

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// hidingShell is a shell that drops its stderr, and so the marker mdsh
// waits for there, as an interactive zsh whose prompt is drawn over it
// can: nothing it runs ever ends.
func hidingShell(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexec /bin/sh \"$@\" 2>/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// within fails the test if f hasn't returned after d.
func within(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s still waiting after %v", what, d)
	}
}

// TestStartContextStuck: a shell whose start never ends is killed once
// ctx ends, rather than holding whoever started it for good.
func TestStartContextStuck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var err error
	within(t, 5*time.Second, "StartContext", func() {
		var sh *Shell
		sh, err = StartContext(ctx, Config{Path: hidingShell(t), Env: []string{"PATH=" + os.Getenv("PATH")}})
		if sh != nil {
			sh.Close()
		}
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the deadline", err)
	}
}

// TestStackStartingShell: while a shell starts, the stack lists it in
// Processes, and KillBusy ends it, so neither the process list nor Kill
// Shell is left with nothing to act on.
func TestStackStartingShell(t *testing.T) {
	s := NewStack(Config{Path: hidingShell(t), Dir: t.TempDir(), Env: []string{"PATH=" + os.Getenv("PATH")}})
	defer s.Close()
	got := make(chan error, 1)
	go func() {
		_, err := s.Get(context.Background(), TopLevel, nil)
		got <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(s.Processes()) == 0 || !s.KillBusy() {
		if time.Now().After(deadline) {
			t.Fatalf("the starting shell isn't listed and busy: %+v", s.Processes())
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case err := <-got:
		if err == nil {
			t.Error("Get started a shell that was killed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Get still waiting after KillBusy")
	}
	if ps := s.Processes(); len(ps) != 0 {
		t.Errorf("Processes lists a killed shell: %+v", ps)
	}
}

// TestRunKillAfter: a script that won't be interrupted, a loop in the
// shell ignoring SIGINT, ends the shell killAfter after ctx ends.
func TestRunKillAfter(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var exited bool
	within(t, 5*time.Second, "RunKillAfter", func() {
		_, exited, err = sh.RunKillAfter(ctx, "trap '' INT\nwhile :; do :; done\n", io.Discard, io.Discard, 300*time.Millisecond)
	})
	if !exited || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("exited %v, err %v; want the shell killed and the deadline", exited, err)
	}
}

// TestTerminalMarkerHidden: a block on the terminal whose marker there
// never comes, here because the block made printf to the terminal print
// nothing, ends when stopped, and at once when its shell is killed.
func TestTerminalMarkerHidden(t *testing.T) {
	const hide = "printf() { [ -t 1 ] && return; command printf \"$@\"; }\n"
	for _, kill := range []bool{false, true} {
		name := "stop"
		if kill {
			name = "kill"
		}
		t.Run(name, func(t *testing.T) {
			sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
			if err != nil {
				t.Skip("no pseudo-terminals:", err)
			}
			defer sh.Close()
			// The block runs in the shell itself, so its printf stays.
			if _, _, err := sh.Run(context.Background(), hide, io.Discard, io.Discard); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ran := make(chan struct{})
			go func() {
				defer close(ran)
				sh.RunTerminal(ctx, "echo hi\n", Terminal{Out: io.Discard}, io.Discard)
			}()
			select {
			case <-ran:
				t.Skip("the marker wasn't hidden")
			case <-time.After(500 * time.Millisecond):
			}
			if kill {
				sh.Kill()
			} else {
				cancel()
			}
			select {
			case <-ran:
			case <-time.After(5 * time.Second):
				sh.Kill()
				t.Fatal("the run still waits for the terminal's marker")
			}
		})
	}
}
