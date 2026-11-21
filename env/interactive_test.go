//go:build unix

package env

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// startI starts shell interactively with a home of its own, with rc
// files as given (name → contents), and closes it when the test ends.
func startI(t *testing.T, shell string, rc map[string]string, args ...string) *Shell {
	t.Helper()
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	ptm.Close()
	pts.Close()
	home := t.TempDir()
	for name, text := range rc {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Start(Config{
		Path:        shell,
		Interactive: true,
		Args:        args,
		Env:         []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TERM=xterm", noCompinit},
		Dir:         home,
		KillAfter:   10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// noCompinit keeps Debian and Ubuntu's /etc/zsh/zshrc from running
// compinit, which on CI runners finds fpath directories it calls
// insecure and stops to ask whether to go on.
const noCompinit = "skip_global_compinit=1"

// runI runs script and returns its status and output, escapes taken out.
func runI(t *testing.T, s *Shell, script string) (int, string) {
	t.Helper()
	var out syncBuf
	rc, exited, err := s.Run(context.Background(), script, &out, &out)
	if err != nil || exited {
		t.Fatalf("%q: exited %v, %v; output %q", script, exited, err, out.String())
	}
	return rc, clean(out.String())
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(stripEsc(s), "\r", ""))
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// stoppedStatus is a command's status when ctrl+z stopped it.
var stoppedStatus = 128 + int(syscall.SIGTSTP)

func TestInteractiveRun(t *testing.T) {
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			s := startI(t, sh, nil, "one", "two")
			if rc, out := runI(t, s, `echo "$# $1"; x=5; f() { echo "f $*"; }`); rc != 0 || out != "2 one" {
				t.Errorf("args: %d %q", rc, out)
			}
			if rc, out := runI(t, s, "f $x; false"); rc != 1 || out != "f 5" {
				t.Errorf("state carried: %d %q", rc, out)
			}
			if rc, out := runI(t, s, "cd /; echo err >&2; (exit 7)"); rc != 7 || out != "err" {
				t.Errorf("stderr shares the terminal: %d %q", rc, out)
			}
			st, ok := s.State()
			if !ok || st.PWD != "/" || st.Status != 7 || st.PID != s.PID() {
				t.Errorf("state: %+v", st)
			}
			exp, err := s.Exported(context.Background())
			if err != nil || exp["HOME"] == "" || exp["MDSH_INTEGRATED"] != "1" {
				t.Errorf("exported: %v %v", err, exp)
			}
			for k := range exp {
				if strings.HasPrefix(k, "__mdsh_") {
					t.Errorf("exported has %s", k)
				}
			}
			if err := s.Want([]string{"x", "unset_one", "bad name"}); err != nil {
				t.Fatal(err)
			}
			runI(t, s, ":")
			st, _ = s.State()
			if len(st.Vars) != 1 || st.Vars["x"] != "5" {
				t.Errorf("wanted vars: %v", st.Vars)
			}
			// A syntax error runs nothing.
			var out syncBuf
			rc, _, err := s.Run(context.Background(), "echo ran\nif", &out, &out)
			if rc != 2 || err != nil || strings.Contains(out.String(), "ran") {
				t.Errorf("syntax error: %d %v %q", rc, err, out.String())
			}
		})
	}
}

// waitFor polls cond for up to 5s.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestInteractiveJobControl(t *testing.T) {
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			s := startI(t, sh, nil)
			// ctrl+z stops the command; the shell carries on.
			go func() {
				waitFor(t, "sleep to run", func() bool { return s.Busy() && fg(s) })
				s.Type([]byte{0x1a})
			}()
			var out syncBuf
			rc, _, err := s.RunLine(context.Background(), "sleep 30", &out)
			if err != nil || rc != stoppedStatus {
				t.Fatalf("ctrl+z: %d %v %q", rc, err, out.String())
			}
			jobs := s.Jobs()
			if len(jobs) != 1 || !jobs[0].Stopped() || !strings.Contains(jobs[0].Command, "sleep 30") {
				t.Fatalf("jobs after ctrl+z: %+v", jobs)
			}
			pg := jobs[0].PGID
			// bg runs it on.
			if rc, _, err := s.RunLine(context.Background(), "bg", &out); err != nil || rc != 0 {
				t.Fatalf("bg: %d %v", rc, err)
			}
			waitFor(t, "job running", func() bool {
				runI(t, s, ":")
				j := s.Jobs()
				return len(j) == 1 && j[0].State == "running" && j[0].PGID == pg
			})
			// fg brings it back; ctrl+c ends it.
			go func() {
				waitFor(t, "fg", func() bool { return s.Busy() && fg(s) })
				s.Type([]byte{0x03})
			}()
			if rc, _, err := s.RunLine(context.Background(), "fg", &out); err != nil || rc != 130 {
				t.Fatalf("fg then ctrl+c: %d %v %q", rc, err, out.String())
			}
			runI(t, s, ":")
			if j := s.Jobs(); len(j) != 0 {
				t.Errorf("jobs after ctrl+c: %+v", j)
			}
			// A stopped job ends with the shell.
			go func() {
				waitFor(t, "sleep to run", func() bool { return s.Busy() && fg(s) })
				s.Type([]byte{0x1a})
			}()
			s.RunLine(context.Background(), "sleep 31", &out)
			jobs = s.Jobs()
			if len(jobs) != 1 {
				t.Fatalf("jobs: %+v", jobs)
			}
			s.Close()
			waitFor(t, "job to end", func() bool { return syscall.Kill(-jobs[0].PGID, 0) != nil })
		})
	}
}

// fg reports whether something other than the shell has the terminal.
func fg(s *Shell) bool {
	pg, err := tcgetpgrp(s.ia.ptm)
	return err == nil && pg != s.PID()
}

func TestInteractiveBusy(t *testing.T) {
	sh := shells(t)[0]
	s := startI(t, sh, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		var out syncBuf
		rc, exited, err := s.Run(ctx, "sleep 20; echo after", &out, &out)
		if rc != 130 || exited || !errors.Is(err, context.DeadlineExceeded) || strings.Contains(out.String(), "after") {
			t.Errorf("timeout: %d %v %v %q", rc, exited, err, out.String())
		}
	}()
	waitFor(t, "busy", func() bool { return s.Busy() && fg(s) })
	ia := s.ia
	// Another line can't be typed while a command runs; RunLine
	// itself waits its turn, so ask runLine as RunLine would.
	if _, _, err := s.runLine(context.Background(), "true", nil); !errors.Is(err, ErrBusy) {
		t.Errorf("while running: %v", err)
	}
	<-done
	// Still alive.
	if rc, out := runI(t, s, "echo alive"); rc != 0 || out != "alive" {
		t.Errorf("after timeout: %d %q", rc, out)
	}
	// Keys typed at the prompt keep mdsh from typing over them.
	s.Type([]byte("ech"))
	if _, _, err := s.RunLine(context.Background(), "true", nil); !errors.Is(err, ErrLineTyped) {
		t.Errorf("half-typed line: %v", err)
	}
	s.Type([]byte{0x15, '\r'}) // ctrl+u, Enter: an empty line
	waitFor(t, "prompt", func() bool {
		ia.mu.Lock()
		defer ia.mu.Unlock()
		return !ia.dirty && ia.phase == phaseInput
	})
	if rc, out := runI(t, s, "echo ok"); rc != 0 || out != "ok" {
		t.Errorf("after clearing the line: %d %q", rc, out)
	}
}

func TestInteractiveExit(t *testing.T) {
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			s := startI(t, sh, nil)
			var out syncBuf
			rc, exited, err := s.Run(context.Background(), "echo bye; exit 3", &out, &out)
			if !exited || !errors.Is(err, ErrExited) || rc != 3 || !strings.Contains(out.String(), "bye") {
				t.Errorf("exit: %d %v %v %q", rc, exited, err, out.String())
			}
			if _, _, err := s.RunLine(context.Background(), "true", nil); !errors.Is(err, ErrExited) {
				t.Errorf("after exit: %v", err)
			}
		})
	}
}

// TestInteractiveRCFiles: the user's rc files run, slow or noisy, and
// marks they print themselves don't confuse mdsh.
func TestInteractiveRCFiles(t *testing.T) {
	foreign := `printf '\033]133;D;0\007\033]133;A\007'`
	rcs := map[string]map[string]string{
		"bash": {".bashrc": "sleep 1; echo hello from bashrc; mine=1\nPROMPT_COMMAND=\"" + foreign + "\"\nPS1='my> '\n"},
		"zsh":  {".zshrc": "sleep 1; echo hello from zshrc; mine=1\nprecmd() { " + foreign + " }\nPS1='my> '\n"},
		"sh":   {".shrc": "sleep 1; echo hello from shrc; mine=1\nPS1='my> '\n"},
	}
	for _, sh := range shells(t) {
		name := filepath.Base(sh)
		t.Run(name, func(t *testing.T) {
			s := startShWithENV(t, sh, rcs[name])
			var view syncBuf
			s.View(&view)
			if code, out := runI(t, s, "echo $mine"); code != 0 || out != "1" {
				t.Errorf("rc var: %d %q", code, out)
			}
			waitFor(t, "the user's prompt", func() bool { return strings.Contains(view.String(), "my> ") })
		})
	}
}

// startShWithENV is startI, with $ENV pointing at ~/.shrc for sh.
func startShWithENV(t *testing.T, sh string, rc map[string]string) *Shell {
	t.Helper()
	home := t.TempDir()
	for name, text := range rc {
		os.WriteFile(filepath.Join(home, name), []byte(text), 0o600)
	}
	env := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TERM=xterm", noCompinit}
	if filepath.Base(sh) == "sh" {
		env = append(env, "ENV="+filepath.Join(home, ".shrc"))
	}
	s, err := Start(Config{Path: sh, Interactive: true, Env: env, Dir: home})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestInteractiveHistory: the lines mdsh types stay out of history.
func TestInteractiveHistory(t *testing.T) {
	for _, sh := range shells(t) {
		name := filepath.Base(sh)
		if name != "bash" && name != "zsh" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			s := startI(t, sh, map[string]string{".zshrc": "HISTSIZE=100\n"})
			s.ia.mu.Lock()
			n := s.ia.states
			s.ia.mu.Unlock()
			s.Type([]byte("echo typed\r"))
			waitFor(t, "typed line", func() bool {
				s.ia.mu.Lock()
				defer s.ia.mu.Unlock()
				return s.ia.states > n && s.ia.phase == phaseInput
			})
			runI(t, s, "echo played")
			var out syncBuf
			s.RunLine(context.Background(), "fc -ln 1", &out)
			h := clean(out.String())
			if !strings.Contains(h, "echo typed") || strings.Contains(h, s.tmp) {
				t.Errorf("history: %q", h)
			}
		})
	}
}

func TestKindOf(t *testing.T) {
	for path, want := range map[string]shellKind{"/bin/sh": kindSh, "/bin/dash": kindSh, "/opt/homebrew/bin/bash": kindBash, "/bin/zsh": kindZsh, "/bin/ksh": kindSh} {
		if k, err := kindOf(path); err != nil || k != want {
			t.Errorf("%s: %v %v", path, k, err)
		}
	}
	if _, err := kindOf("/usr/bin/fish"); err == nil {
		t.Error("fish: no error")
	}
}

// TestInteractiveBackground: a background block starts on an
// interactive shell too, though the shell reports the job as it goes.
func TestInteractiveBackground(t *testing.T) {
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			s := startI(t, sh, nil)
			j, err := s.Background(context.Background(), "echo from-bg", "BGPID")
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, "job", func() bool { _, done, _ := j.Done(); return done })
			if out, _ := j.Output(); string(out) != "from-bg\n" {
				t.Errorf("output %q", out)
			}
			// Through a file: the shell reports the job done on the
			// terminal, as it likes, mixed with anything printed there.
			f := filepath.Join(t.TempDir(), "pid")
			runI(t, s, `printf '%s' "$BGPID" >`+f)
			if got, _ := os.ReadFile(f); string(got) != strconv.Itoa(j.PID) {
				t.Errorf("BGPID %q, PID %d", got, j.PID)
			}
		})
	}
}

// TestInteractiveTypedEvents: a command typed at the terminal, rather
// than by RunLine, is reported with its text, status and output.
func TestInteractiveTypedEvents(t *testing.T) {
	for _, sh := range shells(t) {
		name := filepath.Base(sh)
		t.Run(name, func(t *testing.T) {
			s := startI(t, sh, nil)
			var mu sync.Mutex
			var evs []Event
			s.Watch(func(e Event) {
				if e.Kind == "state" {
					return
				}
				mu.Lock()
				evs = append(evs, e)
				mu.Unlock()
			})
			ended := func(n int) func() bool {
				return func() bool {
					mu.Lock()
					defer mu.Unlock()
					c := 0
					for _, e := range evs {
						if e.Kind == "end" {
							c++
						}
					}
					return c >= n
				}
			}
			s.Type([]byte("echo hi; (exit 3)\r"))
			waitFor(t, "typed command", ended(1))
			runI(t, s, "echo played")
			mu.Lock()
			got := append([]Event(nil), evs...)
			mu.Unlock()
			if len(got) != 4 {
				t.Fatalf("events: %+v", got)
			}
			if got[0].Kind != "start" || got[0].Typed || got[0].Command != "echo hi; (exit 3)" {
				t.Errorf("start: %+v", got[0])
			}
			if got[1].Kind != "end" || got[1].Typed || got[1].Status != 3 || clean(string(got[1].Output)) != "hi" {
				t.Errorf("end: %+v %q", got[1], got[1].Output)
			}
			if !got[2].Typed || !got[3].Typed || got[3].Output != nil {
				t.Errorf("RunLine's events: %+v", got[2:])
			}
		})
	}
}

// TestInteractiveTypedNote: a comment typed at the terminal is reported
// with its text, as a line that ran nothing in bash and zsh, and as a
// command in sh, which marks it as one.
func TestInteractiveTypedNote(t *testing.T) {
	for _, sh := range shells(t) {
		name := filepath.Base(sh)
		t.Run(name, func(t *testing.T) {
			s := startI(t, sh, nil)
			var mu sync.Mutex
			var cmd []string
			s.Watch(func(e Event) {
				if e.Kind == "line" || e.Kind == "start" {
					mu.Lock()
					cmd = append(cmd, e.Kind+" "+e.Command)
					mu.Unlock()
				}
			})
			s.Type([]byte("# a note\r"))
			want := "line # a note"
			if name == "sh" {
				want = "start # a note"
			}
			waitFor(t, "the note", func() bool {
				mu.Lock()
				defer mu.Unlock()
				return len(cmd) > 0
			})
			mu.Lock()
			defer mu.Unlock()
			if cmd[0] != want {
				t.Errorf("got %q, want %q", cmd, want)
			}
		})
	}
}

// TestInteractiveRedraw: the prompt drawn again, as bash and zsh draw
// it when the terminal is resized, with a line half typed, isn't a line
// that ran: the line runs whole when enter is pressed, and RunLine
// still knows it is being typed.
func TestInteractiveRedraw(t *testing.T) {
	for _, sh := range shells(t) {
		name := filepath.Base(sh)
		if name == "sh" {
			continue // its prompt isn't drawn again
		}
		t.Run(name, func(t *testing.T) {
			s := startI(t, sh, nil)
			var mu sync.Mutex
			var cmd []string
			s.Watch(func(e Event) {
				if e.Kind == "line" || e.Kind == "start" {
					mu.Lock()
					cmd = append(cmd, e.Kind+" "+e.Command)
					mu.Unlock()
				}
			})
			s.Type([]byte("echo re"))
			time.Sleep(200 * time.Millisecond)
			s.SetSize(100, 30)
			time.Sleep(300 * time.Millisecond)
			if _, _, err := s.RunLine(context.Background(), "true", io.Discard); !errors.Is(err, ErrLineTyped) {
				t.Errorf("RunLine with a line half typed: %v", err)
			}
			s.Type([]byte("drawn\r"))
			waitFor(t, "the command", func() bool {
				mu.Lock()
				defer mu.Unlock()
				return len(cmd) > 0
			})
			mu.Lock()
			defer mu.Unlock()
			if want := []string{"start echo redrawn"}; !slices.Equal(cmd, want) {
				t.Errorf("got %q, want %q", cmd, want)
			}
		})
	}
}

// TestInteractiveSuspend: RunLine while a command typed at the terminal
// holds the shell stops it, runs the line and brings it back; one that
// won't stop leaves RunLine busy.
func TestInteractiveSuspend(t *testing.T) {
	for _, sh := range shells(t) {
		name := filepath.Base(sh)
		t.Run(name, func(t *testing.T) {
			s := startI(t, sh, nil)
			var mu sync.Mutex
			var evs []Event
			s.Watch(func(e Event) {
				if e.Kind == "start" || e.Kind == "end" {
					mu.Lock()
					evs = append(evs, e)
					mu.Unlock()
				}
			})
			s.Type([]byte("sleep 30\r"))
			waitFor(t, "sleep to run", func() bool { return fg(s) })
			if rc, out := runI(t, s, "echo played"); rc != 0 || out != "played" {
				t.Fatalf("played: %d %q", rc, out)
			}
			waitFor(t, "sleep back", func() bool {
				mu.Lock()
				defer mu.Unlock()
				return len(evs) > 0 && evs[len(evs)-1].Resumed
			})
			waitFor(t, "sleep in the foreground", func() bool { return fg(s) })
			s.Type([]byte{0x03})
			waitFor(t, "prompt", func() bool {
				s.ia.mu.Lock()
				defer s.ia.mu.Unlock()
				return s.ia.phase == phaseInput
			})
			mu.Lock()
			var got []string
			for _, e := range evs {
				got = append(got, fmt.Sprintf("%s typed=%v resumed=%v %d", e.Kind, e.Typed, e.Resumed, e.Status))
			}
			mu.Unlock()
			want := []string{
				"start typed=false resumed=false 0",
				"end typed=false resumed=false " + strconv.Itoa(stoppedStatus),
				"start typed=true resumed=false 0",
				"end typed=true resumed=false 0",
				"start typed=false resumed=true 0",
				"end typed=false resumed=false 130",
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			if j := s.Jobs(); len(j) != 0 {
				t.Errorf("jobs left: %+v", j)
			}

			// One that ignores ctrl+z keeps the shell.
			s.Type([]byte("sh -c \"trap '' TSTP; exec sleep 30\"\r"))
			// Once sh has set the trap and become sleep: sooner, ctrl+z
			// stops sh.
			waitFor(t, "sleep to run", func() bool {
				if !fg(s) {
					return false
				}
				for _, j := range ttyJobs(strings.TrimPrefix(s.ia.ttyName, "/dev/"), s.PID()) {
					if j.Command == "sleep 30" {
						return true
					}
				}
				return false
			})
			if _, _, err := s.RunLine(context.Background(), "echo no", io.Discard); !errors.Is(err, ErrNotStopped) {
				t.Errorf("RunLine: %v", err)
			}
		})
	}
}
