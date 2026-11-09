package env

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestKeepInterrupts checks that a script whose
// context ends is interrupted, whatever it is waiting on, and the shell
// lives on with what the script set before.
func TestKeepInterrupts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGINT")
	}
	cases := map[string]string{
		"command": "sleep 30\necho after",
		// The trap returns from f, then the next interrupt reaches the
		// sleep after it.
		"function": "f() { sleep 30; echo after; }\nf\nsleep 30\necho after",
		"loop":     "while :; do :; done\necho after",
		// Killed after interruptFor, the child didn't die of SIGINT, so
		// bash (as macOS's sh) lets the script go on, until the next
		// interrupt reaches the sleep after it.
		"ignores": "sh -c 'trap \"\" INT; sleep 30'\nsleep 30\necho after",
	}
	for _, path := range shells(t) {
		for name, body := range cases {
			t.Run(filepath.Base(path)+"/"+name, func(t *testing.T) {
				sh, err := Start(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}})
				if err != nil {
					t.Fatal(err)
				}
				defer sh.Close()
				// The interrupt comes 300ms after the script starts, not
				// after Run is called: see cancelOn.
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var out, errb bytes.Buffer
				w := &cancelOn{text: "ready", after: 300 * time.Millisecond, cancel: cancel, w: &out}
				start := time.Now()
				rc, exited, err := sh.Run(ctx, "X=kept\necho ready\n"+body+"\n", w, &errb)
				if !errors.Is(err, context.Canceled) || exited || rc != 130 {
					t.Fatalf("Run = %d, %v, %v; want 130, not exited, the interrupt", rc, exited, err)
				}
				if d := time.Since(start); d > 5*time.Second {
					t.Errorf("stopping took %v", d)
				}
				if strings.Contains(out.String(), "after") {
					t.Errorf("the script went on after the interrupt: %q", out.String())
				}
				out.Reset()
				if _, exited, err := sh.Run(context.Background(), "echo \"$X\"\nsleep 0.6\necho ok\n", &out, &errb); err != nil || exited {
					t.Fatalf("the next run: exited %v, %v", exited, err)
				}
				if got := out.String(); got != "kept\nok\n" {
					t.Errorf("the next run printed %q, want what the first set, and no stray interrupt", got)
				}
				if sh.Exited() {
					t.Error("the shell ended")
				}
			})
		}
	}
}

// TestKeepStatusAndKill checks the status a script ends with is kept, and
// that Kill ends a script that ignores the interrupt.
func TestKeepStatusAndKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGINT")
	}
	for _, path := range shells(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			sh, err := Start(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}})
			if err != nil {
				t.Fatal(err)
			}
			defer sh.Close()
			var out bytes.Buffer
			if rc, _, err := sh.Run(context.Background(), "false", &out, &out); err != nil || rc != 1 {
				t.Errorf("false = %d, %v; want 1", rc, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := &cancelOn{text: "ready", cancel: cancel}
			go func() {
				time.Sleep(interruptFor + time.Second)
				if !sh.Busy() {
					t.Error("not busy while the loop runs")
				}
				sh.Kill()
			}()
			_, exited, err := sh.Run(ctx, "trap '' INT\necho ready\nwhile :; do :; done\n", ready, ready)
			if !exited || !errors.Is(err, context.Canceled) {
				t.Errorf("Run after Kill = exited %v, %v; want exited and the interrupt", exited, err)
			}
		})
	}
}

// TestSetArgs checks new positional parameters reach the shells as they
// are, keeping what scripts set, and shells started later.
func TestSetArgs(t *testing.T) {
	for _, path := range shells(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			s := NewStack(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}, Args: []string{"a"}})
			defer s.Close()
			ctx := context.Background()
			top, err := s.Get(ctx, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if _, _, err := top.Shell.Run(ctx, "X=kept\nexport X", &out, &out); err != nil {
				t.Fatal(err)
			}
			if err := s.SetArgs(ctx, []string{"b c", "d"}); err != nil {
				t.Fatal(err)
			}
			child, err := s.Get(ctx, "child", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range []*Env{top, child} {
				out.Reset()
				if _, _, err := e.Shell.Run(ctx, `echo "$X $# $1"`, &out, &out); err != nil {
					t.Fatal(err)
				}
				if got := out.String(); got != "kept 2 b c\n" {
					t.Errorf("env %q printed %q, want kept 2 b c", e.Name, got)
				}
			}
		})
	}
}

// TestKillAfter checks that Config.KillAfter ends the shell of a script
// that won't stop, and only that one.
func TestKillAfter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGINT")
	}
	for _, path := range shells(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			sh, err := Start(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}, KillAfter: 500 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			defer sh.Close()
			// run interrupts script once it prints "ready"; see cancelOn.
			run := func(script string) (bool, error) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				w := &cancelOn{text: "ready", cancel: cancel}
				_, exited, err := sh.Run(ctx, script, w, w)
				return exited, err
			}
			if exited, err := run("X=kept\necho ready\nsleep 30"); exited || !errors.Is(err, context.Canceled) {
				t.Fatalf("a script that stops = exited %v, %v; want the shell kept", exited, err)
			}
			start := time.Now()
			if exited, _ := run("trap '' INT\necho ready\nwhile :; do :; done"); !exited {
				t.Fatal("a script that won't stop left its shell running")
			}
			if d := time.Since(start); d > 5*time.Second {
				t.Errorf("killing took %s, want about KillAfter", d)
			}
		})
	}
}

// cancelOn is an io.Writer that calls cancel, after a while if after is
// set, once what is written to it holds text, and passes it on to w if
// set. The tests interrupt a script this way, once it says it is
// running, rather than with a deadline from when Run was called: Run
// checks the script's syntax in another shell first, under the same
// ctx, and on a busy machine starting that shell can outlast a short
// deadline, so the script never runs.
type cancelOn struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	text   string
	after  time.Duration
	cancel func()
	w      io.Writer
	once   sync.Once
}

func (c *cancelOn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Write(p)
	if bytes.Contains(c.buf.Bytes(), []byte(c.text)) {
		c.once.Do(func() { time.AfterFunc(c.after, c.cancel) })
	}
	if c.w != nil {
		return c.w.Write(p)
	}
	return len(p), nil
}
