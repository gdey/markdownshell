package notebook

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// sync catches the session up with an edited document. Blocks that
// kept their identity keep their results. A document that gains its
// first tty block needs new shells.
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
		sess, err := n.newSession(n.opts.Run)
		if err != nil {
			return err
		}
		n.closeSession()
		n.sess = sess
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

// Limits on what listFiles lists, so a document in a big folder, such as
// a home folder, doesn't stall completion.
const (
	maxListDepth = 3
	maxListFiles = 500
)

// listFiles lists the regular files under dir, as sorted slash paths
// relative to it. It leaves out hidden files and folders, node_modules,
// vendor and .assets folders (res:// paths name what is in those), goes
// at most maxListDepth folders down and stops at maxListFiles. It is
// empty if dir doesn't exist.
func listFiles(dir string) []string {
	var files []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if len(files) >= maxListFiles {
			return filepath.SkipAll
		}
		if err != nil {
			return filepath.SkipDir
		}
		if path == dir {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || strings.HasSuffix(name, ".assets") || strings.Count(rel, string(filepath.Separator)) >= maxListDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !d.Type().IsRegular() {
			return nil
		}
		if rel, err := filepath.Rel(dir, path); err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(files)
	return files
}

// Problem is something wrong in a fence's info string, for the block
// editor to underline. From and To are byte offsets in the info string.
type Problem struct {
	From     int    `json:"from"`
	To       int    `json:"to"`
	Severity string `json:"severity"` // error or warning
	Message  string `json:"message"`
}

// CheckInfo checks the attributes in a fence's info string, such as
// sh {#build timeout=5m}, against doc.BlockAttrs.
func CheckInfo(info string) []Problem {
	in := doc.SplitInfo(info)
	include := false
	for _, t := range in.Tokens {
		if t == doc.IncludeAttr {
			include = true
		}
	}
	input := (in.Lang == doc.TextboxLang || in.Lang == doc.ChoiceLang) && !include
	var problems []Problem
	at := len(in.Lang)
	for _, tok := range in.Tokens {
		from := strings.Index(info[at:], tok)
		if from < 0 {
			continue
		}
		from += at
		at = from + len(tok)
		add := func(severity, format string, args ...any) {
			problems = append(problems, Problem{From: from, To: at, Severity: severity, Message: fmt.Sprintf(format, args...)})
		}
		if strings.HasPrefix(tok, ".") {
			continue
		}
		key, value, set := strings.Cut(tok, "=")
		if strings.HasPrefix(tok, "#") {
			key, value, set = "#", tok[1:], true
		}
		spec, ok := doc.LookupAttr(key)
		switch {
		case !ok:
			add("warning", "mdsh doesn't know %s and leaves it as it is", key)
			continue
		case include && !spec.Include:
			if key == "#" {
				add("error", "an include block has no name")
			} else {
				add("error", "an include block never runs, so it can't have %s", key)
			}
			continue
		case !include && spec.Include:
			add("warning", "%s only means something with <include", key)
			continue
		case len(spec.Inputs) > 0 && (include || !slices.Contains(spec.Inputs, in.Lang)):
			add("warning", "%s only means something on a %s", key, strings.Join(spec.Inputs, " or "))
			continue
		case input && len(spec.Inputs) == 0 && key != "#" && key != "show":
			add("warning", "a %s never runs, so %s means nothing on it", in.Lang, key)
			continue
		}
		if err := spec.Check(unquote(value), set); err != nil {
			add("error", "%s", err)
		}
	}
	return problems
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
	d, err := doc.Parse([]byte(text))
	if err != nil {
		return nil
	}
	var out []SourceBlock
	for _, b := range d.Blocks {
		if b.IsInclude() || b.IsTape() {
			continue
		}
		line := strings.Count(text[:b.Span.Start], "\n") + 1
		out = append(out, SourceBlock{Line: line, ID: b.ID, Runs: !b.IsInput()})
	}
	return out
}

// unquote matches doc's reading of "quoted" values.
func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`)
	}
	return v
}

// lineRe is the line number a document warning starts with.
var lineRe = regexp.MustCompile(`^line (\d+): `)

// logWarnings logs, for the console, the document's warnings that are
// new since it last did, such as a depends= naming no block. One that
// only moved to another line isn't new. The file and the line go with
// it as attributes, so the console can take the source pane there.
func (n *Notebook) logWarnings() {
	now := make(map[string]bool, len(n.d.Warnings))
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
		if line > 0 {
			slog.Warn(key, "file", n.file, "line", line)
		} else {
			slog.Warn(key, "file", n.file)
		}
	}
	n.warned = now
}
