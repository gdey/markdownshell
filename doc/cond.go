package doc

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Ref names a variable: in the caller's environment ({{$VAR}}, BlockID
// empty), or as a block left it ({{#block/env(VAR)}}). Var can also be
// one of the block facts below, for the most recent block ({{$?}}) or a
// named one ({{#block/?}}).
type Ref struct {
	BlockID string
	Var     string
}

// Block facts, which end in ? so they can't clash with shell variables.
const (
	Status   = "?"    // exit status
	StatusAs = "err?" // the same, spelled out
	Ran      = "ran?" // "true" if the block ran, else "false"
)

// Fact reports whether r is a block fact rather than a variable.
func (r Ref) Fact() bool { return strings.HasSuffix(r.Var, "?") }

// IsStatus reports whether r is an exit status, under either name.
func (r Ref) IsStatus() bool { return r.Var == Status || r.Var == StatusAs }

func (r Ref) String() string {
	switch {
	case r.BlockID == "":
		return "{{$" + r.Var + "}}"
	case r.Fact():
		return "{{#" + r.BlockID + "/" + r.Var + "}}"
	}
	return "{{#" + r.BlockID + "/env(" + r.Var + ")}}"
}

// newRef builds a Ref from the refBody groups: the $ name, or the block
// id and what follows its slash.
func newRef(dollar, block, tail string) Ref {
	if block == "" {
		return Ref{Var: dollar}
	}
	if strings.HasSuffix(tail, "?") {
		return Ref{BlockID: block, Var: tail}
	}
	return Ref{BlockID: block, Var: tail[len("env(") : len(tail)-1]}
}

// Lookup returns a ref's value; ok is false when it isn't known.
type Lookup func(Ref) (value string, ok bool)

// Cond is a condition such as
//
//	{{$DEBUG}}                    true unless empty, 0, false or no
//	!{{$DEBUG}}                   the opposite
//	{{#detect/env(OS)}}==Linux    string comparison; != also works
//	{{$?}}==0                     the most recent block's exit status
//	{{#build/?}}!=0               block build's exit status
//	{{#build/err?}}               the same; true when build failed
//	{{$ran?}}, {{#build/ran?}}    whether the block ran
//	true, false                   constants
//	once                          in run= only: the block has no output yet
type Cond struct {
	Ref *Ref // nil for a constant
	// Once is run=once: run the block only if it has no output, saved
	// after it or from running earlier. The runner checks that; Eval
	// reports true.
	Once  bool
	Not   bool
	Op    string // "", "==" or "!="
	Value string // compared against, or the constant
	Text  string // as written
}

var (
	facts   = `\?|err\?|ran\?`
	refBody = `(?:\$(` + facts + `|[A-Za-z_][A-Za-z0-9_]*)|#([^/{}\s]+)/(env\([A-Za-z_][A-Za-z0-9_]*\)|` + facts + `))`
	condRE  = regexp.MustCompile(`^(!?)\{\{` + refBody + `\}\}(?:(==|!=)(.*))?$`)
)

// ParseCond parses a condition.
func ParseCond(s string) (Cond, error) {
	s = strings.TrimSpace(s)
	c := Cond{Text: s}
	switch strings.ToLower(s) {
	case "true", "yes", "1", "":
		c.Value = "true"
		return c, nil
	case "false", "no", "0":
		c.Value = "false"
		return c, nil
	case "once":
		c.Value, c.Once = "true", true
		return c, nil
	}
	m := condRE.FindStringSubmatch(s)
	if m == nil {
		return c, fmt.Errorf("bad condition %q: want {{$VAR}}, {{#block/env(VAR)}}, {{$?}}, {{$ran?}} or a block's {{#block/?}} or {{#block/ran?}}, optionally with ==value or !=value, or true, false or once", s)
	}
	c.Not = m[1] == "!"
	ref := newRef(m[2], m[3], m[4])
	c.Ref = &ref
	c.Op, c.Value = m[5], strings.TrimSpace(m[6])
	if c.Not && c.Op != "" {
		return c, fmt.Errorf("bad condition %q: use != instead of ! with a comparison", s)
	}
	return c, nil
}

// Eval reports whether c holds. An unknown ref reads as empty.
func (c Cond) Eval(lookup Lookup) bool {
	if c.Ref == nil {
		return c.Value == "true"
	}
	v, _ := lookup(*c.Ref)
	switch c.Op {
	case "==":
		return v == c.Value
	case "!=":
		return v != c.Value
	}
	return truthy(v) != c.Not
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// The block attributes that hold conditions.
var condAttrs = []string{"run", "show", "show-output", "show-stderr", "collapse", "collapse-output"}

// Cond returns the condition in a block attribute, and whether it is set.
// Parse has already checked that it is valid.
func (b *Block) Cond(attr string) (Cond, bool) {
	v, ok := b.Attrs[attr]
	if !ok {
		return Cond{}, false
	}
	c, _ := ParseCond(v)
	return c, true
}

// Section is prose between <!-- mdsh:if COND --> and <!-- mdsh:end -->
// lines. When COND is false, printing leaves out everything from the if
// line to the end line, blocks included.
type Section struct {
	If, End Span // the marker lines
	Cond    Cond
}

const (
	ifOpen  = "<!-- mdsh:if "
	ifClose = "<!-- mdsh:end -->"
)

// Fold is a <!-- mdsh:collapse --> line, optionally with a condition,
// as in <!-- mdsh:collapse {{$MDSH_VAR_verbose}}!=yes -->. When the
// condition holds, mdsh-gui opens the file with the section of the
// heading above the line folded. Printing leaves the line out.
type Fold struct {
	Span Span // the line
	Cond Cond
}

const foldOpen = "<!-- mdsh:collapse"

// foldLine reports whether line is an mdsh:collapse line, and its
// condition, empty when it has none.
func foldLine(line []byte) (cond string, ok bool) {
	l := string(bytes.TrimSpace(line))
	rest, ok := strings.CutPrefix(l, foldOpen)
	if !ok || !strings.HasSuffix(rest, "-->") {
		return "", false
	}
	rest = rest[:len(rest)-3]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", false // such as mdsh:collapsed
	}
	return strings.TrimSpace(rest), true
}

// sectionLine reports whether line opens or closes a section.
func sectionLine(line []byte) (cond string, open, close bool) {
	l := string(bytes.TrimSpace(line))
	if l == ifClose {
		return "", false, true
	}
	if strings.HasPrefix(l, ifOpen) && strings.HasSuffix(l, "-->") {
		return strings.TrimSpace(l[len(ifOpen) : len(l)-3]), true, false
	}
	return "", false, false
}

// Refs lists every variable the document's placeholders, conditions and
// sections refer to.
func (d *Document) Refs() []Ref {
	var refs []Ref
	for _, p := range d.Placeholders {
		refs = append(refs, p.Ref)
	}
	for _, b := range d.Blocks {
		for _, a := range condAttrs {
			if c, ok := b.Cond(a); ok && c.Ref != nil {
				refs = append(refs, *c.Ref)
			}
		}
	}
	for _, s := range d.Sections {
		if s.Cond.Ref != nil {
			refs = append(refs, *s.Cond.Ref)
		}
	}
	for _, f := range d.Folds {
		if f.Cond.Ref != nil {
			refs = append(refs, *f.Cond.Ref)
		}
	}
	// An included tape's lines are read when the document runs; see
	// run.Run.
	for _, b := range d.Blocks {
		if b.IsTape() && !b.IsInclude() {
			refs = append(refs, TextRefs(b.Body)...)
		}
	}
	return refs
}

// errOnce is the error for once anywhere but run=.
var errOnce = errors.New("once only means something in run=")
