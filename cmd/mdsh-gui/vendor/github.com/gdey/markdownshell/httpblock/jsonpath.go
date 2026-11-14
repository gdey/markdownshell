package httpblock

import (
	"fmt"
	"strconv"
	"strings"
)

// path is a compiled JSONPath: the subset checks need, which is $
// followed by .name, ['name'], [n] (negative counts from the end),
// .* or [*], and ..name.
type path struct {
	steps []step
	// definite is true when the path names at most one value: it has
	// no wildcard or descent, so its match is a value, not a list.
	definite bool
}

type step struct {
	kind  int // stepName, stepIndex, stepAll, stepDescend
	name  string
	index int
}

const (
	stepName = iota
	stepIndex
	stepAll
	stepDescend
)

// CheckPath reports whether s is a JSONPath checks can read.
func CheckPath(s string) error {
	_, err := compilePath(s)
	return err
}

// compilePath reads a JSONPath.
func compilePath(s string) (*path, error) {
	bad := func(why string) (*path, error) {
		return nil, fmt.Errorf("jsonpath %s: %s", s, why)
	}
	if !strings.HasPrefix(s, "$") {
		return bad("want a path starting with $")
	}
	p := &path{definite: true}
	for i := 1; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], ".."):
			i += 2
			n := nameEnd(s, i)
			if n == i {
				return bad("want a name after ..")
			}
			p.steps = append(p.steps, step{kind: stepDescend, name: s[i:n]})
			p.definite = false
			i = n
		case s[i] == '.':
			i++
			if i < len(s) && s[i] == '*' {
				p.steps = append(p.steps, step{kind: stepAll})
				p.definite = false
				i++
				continue
			}
			n := nameEnd(s, i)
			if n == i {
				return bad("want a name after .")
			}
			p.steps = append(p.steps, step{kind: stepName, name: s[i:n]})
			i = n
		case s[i] == '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return bad("no closing ]")
			}
			in := strings.TrimSpace(s[i+1 : i+end])
			i += end + 1
			switch {
			case in == "*":
				p.steps = append(p.steps, step{kind: stepAll})
				p.definite = false
			case len(in) >= 2 && (in[0] == '\'' && in[len(in)-1] == '\'' || in[0] == '"' && in[len(in)-1] == '"'):
				p.steps = append(p.steps, step{kind: stepName, name: in[1 : len(in)-1]})
			default:
				n, err := strconv.Atoi(in)
				if err != nil {
					return bad(fmt.Sprintf("[%s]: want an index, a quoted name or *", in))
				}
				p.steps = append(p.steps, step{kind: stepIndex, index: n})
			}
		default:
			return bad(fmt.Sprintf("unexpected %q", s[i:]))
		}
	}
	return p, nil
}

// nameEnd is where a name starting at i in s ends.
func nameEnd(s string, i int) int {
	for i < len(s) && s[i] != '.' && s[i] != '[' {
		i++
	}
	return i
}

// eval returns the values p finds in v, a value from encoding/json.
func (p *path) eval(v any) []any {
	cur := []any{v}
	for _, st := range p.steps {
		var next []any
		for _, c := range cur {
			switch st.kind {
			case stepName:
				if m, ok := c.(map[string]any); ok {
					if x, ok := m[st.name]; ok {
						next = append(next, x)
					}
				}
			case stepIndex:
				if a, ok := c.([]any); ok {
					i := st.index
					if i < 0 {
						i += len(a)
					}
					if i >= 0 && i < len(a) {
						next = append(next, a[i])
					}
				}
			case stepAll:
				switch x := c.(type) {
				case []any:
					next = append(next, x...)
				case map[string]any:
					for _, k := range sortedKeys(x) {
						next = append(next, x[k])
					}
				}
			case stepDescend:
				next = append(next, descend(c, st.name)...)
			}
		}
		cur = next
	}
	return cur
}

// descend finds every value under key name, at any depth in v.
func descend(v any, name string) []any {
	var out []any
	switch x := v.(type) {
	case map[string]any:
		if y, ok := x[name]; ok {
			out = append(out, y)
		}
		for _, k := range sortedKeys(x) {
			out = append(out, descend(x[k], name)...)
		}
	case []any:
		for _, y := range x {
			out = append(out, descend(y, name)...)
		}
	}
	return out
}
