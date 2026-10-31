package doc

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// AttrKind is what kind of value a block attribute takes.
type AttrKind string

const (
	AttrFlag     AttrKind = "flag"     // no value: skip, tty
	AttrText     AttrKind = "text"     // any text: env=, file=, a confirm question
	AttrCond     AttrKind = "cond"     // a condition: run=, show=
	AttrChoice   AttrKind = "choice"   // one of Values: out=, on_error=
	AttrDuration AttrKind = "duration" // a Go duration: timeout=30s
	AttrNumber   AttrKind = "number"   // a whole number: frame=, from=
	AttrList     AttrKind = "list"     // comma-separated names: share=
)

// AttrSpec describes a block attribute, for tools that help write them,
// such as mdsh-gui's attribute form and completion.
type AttrSpec struct {
	// Name is the attribute's key; "#" is the block id, written #id.
	Name string   `json:"name"`
	Kind AttrKind `json:"kind"`
	// Bare is true when a value is optional, as for confirm and
	// show-stderr. Flags are always bare.
	Bare bool `json:"bare,omitempty"`
	// Values are the choices for AttrChoice. A value ending in ":" may
	// be followed by a path, as in out=image:plots/sales.png.
	Values []string `json:"values,omitempty"`
	// Default says what happens without the attribute.
	Default string `json:"default,omitempty"`
	// Help is one line on what the attribute does.
	Help string `json:"help"`
	// Include is true for the attributes of include blocks. Include
	// blocks take none of the others.
	Include bool `json:"include,omitempty"`
	// Inputs are the input block languages, textbox and choice, that
	// the attribute is for. Empty for the attributes of other blocks.
	Inputs []string `json:"inputs,omitempty"`
}

// BlockAttrs are the block attributes mdsh reads, in the order the spec
// lists them. Others are kept untouched.
var BlockAttrs = []AttrSpec{
	{Name: "#", Kind: AttrText, Default: "block-N", Help: "The block's name, for output, placeholders and --only"},
	{Name: "skip", Kind: AttrFlag, Default: "off", Help: "Don't run the block"},
	{Name: "run", Kind: AttrCond, Default: "run", Help: "Run only if the condition holds; once runs it only while it has no output"},
	{Name: "show", Kind: AttrCond, Default: "shown", Help: "Print the code only if the condition holds"},
	{Name: "show-output", Kind: AttrCond, Default: "shown", Help: "Print the output only if the condition holds"},
	{Name: "show-stderr", Kind: AttrCond, Bare: true, Default: "front matter show_stderr, else only when the block fails", Help: "Show stderr, or only if the condition holds"},
	{Name: "collapse", Kind: AttrCond, Bare: true, Default: "expanded", Help: "In mdsh-gui, open with the code folded to one line, or only if the condition holds"},
	{Name: "collapse-output", Kind: AttrCond, Bare: true, Default: "expanded", Help: "In mdsh-gui, open with the output folded to one line, or only if the condition holds"},
	{Name: "env", Kind: AttrText, Default: "top-level env", Help: "Run in a named sub-env; a/b stacks b on a"},
	{Name: "share", Kind: AttrList, Default: "front matter env.share, else all", Help: "Exported variables a new sub-env gets from its parent; empty for none"},
	{Name: "on_error", Kind: AttrChoice, Values: []string{"stop", "continue"}, Default: "front matter on_error", Help: "Stop the run if this block fails, or go on"},
	{Name: "depends", Kind: AttrList, Default: "none", Help: "Blocks to run first, in order: each one that hasn't run this session is asked about"},
	{Name: "confirm", Kind: AttrText, Bare: true, Default: "off", Help: "Ask before running, with this question if given"},
	{Name: "timeout", Kind: AttrDuration, Default: "none", Help: "Kill the block's shell after this long; stops the run. A background block is killed alone"},
	{Name: "background", Kind: AttrFlag, Default: "off", Help: "Start the block and go on to the next; its output is kept apart, and the run waits for it at the end"},
	{Name: "tty", Kind: AttrFlag, Default: "off", Help: "Run on a terminal, for commands that need one; the session is saved as an animated GIF"},
	{Name: "frame", Kind: AttrNumber, Default: "the last", Help: "With tty, the frame of the GIF that still views show, counting from 1"},
	{Name: "out", Kind: AttrChoice, Values: []string{"text", "markdown", "image", "image:", "screenshot", "screenshot:", "recording:"}, Default: "text", Help: "How output is inserted: fenced text, raw markdown, or an image saved beside the file"},
	{Name: "embed", Kind: AttrChoice, Values: []string{"inline"}, Default: "off", Help: "With out=image or out=screenshot, write a data URI instead of a file"},
	{Name: IncludeAttr, Kind: AttrFlag, Include: true, Help: "Fill the block with lines from another file; it never runs"},
	{Name: "file", Kind: AttrText, Include: true, Help: "The file to include, relative to the document's folder; res://name is name in the document's assets folder"},
	{Name: "from", Kind: AttrNumber, Include: true, Default: "the first line", Help: "The first line to include, counting from 1"},
	{Name: "to", Kind: AttrNumber, Include: true, Default: "the last line", Help: "The last line to include"},
	{Name: "number_on", Kind: AttrFlag, Include: true, Default: "off", Help: "Show each included line's number in the file"},
	{Name: "lines", Kind: AttrNumber, Inputs: []string{TextboxLang}, Default: "1", Help: "The most lines a textbox holds; 1 or less is one line"},
	{Name: "mode", Kind: AttrChoice, Values: []string{ChoiceDropdown, ChoiceRadio, ChoiceMulti}, Inputs: []string{ChoiceLang}, Default: ChoiceDropdown, Help: "Pick one from a dropdown or radio buttons, or check any number of boxes"},
	{Name: "placeholder", Kind: AttrText, Inputs: []string{TextboxLang, ChoiceLang}, Default: "none", Help: "What an empty textbox, or a dropdown with nothing chosen, shows in an interactive view"},
	{Name: "var", Kind: AttrText, Inputs: []string{TextboxLang, ChoiceLang}, Default: "none", Help: "Bind the text, or the checked options one per line, to $MDSH_VAR_<name> for blocks and conditions"},
}

// LookupAttr finds the spec for an attribute name.
func LookupAttr(name string) (AttrSpec, bool) {
	i := slices.IndexFunc(BlockAttrs, func(s AttrSpec) bool { return s.Name == name })
	if i < 0 {
		return AttrSpec{}, false
	}
	return BlockAttrs[i], true
}

// Check reports whether value suits the attribute. set is false when
// the attribute is given bare, without =.
func (s AttrSpec) Check(value string, set bool) error {
	if !set {
		if s.Kind == AttrFlag || s.Bare {
			return nil
		}
		return fmt.Errorf("%s needs a value", s.Name)
	}
	switch s.Kind {
	case AttrFlag:
		if s.Name == "number_on" { // number_on=off turns it off
			return nil
		}
		return fmt.Errorf("%s takes no value", s.Name)
	case AttrCond:
		c, err := ParseCond(value)
		if err == nil && c.Once && s.Name != "run" {
			return errOnce
		}
		return err
	case AttrChoice:
		for _, v := range s.Values {
			if path, ok := strings.CutSuffix(v, ":"); ok {
				if strings.HasPrefix(value, path+":") && len(value) > len(v) {
					return nil
				}
				continue
			}
			if value == v {
				return nil
			}
		}
		return fmt.Errorf("%s= is one of %s", s.Name, strings.Join(s.Values, ", "))
	case AttrDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%s= is a duration such as 30s or 5m", s.Name)
		}
	case AttrNumber:
		if n, err := strconv.Atoi(value); err != nil || n < 0 && s.Name != "lines" {
			return fmt.Errorf("%s= is a whole number", s.Name)
		}
	case AttrText:
		if s.Name == "#" && value == "" {
			return fmt.Errorf("an id can't be empty")
		}
		if s.Name == "var" && !ValidVar(value) {
			return fmt.Errorf("var= is a name of letters, digits and _")
		}
	}
	return nil
}

// Info is a fence info string split for editing: the language and the
// attribute tokens as written, so unknown attributes and the order
// survive an edit.
type Info struct {
	Lang   string
	Tokens []string // such as #build, env=build, confirm="Go on?", .sh
}

// SplitInfo splits a fence info string for editing.
func SplitInfo(info string) Info {
	info = strings.TrimSpace(info)
	rest := info
	var in Info
	if !strings.HasPrefix(info, "{") {
		i := strings.IndexFunc(info, func(r rune) bool { return unicode.IsSpace(r) || r == '{' })
		if i < 0 {
			return Info{Lang: info}
		}
		in.Lang, rest = info[:i], strings.TrimSpace(info[i:])
	}
	rest = strings.TrimSuffix(strings.TrimPrefix(rest, "{"), "}")
	in.Tokens = splitAttrs(rest)
	return in
}

// String writes the info string back: lang {tokens}.
func (in Info) String() string {
	if len(in.Tokens) == 0 {
		return in.Lang
	}
	attrs := "{" + strings.Join(in.Tokens, " ") + "}"
	if in.Lang == "" {
		return attrs
	}
	return in.Lang + " " + attrs
}

// tokenKey is the attribute a token sets: "#" for #id, "" for a class.
func tokenKey(tok string) string {
	switch {
	case strings.HasPrefix(tok, "#"):
		return "#"
	case strings.HasPrefix(tok, "."):
		return ""
	}
	k, _, _ := strings.Cut(tok, "=")
	return k
}

// Set sets key to value, in place if it is already there, else at the
// end. bare writes the key without a value, as for skip or confirm.
// For "#" the value is the id.
func (in *Info) Set(key, value string, bare bool) {
	tok := key
	switch {
	case key == "#":
		tok = "#" + value
	case !bare:
		tok = key + "=" + quoteAttr(value)
	}
	for i, t := range in.Tokens {
		if tokenKey(t) == key {
			in.Tokens[i] = tok
			return
		}
	}
	if key == "#" {
		in.Tokens = slices.Insert(in.Tokens, 0, tok) // ids go first
		return
	}
	in.Tokens = append(in.Tokens, tok)
}

// Del removes key.
func (in *Info) Del(key string) {
	in.Tokens = slices.DeleteFunc(in.Tokens, func(t string) bool { return tokenKey(t) == key })
}

// quoteAttr quotes a value that ParseInfo would otherwise split or
// misread.
func quoteAttr(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\"{}") {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}
