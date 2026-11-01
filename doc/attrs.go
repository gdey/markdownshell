package doc

import (
	"strings"
	"unicode"
)

// Attrs are a block's Pandoc-style attributes. The block id is stored
// under "#". A bare key such as skip is stored with an empty value.
type Attrs map[string]string

// Has reports whether key was set, with or without a value.
func (a Attrs) Has(key string) bool {
	_, ok := a[key]
	return ok
}

// List splits a comma-separated value. It returns nil when key is unset,
// and an empty, non-nil slice when key is set to nothing.
func (a Attrs) List(key string) []string {
	v, ok := a[key]
	if !ok {
		return nil
	}
	out := []string{}
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ParseInfo splits a fence info string such as
//
//	sh {#build env=build share=GREETING skip}
//
// into its language and attributes. The Pandoc form {.sh #build} is also
// accepted; its first class is the language.
func ParseInfo(info string) (lang string, attrs Attrs) {
	attrs = Attrs{}
	info = strings.TrimSpace(info)
	rest := info
	if !strings.HasPrefix(info, "{") {
		i := strings.IndexFunc(info, func(r rune) bool { return unicode.IsSpace(r) || r == '{' })
		if i < 0 {
			return info, attrs
		}
		lang, rest = info[:i], strings.TrimSpace(info[i:])
	}
	if !strings.HasPrefix(rest, "{") {
		return lang, attrs
	}
	rest = strings.TrimSuffix(strings.TrimPrefix(rest, "{"), "}")
	for _, tok := range splitAttrs(rest) {
		switch {
		case strings.HasPrefix(tok, "#"):
			attrs["#"] = tok[1:]
		case strings.HasPrefix(tok, "."):
			if lang == "" {
				lang = tok[1:]
			}
		default:
			k, v, _ := strings.Cut(tok, "=")
			attrs[k] = unquote(v)
		}
	}
	return lang, attrs
}

// splitAttrs splits on spaces outside double quotes.
func splitAttrs(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`)
	}
	return v
}

// SetFlag returns info, a fence info string, with the bare attribute
// key added (on) or taken out, keeping the rest of it as it is:
//
//	SetFlag("sh {#a}", "collapse", true)     // sh {#a collapse}
//	SetFlag("sh {#a collapse}", "collapse", false) // sh {#a}
//
// It returns info unchanged when key is already as asked, when key holds
// a value, such as a condition, which it leaves alone, or when info has
// text after the language that isn't attributes, as in sh foo.
func SetFlag(info, key string, on bool) string {
	info = strings.TrimSpace(info)
	open := 0 // where the { is
	if !strings.HasPrefix(info, "{") {
		i := strings.IndexFunc(info, func(r rune) bool { return unicode.IsSpace(r) || r == '{' })
		if i < 0 {
			if on {
				return strings.TrimSpace(info + " {" + key + "}")
			}
			return info
		}
		rest := strings.TrimLeftFunc(info[i:], unicode.IsSpace)
		if !strings.HasPrefix(rest, "{") {
			return info
		}
		open = len(info) - len(rest)
	}
	close := len(info)
	if strings.HasSuffix(info, "}") {
		close--
	}
	inner := info[open+1 : close]
	for _, t := range attrTokens(inner) {
		k, v, hasValue := strings.Cut(inner[t.Start:t.End], "=")
		if k != key {
			continue
		}
		if on || hasValue && v != "" {
			return info
		}
		// Take out the token and the space before it, or after it when
		// it comes first.
		from, to := t.Start, t.End
		for from > 0 && unicode.IsSpace(rune(inner[from-1])) {
			from--
		}
		if from == 0 {
			for to < len(inner) && unicode.IsSpace(rune(inner[to])) {
				to++
			}
		}
		inner = inner[:from] + inner[to:]
		if strings.TrimSpace(inner) == "" {
			return strings.TrimSpace(info[:open])
		}
		return info[:open] + "{" + inner + "}"
	}
	if !on {
		return info
	}
	inner = strings.TrimRightFunc(inner, unicode.IsSpace)
	if strings.TrimSpace(inner) != "" {
		inner += " "
	}
	return info[:open] + "{" + inner + key + "}"
}

// attrTokens are where the tokens of s are, split as splitAttrs splits
// them.
func attrTokens(s string) []Span {
	var out []Span
	start, quoted := -1, false
	for i, r := range s {
		switch {
		case unicode.IsSpace(r) && !quoted:
			if start >= 0 {
				out = append(out, Span{start, i})
				start = -1
			}
		default:
			if r == '"' {
				quoted = !quoted
			}
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		out = append(out, Span{start, len(s)})
	}
	return out
}
