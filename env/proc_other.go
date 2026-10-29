//go:build !unix

package env

import (
	"os"
	"os/exec"
)

func ownGroup(*exec.Cmd) {}

func kill(cmd *exec.Cmd) { cmd.Process.Kill() }

// killTree ends process pid. Here it can't find what pid started.
func killTree(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}
