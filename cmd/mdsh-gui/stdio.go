package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
)

// serveStdio serves the notebook on stdin and stdout until stdin ends, as
// mdsh --serve=stdio does, for a program that starts mdsh-gui, such as an
// editor plugin: with the file named open, run with its arguments, or the
// start screen listing the markdown files under the folder named.
func serveStdio(o options) error {
	args := o.args
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
	opts := serve.Options{Settings: store, Cwd: cwd, Dir: dir, Source: o.source, Log: os.Stderr}
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
