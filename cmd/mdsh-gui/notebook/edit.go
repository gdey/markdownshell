package notebook

import (
	"errors"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/lsp"
	"github.com/gdey/markdownshell/run"
)

// sync catches the session up with an edited document. Blocks that
// kept their identity keep their results. A document that gains its
// first tty block needs new shells, as does front matter that parses
// now and didn't, since its shells were readied with the defaults.
func (n *Notebook) sync() error {
	n.logWarnings()
	n.waitReap()
	old := make(map[*doc.Block]*run.Result, len(n.results))
	for _, r := range n.results {
		old[r.Block] = r
	}
	n.results = make([]*run.Result, len(n.d.Blocks))
	clear(n.index)
	for i, b := range n.d.Blocks {
		n.index[b] = i
		if n.results[i] = old[b]; n.results[i] == nil {
			n.results[i] = &run.Result{Block: b}
		}
	}
	if err := n.sess.Sync(); err != nil {
		if !errors.Is(err, run.ErrRestart) {
			return err
		}
		why := whyTTY
		if errors.Is(err, run.ErrFrontMatterFixed) {
			why = whyFrontMatter
		}
		if n.ran() {
			// Starting the shells again would lose what blocks set in
			// them; that is the user's call, with Restart Shells. Until
			// then the tty block can't run.
			n.opts.Events.Event("restart.needed", audit.Fields{"reason": why})
		} else {
			sess, err := n.newSession(n.opts.Run)
			if err != nil {
				return err
			}
			n.replaceSession(sess, why)
		}
	}
	// The session reads include blocks that are new, so a typed
	// include shows its file's lines without a run.
	for i, r := range n.sess.Results() {
		if r.Block.IsInclude() && old[r.Block] == nil {
			n.results[i] = r
		}
	}
	return nil
}

// Names are what attribute completion offers: block ids, for conditions,
// env names, and files for paths: those in the document's assets folder,
// for res:// paths, and those beside the document.
type Names struct {
	IDs    []string `json:"ids"`
	Envs   []string `json:"envs"`
	Assets []string `json:"assets"`
	Files  []string `json:"files"`
	// AssetsDir is the assets folder's name, <doc>.assets.
	AssetsDir string `json:"assetsDir"`
}

// Names lists the document's block ids and envs, the files in its assets
// folder (<doc>.assets) as slash paths inside it, and the files in its
// folder as slash paths relative to it.
func (n *Notebook) Names() Names {
	n.mu.Lock()
	defer n.mu.Unlock()
	var names Names
	names.AssetsDir = filepath.Base(n.file) + ".assets"
	names.Assets = listFiles(n.file + ".assets")
	names.Files = slices.DeleteFunc(listFiles(filepath.Dir(n.file)), func(f string) bool { return f == filepath.Base(n.file) })
	for _, b := range n.d.Blocks {
		if b.IsInclude() || b.IsTape() {
			continue
		}
		names.IDs = append(names.IDs, b.ID)
		if e := b.Attrs["env"]; e != "" && !slices.Contains(names.Envs, e) {
			names.Envs = append(names.Envs, e)
		}
	}
	slices.Sort(names.Envs)
	return names
}

// listFiles lists the files under dir as lsp.ListFiles does: sorted
// slash paths relative to it, leaving out hidden files, node_modules,
// vendor and .assets folders, and a big folder's depths.
func listFiles(dir string) []string { return lsp.ListFiles(dir) }

// Problem is something wrong in a fence's info string, for the block
// editor to underline. From and To are byte offsets in the info string.
type Problem struct {
	From     int    `json:"from"`
	To       int    `json:"to"`
	Severity string `json:"severity"` // error or warning
	Message  string `json:"message"`
}

// CheckInfo checks the attributes in a fence's info string, such as
// sh {#build timeout=5m}, against doc.BlockAttrs, as the language
// server does.
func CheckInfo(info string) []Problem {
	var out []Problem
	for _, p := range lsp.CheckInfo(info) {
		out = append(out, Problem{From: p.From, To: p.To, Severity: p.Severity, Message: p.Message})
	}
	return out
}

// SourceBlock is a block in the source pane's text, for completing and
// checking depends= there.
type SourceBlock struct {
	// Line is the line its opening fence is on, from 1.
	Line int    `json:"line"`
	ID   string `json:"id"`
	// Runs says whether depends= can name it: an input never runs.
	Runs bool `json:"runs"`
}

// SourceBlocks lists the blocks in text, the source pane's, in order,
// leaving out includes and tapes, which have no name. It is nil if text
// doesn't parse.
func SourceBlocks(text string) []SourceBlock {
	var out []SourceBlock
	for _, b := range lsp.SourceBlocks(text) {
		out = append(out, SourceBlock{Line: b.Line, ID: b.ID, Runs: b.Runs})
	}
	return out
}

// lineRe is the line number a document warning starts with.
var lineRe = regexp.MustCompile(`^line (\d+): `)

// logWarnings logs, for the console, the document's warnings that are
// new since it last did, such as a depends= naming no block. One that
// only moved to another line isn't new. The file and the line go with
// it as attributes, so the console can take the source pane there, and
// a "fix" attribute names the fix, if it has one (see FixWarning). A
// logged warning the document no longer has goes to Options.Resolved.
func (n *Notebook) logWarnings() {
	now := make(map[string]bool, len(n.d.Warnings))
	var fixes map[string]string
	for _, w := range n.d.Warnings {
		key, line := w, 0
		if m := lineRe.FindStringSubmatch(w); m != nil {
			key = w[len(m[0]):]
			line, _ = strconv.Atoi(m[1])
		}
		now[key] = true
		if n.warned[key] {
			continue
		}
		if fixes == nil {
			fixes = map[string]string{}
			for _, f := range n.d.Fixes() {
				fixes[f.Warning] = f.Label
			}
		}
		args := []any{"file", n.file}
		if line > 0 {
			args = append(args, "line", line)
		}
		if f := fixes[key]; f != "" {
			args = append(args, "fix", f)
		}
		slog.Warn(key, args...)
	}
	if n.opts.Resolved != nil {
		for key := range n.warned {
			if !now[key] {
				n.opts.Resolved(n.file, key)
			}
		}
	}
	n.warned = now
}

// Fix is an edit that makes a warning go away.
type Fix struct {
	// Label names it for a button.
	Label string `json:"label"`
	// Detail says what it does, for asking first.
	Detail string `json:"detail"`
}

// errNoFix is what fixing a warning the document doesn't have, or has
// no fix for, fails with.
var errNoFix = errors.New("that warning no longer applies, or has no fix")

// fixFor finds the fix for warning, as logged, without its line.
func (n *Notebook) fixFor(warning string) (doc.Fix, error) {
	for _, f := range n.d.Fixes() {
		if f.Warning == warning {
			return f, nil
		}
	}
	return doc.Fix{}, errNoFix
}

// DescribeFix says what FixWarning would do for warning, as logged.
func (n *Notebook) DescribeFix(warning string) (Fix, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	f, err := n.fixFor(warning)
	return Fix{Label: f.Label, Detail: f.Detail}, err
}

// FixWarning makes the fix for warning, as logged: it deletes an output
// copy for a block that isn't there, or takes an id that names no block
// that runs out of a depends=. It is a rendered-view edit, so Undo takes
// it back.
func (n *Notebook) FixWarning(warning string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	f, err := n.fixFor(warning)
	if err != nil {
		return err
	}
	return n.change("fix warning", "", f.Src, nil)
}
