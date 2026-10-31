package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/ui"
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
	opts := serve.Options{Settings: store, Cwd: cwd, Dir: dir, Source: o.source, Log: os.Stderr, Build: thisBuild()}
	if len(args) > 0 {
		opts.File, opts.Args = args[0], args[1:]
	}
	// The Neovim pane's language server is this program, run with --lsp.
	if exe, err := os.Executable(); err == nil {
		opts.LSP = func(file string) []string { return []string{exe, "--lsp", file} }
	}
	s, err := serve.New(opts)
	if err != nil {
		return err
	}
	// What the notebook logs goes to the Console pane, as well as to
	// stderr.
	slog.SetDefault(slog.New(ui.NewConsoleHandler(slog.NewTextHandler(os.Stderr, nil), s.Console())))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return s.ServeStdio(ctx, os.Stdin, os.Stdout)
}
