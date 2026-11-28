//go:build !unix

package env

import (
	"os"
	"os/exec"
)

func ownGroup(*exec.Cmd) {}

// unignoreInterrupt has nothing to do here, without SIGINT.
func unignoreInterrupt() {}

func kill(cmd *exec.Cmd) { cmd.Process.Kill() }

// interrupt can't reach a script's commands here without ending the
// shell, so it does nothing; Kill ends it.
func interrupt(*exec.Cmd) {}

// killChildren can't find what pid started here.
func killChildren(int, map[int]bool, map[int]bool) map[int]bool { return nil }

// killTree ends process pid. Here it can't find what pid started.
func killTree(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}

// psRows can't list processes here.
func psRows() map[int]psRow { return nil }

// watchStops does nothing here, where processes aren't stopped.
func watchStops(int, <-chan struct{}) {}
