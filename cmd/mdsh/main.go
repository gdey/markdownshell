// Command mdsh runs the code blocks in a markdown file.
//
//	mdsh [run] [flags] file.md [args...]
//
// With no subcommand, mdsh runs the file, so a file starting with
// #!/usr/bin/env mdsh is executable.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func main() {
	os.Exit(mdsh(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// version is set by release builds with -ldflags "-X main.version=...".
var version = ""

// versionString returns the release version, or the module version for a
// go install build, or "dev".
func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// errFailed means a block failed; its output already says why.
var errFailed = errors.New("a block failed")

func mdsh(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var err error
	switch {
	case len(args) > 0 && args[0] == "completion":
		err = completionCmd(args[1:], stdout)
	case len(args) > 0 && args[0] == "__blocks":
		err = blocksCmd(args[1:], stdout)
	case len(args) > 0 && args[0] == "run":
		err = runCmd(args[1:], stdin, stdout, stderr)
	default:
		err = runCmd(args, stdin, stdout, stderr)
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errFailed):
		return 1
	case errors.Is(err, flag.ErrHelp):
		return 0
	default:
		fmt.Fprintln(stderr, "mdsh:", err)
		return 2
	}
}

func runCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mdsh run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: mdsh [run] [flags] file.md [args...]")
		fmt.Fprintln(stderr, "       mdsh completion bash|zsh")
		fmt.Fprintln(stderr, "       use - as the file to read markdown from stdin")
		fs.PrintDefaults()
	}
	hideCode := fs.Bool("hide-code", false, "print prose and output, not runnable code blocks")
	hideOutput := fs.Bool("hide-output", false, "print prose and code, not output (blocks still run unless --hide-code is also set)")
	write := fs.Bool("w", false, "write output back into the file instead of printing")
	outFile := fs.String("o", "", "like -w, but write to this file and leave the input alone")
	only := fs.String("only", "", "run only this block, plus earlier blocks in its env chain")
	stop := fs.Bool("stop-on-error", false, "stop at the first failing block")
	yes := fs.Bool("yes", false, "run blocks marked confirm without asking")
	color := fs.String("color", "auto", "style printed markdown: auto, always or never")
	noColor := fs.Bool("no-color", false, "same as --color=never")
	interactive := fs.Bool("i", false, "interactive mode (not implemented yet)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, "mdsh", versionString())
		return nil
	}
	if *noColor {
		*color = "never"
	}
	switch *color {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("--color must be auto, always or never, got %q", *color)
	}
	if *interactive {
		return errors.New("interactive mode is not implemented yet")
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("no file given")
	}
	file, blockArgs := fs.Arg(0), fs.Args()[1:]
	target := file
	if *outFile != "" {
		*write, target = true, *outFile
	}
	if target == "-" && *write {
		return errors.New("-w needs a file, not stdin; use -o to name one")
	}

	var src []byte
	var err error
	if file == "-" {
		src, err = io.ReadAll(stdin)
		file = ""
	} else {
		src, err = os.ReadFile(file)
	}
	if err != nil {
		return err
	}
	d, err := doc.Parse(src)
	if err != nil {
		return fmt.Errorf("%s: %w", displayName(file), err)
	}

	styled := *color == "always" || (*color == "auto" && isTerminal(stdout) && os.Getenv("NO_COLOR") == "")

	// Both hidden: print the prose only, without running anything.
	if *hideCode && *hideOutput && !*write {
		results := run.Includes(d, run.Options{File: file})
		if err := print(stdout, render.Prose(d, results, os.Environ()), styled); err != nil {
			return err
		}
		for _, r := range results {
			if r.Failed() {
				return errFailed
			}
		}
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := run.Options{
		File:        file,
		Args:        blockArgs,
		StopOnError: *stop,
		Only:        *only,
	}
	if file != "" {
		if abs, err := filepath.Abs(file); err == nil {
			opts.File = abs
		}
	}
	if *write {
		// Image links are written relative to the output file.
		if abs, err := filepath.Abs(target); err == nil {
			opts.LinkDir = filepath.Dir(abs)
		}
		opts.OnEvent = func(e run.Event) {
			if e.Kind == run.Done {
				status := "ok"
				if e.Result.Failed() {
					status = "failed"
				}
				name := e.Block.ID
				if e.Block.IsInclude() {
					name = "include " + e.Block.Attrs["file"]
				}
				fmt.Fprintf(stderr, "%s: %s\n", name, status)
			}
		}
	}

	if *yes {
		opts.Confirm = func(*doc.Block) bool { return true }
	} else if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		opts.Confirm = func(b *doc.Block) bool { return confirm(tty, tty, b) }
	}

	results, runErr := run.Run(ctx, d, opts)
	if results == nil {
		return runErr
	}
	for _, r := range results {
		if r.Declined && runErr == nil {
			fmt.Fprintf(stderr, "mdsh: stopped before block %s\n", r.Block.ID)
		}
	}
	if errors.Is(runErr, run.ErrNoConfirm) {
		runErr = fmt.Errorf("%w; no terminal to ask on (use --yes to run it)", runErr)
	}

	if *write {
		out := render.Markdown(d, results, render.Options{WriteBack: true, Env: os.Environ()})
		mode := os.FileMode(0o644)
		if info, err := os.Stat(file); err == nil {
			mode = info.Mode().Perm()
		}
		if err := writeFile(target, out, mode); err != nil {
			return err
		}
	} else {
		out := render.Markdown(d, results, render.Options{HideCode: *hideCode, HideOutput: *hideOutput, Env: os.Environ()})
		if err := print(stdout, out, styled); err != nil {
			return err
		}
	}
	if runErr != nil {
		return runErr
	}
	for _, r := range results {
		if r.Failed() {
			return errFailed
		}
	}
	return nil
}

// confirm shows block b and asks whether to run it. Anything but run
// stops, so pressing enter is the safe choice.
func confirm(in io.Reader, out io.Writer, b *doc.Block) bool {
	q := b.Attrs["confirm"]
	if q == "" {
		q = "Run this block?"
	}
	fmt.Fprintf(out, "\n--- block %s ---\n%s---\n", b.ID, b.Body)
	r := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "%s [r]un / [s]top: ", q)
		line, err := r.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "r", "run", "y", "yes":
			return true
		case "s", "stop", "n", "no", "":
			return false
		}
		if err != nil {
			return false
		}
	}
}

func print(w io.Writer, md []byte, styled bool) error {
	if styled {
		r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(termWidth(w)))
		if err == nil {
			if out, err := r.RenderBytes(md); err == nil {
				md = out
			}
		}
	}
	_, err := w.Write(md)
	return err
}

// writeFile atomically replaces path's contents. An existing file keeps
// its mode; a new one gets mode.
func writeFile(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mdsh-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func termWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			return width
		}
	}
	return 80
}

func displayName(file string) string {
	if file == "" {
		return "stdin"
	}
	return file
}
