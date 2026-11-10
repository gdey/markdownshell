//go:build !linux && !darwin

package main

import "os"

// noEcho does nothing here: see echo_unix.go.
func noEcho(*os.File) (restore func()) { return func() {} }
