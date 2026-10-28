//go:build unix

package env

import (
	"os/exec"
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
