// Command mdsh runs the code blocks in a markdown file.
//
//	mdsh [run] [flags] file.md [args...]
//
// With no subcommand, mdsh runs the file, so a file starting with
// #!/usr/bin/env mdsh is executable.
package main

import (
	"bufio"
	"bytes"
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
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/report"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termimg"
	"github.com/gdey/markdownshell/tui"
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
	var noRun bool
	fs.BoolVar(&noRun, "no-run", false, "print the document with the output saved in it, without running any block (-o saves the printout)")
	fs.BoolVar(&noRun, "n", false, "short for --no-run")
	write := fs.Bool("w", false, "write output back into the file instead of printing")
	outFile := fs.String("o", "", "like -w, but write to this file and leave the input alone")
	pdfFile := fs.String("pdf", "", "write what would be printed to this PDF file instead (- for stdout)")
	htmlFile := fs.String("html", "", "write what would be printed to this HTML file instead (- for stdout)")
	only := fs.String("only", "", "run only this block, plus earlier blocks in its env chain")
	stop := fs.Bool("stop-on-error", false, "stop at the first failing block")
	yes := fs.Bool("yes", false, "run blocks marked confirm without asking")
	step := fs.Bool("step", false, "ask before running each block, as if every block were marked confirm")
	color := fs.String("color", "auto", "style printed markdown: auto, always or never")
	noColor := fs.Bool("no-color", false, "same as --color=never")
	interactive := fs.Bool("i", false, "open the file in a terminal UI that runs blocks as you pick them; with no file, or a folder, pick one first")
	autosave := fs.Bool("autosave", false, "with -i, save output after every block that runs")
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
	if fs.NArg() < 1 && !*interactive {
		fs.Usage()
		return errors.New("no file given")
	}
	if *interactive {
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		for _, name := range []string{"w", "o", "pdf", "html", "no-run", "n", "only", "hide-code", "hide-output"} {
			if set[name] {
				return fmt.Errorf("-%s can't be used with -i", name)
			}
		}
		var args []string
		if fs.NArg() > 1 {
			args = fs.Args()[1:]
		}
		return interactiveCmd(fs.Arg(0), args, *stop, *step, *yes, *autosave, stdin, stdout)
	}
	file, blockArgs := fs.Arg(0), fs.Args()[1:]
	target := file
	if *outFile != "" {
		*write, target = true, *outFile
	}
	if noRun && *outFile == "" && *write {
		return errors.New("--no-run can't be used with -w; use -o to save what it prints")
	}
	if *pdfFile != "" && *htmlFile != "" {
		return errors.New("--pdf and --html can't be used together")
	}
	// reportFile is the file --pdf or --html writes, in its format.
	reportFile, format := *pdfFile, report.PDF
	if *htmlFile != "" {
		reportFile, format = *htmlFile, report.HTML
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
	for _, w := range d.Warnings {
		fmt.Fprintf(stderr, "mdsh: %s: warning: %s\n", displayName(file), w)
	}

	styled := *color == "always" || (*color == "auto" && isTerminal(stdout) && os.Getenv("NO_COLOR") == "")
	// show prints the document with results, or writes it as a report
	// with image links read from linkDir.
	show := func(results []*run.Result, opts report.Options, linkDir string) error {
		if reportFile == "" {
			return print(stdout, render.Markdown(d, results, render.Options{HideCode: opts.HideCode, HideOutput: opts.HideOutput, Env: opts.Env}), styled)
		}
		if linkDir == "" && file != "" {
			if abs, err := filepath.Abs(file); err == nil {
				linkDir = filepath.Dir(abs)
			}
		}
		opts.Format, opts.Dir, opts.Title = format, linkDir, report.Title(file)
		out, err := report.Render(d, results, opts)
		if err != nil {
			return err
		}
		if reportFile == "-" {
			_, err = stdout.Write(out)
			return err
		}
		return writeFile(reportFile, out, 0o644)
	}
	reportOpts := report.Options{HideCode: *hideCode, HideOutput: *hideOutput, Env: os.Environ()}

	// Nothing to run, or nowhere to show what it printed: print the
	// document with the output already saved in it. Include blocks are
	// still filled in, since reading a file runs nothing. -o saves what
	// would be printed, unstyled.
	if noRun || (*hideCode && *hideOutput && !*write) {
		results := run.Includes(d, run.Options{File: file})
		if *outFile != "" {
			out, _ := report.Render(d, results, reportOpts)
			err = writeFile(*outFile, out, 0o644)
		}
		if err == nil && (*outFile == "" || reportFile != "") {
			err = show(results, reportOpts, "")
		}
		if err != nil {
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
		Step:        *step,
	}
	if file != "" {
		if abs, err := filepath.Abs(file); err == nil {
			opts.File = abs
		}
	}
	renderOpts := render.Options{HideCode: *hideCode, HideOutput: *hideOutput, Env: reportOpts.Env}
	// When the run stops to ask, print the document as it goes, so each
	// question comes after the prose and code it is about, and each
	// block's output shows before the next question. With -w this is
	// printed as well as written, in place of the status lines.
	var live *streamer
	if reportFile == "" && asks(d, opts) {
		live = newStreamer(d, stdout, styled, renderOpts)
		opts.OnEvent = live.event
	}
	if *write {
		// Image links are written relative to the output file.
		if abs, err := filepath.Abs(target); err == nil {
			opts.LinkDir = filepath.Dir(abs)
		}
	}
	if *write && live == nil {
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
				// The output goes to the file, so show why it failed
				// here too.
				if e.Result.Failed() {
					io.WriteString(stderr, render.Output(e.Result, true))
				}
			}
		}
	}

	// The terminal, if there is one, asks the questions and runs blocks
	// marked tty.
	tty, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if ttyErr == nil {
		defer tty.Close()
		opts.TTY = tty
	}
	if *yes {
		opts.Confirm = func(*doc.Block) run.Answer { return run.Go }
	} else if tty != nil {
		p := newPrompter(tty, tty)
		// The streamed document already shows the code, unless it is
		// hidden.
		p.quiet = live != nil && !*hideCode
		opts.Confirm = p.confirm
	}
	if live != nil && opts.Confirm != nil {
		confirm := opts.Confirm
		opts.Confirm = func(b *doc.Block) run.Answer {
			live.upTo(b)
			return confirm(b)
		}
	}

	results, runErr := run.Run(ctx, d, opts)
	if results == nil {
		return runErr
	}
	if errors.Is(runErr, run.ErrNoConfirm) {
		runErr = fmt.Errorf("%w; no terminal to ask on (use --yes to run it)", runErr)
	}
	// Say why the run stopped early, before the rest of the document and
	// again after it, so it is seen whether or not that scrolls by.
	note := ""
	if n := stopNote(results); n != "" && runErr == nil {
		note = "mdsh: " + n
	}
	if note != "" && (live != nil || (!*write && reportFile == "")) {
		fmt.Fprintln(stderr, note)
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
	}
	if live != nil {
		if err := live.finish(results); err != nil {
			return err
		}
	} else if !*write || reportFile != "" {
		if err := show(results, reportOpts, opts.LinkDir); err != nil {
			return err
		}
	}
	if note != "" {
		fmt.Fprintln(stderr, note)
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

// stopNote says why a run stopped before its last block, or returns ""
// if it didn't.
func stopNote(results []*run.Result) string {
	for i, r := range results {
		switch {
		case r.Declined:
			return fmt.Sprintf("stopped before block %s", r.Block.ID)
		case !r.Ran || !unreached(results[i+1:]):
		case r.Exited && r.Err != nil:
			return fmt.Sprintf("block %s %v, which ended its shell, so the run stopped there", r.Block.ID, r.Err)
		case r.Exited:
			return fmt.Sprintf("block %s ended its shell (exit status %d, from exit or set -e), so the run stopped there", r.Block.ID, r.Status)
		case r.Failed():
			return fmt.Sprintf("block %s failed (exit status %d) and the run stops on errors (on_error: stop or --stop-on-error)", r.Block.ID, r.ExitStatus())
		}
	}
	return ""
}

// unreached reports whether any of results is a block the run would
// have run but never got to.
func unreached(results []*run.Result) bool {
	for _, r := range results {
		if !r.Ran && !r.Skipped && run.Runnable(r.Block) && !r.Block.Attrs.Has("skip") {
			return true
		}
	}
	return false
}

// asks reports whether a run of d may stop to ask before a block.
func asks(d *doc.Document, opts run.Options) bool {
	if opts.Step || d.Options.Step {
		return true
	}
	for _, b := range d.Blocks {
		if b.Attrs.Has("confirm") {
			return true
		}
	}
	return false
}

// streamer prints the rendered document as a run reaches each block:
// the prose and code before the block starts, its output once it is
// done, and the rest at the end.
type streamer struct {
	d       *doc.Document
	print   func(md []byte) error
	opts    render.Options
	index   map[*doc.Block]int
	results []*run.Result
	printed int // pieces printed so far
	err     error
}

func newStreamer(d *doc.Document, w io.Writer, styled bool, opts render.Options) *streamer {
	s := &streamer{d: d, print: newPrinter(w, styled), opts: opts, index: map[*doc.Block]int{}, results: make([]*run.Result, len(d.Blocks))}
	for i, b := range d.Blocks {
		s.index[b] = i
	}
	return s
}

func (s *streamer) event(e run.Event) {
	switch e.Kind {
	case run.Started:
		s.upTo(e.Block)
	case run.Done:
		i := s.index[e.Block]
		s.results[i] = e.Result
		if s.printed > 3*i+2 && e.Block.Attrs.Has("background") && s.err == nil {
			// A background block ends after later blocks were printed,
			// so its output goes here, under its name.
			s.err = s.print([]byte(fmt.Sprintf("*Background block %s ended:*\n\n%s", e.Block.ID, render.Output(e.Result, e.Result.Failed()))))
			return
		}
		s.flush(3*i + 3)
	}
}

// upTo prints everything before b's output.
func (s *streamer) upTo(b *doc.Block) { s.flush(3*s.index[b] + 2) }

// flush prints the pieces before piece n that aren't printed yet.
func (s *streamer) flush(n int) {
	if n <= s.printed || s.err != nil {
		return
	}
	pieces := render.Pieces(s.d, s.results, s.opts)
	md := bytes.Join(pieces[s.printed:n], nil)
	s.printed = n
	if len(md) > 0 {
		s.err = s.print(md)
	}
}

// finish prints the rest of the document with the run's final results.
func (s *streamer) finish(results []*run.Result) error {
	s.results = results
	s.flush(3*len(s.d.Blocks) + 1)
	return s.err
}

// interactiveCmd opens file in the TUI. With no file, or a folder, the
// TUI starts with a list of the markdown files under it to pick from.
// Output is saved when asked, or with autosave after every block that
// runs.
func interactiveCmd(file string, args []string, stop, step, yes, autosave bool, stdin io.Reader, stdout io.Writer) error {
	if file == "-" {
		return errors.New("-i needs a file, not stdin")
	}
	if f, ok := stdin.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) || !isTerminal(stdout) {
		return errors.New("-i needs a terminal")
	}
	c := tui.Config{Dir: "."}
	if file != "" {
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		if info.IsDir() {
			c.Dir = file
		} else {
			c.File = file
		}
	}
	if conf, err := os.UserConfigDir(); err == nil {
		c.State = filepath.Join(conf, "mdsh", "tui.json")
	}
	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}
	if os.Getenv("NO_COLOR") != "" {
		style = "notty"
	}
	// Blocks marked tty get the terminal while they run.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer tty.Close()
	protocol := termimg.Detect(os.Getenv)
	c.Open = func(file string) (*doc.Document, tui.Options, error) {
		abs, err := filepath.Abs(file)
		if err != nil {
			return nil, tui.Options{}, err
		}
		src, err := os.ReadFile(abs)
		if err != nil {
			return nil, tui.Options{}, err
		}
		d, err := doc.Parse(src)
		if err != nil {
			return nil, tui.Options{}, fmt.Errorf("%s: %w", file, err)
		}
		return d, tui.Options{
			Run: run.Options{
				TTY:         tty,
				File:        abs,
				LinkDir:     filepath.Dir(abs),
				Args:        args,
				Environ:     os.Environ(),
				StopOnError: stop,
				Step:        step,
			},
			Yes:      yes,
			Save:     func(file string, out []byte) error { return writeFile(file, out, 0o644) },
			AutoSave: autosave,
			Style:    style,
			Images:   style != "notty",
			Protocol: protocol,
		}, nil
	}
	return tui.Run(c)
}

// prompter asks whether to run blocks. It remembers the [o] answer,
// which skips every later block that has output saved in the file.
type prompter struct {
	in        *bufio.Reader
	out       io.Writer
	skipSaved bool
	// quiet leaves the block's code out of the question, for when it
	// has just been printed.
	quiet bool
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{in: bufio.NewReader(in), out: out}
}

// confirm shows block b and asks whether to run it, skip it or stop.
// Pressing enter stops, so it is the safe choice.
func (p *prompter) confirm(b *doc.Block) run.Answer {
	if p.skipSaved && b.HasOutput() {
		fmt.Fprintf(p.out, "--- block %s: skipped, keeping its saved output ---\n", b.ID)
		return run.Skip
	}
	q := b.Attrs["confirm"]
	if q == "" {
		q = "Run this block?"
	}
	saved := ""
	if b.HasOutput() {
		saved = " (has saved output)"
	}
	if p.quiet {
		fmt.Fprintf(p.out, "\n--- block %s%s ---\n", b.ID, saved)
	} else {
		fmt.Fprintf(p.out, "\n--- block %s%s ---\n%s---\n", b.ID, saved, b.Body)
	}
	for {
		fmt.Fprintf(p.out, "%s [r]un / s[k]ip / [o]nly run blocks without saved output / [s]top: ", q)
		line, err := p.in.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "r", "run", "y", "yes":
			return run.Go
		case "k", "skip":
			return run.Skip
		case "o", "only":
			p.skipSaved = true
			if b.HasOutput() {
				return run.Skip
			}
			return run.Go
		case "s", "stop", "n", "no", "":
			return run.Stop
		}
		if err != nil {
			return run.Stop
		}
	}
}

func print(w io.Writer, md []byte, styled bool) error { return newPrinter(w, styled)(md) }

// newPrinter returns a func that prints markdown to w, styled if asked.
// It sets the style up once, so it can print a document in pieces.
func newPrinter(w io.Writer, styled bool) func(md []byte) error {
	var r *glamour.TermRenderer
	if styled {
		r, _ = glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(termWidth(w)))
	}
	return func(md []byte) error {
		if r != nil {
			if out, err := r.RenderBytes(md); err == nil {
				md = out
			}
		}
		_, err := w.Write(md)
		return err
	}
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
