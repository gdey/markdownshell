// Package run executes a document's blocks in their envs.
package run

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/docroot"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/httpblock"
	"github.com/gdey/markdownshell/tape"
	"github.com/gdey/markdownshell/termrec"
	"github.com/gdey/markdownshell/textimg"
)

// Options control a run.
type Options struct {
	// HTTPTransport, if set, makes http blocks' requests instead of a
	// transport with the system's proxy and certificates. Tests use it.
	HTTPTransport http.RoundTripper
	// File is the document's path, used for MDSH_FILE and image paths.
	// Empty when the document came from stdin.
	File string
	// Dir is the working directory for every env. Defaults to File's
	// directory, or the current directory.
	Dir string
	// LinkDir is where image links are relative to. Defaults to Dir; set
	// it when the rendered markdown is saved somewhere else.
	LinkDir string
	// Args are the positional parameters, $1 and on.
	Args []string
	// Environ is the caller's environment. Defaults to os.Environ().
	Environ []string
	// AllowOutside lets includes and out= files reach outside the
	// document's root (see package docroot), as mdsh --allow-outside
	// does. Leave it off unless the document is trusted.
	AllowOutside bool
	// AllowedOutside are files, as the document writes them in an
	// include's file= or an out=, that may reach outside the document's
	// root though AllowOutside is off: the list run.Outside gave when
	// the user allowed it, so a file added since stays inside.
	AllowedOutside []string
	// Remote marks a document loaded from a URL (http, https, ftp,
	// sftp). Its shells get only the caller's variables its env.share
	// and share= name, without env.Base (PATH, HOME, USER, ...), and
	// none at all when it names none. See TopEnv.
	Remote bool
	// Shell overrides the front matter's shell.
	Shell string
	// Source runs in every shell as it starts, after the files the
	// front matter's env.source names, such as aliases and functions
	// read from the files given to --source.
	Source string
	// StopOnError stops at the first failing block, whatever the front
	// matter says.
	StopOnError bool
	// Only runs the named block and the earlier blocks in its env chain.
	Only string
	// OnEvent, if set, is called as blocks start, write and finish.
	OnEvent func(Event)
	// ApproveSources is asked, right before the first shell starts,
	// whether to read in the files the front matter's env.source names:
	// paths are the files that would be read, each made absolute. Go
	// sources them into every shell; Skip starts the shells without
	// them; Stop starts no shell, and what needed one fails with
	// ErrSourcesNotApproved, which stops a run. If ApproveSources is
	// nil, a document with env.source stops that way, since nobody can
	// say yes. A shell that only answers a question, as the History
	// pane's prompt does, asks too. Source, the caller's own, is not
	// asked about. See package trust for remembering the answer.
	ApproveSources func(paths []string) Answer
	// Step asks Confirm before every block that would run, not just
	// those with the confirm attribute.
	Step bool
	// Confirm is asked before each block that has the confirm attribute,
	// or every block in step mode, right before it runs, after the
	// blocks its depends= names. Its answer runs the block, skips it
	// (keeping any output saved in the file) or stops the run there.
	// If Confirm is nil, the run stops at the first such block with
	// ErrNoConfirm, since nobody can answer.
	Confirm func(*doc.Block) Answer
	// Depend is asked before running dep, a block that b's depends=
	// names and that hasn't run in this session (or failed when it
	// did). The whole chain is worked out first; then each dep is
	// asked about from the top of the chain down, right before it would
	// run, once the blocks it needs have run. Go runs dep then, Skip
	// goes on without it, and Stop runs neither it, the rest of the
	// chain nor b. b's own Confirm comes after. Answering Go also
	// answers dep's own confirm. If Depend is nil, the run stops there
	// with ErrNoConfirm, as for Confirm.
	// A dep with nothing to run, only blank lines and # comments, and
	// no confirm attribute, runs without asking.
	Depend func(dep, b *doc.Block) Answer
	// DependOff is asked instead of Depend about a dependency that is
	// switched off where it stands: marked skip, or with a run=
	// condition that is false (why is "skip", "run=" or "run=once").
	// Go runs it anyway; Skip treats it as done without running it, so
	// b still runs; Stop is as for Depend. It is asked even when Depend
	// always answers Go, as mdsh --yes does, since skip reads as "this
	// won't run". If DependOff is nil, such a dependency is skipped,
	// and the audit log says why.
	DependOff func(dep, b *doc.Block, why string) Answer
	// Ask is asked the questions blocks put with mdsh ctl ask and mdsh
	// ctl confirm, while the block that asked runs: q.Kind is AskCtl or
	// AskCtlConfirm. It returns the answer and who gave it, or ByNone
	// when nobody can answer, and then the call's --default answers, or
	// the call exits ctl.NoAnswer. For AskCtlConfirm the answer is
	// ctl.Yes or ctl.NoValue; for AskCtl with Choices it is one of them.
	// ctx ends when the block does, as when its timeout= ends it; the
	// question is then withdrawn, and Ask should stop asking and return
	// ByNone. Ask is called from the control socket's goroutine, so it
	// may be called while the run waits on Confirm or Depend. If Ask is
	// nil, or Mode is ModeReport, nobody can answer.
	Ask func(ctx context.Context, q Question) (answer string, by Answered)
	// TTY is the terminal that blocks marked tty run on. If it is nil,
	// such blocks fail with ErrNoTTY.
	TTY *os.File
	// Terminal, if TTY is nil, is a terminal the caller draws itself,
	// such as a pane in a window, for blocks marked tty to run on.
	Terminal *env.Terminal
	// TakeTTY, if set, is called before a tty block uses TTY, so a front
	// end that draws on the terminal can step aside. The block runs once
	// it returns, and calls the function it returned when done with TTY.
	TakeTTY func(*doc.Block) (giveBack func())
	// Mode is $MDSH_MODE: ModeRunning, ModeInteractive or ModeReport.
	// Defaults to ModeRunning.
	Mode string
	// Audit, if set, gets the run's events: blocks starting, ending
	// and skipped, envs starting, and calls to the control socket. The
	// caller writes run.start and run.end.
	Audit *audit.Log
	// PTY starts every shell with a terminal of its own, as a document
	// with a tty block does, so Session.RunTyped can run any block on
	// Terminal.
	PTY bool
	// KillAfter, if set, kills a block's shell when the block is still
	// running that long after the run was stopped or the block timed
	// out. Stopping a block interrupts it, as ctrl+c would, and keeps
	// its shell and what earlier blocks set there; one that won't stop
	// runs on until Session.Kill, or until KillAfter. Zero is for a
	// caller that asks the user before Session.Kill. See
	// env.Config.KillAfter.
	KillAfter time.Duration
}

// KillAfter is Options.KillAfter for a run nobody can be asked about,
// such as mdsh on the command line.
const KillAfter = 5 * time.Second

// The values of $MDSH_MODE, which tells blocks how they are being run.
const (
	// ModeRunning is a run from the command line, such as mdsh doc.md or
	// mdsh -w doc.md. It is the default.
	ModeRunning = "running"
	// ModeInteractive is a run where someone picks blocks as they go:
	// mdsh -i and mdsh-gui.
	ModeInteractive = "interactive"
	// ModeReport is a run for a report, such as mdsh --pdf.
	ModeReport = "report"
)

// Answer is a reply to Confirm.
type Answer int

const (
	Stop Answer = iota
	Go
	Skip
)

// ErrNoConfirm is returned when a block needs confirming and the run has
// no way to ask.
var ErrNoConfirm = errors.New("block needs confirmation")

// ErrSourcesNotApproved is the error when a shell would start for a
// document whose env.source files nobody approved; see
// Options.ApproveSources.
var ErrSourcesNotApproved = errors.New("env.source files not approved")

// ErrDependency is a block's error when a block its depends= names
// failed, so it didn't run.
var ErrDependency = errors.New("a block it depends on failed")

// ErrNoTTY is a tty block's error when the run has no terminal to give it.
var ErrNoTTY = errors.New("block needs a terminal (tty) and there is none")

// EventKind says what an Event reports.
type EventKind int

const (
	Started EventKind = iota
	Stdout
	Stderr
	Done
	// Status is a block reporting progress with mdsh ctl status. It
	// replaces the block's last status, and the block's Done ends it.
	// It comes from the control socket's goroutine, while the block
	// runs, so OnEvent must be safe to call from more than one.
	Status
	// Unanswered is a block's mdsh ctl ask or confirm that nobody
	// answered and that had no --default, so the call exited
	// ctl.NoAnswer. Question is the question. It comes from the control
	// socket's goroutine, as Status does. A front end shows it with the
	// block as an mdsh error: "no answer: QUESTION".
	Unanswered
)

// Event reports progress. Data is set for Stdout and Stderr, Result for
// Done, Progress for Status, and Question for Unanswered.
type Event struct {
	Kind     EventKind
	Block    *doc.Block
	Data     []byte
	Result   *Result
	Progress *Progress
	Question *Question
}

// Result is what one block did.
type Result struct {
	Block   *doc.Block
	Ran     bool // false if skipped or not reached
	Skipped bool // skip, run= false, unknown language, not selected by Only, or skipped at the prompt
	Stdout  []byte
	Stderr  []byte
	Status  int
	// Exited is true when the block ended its shell, as exit does, or its
	// shell was killed.
	Exited bool
	// Running is true while a background block is still going. Its
	// output and status come in once it ends.
	Running bool
	Err     error
	// Image is the markdown image target when the block has out=image:
	// a path relative to the document, or a data: URI.
	Image string
	// Vars holds the variables placeholders ask for, as the block left
	// them. Unset variables are empty.
	Vars map[string]string
	// Declined is true when the block asked for confirmation and the run
	// stopped there instead.
	Declined bool
	// Seq orders the session's runs: each block run gets the next one,
	// counting from 1. It is 0 for a block that hasn't run.
	Seq uint64
	// Deps holds, for each block the block's depends= names, that
	// block's Seq when this one started, so Stale can tell when one has
	// run again since, or had its run cleared.
	Deps map[string]uint64
	// Duration is how long the block ran, from its Started event to its
	// Done, for a block that ran in a Session. A background block's runs
	// until Reap sees it end.
	Duration time.Duration
	// TimedOut is true when the block's timeout= ended it, which stops
	// the run.
	TimedOut bool
	// Request is an http block's request as written, placeholders and
	// all, and HTTP what came back, with its checks' results. HTTP
	// holds the headers unmasked: a view masks them as the output does
	// (see httpblock.Format). Both are nil for other blocks, and HTTP
	// is nil when the request got no response.
	Request *httpblock.Request
	HTTP    *httpblock.Response
}

// Failed reports whether the block ran and did not succeed.
func (r *Result) Failed() bool { return r.Ran && (r.Status != 0 || r.Exited || r.Err != nil) }

// ExitStatus is the block's exit status, or 1 when mdsh failed to run
// it (r.Err) and there is no status.
func (r *Result) ExitStatus() int {
	if r.Status == 0 && r.Err != nil {
		return 1
	}
	return r.Status
}

// Runnable reports whether mdsh knows how to run b.
func Runnable(b *doc.Block) bool {
	return (b.Lang == "sh" || b.Lang == "zsh" || b.Lang == httpblock.Lang) && !b.IsInclude()
}

// UsesShell reports whether b runs in a shell: it is runnable and not
// an http block, which mdsh runs itself.
func UsesShell(b *doc.Block) bool { return Runnable(b) && !b.IsHTTP() }

// Shell is the shell d's blocks run in: the front matter's shell, else
// zsh if any block is a zsh block, else "" for the default /bin/sh.
func Shell(d *doc.Document) string {
	if d.Options.Shell != "" {
		return d.Options.Shell
	}
	for _, b := range d.Blocks {
		if b.Lang == "zsh" && Runnable(b) {
			return "zsh"
		}
	}
	return ""
}

// Run executes d's blocks in order and returns one result per block.
// It stops early when a block ends its shell, or when a block fails and
// on_error is stop. Blocks after the stop are returned with Ran false.
func Run(ctx context.Context, d *doc.Document, opts Options) ([]*Result, error) {
	s, err := NewSession(d, opts)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	err = s.RunFrom(ctx, 0)
	return s.Results(), err
}

// NewLookup resolves refs for conditions and placeholders: {{$VAR}} from
// environ (unless environ is nil) as the front matter's env.share filters
// it for blocks (see CallerVars), {{$MDSH_VAR_name}} from the textbox or
// choice in d that binds it with var=, {{#block/env(VAR)}}, {{#block/?}} and
// {{#block/ran?}} from the block's result, and otherwise from a value a
// placeholder stored in the file. It reads results as they are when it
// is called, so it sees blocks that finish after it is made. {{$?}} and
// {{$ran?}} depend on where they are used; see LookupAt.
func NewLookup(d *doc.Document, results []*Result, environ []string) doc.Lookup {
	var caller map[string]string
	if environ != nil {
		caller = CallerVars(d, environ)
	}
	stored := map[doc.Ref]string{}
	for _, p := range d.Placeholders {
		if p.HasValue {
			stored[p.Ref] = p.Value
		}
	}
	return func(ref doc.Ref) (string, bool) {
		if ref.BlockID == "" && !ref.Fact() && strings.HasPrefix(ref.Var, doc.VarPrefix) {
			// An input's var=, as it is in the document now.
			for _, b := range d.Blocks {
				if name, value, ok := b.Var(); ok && name == ref.Var {
					return value, true
				}
			}
		}
		if ref.BlockID == "" && !ref.Fact() && caller != nil {
			return caller[ref.Var], true
		}
		for _, r := range results {
			if r != nil && r.Block.ID == ref.BlockID && (r.Ran || ref.Var == doc.Ran) {
				if v, ok := r.fact(ref); ok {
					return v, true
				}
				if v, ok := r.Vars[ref.Var]; ok {
					return v, true
				}
			}
		}
		v, ok := stored[ref]
		return v, ok
	}
}

// ExposeVar lists, comma-separated, the caller's variables that {{$VAR}}
// may read. mdsh --expose adds to it.
const ExposeVar = "MDSH_EXPOSE"

// CallerVars is what {{$VAR}} reads from environ: mdsh's own MDSH_
// variables, and the caller's variables that the person running mdsh
// named in $MDSH_EXPOSE, if d's front matter shares them with its blocks
// (env.share). Others read as unset. The list has to come from the
// person, not the document: a document someone sent could name
// $GITHUB_TOKEN in its own front matter as easily as in a placeholder.
func CallerVars(d *doc.Document, environ []string) map[string]string {
	all := env.Vars(environ)
	exposed := map[string]string{}
	for _, name := range strings.Split(all[ExposeVar], ",") {
		if v, ok := all[strings.TrimSpace(name)]; ok {
			exposed[strings.TrimSpace(name)] = v
		}
	}
	vars := env.Filter(exposed, d.Options.Env.Share)
	for k, v := range all {
		if IsOwnVar(k) {
			vars[k] = v
		}
	}
	return vars
}

// WithExpose returns environ with names added to $MDSH_EXPOSE.
func WithExpose(environ []string, names []string) []string {
	if len(names) == 0 {
		return environ
	}
	list := names
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, ExposeVar+"="); ok {
			if v != "" {
				list = append(strings.Split(v, ","), names...)
			}
			continue
		}
		out = append(out, kv)
	}
	return append(out, ExposeVar+"="+strings.Join(list, ","))
}

// IsOwnVar reports whether name is one of mdsh's own variables, such as
// MDSH_MODE, rather than one from the caller's environment.
func IsOwnVar(name string) bool { return strings.HasPrefix(name, "MDSH_") }

// fact returns r's value for a block fact such as {{#block/?}}.
func (r *Result) fact(ref doc.Ref) (string, bool) {
	switch {
	case ref.IsStatus():
		return strconv.Itoa(r.ExitStatus()), true
	case ref.Var == doc.Ran:
		return strconv.FormatBool(r.Ran), true
	}
	return "", false
}

// LookupAt wraps lookup for refs whose meaning depends on where they are
// used. {{$?}} is the exit status of the last block before pos that ran;
// {{$ran?}} is whether the last runnable block before pos ran. For a
// block's run= that is the block before it; for show= and the other
// display conditions, pos is past the block, so it is the block itself.
// With no such block they keep a stored value or read as unknown.
func LookupAt(lookup doc.Lookup, d *doc.Document, results []*Result, pos int) doc.Lookup {
	return func(ref doc.Ref) (string, bool) {
		if ref.BlockID != "" || !ref.Fact() {
			return lookup(ref)
		}
		for i := len(d.Blocks) - 1; i >= 0; i-- {
			b := d.Blocks[i]
			if b.Span.End > pos || i >= len(results) || results[i] == nil || !Runnable(b) {
				continue
			}
			if r := results[i]; r.Ran || ref.Var == doc.Ran {
				return r.fact(ref)
			}
		}
		return lookup(ref)
	}
}

func stopOnError(d *doc.Document, b *doc.Block, opts Options) bool {
	if opts.StopOnError {
		return true
	}
	if v, ok := b.Attrs["on_error"]; ok {
		return v == "stop"
	}
	return d.Options.OnError == "stop"
}

// tapeFor returns the tape for b: its text, or for an included tape the
// file's lines, or the error reading them. It returns nil if b has none.
func tapeFor(d *doc.Document, results []*Result, b *doc.Block) *Result {
	t := d.Tape(b)
	if t == nil {
		return nil
	}
	for _, r := range results {
		if r.Block == t && t.IsInclude() {
			return r
		}
	}
	return &Result{Block: t, Stdout: []byte(t.Body)}
}

// runBlock runs b after pre, its prelude. taped is its tape, if it has
// one, and lookup fills in the tape's placeholders.
func runBlock(ctx context.Context, stack *env.Stack, d *doc.Document, b *doc.Block, pre string, r *Result, opts Options, emit func(Event), vars []string, taped *Result, lookup doc.Lookup) {
	r.Ran = true
	e, err := envFor(ctx, stack, d, b)
	if err != nil {
		r.Err = err
		return
	}

	if t, ok := b.Attrs["timeout"]; ok {
		dur, err := time.ParseDuration(t)
		if err != nil {
			r.Err = fmt.Errorf("timeout: %w", err)
			return
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}

	var stdout, stderr bytes.Buffer
	out := io.MultiWriter(&stdout, eventWriter{Stdout, b, emit})
	errw := io.MultiWriter(&stderr, eventWriter{Stderr, b, emit})

	var rec *termrec.Recorder
	switch {
	case !b.Attrs.Has("tty"):
		if kind, _ := outKind(b); kind == "image" {
			r.Status, r.Exited, err = e.Shell.Run(ctx, pre+b.Body, out, errw)
			break
		}
		// Text output is shown as text, so backspace overstrikes, as
		// man prints bold and underline, are applied.
		o, ew := &overstrikeWriter{w: out}, &overstrikeWriter{w: errw}
		r.Status, r.Exited, err = e.Shell.Run(ctx, pre+b.Body, o, ew)
		o.Flush()
		ew.Flush()
	case taped != nil:
		// The tape types for the block, so it needs no terminal. It
		// runs on a fixed size, so it records the same anywhere.
		rec, err = runTaped(ctx, e.Shell, pre+b.Body, taped, lookup, r, opts)
	case opts.TTY == nil && opts.Terminal != nil:
		rec = termrec.NewRecorder(opts.Terminal.Dims())
		giveBack := takeTTY(b, opts)
		r.Status, r.Exited, err = e.Shell.RunTerminal(ctx, pre+b.Body, *opts.Terminal, rec)
		giveBack()
	case opts.TTY == nil:
		err = ErrNoTTY
	default:
		// The block talks to the terminal, so its output is a recording
		// of the session.
		cols, rows, serr := term.GetSize(int(opts.TTY.Fd()))
		if serr != nil || cols <= 0 || rows <= 0 {
			cols, rows = 80, 24
		}
		rec = termrec.NewRecorder(cols, rows)
		giveBack := takeTTY(b, opts)
		r.Status, r.Exited, err = e.Shell.RunTTY(ctx, pre+b.Body, opts.TTY, rec)
		giveBack()
	}
	r.Stdout, r.Stderr = stdout.Bytes(), stderr.Bytes()
	if err != nil && !errors.Is(err, env.ErrExited) {
		if errors.Is(err, context.DeadlineExceeded) {
			r.TimedOut = true
			err = fmt.Errorf("timed out after %s", b.Attrs["timeout"])
		}
		r.Err = err
	}

	if kind, _ := outKind(b); (kind == "image" || kind == "screenshot") && r.Err == nil && len(r.Stdout) > 0 {
		r.Image, r.Err = saveImage(b, r.Stdout, opts)
	}
	// A recording is saved even when the block failed, to show how.
	if rec != nil {
		img, err := saveRecording(b, rec.Recording(), opts)
		r.Image = img
		if r.Err == nil {
			r.Err = err
		}
	}

	if len(vars) > 0 && !r.Exited {
		r.Vars, err = captureVars(ctx, e.Shell, vars)
		if err != nil && r.Err == nil {
			r.Err = err
		}
	}
}

// envFor returns the env b runs in.
func envFor(ctx context.Context, stack *env.Stack, d *doc.Document, b *doc.Block) (*env.Env, error) {
	share := b.Attrs.List("share")
	if share == nil {
		share = d.Options.Env.Share
	}
	return stack.Get(ctx, b.Attrs["env"], share)
}

// captureVars reads vars from sh, exported or not.
func captureVars(ctx context.Context, sh *env.Shell, vars []string) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range vars {
		if _, ok := out[v]; ok {
			continue
		}
		var buf bytes.Buffer
		// v is a validated name, so it is safe to splice in.
		if _, _, err := sh.Run(ctx, `printf '%s' "${`+v+`-}"`, &buf, io.Discard); err != nil {
			return out, fmt.Errorf("reading $%s: %w", v, err)
		}
		out[v] = buf.String()
	}
	return out, nil
}

// prelude sets the MDSH_ variables, the MDSH_VAR_ ones that inputs bind
// with var= among them. It shares the body's first line so line numbers
// in shell errors match the block.
func prelude(d *doc.Document, b *doc.Block, opts Options) string {
	dir := opts.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	names := []string{"MDSH_BLOCK", "MDSH_FILE", "MDSH_DIR", "MDSH_MODE"}
	set := fmt.Sprintf("MDSH_BLOCK=%s MDSH_FILE=%s MDSH_DIR=%s MDSH_MODE=%s", quote(b.ID), quote(opts.File), quote(dir), quote(mode(opts.Mode)))
	for _, in := range d.Blocks {
		if name, value, ok := in.Var(); ok {
			names = append(names, name)
			set += " " + name + "=" + oneLine(value)
		}
	}
	return set + "; export " + strings.Join(names, " ") + "; "
}

// oneLine quotes s for the shell without a newline, so the prelude stays
// on the body's first line: a value of several lines is printed by
// printf, one quoted line at a time.
func oneLine(s string) string {
	if !strings.Contains(s, "\n") {
		return quote(s)
	}
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		lines = append(lines, quote(l))
	}
	return `"$(printf '%s\n' ` + strings.Join(lines, " ") + `)"`
}

// mode is the value of $MDSH_MODE for Options.Mode.
func mode(m string) string {
	if m == "" {
		return ModeRunning
	}
	return m
}

// WithMode is environ with MDSH_MODE set to mode, so conditions such as
// show={{$MDSH_MODE}}!=report can read it.
func WithMode(environ []string, mode string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if !strings.HasPrefix(kv, "MDSH_MODE=") {
			out = append(out, kv)
		}
	}
	return append(out, "MDSH_MODE="+mode)
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// outKind splits the out= attribute: "image:plots/a.png" is ("image",
// "plots/a.png"). A tty block's output is always a recording; out= only
// picks its path.
func outKind(b *doc.Block) (kind, arg string) {
	kind, arg, _ = strings.Cut(b.Attrs["out"], ":")
	switch {
	case b.Attrs.Has("tty"):
		kind = "recording"
	case kind == "":
		kind = "text"
	}
	return kind, arg
}

// tapeCols and tapeRows are the terminal size a taped block runs on.
const tapeCols, tapeRows = 80, 24

// takeTTY asks opts.TakeTTY, if set, for the terminal for b.
func takeTTY(b *doc.Block, opts Options) (giveBack func()) {
	if opts.TakeTTY == nil {
		return func() {}
	}
	return opts.TakeTTY(b)
}

// runTaped runs script on sh's terminal with its input typed by the tape
// in taped, sets r's status, and returns the recording. Placeholders in
// what the tape types in double quotes are filled in from lookup as it
// starts, so values
// such as passwords stay out of the document. What the block shows is
// also shown on opts.TTY, if there is one.
func runTaped(ctx context.Context, sh *env.Shell, script string, taped *Result, lookup doc.Lookup, r *Result, opts Options) (*termrec.Recorder, error) {
	if taped.Err != nil {
		return nil, fmt.Errorf("tape: %w", taped.Err)
	}
	cmds, err := tape.Parse(string(taped.Stdout))
	if err != nil {
		return nil, err
	}
	for i, c := range cmds {
		if c.Literal {
			continue
		}
		if cmds[i].Text, err = doc.Expand(c.Text, lookup); err != nil {
			return nil, fmt.Errorf("tape line %d: %w", c.Line, err)
		}
	}
	rec := termrec.NewRecorder(tapeCols, tapeRows)
	screen := termrec.NewScreen(tapeCols, tapeRows)
	out := []io.Writer{rec, screen}
	switch {
	case opts.TTY != nil:
		out = append(out, opts.TTY)
		defer takeTTY(r.Block, opts)()
	case opts.Terminal != nil && opts.Terminal.Out != nil:
		out = append(out, opts.Terminal.Out)
		defer takeTTY(r.Block, opts)()
	}
	r.Status, r.Exited, err = sh.RunScripted(ctx, script, tapeCols, tapeRows, io.MultiWriter(out...), func(ctx context.Context, keys io.Writer) error {
		return tape.Play(ctx, cmds, keys, screen.Text)
	})
	return rec, err
}

// hasTTY reports whether any block in d may run on the terminal.
func hasTTY(d *doc.Document) bool {
	for _, b := range d.Blocks {
		if Runnable(b) && b.Attrs.Has("tty") {
			return true
		}
	}
	return false
}

// saveRecording saves a tty block's session as an asciinema cast and
// returns the link to it: <doc>.assets/<block id>.cast, the path
// out=recording:path names, or a data: URI with embed=inline. A path
// ending in .gif asks for an animated GIF instead, made from the cast;
// the cast is then saved too, at the GIF's path with a .cast extension
// (or <doc>.assets/<block id>.cast with embed=inline). A session that
// showed nothing saves nothing.
func saveRecording(b *doc.Block, rec termrec.Recording, opts Options) (string, error) {
	if len(rec.Events) == 0 {
		return "", nil
	}
	_, rel := outKind(b)
	rel, err := resolveRes(rel, opts)
	if err != nil {
		return "", err
	}
	var cast bytes.Buffer
	if err := termrec.WriteCast(&cast, rec); err != nil {
		return "", fmt.Errorf("recording: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(rel), ".gif") {
		return saveAsset(b, rel, cast.Bytes(), CastMIME, ".cast", opts)
	}
	data, err := termrec.GIF(rec)
	if err != nil {
		return "", fmt.Errorf("recording: %w", err)
	}
	castRel := ""
	if !b.Attrs.Has("embed") || b.Attrs["embed"] != "inline" {
		castRel = strings.TrimSuffix(rel, filepath.Ext(rel)) + ".cast"
	}
	if err := writeAsset(assetPath(b, castRel, ".cast", opts), cast.Bytes(), docRootFor(rel, opts)); err != nil {
		return "", err
	}
	return saveAsset(b, rel, data, "image/gif", ".gif", opts)
}

// CastMIME is the media type of an asciicast v2 file, for data: URIs.
const CastMIME = "application/x-asciicast"

// OutKind returns the block's output kind: text, markdown, image,
// screenshot, or recording for a tty block.
func OutKind(b *doc.Block) string {
	k, _ := outKind(b)
	return k
}

// saveImage saves a block's stdout as an image. With out=image, stdout
// must already be an image. With out=screenshot, the text is drawn as a
// terminal-style image: PNG if the path ends in .png, else SVG.
func saveImage(b *doc.Block, data []byte, opts Options) (string, error) {
	kind, rel := outKind(b)
	rel, err := resolveRes(rel, opts)
	if err != nil {
		return "", err
	}
	var mime, ext string
	if kind == "image" {
		if mime, ext = sniffImage(data); mime == "" {
			return "", fmt.Errorf("out=image: output is not a PNG, JPEG, GIF or SVG image (use out=screenshot to draw text)")
		}
	} else {
		if strings.EqualFold(filepath.Ext(rel), ".png") {
			if data, err = textimg.PNG(string(data)); err != nil {
				return "", err
			}
			mime, ext = "image/png", ".png"
		} else {
			data, mime, ext = textimg.SVG(string(data)), "image/svg+xml", ".svg"
		}
	}
	return saveAsset(b, rel, data, mime, ext, opts)
}

// saveAsset writes a block's image to rel, by default
// <doc>.assets/<block id><ext>, or makes it a data: URI with
// embed=inline, and returns the link to it.
func saveAsset(b *doc.Block, rel string, data []byte, mime, ext string, opts Options) (string, error) {
	if b.Attrs.Has("embed") && b.Attrs["embed"] == "inline" {
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	if rel == "" {
		rel = defaultAsset(b, ext, opts)
	}
	path := assetPath(b, rel, ext, opts)
	if err := writeAsset(path, data, docRootFor(rel, opts)); err != nil {
		return "", err
	}
	if opts.LinkDir != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if link, err := filepath.Rel(opts.LinkDir, abs); err == nil {
			return filepath.ToSlash(link), nil
		}
		return filepath.ToSlash(abs), nil
	}
	return rel, nil
}

// defaultAsset is where a block's file goes when nothing says where:
// <doc>.assets/<block id><ext>, relative to the document.
func defaultAsset(b *doc.Block, ext string, opts Options) string {
	return filepath.ToSlash(filepath.Join(assetsDir(opts), b.ID+ext))
}

// assetsDir is the document's assets folder, <doc>.assets, relative to
// opts.Dir.
func assetsDir(opts Options) string {
	base := "mdsh"
	if opts.File != "" {
		base = filepath.Base(opts.File)
	}
	return base + ".assets"
}

// ResScheme starts a path in the document's assets folder:
// res://notes.md is <doc>.assets/notes.md.
const ResScheme = "res://"

// resolveRes turns a res:// path into one relative to opts.Dir, in the
// document's assets folder. Other paths are returned as they are. A
// res:// path must name a file inside the folder.
func resolveRes(path string, opts Options) (string, error) {
	name, ok := strings.CutPrefix(path, ResScheme)
	if !ok {
		return path, nil
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if name == "" || clean == "." || filepath.IsAbs(name) || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%s must name a file in %s", path, assetsDir(opts))
	}
	return filepath.ToSlash(filepath.Join(assetsDir(opts), clean)), nil
}

// assetPath is the file rel names, or the default for b and ext if rel
// is empty.
func assetPath(b *doc.Block, rel, ext string, opts Options) string {
	if rel == "" {
		rel = defaultAsset(b, ext, opts)
	}
	return filepath.Join(opts.Dir, filepath.FromSlash(rel))
}

// writeAsset writes a block's file, which must be inside root unless
// root is "": see docRootFor. A symlink at path is replaced, not
// followed.
func writeAsset(path string, data []byte, root string) error {
	err := docroot.WriteFile(root, path, data)
	if pe := (*fs.PathError)(nil); errors.As(err, &pe) {
		return fmt.Errorf("can't write %s: %w", path, pe.Err)
	}
	return err
}

// docRootFor is the folder a read or write of rel, as the document
// writes it, must stay in, or "" with opts.AllowOutside or when rel is
// in opts.AllowedOutside: see docroot.Root.
func docRootFor(rel string, opts Options) string {
	if opts.AllowOutside || (rel != "" && slices.Contains(opts.AllowedOutside, rel)) {
		return ""
	}
	dir := opts.Dir
	if dir == "" && opts.File != "" {
		dir = filepath.Dir(opts.File)
	}
	return docroot.Root(dir)
}

func sniffImage(data []byte) (mime, ext string) {
	switch t := http.DetectContentType(data); t {
	case "image/png":
		return t, ".png"
	case "image/jpeg":
		return t, ".jpg"
	case "image/gif":
		return t, ".gif"
	}
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	if bytes.Contains(head, []byte("<svg")) {
		return "image/svg+xml", ".svg"
	}
	return "", ""
}

// selectBlocks returns the blocks to run. With only set, that is the named
// block plus earlier blocks whose env is it or one of its ancestors.
func selectBlocks(d *doc.Document, only string) (map[*doc.Block]bool, error) {
	sel := map[*doc.Block]bool{}
	if only == "" {
		for _, b := range d.Blocks {
			sel[b] = true
		}
		return sel, nil
	}
	target := d.Block(only)
	if target == nil {
		return nil, fmt.Errorf("no block with id %q", only)
	}
	tenv := strings.Trim(target.Attrs["env"], "/")
	for _, b := range d.Blocks {
		benv := strings.Trim(b.Attrs["env"], "/")
		if benv == "" || benv == tenv || strings.HasPrefix(tenv, benv+"/") {
			sel[b] = true
		}
		if b == target {
			break
		}
	}
	return sel, nil
}

type eventWriter struct {
	kind  EventKind
	block *doc.Block
	emit  func(Event)
}

func (w eventWriter) Write(p []byte) (int, error) {
	w.emit(Event{Kind: w.kind, Block: w.block, Data: append([]byte(nil), p...)})
	return len(p), nil
}
