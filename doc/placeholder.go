package doc

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// Placeholder is a reference in the prose to a variable, as a block left
// it or from the caller's environment:
//
//	{{#setup/env(GREETING)}}
//	{{#setup/env(GREETING):"hello"}}
//	{{$USER}}
//	{{$USER:"j"}}
//	{{$?}}                 the most recent block's exit status
//	{{#build/?:"0"}}       block build's exit status
//
// The quoted part holds the last value seen, so the file reads well
// without running it. Write-back fills it in.
type Placeholder struct {
	Span Span
	Ref
	// Value is the stored value, unescaped. HasValue is false when the
	// placeholder has no :"..." part.
	Value    string
	HasValue bool
}

var placeholderRE = regexp.MustCompile(`\{\{` + refBody + `(?::"((?:[^"\\]|\\.)*)")?\}\}`)

// scanPlaceholders appends the placeholders in the prose line src[start:end],
// skipping inline code spans.
func scanPlaceholders(src []byte, start, end int, out []*Placeholder) []*Placeholder {
	line := src[start:end]
	code := codeSpans(line)
	for _, m := range placeholderRE.FindAllSubmatchIndex(line, -1) {
		if inSpans(m[0], code) {
			continue
		}
		p := &Placeholder{Span: Span{start + m[0], start + m[1]}}
		p.Ref = newRef(group(line, m, 1), group(line, m, 2), group(line, m, 3))
		if m[8] >= 0 {
			p.Value, p.HasValue = UnescapeValue(string(line[m[8]:m[9]])), true
		}
		out = append(out, p)
	}
	return out
}

// group returns submatch i of m in line, or "" if it didn't match.
func group(line []byte, m []int, i int) string {
	if m[2*i] < 0 {
		return ""
	}
	return string(line[m[2*i]:m[2*i+1]])
}

// codeSpans returns the [start, end) ranges of inline code in line.
func codeSpans(line []byte) [][2]int {
	var spans [][2]int
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		open := i
		i += n
		// Find a closing run of exactly n backticks.
		for j := i; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			m := 0
			for j+m < len(line) && line[j+m] == '`' {
				m++
			}
			if m == n {
				spans = append(spans, [2]int{open, j + m})
				i = j + m
				break
			}
			j += m
		}
	}
	return spans
}

func inSpans(pos int, spans [][2]int) bool {
	for _, s := range spans {
		if pos >= s[0] && pos < s[1] {
			return true
		}
	}
	return false
}

// EscapeValue quotes a value for the :"..." part of a placeholder.
func EscapeValue(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// UnescapeValue reverses EscapeValue.
func UnescapeValue(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b bytes.Buffer
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			i++
			if v[i] == 'n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(v[i])
			}
			continue
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// Source renders the placeholder as it is written in the file, holding value.
func (p *Placeholder) Source(value string) string {
	ref := p.Ref.String()
	return ref[:len(ref)-2] + `:"` + EscapeValue(value) + `"}}`
}

// TextRefs lists the placeholders in text, which unlike prose may be
// code: a tape's input, say.
func TextRefs(text string) []Ref {
	var refs []Ref
	for _, m := range placeholderRE.FindAllStringSubmatchIndex(text, -1) {
		line := []byte(text)
		refs = append(refs, newRef(group(line, m, 1), group(line, m, 2), group(line, m, 3)))
	}
	return refs
}

// Expand replaces each placeholder in text with its value from lookup,
// ignoring any stored :"..." value. A placeholder lookup doesn't know is
// an error.
func Expand(text string, lookup Lookup) (string, error) {
	var err error
	out := placeholderRE.ReplaceAllStringFunc(text, func(m string) string {
		refs := TextRefs(m)
		v, ok := lookup(refs[0])
		if !ok && err == nil {
			err = fmt.Errorf("%s has no value", refs[0])
		}
		return v
	})
	return out, err
}
