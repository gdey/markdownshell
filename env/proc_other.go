//go:build !unix

package env

import "os/exec"

func ownGroup(*exec.Cmd) {}

func kill(cmd *exec.Cmd) { cmd.Process.Kill() }
