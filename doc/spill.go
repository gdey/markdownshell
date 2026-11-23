package doc

import (
	"fmt"
	"strings"
)

// SpillsOutput reports whether b's output goes to a file, with out=file
// or out=file:path, and its output fence is an include of that file.
func (b *Block) SpillsOutput() bool {
	v := b.Attrs["out"]
	return v == "file" || strings.HasPrefix(v, "file:")
}

// WindowAttrs are the attributes of an output fence that is an include
// of a block's out=file, which say what part of the file it shows. They
// are the author's: write-back keeps them as the fence has them.
var WindowAttrs = []string{"from", "to", "match", "before", "lines", "until", "number_on"}

// windowOf is the window attributes in a, or nil when it has none.
func windowOf(a Attrs) Attrs {
	var w Attrs
	for _, k := range WindowAttrs {
		if v, ok := a[k]; ok {
			if w == nil {
				w = Attrs{}
			}
			w[k] = v
		}
	}
	return w
}

// SpillInclude is the include an out=file output fence is: file= file,
// with window, the fence's window attributes. With none, it is the
// first lines lines, or the whole file for lines 0.
func SpillInclude(file string, window Attrs, lines int) (Include, error) {
	a := Attrs{IncludeAttr: "", "file": file}
	for k, v := range window {
		a[k] = v
	}
	if len(window) == 0 || len(window) == 1 && window.Has("number_on") {
		if lines > 0 {
			a["lines"] = fmt.Sprint(lines)
		}
	}
	return (&Block{Attrs: a}).Include()
}

// checkSpill checks the attributes out=file can't go with.
func checkSpill(b *Block) error {
	if !b.SpillsOutput() {
		return nil
	}
	switch {
	case b.Attrs.Has("secret"):
		return fmt.Errorf("block %s: out=file can't go with secret: the file would hold the output unmasked", b.ID)
	case b.Attrs.Has("tty"):
		return fmt.Errorf("block %s: a tty block's output is a recording, so it can't have out=file", b.ID)
	case b.IsHTTP():
		return fmt.Errorf("block %s: an http block's output is the response, so it can't have out=file", b.ID)
	}
	return nil
}

// checkWindow checks the window attributes of output fence f, when it is
// an include of an out=file file.
func checkWindow(f *Block) error {
	if !f.Attrs.Has(IncludeAttr) {
		return nil
	}
	if _, err := SpillInclude("file", windowOf(f.Attrs), 0); err != nil {
		return fmt.Errorf("output for %s: %w", f.Attrs["for"], err)
	}
	return nil
}
