//go:build unix

package env

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ownGroup starts cmd in a process group of its own, so kill reaches
// whatever a block started too.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// kill ends cmd and its process group, so a child such as a sleep
// doesn't run on after its block was stopped.
func kill(cmd *exec.Cmd) {
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Process.Kill()
}

// killTree ends process pid and everything it started. A background
// job shares its shell's process group, so it is found by walking the
// process tree that ps lists.
func killTree(pid int) {
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
	// Kill from the top down, so the job's parts can't run on after it
	// or start more.
	for todo := []int{pid}; len(todo) > 0; {
		p := todo[0]
		todo = append(todo[1:], children[p]...)
		syscall.Kill(p, syscall.SIGKILL)
	}
}
