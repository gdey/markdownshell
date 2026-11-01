package lsp

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/doc"
)

// Problem is something wrong in a fence's info string. From and To are
// byte offsets in the info string.
type Problem struct {
	From     int
	To       int
	Severity string // error or warning
	Message  string
}

// CheckInfo checks the attributes in a fence's info string, such as
// sh {#build timeout=5m}, against doc.BlockAttrs.
func CheckInfo(info string) []Problem {
	in := doc.SplitInfo(info)
	include := slices.Contains(in.Tokens, doc.IncludeAttr)
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

// SourceBlock is a block in a document's text, for completing and
// checking depends=.
type SourceBlock struct {
	// Line is the line its opening fence is on, from 1.
	Line int
	ID   string
	// Runs says whether depends= can name it: an input never runs.
	Runs bool
	// Env is its env=, if any.
	Env string
}

// SourceBlocks lists the blocks in text, in order, leaving out includes
// and tapes, which have no name. It is nil if text doesn't parse.
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
		out = append(out, SourceBlock{Line: line, ID: b.ID, Runs: !b.IsInput(), Env: b.Attrs["env"]})
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

// Limits on what ListFiles lists, so a document in a big folder, such as
// a home folder, doesn't stall completion.
const (
	maxListDepth = 3
	maxListFiles = 500
)

// ListFiles lists the regular files under dir, as sorted slash paths
// relative to it. It leaves out hidden files and folders, node_modules,
// vendor and .assets folders (res:// paths name what is in those), goes
// at most maxListDepth folders down and stops at maxListFiles. It is
// empty if dir doesn't exist.
func ListFiles(dir string) []string {
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

// fenceRe matches an opening fence line: the fence, then the info string.
var fenceRe = regexp.MustCompile("^(\\s*(?:`{3,}|~{3,}))(.*)$")

// infoRange is where the info string is on line, if it opens a block's
// fence: its byte offset and text. Output and tape fences carry mdsh's
// own attributes, not a block's.
func infoRange(line string) (from int, info string, ok bool) {
	m := fenceRe.FindStringSubmatchIndex(line)
	if m == nil {
		return 0, "", false
	}
	info = line[m[4]:m[5]]
	if outputRe.MatchString(info) {
		return 0, "", false
	}
	return m[4], info, true
}

var outputRe = regexp.MustCompile(`^\s*(output|tape)\b`)

// dependsRe finds a depends= in an info string: its value, quoted or not.
var dependsRe = regexp.MustCompile(`([{\s]depends=)(?:"([^"]*)"?|([^\s}]*))`)

// varRe finds a var= in an info string.
var varRe = regexp.MustCompile(`[{\s]var="?([A-Za-z0-9_]+)"?`)

var inputRe = regexp.MustCompile(`^\s*(?:textbox|choice)\b`)

// Diagnostic is a problem at a place in a document: Line counts from 0
// and the columns are bytes in that line.
type Diagnostic struct {
	Line, From, To int
	Severity       string // error or warning
	Message        string
}

// Check checks every fence's attributes in text: unknown or misused
// ones, depends= names that aren't a block that runs, and a var= bound
// twice.
func Check(text string) []Diagnostic {
	lines := strings.Split(text, "\n")
	var blocks []SourceBlock
	if strings.Contains(text, "depends=") {
		blocks = SourceBlocks(text)
	}
	fences := map[int]bool{}
	for _, b := range blocks {
		fences[b.Line-1] = true
	}
	vars := map[string]int{}
	var out []Diagnostic
	for n, line := range lines {
		if !strings.Contains(line, "{") {
			continue
		}
		from, info, ok := infoRange(line)
		if !ok || strings.TrimSpace(info) == "" {
			continue
		}
		for _, p := range CheckInfo(info) {
			out = append(out, Diagnostic{Line: n, From: from + p.From, To: from + p.To, Severity: p.Severity, Message: p.Message})
		}
		if fences[n] {
			out = append(out, checkDepends(blocks, n, from, info)...)
		}
		// Only one textbox or choice can bind a variable, or the last
		// would quietly win.
		if !inputRe.MatchString(info) || strings.Contains(info, "<include") {
			continue
		}
		m := varRe.FindStringSubmatchIndex(info)
		if m == nil {
			continue
		}
		name := info[m[2]:m[3]]
		first, seen := vars[name]
		if !seen {
			vars[name] = n
			continue
		}
		out = append(out, Diagnostic{Line: n, From: from + m[0] + 1, To: from + m[1], Severity: "error",
			Message: fmt.Sprintf("var=%s is already bound on line %d, so this one is ignored", name, first+1)})
	}
	return out
}

// checkDepends flags the names in a fence's depends= that aren't a block
// that runs: mdsh ignores them.
func checkDepends(blocks []SourceBlock, line, from int, info string) []Diagnostic {
	m := dependsRe.FindStringSubmatchIndex(info)
	if m == nil {
		return nil
	}
	var value string
	at := from + m[3]
	if m[4] >= 0 {
		value = info[m[4]:m[5]]
		at++
	} else {
		value = info[m[6]:m[7]]
	}
	var out []Diagnostic
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		start := at + strings.Index(part, name)
		at += len(part) + 1
		if name == "" {
			continue
		}
		i := slices.IndexFunc(blocks, func(b SourceBlock) bool { return b.ID == name })
		if i >= 0 && blocks[i].Runs {
			continue
		}
		msg := fmt.Sprintf("There is no block %s, so depends= ignores it", name)
		if i >= 0 {
			msg = fmt.Sprintf("%s is an input, which never runs, so depends= ignores it", name)
		}
		out = append(out, Diagnostic{Line: line, From: start, To: start + len(name), Severity: "error", Message: msg})
	}
	return out
}
