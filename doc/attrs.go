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
