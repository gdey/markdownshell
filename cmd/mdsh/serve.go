package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
)

// serveFlag is --serve, with an optional address: --serve alone listens
// on serve.DefaultAddr, --serve=host:port where it says, and
// --serve=stdio serves one client on stdin and stdout. It is a bool-style
// flag, so the address needs the =.
type serveFlag struct {
	on   bool
	addr string
}

func (f *serveFlag) String() string   { return f.addr }
func (f *serveFlag) IsBoolFlag() bool { return true }

func (f *serveFlag) Set(v string) error {
	switch v {
	case "true":
		f.on = true
	case "false":
		f.on, f.addr = false, ""
	default:
		f.on, f.addr = true, v
	}
	return nil
}

// serveCmd serves the notebook to a browser until interrupted: with file
// open, run with args, or with the start screen listing the markdown
// files under the current folder, or under file if it is a folder.
func serveCmd(addr, file string, args []string, source string, stderr io.Writer) error {
	if file == "-" {
		return errors.New("--serve needs a file or folder, not stdin")
	}
	path, err := settings.DefaultPath()
	if err != nil {
		return err
	}
	store, err := settings.Open(path)
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	opts := serve.Options{Addr: addr, Settings: store, Cwd: cwd, File: file, Args: args, Source: source, Log: stderr}
	if file != "" {
		if fi, err := os.Stat(file); err == nil && fi.IsDir() {
			if len(args) > 0 {
				return errors.New("arguments after a folder are only for a file")
			}
			dir, err := filepath.Abs(file)
			if err != nil {
				return err
			}
			opts.Dir, opts.File = dir, ""
		}
	}
	stdio := addr == serve.Stdio
	if stdio {
		opts.Addr = ""
	}
	s, err := serve.New(opts)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if stdio {
		return s.ServeStdio(ctx, os.Stdin, os.Stdout)
	}
	return s.ListenAndServe(ctx)
}
