// Package run executes a document's blocks in their envs.
package run

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/textimg"
)

// Options control a run.
type Options struct {
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
	// Shell overrides the front matter's shell.
	Shell string
	// StopOnError stops at the first failing block, whatever the front
	// matter says.
	StopOnError bool
	// Only runs the named block and the earlier blocks in its env chain.
	Only string
	// OnEvent, if set, is called as blocks start, write and finish.
	OnEvent func(Event)
	// Confirm is asked before each block that has the confirm attribute.
	// It returns true to run the block and false to stop the run there.
	// If Confirm is nil, the run stops at the first such block with
	// ErrNoConfirm, since nobody can answer.
	Confirm func(*doc.Block) bool
}

// ErrNoConfirm is returned when a block needs confirming and the run has
// no way to ask.
var ErrNoConfirm = errors.New("block needs confirmation")

// EventKind says what an Event reports.
type EventKind int

const (
	Started EventKind = iota
	Stdout
	Stderr
	Done
)

// Event reports progress. Data is set for Stdout and Stderr, Result for Done.
type Event struct {
	Kind   EventKind
	Block  *doc.Block
	Data   []byte
	Result *Result
}

// Result is what one block did.
type Result struct {
	Block   *doc.Block
	Ran     bool // false if skipped or not reached
	Skipped bool // skip, run= false, unknown language, or not selected by Only
	Stdout  []byte
	Stderr  []byte
	Status  int
	// Exited is true when the block ended its shell (exit, or a timeout).
	Exited bool
	Err    error
	// Image is the markdown image target when the block has out=image:
	// a path relative to the document, or a data: URI.
	Image string
	// Vars holds the variables placeholders ask for, as the block left
	// them. Unset variables are empty.
	Vars map[string]string
	// Declined is true when the block asked for confirmation and the run
	// stopped there instead.
	Declined bool
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
func Runnable(b *doc.Block) bool { return b.Lang == "sh" }

// Run executes d's blocks in order and returns one result per block.
// It stops early when a block ends its shell, or when a block fails and
// on_error is stop. Blocks after the stop are returned with Ran false.
func Run(ctx context.Context, d *doc.Document, opts Options) ([]*Result, error) {
	if opts.Environ == nil {
		opts.Environ = os.Environ()
	}
	if opts.Dir == "" && opts.File != "" {
		opts.Dir = filepath.Dir(opts.File)
	}
	shell := opts.Shell
	if shell == "" {
		shell = d.Options.Shell
	}
	selected, err := selectBlocks(d, opts.Only)
	if err != nil {
		return nil, err
	}

	stack := env.NewStack(env.Config{
		Path: shell,
		Env:  env.Environ(env.Filter(env.Vars(opts.Environ), d.Options.Env.Share)),
		Dir:  opts.Dir,
		Args: opts.Args,
	})
	defer stack.Close()

	results := make([]*Result, len(d.Blocks))
	for i, b := range d.Blocks {
		results[i] = &Result{Block: b}
	}
	emit := func(e Event) {
		if opts.OnEvent != nil {
			opts.OnEvent(e)
		}
	}

	wanted := map[string][]string{}
	for _, ref := range d.Refs() {
		if ref.BlockID != "" && !ref.Fact() {
			wanted[ref.BlockID] = append(wanted[ref.BlockID], ref.Var)
		}
	}
	lookup := NewLookup(d, results, opts.Environ)

	for i, b := range d.Blocks {
		r := results[i]
		if !Runnable(b) || b.Attrs.Has("skip") || !selected[b] {
			r.Skipped = true
			continue
		}
		if c, ok := b.Cond("run"); ok && !c.Eval(LookupAt(lookup, d, results, b.Span.Start)) {
			r.Skipped = true
			continue
		}
		if b.Attrs.Has("confirm") {
			if opts.Confirm == nil {
				r.Declined = true
				return results, fmt.Errorf("%w: %s", ErrNoConfirm, b.ID)
			}
			if !opts.Confirm(b) {
				r.Declined = true
				break
			}
		}
		emit(Event{Kind: Started, Block: b})
		runBlock(ctx, stack, d, b, r, opts, emit, wanted[b.ID])
		emit(Event{Kind: Done, Block: b, Result: r})
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		if r.Exited || (r.Failed() && stopOnError(d, b, opts)) {
			break
		}
	}
	return results, nil
}

// NewLookup resolves refs for conditions and placeholders: {{$VAR}} from
// environ (unless environ is nil), {{#block/env(VAR)}}, {{#block/?}} and
// {{#block/ran?}} from the block's result, and otherwise from a value a
// placeholder stored in the file. It reads results as they are when it
// is called, so it sees blocks that finish after it is made. {{$?}} and
// {{$ran?}} depend on where they are used; see LookupAt.
func NewLookup(d *doc.Document, results []*Result, environ []string) doc.Lookup {
	var caller map[string]string
	if environ != nil {
		caller = env.Vars(environ)
	}
	stored := map[doc.Ref]string{}
	for _, p := range d.Placeholders {
		if p.HasValue {
			stored[p.Ref] = p.Value
		}
	}
	return func(ref doc.Ref) (string, bool) {
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

func runBlock(ctx context.Context, stack *env.Stack, d *doc.Document, b *doc.Block, r *Result, opts Options, emit func(Event), vars []string) {
	r.Ran = true
	share := b.Attrs.List("share")
	if share == nil {
		share = d.Options.Env.Share
	}
	e, err := stack.Get(ctx, b.Attrs["env"], share)
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

	r.Status, r.Exited, err = e.Shell.Run(ctx, prelude(b, opts)+b.Body, out, errw)
	r.Stdout, r.Stderr = stdout.Bytes(), stderr.Bytes()
	if err != nil && !errors.Is(err, env.ErrExited) {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("timed out after %s", b.Attrs["timeout"])
		}
		r.Err = err
	}

	if kind, _ := outKind(b); (kind == "image" || kind == "screenshot") && r.Err == nil && len(r.Stdout) > 0 {
		r.Image, r.Err = saveImage(b, r.Stdout, opts)
	}

	if len(vars) > 0 && !r.Exited {
		r.Vars, err = captureVars(ctx, e.Shell, vars)
		if err != nil && r.Err == nil {
			r.Err = err
		}
	}
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

// prelude sets the MDSH_ variables. It shares the body's first line so
// line numbers in shell errors match the block.
func prelude(b *doc.Block, opts Options) string {
	dir := opts.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return fmt.Sprintf("MDSH_BLOCK=%s MDSH_FILE=%s MDSH_DIR=%s; export MDSH_BLOCK MDSH_FILE MDSH_DIR; ",
		quote(b.ID), quote(opts.File), quote(dir))
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// outKind splits the out= attribute: "image:plots/a.png" is ("image", "plots/a.png").
func outKind(b *doc.Block) (kind, arg string) {
	kind, arg, _ = strings.Cut(b.Attrs["out"], ":")
	if kind == "" {
		kind = "text"
	}
	return kind, arg
}

// OutKind returns the block's output kind: text, markdown, image or screenshot.
func OutKind(b *doc.Block) string {
	k, _ := outKind(b)
	return k
}

// saveImage saves a block's stdout as an image. With out=image, stdout
// must already be an image. With out=screenshot, the text is drawn as a
// terminal-style image: PNG if the path ends in .png, else SVG.
func saveImage(b *doc.Block, data []byte, opts Options) (string, error) {
	kind, rel := outKind(b)
	var mime, ext string
	if kind == "image" {
		if mime, ext = sniffImage(data); mime == "" {
			return "", fmt.Errorf("out=image: output is not a PNG, JPEG, GIF or SVG image (use out=screenshot to draw text)")
		}
	} else {
		var err error
		if strings.EqualFold(filepath.Ext(rel), ".png") {
			if data, err = textimg.PNG(string(data)); err != nil {
				return "", err
			}
			mime, ext = "image/png", ".png"
		} else {
			data, mime, ext = textimg.SVG(string(data)), "image/svg+xml", ".svg"
		}
	}
	if b.Attrs.Has("embed") && b.Attrs["embed"] == "inline" {
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	if rel == "" {
		base := "mdsh"
		if opts.File != "" {
			base = filepath.Base(opts.File)
		}
		rel = filepath.ToSlash(filepath.Join(base+".assets", b.ID+ext))
	}
	path := filepath.Join(opts.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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
