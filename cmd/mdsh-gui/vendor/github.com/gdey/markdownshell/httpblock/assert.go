package httpblock

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// query reads q from the response. found is false when there is
// nothing there: no such header, no match.
func (resp *Response) query(q Query) (v any, found bool, err error) {
	switch q.What {
	case "status":
		v, found = float64(resp.Code), true
	case "duration":
		v, found = float64(resp.Duration.Milliseconds()), true
	case "header":
		vals := resp.Header.Values(q.Arg)
		switch len(vals) {
		case 0:
		case 1:
			v, found = vals[0], true
		default:
			list := make([]any, len(vals))
			for i, s := range vals {
				list[i] = s
			}
			v, found = list, true
		}
	case "body":
		v, found = string(resp.Body), true
	case "regex":
		re, err := regexp.Compile(q.Arg)
		if err != nil {
			return nil, false, err
		}
		m := re.FindSubmatch(resp.Body)
		switch {
		case m == nil:
		case len(m) > 1:
			v, found = string(m[1]), true
		default:
			v, found = string(m[0]), true
		}
	case "jsonpath":
		p, err := compilePath(q.Arg)
		if err != nil {
			return nil, false, err
		}
		var doc any
		if err := json.Unmarshal(resp.Body, &doc); err != nil {
			return nil, false, fmt.Errorf("the body isn't JSON")
		}
		matches := p.eval(doc)
		switch {
		case p.definite && len(matches) == 1:
			v, found = matches[0], true
		case !p.definite && len(matches) > 0:
			v, found = matches, true
		}
	default:
		return nil, false, fmt.Errorf("unknown query %q", q.What)
	}
	if q.Count && found {
		switch x := v.(type) {
		case []any:
			v = float64(len(x))
		case map[string]any:
			v = float64(len(x))
		default:
			return nil, false, fmt.Errorf("count: %s isn't a collection", show(v))
		}
	}
	return v, found, nil
}

// assert runs one check, and says what the response had when it
// fails.
func (resp *Response) assert(a Assert) (ok bool, got string) {
	v, found, err := resp.query(a.Query)
	if err != nil {
		return false, err.Error()
	}
	if a.Op == "exists" {
		return found != a.Not, gotFor(v, found)
	}
	if !found {
		return false, "nothing"
	}
	want := literal(a.Value)
	var res bool
	switch a.Op {
	case "==":
		res = equal(v, want)
	case "!=":
		res = !equal(v, want)
	case "<", "<=", ">", ">=":
		x, ok1 := number(v)
		y, ok2 := number(want)
		if !ok1 || !ok2 {
			return false, show(v)
		}
		res = map[string]bool{"<": x < y, "<=": x <= y, ">": x > y, ">=": x >= y}[a.Op]
	case "contains":
		switch x := v.(type) {
		case string:
			res = strings.Contains(x, text(want))
		case []any:
			for _, e := range x {
				if equal(e, want) {
					res = true
					break
				}
			}
		}
	case "startsWith":
		s, isStr := v.(string)
		res = isStr && strings.HasPrefix(s, text(want))
	case "endsWith":
		s, isStr := v.(string)
		res = isStr && strings.HasSuffix(s, text(want))
	case "matches":
		s, isStr := v.(string)
		re, err := regexp.Compile(text(want))
		res = isStr && err == nil && re.MatchString(s)
	case "isEmpty":
		switch x := v.(type) {
		case string:
			res = x == ""
		case []any:
			res = len(x) == 0
		case map[string]any:
			res = len(x) == 0
		}
	case "isNumber":
		_, res = v.(float64)
	case "isString":
		_, res = v.(string)
	case "isBoolean":
		_, res = v.(bool)
	case "isCollection":
		switch v.(type) {
		case []any, map[string]any:
			res = true
		}
	}
	return res != a.Not, show(v)
}

func gotFor(v any, found bool) string {
	if !found {
		return "nothing"
	}
	return show(v)
}

// literal reads a check's value: a JSON literal, else the bare word.
func literal(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}

// equal compares a value from the response with a check's value. A
// string compares with a number or a boolean by its text, so a header
// equals 3.
func equal(v, want any) bool {
	if reflect.DeepEqual(v, want) {
		return true
	}
	_, s1 := v.(string)
	_, s2 := want.(string)
	if s1 != s2 {
		return text(v) == text(want)
	}
	return false
}

// number reads v as a number: a JSON number, or a string that holds one.
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

// text is v as a capture stores it: a string as it is, anything else as
// JSON.
func text(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// show is v as a failed check reports it: as JSON, cut short.
func show(v any) string {
	b, err := json.Marshal(v)
	s := string(b)
	if err != nil {
		s = fmt.Sprint(v)
	}
	const max = 120
	if len(s) > max {
		cut := max
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
