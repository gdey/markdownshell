package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
)

// stdioFlag is how mdsh-gui is asked to run without a window, serving
// the notebook on stdin and stdout as mdsh --serve=stdio does, for a
// program that starts it, such as an editor plugin.
const stdioFlag = "--serve=" + serve.Stdio

// serveStdio serves the notebook on stdin and stdout until stdin ends:
// with file open, run with args, or the start screen listing the markdown
// files under folder.
func serveStdio(args []string) error {
	path, err := settings.DefaultPath()
	if err != nil {
		return err
	}
	store, err := settings.Open(path)
	if err != nil {
		return err
	}
	dir, err := argDir(args)
	if err != nil {
		return err
	}
	if dir != "" {
		args = nil
	}
	cwd, _ := os.Getwd()
	opts := serve.Options{Settings: store, Cwd: cwd, Dir: dir, Log: os.Stderr}
	if len(args) > 0 {
		opts.File, opts.Args = args[0], args[1:]
	}
	s, err := serve.New(opts)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return s.ServeStdio(ctx, os.Stdin, os.Stdout)
}
