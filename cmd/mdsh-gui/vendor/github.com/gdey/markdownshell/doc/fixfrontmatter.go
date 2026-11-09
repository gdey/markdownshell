package doc

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
)

// plainValue matches a key: value line whose value is plain text, not
// quoted, a block scalar, a flow list or map, an anchor, an alias or a
// tag.
var plainValue = regexp.MustCompile(`^([ \t]*)([A-Za-z0-9_.-]+):[ \t]+([^"'|>\[\]{}&*!#%@` + "`" + `\s].*?)[ \t]*(\r?)$`)

// frontMatterFix is the fix for front matter that doesn't parse, when
// mdsh can tell what was meant; its Warning is FrontMatterErr's text.
// For now that is a plain value with a colon and a space in it, as in
//
//	title: Setup: the first steps
//
// which YAML reads as a key in a key, and which quotes fix. Every such
// value is quoted, and the fix is offered only if the front matter then
// parses. It reports false when there is no fix, or nothing to fix.
func (d *Document) frontMatterFix() (Fix, bool) {
	if d.FrontMatterErr == nil || d.FrontMatter.End <= d.FrontMatter.Start {
		return Fix{}, false
	}
	fm := d.Src[d.FrontMatter.Start:d.FrontMatter.End]
	lines := bytes.SplitAfter(fm, []byte("\n"))
	var keys []string
	// scalar is the indent of the key whose value is a | or > block
	// scalar the lines are in, or -1: text in one is left alone.
	scalar := -1
	// The first and last lines are the --- lines.
	for i := 1; i < len(lines)-1; i++ {
		line := bytes.TrimRight(lines[i], "\r\n")
		indent := len(line) - len(bytes.TrimLeft(line, " \t"))
		if scalar >= 0 && (indent > scalar || len(bytes.TrimSpace(line)) == 0) {
			continue
		}
		scalar = -1
		if k, v, ok := bytes.Cut(bytes.TrimSpace(line), []byte(": ")); ok && len(k) > 0 && (bytes.HasPrefix(bytes.TrimSpace(v), []byte("|")) || bytes.HasPrefix(bytes.TrimSpace(v), []byte(">"))) {
			scalar = indent
			continue
		}
		m := plainValue.FindSubmatch(bytes.TrimSuffix(lines[i], []byte("\n")))
		if m == nil {
			continue
		}
		v := string(m[3])
		// A # after a space starts a comment, which quotes would pull
		// into the value.
		if !strings.Contains(v, ": ") && !strings.HasSuffix(v, ":") || strings.Contains(v, " #") {
			continue
		}
		nl := ""
		if bytes.HasSuffix(lines[i], []byte("\n")) {
			nl = "\n"
		}
		lines[i] = []byte(string(m[1]) + string(m[2]) + ": " + strconv.Quote(v) + string(m[4]) + nl)
		keys = append(keys, string(m[2]))
	}
	if len(keys) == 0 {
		return Fix{}, false
	}
	src := append(append(append([]byte(nil), d.Src[:d.FrontMatter.Start]...), bytes.Join(lines, nil)...), d.Src[d.FrontMatter.End:]...)
	nd, err := Parse(src)
	if err != nil || nd.FrontMatterErr != nil {
		return Fix{}, false
	}
	what := "the " + keys[0] + " value"
	if len(keys) > 1 {
		what = "the " + strings.Join(keys[:len(keys)-1], ", ") + " and " + keys[len(keys)-1] + " values"
	}
	return Fix{
		Warning: d.FrontMatterErr.Error(),
		Label:   "Add quotes",
		Detail:  "A colon followed by a space in " + what + " makes YAML read it as another key. Put quotes around " + what + ", so the front matter reads as meant and blocks can run.",
		Src:     src,
	}, true
}
