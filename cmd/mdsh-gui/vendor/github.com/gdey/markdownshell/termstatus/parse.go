package termstatus

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on the encoded side of a report, from the spec: a key, and the
// base64 of msg and title before decoding.
const (
	maxKey          = 16
	maxMsgEncoded   = 2732
	maxTitleEncoded = 256
)

// QueryReply is what a terminal that speaks the protocol answers Query
// with: the states and kinds it knows, which are the spec's.
const QueryReply = "\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth\x1b\\"

// IsQuery reports whether body, a report's body after "7501;", is the
// feature query, "?".
func IsQuery(body string) bool { return strings.TrimSpace(body) == "?" }

// Parse reads body, the body of an OSC 7501 report after "7501;", as the
// spec says a terminal should: pairs split on :, a pair with no = or a
// value with characters outside the grammar skipped, unknown keys
// ignored, the last of a repeated key kept. It returns false for a
// report to ignore whole: no state or an unknown one, a bad id, a title
// or msg that isn't base64, is too long, or decodes to something with a
// control character in it. Progress is NoProgress when there is none.
func Parse(body string) (Report, bool) {
	r := Report{Progress: NoProgress}
	if len(body) > MaxSequence {
		return Report{}, false
	}
	seen := map[string]string{}
	for _, pair := range strings.Split(body, ":") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !validKey(k) || !validValue(v) {
			continue
		}
		seen[k] = v
	}
	switch State(seen["state"]) {
	case Idle, Working, Done, Blocked, Error, Clear:
		r.State = State(seen["state"])
	default:
		return Report{}, false
	}
	if id, ok := seen["id"]; ok {
		if !validID(id) {
			return Report{}, false
		}
		r.ID = id
	}
	if r.State == Blocked {
		switch Kind(seen["kind"]) {
		case Permission, Question, Auth:
			r.Kind = Kind(seen["kind"])
		}
	}
	if r.State == Working || r.State == Blocked {
		if n, err := strconv.Atoi(seen["progress"]); err == nil && n >= 0 && n <= 100 && seen["progress"] == strconv.Itoa(n) {
			r.Progress = n
		}
	}
	if app := seen["app"]; validName(app) {
		r.App = app
	}
	var ok bool
	if r.Title, ok = decode(seen["title"], maxTitleEncoded, MaxTitle); !ok {
		return Report{}, false
	}
	if r.Msg, ok = decode(seen["msg"], maxMsgEncoded, MaxMsg); !ok {
		return Report{}, false
	}
	return r, true
}

// decode reads v, base64 with or without padding, as text of at most
// decoded bytes with no control characters.
func decode(v string, encoded, decoded int) (string, bool) {
	if v == "" {
		return "", true
	}
	if len(v) > encoded {
		return "", false
	}
	b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(v, "="))
	if err != nil || len(b) > decoded || !utf8.Valid(b) {
		return "", false
	}
	s := string(b)
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

func validKey(k string) bool {
	if k == "" || len(k) > maxKey {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 'a' || k[i] > 'z' {
			return false
		}
	}
	return true
}

func validValue(v string) bool {
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_', c == '.', c == ',', c == '+', c == '/', c == '=', c == '-':
		default:
			return false
		}
	}
	return true
}
