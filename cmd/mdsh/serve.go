//go:build serve

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/trust"
	"github.com/gdey/markdownshell/ui"
)

// serveBuilt is whether this mdsh has --serve. It is built in only with
// -tags serve: anyone who can reach the server, with its token, can run
// commands as the user, so a plain build leaves it out.
const serveBuilt = true

// addServeFlag adds --serve to fs.
func addServeFlag(fs *flag.FlagSet) *serveFlag {
	f := &serveFlag{}
	fs.Var(f, "serve", "serve the notebook to a web browser, on `host:port` (default "+serve.DefaultAddr+"), or with =stdio to the program that started mdsh, as JSON lines on stdin and stdout; anyone who can reach it with its token can run commands as you")
	fs.StringVar(&f.tls.Cert, "serve-cert", "", "with --serve, the PEM certificate `file` to serve HTTPS with; without it, an address other machines can reach gets a self-signed one")
	fs.StringVar(&f.tls.Key, "serve-key", "", "with --serve-cert, its PEM private key `file`")
	fs.StringVar(&f.tls.ClientCA, "serve-client-ca", "", "with --serve, require a client certificate signed by a CA in this PEM `file` (mutual TLS)")
	return f
}

// serveFlag is --serve, with an optional address: --serve alone listens
// on serve.DefaultAddr, --serve=host:port where it says, and
// --serve=stdio serves one client on stdin and stdout. It is a bool-style
// flag, so the address needs the =.
type serveFlag struct {
	on   bool
	addr string
	// tls is --serve-cert, --serve-key and --serve-client-ca.
	tls serve.TLS
}

// stdio is whether it is --serve=stdio.
func (f *serveFlag) stdio() bool { return f.addr == serve.Stdio }

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
func serveCmd(f *serveFlag, file string, args []string, source string, stderr io.Writer) error {
	addr, tlsOpts := f.addr, f.tls
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
	bi, _ := debug.ReadBuildInfo()
	// A self-signed certificate is kept beside the settings, so a browser
	// told to trust it once keeps trusting it.
	tlsOpts.CertDir = filepath.Dir(path)
	opts := serve.Options{Addr: addr, Settings: store, Cwd: cwd, File: file, Args: args, Source: source, Log: stderr, Build: ui.BuildInfo(version, bi), TLS: tlsOpts, Trust: trust.OpenDefault()}
	// The Neovim pane's language server is mdsh lsp.
	if exe, err := os.Executable(); err == nil {
		opts.LSP = func(string) []string { return []string{exe, "lsp"} }
	}
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
	// What the notebook logs goes to the Console pane, as well as here.
	slog.SetDefault(slog.New(ui.NewConsoleHandler(slog.NewTextHandler(stderr, nil), s.Console())))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if stdio {
		return s.ServeStdio(ctx, os.Stdin, os.Stdout)
	}
	return s.ListenAndServe(ctx)
}
