//go:build !darwin && !linux

package main

// newCLIHost is for a platform whose build carries no mdsh.
func newCLIHost() *cliHost {
	return &cliHost{pathDirs: envPathDirs()}
}
