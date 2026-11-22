//go:build unix

package run_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/run"
)

// lockedBuf is a buffer safe to write from the shell's reader and read
// from the test.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// TestInteractiveShellSession runs a document's top-level shell
// interactively: blocks run on its terminal, ctrl+z stops a command
// and fg brings it back, jobs are logged, and a sub-env stays scripted.
func TestInteractiveShellSession(t *testing.T) {
	for _, name := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			// skip_global_compinit isn't in the base set the top-level
			// shell gets, so the document shares everything.
			src := "---\nmdsh:\n  env:\n    share: [\"...\"]\n---\n" +
				"```sh {#a}\nexport Y=from-a; cd /; echo out; echo err >&2\n```\n\n" +
				"```sh {#b}\nsleep 30\n```\n\n" +
				"```sh {#c}\nfg\n```\n\n" +
				"```sh {#d env=sub}\necho \"$Y\"; echo e2 >&2\n```\n"
			d := parse(t, src)
			dir := t.TempDir()
			home := t.TempDir()
			keys := make(chan []byte, 4)
			var shown, log lockedBuf
			s, err := run.NewSession(d, run.Options{
				Dir:              dir,
				File:             filepath.Join(dir, "d.md"),
				Shell:            path,
				Environ:          []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TERM=xterm", "skip_global_compinit=1"},
				PTY:              true,
				Terminal:         &env.Terminal{Keys: keys, Out: &shown},
				InteractiveShell: true,
				Audit:            audit.New(&log),
				KillAfter:        10 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.Background()

			r, err := s.RunBlock(ctx, 0)
			if err != nil || r.Failed() {
				t.Fatalf("a: %v %+v", err, r)
			}
			if got := string(r.Stdout); got != "out\nerr\n" || len(r.Stderr) != 0 {
				t.Errorf("a: stdout %q stderr %q: want one stream", got, r.Stderr)
			}
			if !s.InteractiveShell() {
				t.Fatal("top-level shell isn't interactive")
			}
			st, ok, err := s.TopState(ctx)
			if err != nil || !ok || st.PWD != "/" {
				t.Fatalf("state: %v %v %+v", err, ok, st)
			}

			// ctrl+z, typed while b runs, stops it.
			go func() {
				time.Sleep(500 * time.Millisecond)
				keys <- []byte{0x1a}
			}()
			r, _, err = s.RunTyped(ctx, 1)
			if err != nil || r.Status != 128+int(syscall.SIGTSTP) {
				t.Fatalf("b: %v status %d err %v", err, r.Status, r.Err)
			}
			// fg brings it back; ctrl+c ends it.
			go func() {
				time.Sleep(500 * time.Millisecond)
				keys <- []byte{0x03}
			}()
			r, _, err = s.RunTyped(ctx, 2)
			if err != nil || r.Status != 130 {
				t.Fatalf("c: %v status %d err %v out %q", err, r.Status, r.Err, r.Stdout)
			}

			// A sub-env is scripted, with the parent's exports.
			r, err = s.RunBlock(ctx, 3)
			if err != nil || r.Failed() {
				t.Fatalf("d: %v %+v", err, r)
			}
			if string(r.Stdout) != "from-a\n" || string(r.Stderr) != "e2\n" {
				t.Errorf("d: stdout %q stderr %q", r.Stdout, r.Stderr)
			}

			s.Close()
			l := log.String()
			if !strings.Contains(l, `"interactive":true`) {
				t.Errorf("env.start not marked interactive:\n%s", l)
			}
			// sh reports no jobs at its prompt, its hook running in a
			// subshell; bash and zsh do, so their jobs are logged.
			if name != "sh" {
				for _, ev := range []string{"job.stopped", "job.ended"} {
					if !strings.Contains(l, `"`+ev+`"`) {
						t.Errorf("no %s in the audit log:\n%s", ev, l)
					}
				}
			}
		})
	}
}

// TestInteractiveSubEnvSession: a sub-env the session is told to run
// interactively starts on a terminal of its own, made with the share of
// its first block, and its blocks run there.
func TestInteractiveSubEnvSession(t *testing.T) {
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	src := "---\nmdsh:\n  env:\n    share: [\"...\"]\n---\n" +
		"```sh {#a}\nexport Y=from-a Z=dropped\n```\n\n" +
		"```sh {#b env=build share=Y}\necho \"y=$Y z=${Z-}\"; echo e2 >&2\n```\n\n" +
		"```sh {#c env=build/test}\ntrue\n```\n"
	d := parse(t, src)
	dir := t.TempDir()
	home := t.TempDir()
	var shown lockedBuf
	s, err := run.NewSession(d, run.Options{
		Dir:              dir,
		File:             filepath.Join(dir, "d.md"),
		Shell:            path,
		Environ:          []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TERM=xterm"},
		PTY:              true,
		Terminal:         &env.Terminal{Keys: make(chan []byte), Out: &shown},
		InteractiveShell: true,
		KillAfter:        10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if got := strings.Join(s.Envs(), ","); got != ",build,build/test" {
		t.Errorf("Envs: %q", got)
	}
	if _, err := s.RunBlock(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if sh, err := s.Shell(ctx, "build"); err != nil || sh != nil {
		t.Fatalf("build before SetInteractive: %v, %v", sh, err)
	}
	if !s.Scripted("build") || !s.SetInteractive("build") {
		t.Fatal("SetInteractive didn't end build's scripted shell")
	}
	sh, err := s.Shell(ctx, "build")
	if err != nil || sh == nil {
		t.Fatalf("build: %v, %v", sh, err)
	}
	top, _ := s.TopShell(ctx)
	if sh == top {
		t.Fatal("build is the top level's shell")
	}
	r, err := s.RunBlock(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(r.Stdout); !strings.Contains(got, "y=from-a z=") || strings.Contains(got, "dropped") || !strings.Contains(got, "e2") {
		t.Errorf("build's block showed %q", got)
	}
	if again, _ := s.Shell(ctx, "build"); again != sh {
		t.Error("the block ran in another shell")
	}
}
