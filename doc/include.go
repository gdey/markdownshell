package doc

import (
	"fmt"
	"regexp"
	"strconv"
)

// IncludeAttr marks an include block:
//
//	```go {<include file=main.go from=10 to=20 number_on}
//	```
//
// mdsh writes the selected lines into the block's body. The block never
// runs and has no name.
const IncludeAttr = "<include"

// Include is what an include block asks for. Lines are numbered from 1
// and the range is inclusive. From 0 or 1 is the first line; To 0 is the
// end of the file.
//
// Match or Symbol start the window by content instead: at the first line
// matching Match, or at the definition of Symbol. Before adds lines above
// the start; Lines or Until end the window, else it runs to the end of
// the file (for Match) or of the definition (for Symbol).
type Include struct {
	File     string
	From, To int
	// Numbered asks for each line to be shown with its line number in
	// the file. The numbers are for display, never stored in the body.
	Numbered bool
	// Match is the match= regexp, checked when parsed.
	Match string
	// Symbol is the symbol= name: a function, a method as T.Method or
	// (*T).Method, a type, a const or a var.
	Symbol string
	// Before is how many lines above the start to show too.
	Before int
	// Lines is how many lines to show from the start, 0 for no limit.
	Lines int
	// Until is the until= regexp: the window ends at the first line
	// matching it at or after the start.
	Until string
	// NoComment is comment=false: a symbol's window leaves out its doc
	// comment.
	NoComment bool
}

// Windowed reports whether inc picks its lines by content, with match=
// or symbol=, rather than by line number.
func (inc Include) Windowed() bool { return inc.Match != "" || inc.Symbol != "" }

// IsInclude reports whether b is an include block.
func (b *Block) IsInclude() bool { return b.Attrs.Has(IncludeAttr) }

// notForIncludes are the attributes that make a block a step of the run,
// so an include block can't have them.
var notForIncludes = []string{"env", "share", "run", "confirm", "timeout", "background", "tty", "out", "on_error", "show-output", "show-stderr", "collapse-output", "depends", "always", "links", "history"}

// Include reads an include block's attributes.
func (b *Block) Include() (Include, error) {
	inc := Include{File: b.Attrs["file"], Match: b.Attrs["match"], Symbol: b.Attrs["symbol"], Until: b.Attrs["until"]}
	for _, a := range notForIncludes {
		if b.Attrs.Has(a) {
			return inc, fmt.Errorf("an include block never runs, so it can't have %s=", a)
		}
	}
	if inc.File == "" {
		return inc, fmt.Errorf("include needs file=")
	}
	for _, a := range []struct {
		key  string
		n    *int
		what string
	}{{"from", &inc.From, "a line number"}, {"to", &inc.To, "a line number"}, {"before", &inc.Before, "a number of lines"}, {"lines", &inc.Lines, "a number of lines"}} {
		v, ok := b.Attrs[a.key]
		if !ok {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return inc, fmt.Errorf("%s= must be %s, got %q", a.key, a.what, v)
		}
		*a.n = n
	}
	if inc.To != 0 && inc.To < inc.From {
		return inc, fmt.Errorf("to=%d is before from=%d", inc.To, inc.From)
	}
	for _, a := range []string{"match", "symbol", "until"} {
		if v, ok := b.Attrs[a]; ok && v == "" {
			return inc, fmt.Errorf("%s= is empty", a)
		}
	}
	for _, re := range []struct{ key, v string }{{"match", inc.Match}, {"until", inc.Until}} {
		if re.v == "" {
			continue
		}
		if _, err := regexp.Compile(re.v); err != nil {
			return inc, fmt.Errorf("%s=: %w", re.key, err)
		}
	}
	// Each pair names the same thing two ways, so it can't be both.
	for _, p := range [][2]string{{"match", "symbol"}, {"from", "match"}, {"from", "symbol"}, {"to", "match"}, {"to", "symbol"}, {"lines", "until"}, {"to", "lines"}, {"to", "until"}} {
		if b.Attrs.Has(p[0]) && b.Attrs.Has(p[1]) {
			return inc, fmt.Errorf("include can't have both %s= and %s=", p[0], p[1])
		}
	}
	if b.Attrs.Has("comment") {
		if inc.Symbol == "" {
			return inc, fmt.Errorf("comment= is for symbol=")
		}
		inc.NoComment = !truthy(b.Attrs["comment"]) && b.Attrs["comment"] != ""
	}
	if b.Attrs.Has("nth") {
		return inc, fmt.Errorf("nth= isn't supported yet: the window starts at the first match")
	}
	if v, ok := b.Attrs["number_on"]; ok {
		inc.Numbered = v == "" || truthy(v)
	}
	return inc, nil
}
