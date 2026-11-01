package doc

import (
	"fmt"
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
type Include struct {
	File     string
	From, To int
	// Numbered asks for each line to be shown with its line number in
	// the file. The numbers are for display, never stored in the body.
	Numbered bool
}

// IsInclude reports whether b is an include block.
func (b *Block) IsInclude() bool { return b.Attrs.Has(IncludeAttr) }

// notForIncludes are the attributes that make a block a step of the run,
// so an include block can't have them.
var notForIncludes = []string{"env", "share", "run", "confirm", "timeout", "background", "tty", "out", "on_error", "show-output", "show-stderr", "collapse-output", "depends"}

// Include reads an include block's attributes.
func (b *Block) Include() (Include, error) {
	inc := Include{File: b.Attrs["file"]}
	for _, a := range notForIncludes {
		if b.Attrs.Has(a) {
			return inc, fmt.Errorf("an include block never runs, so it can't have %s=", a)
		}
	}
	if inc.File == "" {
		return inc, fmt.Errorf("include needs file=")
	}
	for _, a := range []struct {
		key string
		n   *int
	}{{"from", &inc.From}, {"to", &inc.To}} {
		v, ok := b.Attrs[a.key]
		if !ok {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return inc, fmt.Errorf("%s= must be a line number, got %q", a.key, v)
		}
		*a.n = n
	}
	if inc.To != 0 && inc.To < inc.From {
		return inc, fmt.Errorf("to=%d is before from=%d", inc.To, inc.From)
	}
	if v, ok := b.Attrs["number_on"]; ok {
		inc.Numbered = v == "" || truthy(v)
	}
	return inc, nil
}
