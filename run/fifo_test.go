//go:build unix

package run_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// These tests hand data between blocks through named pipes, with the
// examples in examples/fifo and a few smaller cases. A FIFO that is
// never closed blocks forever, so each run has a deadline, and a test
// fails rather than hangs if run.Run doesn't return.

// runFIFO runs src in a temp dir and fails if the run takes longer than
// limit, timeouts and the shells' cleanup included.
func runFIFO(t *testing.T, src string, limit time.Duration) map[string]*run.Result {
	t.Helper()
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	opts := run.Options{Dir: t.TempDir(), Environ: []string{"PATH=" + os.Getenv("PATH")}}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	type ran struct {
		results []*run.Result
		err     error
	}
	done := make(chan ran, 1)
	go func() {
		results, err := run.Run(ctx, d, opts)
		done <- ran{results, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return byID(r.results)
	case <-time.After(limit + 5*time.Second):
		t.Fatalf("run still going after %s", limit+5*time.Second)
		return nil
	}
}

func fifoExample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "examples", "fifo", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantStdout(t *testing.T, r map[string]*run.Result, id, want string) {
	t.Helper()
	if got := string(r[id].Stdout); got != want {
		t.Errorf("%s stdout = %q, want %q", id, got, want)
	}
}

// A background reader in one block gets what a later block writes, and
// a block after that can wait for it.
func TestFIFOHandoff(t *testing.T) {
	r := runFIFO(t, fifoExample(t, "handoff.md"), 10*time.Second)
	wantStdout(t, r, "collect", "reader exited with 0\nhello through the fifo\n")
}

// A descriptor opened with exec stays open across blocks, so the reader
// sees end of file only when a later block closes it.
func TestFIFOHeldOpen(t *testing.T) {
	r := runFIFO(t, fifoExample(t, "held-open.md"), 10*time.Second)
	wantStdout(t, r, "still-open", "reader still running\n")
	wantStdout(t, r, "close", "reader exited with 0\nfirst\nsecond\n")
}

// When the writer never closes, waiting for the reader blocks until the
// block's timeout, which kills the shell and stops the run.
func TestFIFONeverClosed(t *testing.T) {
	start := time.Now()
	r := runFIFO(t, fifoExample(t, "never-closed.md"), 15*time.Second)
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("run took %s; the block's timeout is 2s", d)
	}
	wantStdout(t, r, "writer", "writer done\n")
	w := r["wait"]
	if !w.Exited || w.Err == nil || !strings.Contains(w.Err.Error(), "timed out") {
		t.Errorf("wait: exited %v, err %v; want a timeout that ended the shell", w.Exited, w.Err)
	}
	if r["after"].Ran {
		t.Error("the block after the timeout ran")
	}
}

// A reader in the foreground with no writer blocks on open, and only
// the timeout ends it.
func TestFIFOForegroundReader(t *testing.T) {
	src := "```sh {#mk}\nmkfifo pipe\n```\n" +
		"```sh {#read timeout=1s}\ncat pipe\n```\n" +
		"```sh {#after}\necho after\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	if !r["read"].Exited || r["read"].Err == nil {
		t.Errorf("read: exited %v, err %v; want a timeout", r["read"].Exited, r["read"].Err)
	}
	if r["after"].Ran {
		t.Error("the block after the timeout ran")
	}
}

// A reader and writer still holding a FIFO when the document ends
// don't keep the run from finishing: closing the shell kills its jobs.
func TestFIFOLeftOpenAtEnd(t *testing.T) {
	src := "```sh {#mk}\nmkfifo pipe\ncat pipe > got.txt &\n```\n" +
		"```sh {#open}\nexec 3>pipe\necho one >&3\n```\n" +
		"```sh {#end}\necho end\n```\n"
	start := time.Now()
	r := runFIFO(t, src, 10*time.Second)
	wantStdout(t, r, "end", "end\n")
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("run took %s to finish", d)
	}
}

// A reader in another env's shell gets what the main shell writes.
func TestFIFOAcrossEnvs(t *testing.T) {
	src := "```sh {#mk}\nmkfifo pipe\n```\n" +
		"```sh {#reader env=r}\ncat pipe > got.txt &\nREADER=$!\n```\n" +
		"```sh {#writer}\necho one > pipe\n```\n" +
		"```sh {#collect env=r}\nwait $READER\ncat got.txt\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	wantStdout(t, r, "collect", "one\n")
}

// A background job writes to its shell's stdout, which mdsh reads for
// whichever block is running at the time. So what the job prints after
// its own block ended shows up in a later block's output.
func TestBackgroundOutputLandsInRunningBlock(t *testing.T) {
	src := "```sh {#bg}\n(sleep 0.3; echo late) &\necho bg\n```\n" +
		"```sh {#next}\nsleep 1; echo next\n```\n"
	r := runFIFO(t, src, 10*time.Second)
	wantStdout(t, r, "bg", "bg\n")
	wantStdout(t, r, "next", "late\nnext\n")
}
