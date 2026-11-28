//go:build unix

package env

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestInterruptEndsWithRun checks that once Run returns from a stopped
// script, nothing more is sent to the shell, so no interrupt meant for
// that script reaches the next. ps is made slower than an interrupt
// tick, as on a busy machine, which is when a stray one used to get
// out.
func TestInterruptEndsWithRun(t *testing.T) {
	tree, send := processTree, interrupt
	t.Cleanup(func() { processTree, interrupt = tree, send })
	processTree = func() map[int][]int {
		time.Sleep(3 * interruptEvery)
		return tree()
	}
	var returned, late atomic.Bool
	interrupt = func(cmd *exec.Cmd) {
		if returned.Load() {
			late.Store(true)
		}
		send(cmd)
	}
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	sh, err := Start(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	// Past interruptFor, so ps runs on every round.
	for i := range 4 {
		returned.Store(false)
		ctx, cancel := context.WithCancel(context.Background())
		w := &cancelOn{text: "ready", after: 100 * time.Millisecond, cancel: cancel}
		_, _, err := sh.Run(ctx, "echo ready\nsh -c 'trap \"\" INT; sleep 30'\n", w, w)
		returned.Store(true)
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("round %d: Run = %v, want the interrupt", i, err)
		}
		time.Sleep(5 * interruptEvery)
		if late.Load() {
			t.Fatalf("round %d: an interrupt was sent after Run returned", i)
		}
	}
	var out bytes.Buffer
	if _, _, err := sh.Run(context.Background(), "sleep 0.3\necho ok\n", &out, &out); err != nil || out.String() != "ok\n" {
		t.Errorf("the next run = %v, %q; want ok", err, out.String())
	}
}

// TestKillSparesNewCommands checks that killChildren kills only what
// was found before, so has outlived an interrupt: a command the script
// started since is left to the next interrupt, which ends the script,
// where a kill would let it go on past the command.
func TestKillSparesNewCommands(t *testing.T) {
	tree := processTree
	t.Cleanup(func() { processTree = tree })
	start := func() *exec.Cmd {
		cmd := exec.Command("sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
		return cmd
	}
	old, started := start(), start()
	// The shell, as a pid nothing has, with the two sleeps as children.
	const shell = -2
	processTree = func() map[int][]int {
		return map[int][]int{shell: {old.Process.Pid, started.Process.Pid}}
	}
	found := killChildren(shell, nil, map[int]bool{old.Process.Pid: true})
	if !found[old.Process.Pid] || !found[started.Process.Pid] || len(found) != 2 {
		t.Errorf("found %v, want both sleeps", found)
	}
	if err := old.Wait(); err == nil {
		t.Error("the command found before wasn't killed")
	}
	if err := started.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("the command started since was killed: %v", err)
	}
}
