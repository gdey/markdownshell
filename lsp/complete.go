package lsp

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/gdey/markdownshell/doc"
)

// Names are what completion offers besides the attributes: block ids,
// for conditions, env names, and files for paths: those in the
// document's assets folder, for res:// paths, and those beside the
// document.
type Names struct {
	IDs    []string
	Envs   []string
	Assets []string
	Files  []string
	// AssetsDir is the assets folder's name, <doc>.assets.
	AssetsDir string
}

// NamesFor lists the names in text, the document at file. Without a file
// there are no files to offer.
func NamesFor(text, file string) Names {
	var n Names
	for _, b := range SourceBlocks(text) {
		n.IDs = append(n.IDs, b.ID)
		if b.Env != "" && !slices.Contains(n.Envs, b.Env) {
			n.Envs = append(n.Envs, b.Env)
		}
	}
	slices.Sort(n.Envs)
	if file != "" {
		n.AssetsDir = filepath.Base(file) + ".assets"
		n.Assets = ListFiles(file + ".assets")
		n.Files = slices.DeleteFunc(ListFiles(filepath.Dir(file)), func(f string) bool { return f == filepath.Base(file) })
	}
	return n
}

// Item is a completion: Label is shown, Insert replaces the text from
// the token's start to the cursor.
type Item struct {
	Label  string
	Insert string
	Detail string
	Doc    string
	Kind   string // keyword, property, enum, variable, constant, text
}

// tokenAt finds the attribute token being typed at col (bytes) on line:
// its start and its text so far.
func tokenAt(line string, col int) (start int, tok string, ok bool) {
	from, info, ok := infoRange(line)
	if !ok || col < from || col > from+len(info) {
		return 0, "", false
	}
	before := info[:col-from]
	open := strings.LastIndex(before, "{")
	if open < 0 || strings.LastIndex(before, "}") > open {
		return 0, "", false
	}
	s := open + 1
	quoted := false
	for i := open + 1; i < len(before); i++ {
		switch c := before[i]; {
		case c == '"':
			quoted = !quoted
		case !quoted && unicode.IsSpace(rune(c)):
			s = i + 1
		}
	}
	return from + s, before[s:], true
}

// inputAttrs are the attributes, besides their own, that mean something
// on a textbox or a choice.
var inputAttrs = map[string]bool{"#": true, "show": true}

var presentRe = regexp.MustCompile(`[{\s]([#<\w-]+)`)

// Complete lists the completions at line (from 0) and col (bytes) of
// text, the document at file, and the byte column they replace from.
func Complete(text, file string, line, col int) (from int, items []Item) {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return 0, nil
	}
	l := lines[line]
	col = min(col, len(l))
	start, tok, ok := tokenAt(l, col)
	if !ok {
		return 0, nil
	}
	_, info, _ := infoRange(l)
	key, value, hasValue := strings.Cut(tok, "=")
	if !hasValue {
		return start, attrItems(info, tok)
	}
	spec, ok := doc.LookupAttr(key)
	if !ok {
		return 0, nil
	}
	vstart := start + len(key) + 1
	if key == "depends" {
		return dependsItems(text, line, vstart, value)
	}
	names := NamesFor(text, file)
	opened := strings.HasPrefix(value, `"`)
	if opened {
		value = value[1:]
	}
	// path is a completion for a file path. A value with spaces or
	// braces needs quotes; one with a quote can't be written at all.
	path := func(label, detail string) []Item {
		if strings.Contains(label, `"`) {
			return nil
		}
		insert := label
		switch {
		case opened:
			insert = label + `"`
		case strings.ContainsAny(label, " \t{}"):
			insert = `"` + label + `"`
		}
		return []Item{{Label: label, Insert: insert, Detail: detail, Kind: "text"}}
	}
	assetsDir := "the assets folder"
	if names.AssetsDir != "" {
		assetsDir = names.AssetsDir + "/"
	}
	res := func(prefix string) (out []Item) {
		for _, f := range names.Assets {
			out = append(out, path(prefix+"res://"+f, "in "+assetsDir)...)
		}
		return out
	}
	var values []Item
	switch spec.Kind {
	case doc.AttrChoice:
		if key == "out" {
			if m := outPathRe.FindString(value); m != "" {
				values = res(m)
				break
			}
		}
		for _, v := range spec.Values {
			values = append(values, Item{Label: v, Kind: "enum"})
		}
	case doc.AttrText:
		switch key {
		case "env":
			for _, e := range names.Envs {
				values = append(values, Item{Label: e, Kind: "variable"})
			}
		case "file":
			values = res("")
			if !strings.HasPrefix(value, "res:") {
				for _, f := range names.Files {
					values = append(values, path(f, "beside the document")...)
				}
			}
		}
	case doc.AttrCond:
		values = []Item{
			{Label: "{{$VAR}}", Detail: "true unless empty, 0, false, no or off", Kind: "variable"},
			{Label: "{{$?}}", Detail: "the last block's exit status", Kind: "variable"},
			{Label: "{{$ran?}}", Detail: "whether the last block ran", Kind: "variable"},
		}
		for _, id := range names.IDs {
			values = append(values,
				Item{Label: fmt.Sprintf("{{#%s/?}}", id), Detail: fmt.Sprintf("#%s's exit status", id), Kind: "variable"},
				Item{Label: fmt.Sprintf("{{#%s/ran?}}", id), Detail: fmt.Sprintf("whether #%s ran", id), Kind: "variable"})
		}
		if key == "run" {
			values = append(values, Item{Label: "once", Detail: "only while the block has no output", Kind: "constant"})
		}
	case doc.AttrDuration:
		for _, v := range []string{"30s", "5m", "1h"} {
			values = append(values, Item{Label: v, Kind: "constant"})
		}
	}
	if opened {
		vstart++
	}
	return vstart, values
}

var outPathRe = regexp.MustCompile(`^(image|screenshot|recording):`)

// attrItems are the attributes that can go in info, the fence's info
// string, where tok is being typed: those not there already, and for an
// include, a textbox or a choice only those that mean something on it.
func attrItems(info, tok string) []Item {
	present := map[string]bool{}
	for _, m := range presentRe.FindAllStringSubmatch(info, -1) {
		k := m[1]
		if strings.HasPrefix(k, "#") {
			k = "#"
		}
		present[k] = true
	}
	include := present[doc.IncludeAttr]
	input := ""
	if !include {
		if m := inputLangRe.FindStringSubmatch(info); m != nil {
			input = m[1]
		}
	}
	var out []Item
	for _, a := range doc.BlockAttrs {
		if input != "" {
			if !slices.Contains(a.Inputs, input) && !inputAttrs[a.Name] {
				continue
			}
		} else if len(a.Inputs) > 0 || (a.Include != include && a.Name != doc.IncludeAttr) {
			continue
		}
		if present[a.Name] && !strings.HasPrefix(tok, a.Name) {
			continue
		}
		it := Item{Label: a.Name, Insert: a.Name + "=", Doc: a.Help, Kind: "property"}
		if a.Default != "" {
			it.Detail = "default: " + a.Default
		}
		switch {
		case a.Name == "#":
			it.Label, it.Insert = "#id", "#"
		case a.Kind == doc.AttrFlag:
			it.Insert, it.Kind = a.Name, "keyword"
		case a.Bare:
			it.Insert = a.Name
		}
		out = append(out, it)
	}
	return out
}

var inputLangRe = regexp.MustCompile(`^\s*(textbox|choice)\b`)

// dependsItems offers, for the name being typed in line's depends=
// whose value, so far, starts at from, the blocks above that run and
// aren't listed already.
func dependsItems(text string, line, from int, value string) (int, []Item) {
	blocks := SourceBlocks(text)
	opened := strings.HasPrefix(value, `"`)
	comma := strings.LastIndex(value, ",")
	listed := map[string]bool{}
	head := value
	if opened {
		head = head[1:]
	}
	if c := strings.LastIndex(head, ","); c >= 0 {
		for _, s := range strings.Split(head[:c], ",") {
			listed[strings.TrimSpace(s)] = true
		}
	}
	start := from
	switch {
	case comma >= 0:
		start = from + comma + 1
	case opened:
		start = from + 1
	}
	rest := value[start-from:]
	start += len(rest) - len(strings.TrimLeftFunc(rest, unicode.IsSpace))
	var out []Item
	for _, b := range blocks {
		if b.Line-1 < line && b.Runs && !listed[b.ID] {
			out = append(out, Item{Label: b.ID, Detail: fmt.Sprintf("line %d", b.Line), Kind: "variable"})
		}
	}
	return start, out
}

// Help is the attribute at line (from 0) and col (bytes) of text, with
// where it is in the line, or ok is false.
func Help(text string, line, col int) (spec doc.AttrSpec, from, to int, ok bool) {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return doc.AttrSpec{}, 0, 0, false
	}
	l := lines[line]
	start, info, ok := infoRange(l)
	if !ok || col < start || col > start+len(info) {
		return doc.AttrSpec{}, 0, 0, false
	}
	for _, m := range helpRe.FindAllStringIndex(info, -1) {
		from, to := start+m[0], start+m[1]
		if col < from || col > to {
			continue
		}
		tok := info[m[0]:m[1]]
		key, _, _ := strings.Cut(tok, "=")
		if strings.HasPrefix(tok, "#") {
			key = "#"
		}
		spec, ok := doc.LookupAttr(key)
		return spec, from, to, ok
	}
	return doc.AttrSpec{}, 0, 0, false
}

var helpRe = regexp.MustCompile(`[#<\w-]+(=("[^"]*"|\S*))?`)

// HelpText is spec's help as markdown.
func HelpText(spec doc.AttrSpec) string {
	name := spec.Name
	if name == "#" {
		name = "#id"
	}
	s := fmt.Sprintf("**%s** %s", name, spec.Help)
	if spec.Default != "" {
		s += "\n\nDefault: " + spec.Default
	}
	return s
}
