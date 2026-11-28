//go:build unix

package env

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// unignoreInterrupt makes SIGINT reach the shells mdsh starts, and what
// they run, when mdsh itself was started with it ignored, as a command
// run with & from a script is, or under nohup. An ignored signal stays
// ignored across exec, and a shell can't trap or reset one it started
// with ignored, so stopping a block, which sends SIGINT, did nothing:
// the killed command let the script go on. A signal mdsh handles is
// reset to the default for what it starts, so mdsh handles SIGINT, and
// drops it, which leaves mdsh ignoring it as before.
var unignoreInterrupt = sync.OnceFunc(func() {
	if signal.Ignored(os.Interrupt) {
		signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	}
})

// ownGroup starts cmd in a process group of its own, so kill reaches
// whatever a block started too.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// kill ends cmd and its process group, so a child such as a sleep
// doesn't run on after its block was stopped.
func kill(cmd *exec.Cmd) {
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Process.Kill()
}

// interrupt sends SIGINT to cmd's process group, as ctrl+c would.
func interrupt(cmd *exec.Cmd) { syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) }

// killChildren ends every process pid started, and what they started,
// that is in old, but not pid itself, nor the processes in spare and
// what they started. It returns the processes it found, killed or not,
// for the next call's old: passing what one call found before an
// interrupt to the next after it ends only what outlived that
// interrupt, never a command the script started since, which the
// interrupt that follows ends as it should.
func killChildren(pid int, spare, old map[int]bool) map[int]bool {
	children := processTree()
	found := map[int]bool{}
	for todo := children[pid]; len(todo) > 0; {
		p := todo[0]
		todo = todo[1:]
		if spare[p] {
			continue
		}
		todo = append(todo, children[p]...)
		found[p] = true
		if old[p] {
			syscall.Kill(p, syscall.SIGKILL)
		}
	}
	return found
}

// processTree maps each process ID to the processes it started, as ps
// lists them.
func processTree() map[int][]int {
	out, _ := exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=").Output()
	children := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		p, err1 := strconv.Atoi(f[0])
		pp, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			children[pp] = append(children[pp], p)
		}
	}
	return children
}

// psRows lists every process, as ps does.
func psRows() map[int]psRow {
	out, _ := exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=", "-o", "stat=", "-o", "etime=", "-o", "args=").Output()
	return parsePS(string(out))
}

// killTree ends process pid and everything it started. A background
// job shares its shell's process group, so it is found by walking the
// process tree that ps lists.
func killTree(pid int) {
	children := processTree()
	// Kill from the top down, so the job's parts can't run on after it
	// or start more.
	for todo := []int{pid}; len(todo) > 0; {
		p := todo[0]
		todo = append(todo[1:], children[p]...)
		syscall.Kill(p, syscall.SIGKILL)
	}
}

// stopCheck is how often watchStops looks, besides when a child
// changes state.
const stopCheck = 500 * time.Millisecond

// watchStops, until stop closes, continues the shell, process pid, and
// its process group whenever the shell is stopped. That happens when a
// command run without job control suspends itself, as vim and nvim do
// on ctrl+z with kill(0, SIGTSTP): the signal reaches the whole group,
// the shell too, ignoring it or not on some systems, such as macOS,
// and nothing could ever continue them, so the block would hang. With
// job control the shell's commands run in groups of their own, so a
// stopped job isn't touched.
func watchStops(pid int, stop <-chan struct{}) {
	chld := make(chan os.Signal, 1)
	signal.Notify(chld, syscall.SIGCHLD)
	go func() {
		defer signal.Stop(chld)
		t := time.NewTicker(stopCheck)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-chld:
			case <-t.C:
			}
			if isStopped(pid) {
				syscall.Kill(-pid, syscall.SIGCONT)
			}
		}
	}()
}
