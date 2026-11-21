//go:build !serve

package main

import (
	"errors"
	"flag"
	"io"
)

// serveBuilt is false: this mdsh was built without -tags serve, so it
// has no --serve.
const serveBuilt = false

// serveFlag is never set in an mdsh without --serve.
type serveFlag struct {
	on   bool
	addr string
}

func (*serveFlag) stdio() bool { return false }

// addServeFlag adds nothing: this mdsh has no --serve.
func addServeFlag(*flag.FlagSet) *serveFlag { return &serveFlag{} }

func serveCmd(_ *serveFlag, file string, args []string, source string, stderr io.Writer) error {
	return errors.New("this mdsh was built without --serve; build it with -tags serve")
}
