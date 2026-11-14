package doc

import (
	"fmt"
	"slices"

	"github.com/gdey/markdownshell/httpblock"
)

// IsHTTP reports whether b is an http block: a request mdsh makes
// itself, with checks on what comes back (see package httpblock).
func (b *Block) IsHTTP() bool { return b.Lang == httpblock.Lang && !b.IsInclude() }

// Request reads http block b's request, as written, placeholders and
// all.
func (b *Block) Request() (*httpblock.Request, error) {
	return httpblock.Parse(b.Body)
}

// SecretVar reports whether name is one of http block b's captures
// marked secret: write-back stores a placeholder for it bare.
func (b *Block) SecretVar(name string) bool {
	if !b.IsHTTP() {
		return false
	}
	r, err := b.Request()
	if err != nil {
		return false
	}
	return slices.ContainsFunc(r.Captures, func(c httpblock.Capture) bool { return c.Secret && c.Name == name })
}

// httpNoShell are the attributes an http block can't take, and why.
var httpNoShell = []struct{ attr, why string }{
	{"env", "it has no shell"},
	{"share", "it has no shell"},
	{"tty", "it has no terminal"},
	{"background", "it can't run in the background yet"},
	{"out", "its output is the response"},
}

// checkHTTP checks d's http blocks: the attributes they take, and that
// the request reads, warning when it doesn't so the rest of the file
// still opens. Running the block reports the same error.
func checkHTTP(d *Document) error {
	for _, b := range d.Blocks {
		if !b.IsHTTP() {
			continue
		}
		for _, a := range httpNoShell {
			if b.Attrs.Has(a.attr) {
				return fmt.Errorf("line %d: block %s: an http block takes no %s attribute, since %s", lineNo(d.Src, b.Span.Start), b.ID, a.attr, a.why)
			}
		}
		if _, err := b.Request(); err != nil {
			d.Warnings = append(d.Warnings, fmt.Sprintf("line %d: block %s: %v", lineNo(d.Src, b.Span.Start), b.ID, err))
		}
	}
	return nil
}
