package termstatus

import (
	"bytes"
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

// A program on a terminal puts text on the clipboard with OSC 52:
// ESC ] 52 ; selection ; base64 ST. The Scanner finds these too, when
// its Clipboard is set, for a terminal mdsh draws itself, which would
// otherwise drop them.

// MaxCopy is the most text, in bytes, a program may copy at once.
const MaxCopy = 1 << 20

var copyPrefix = []byte("\x1b]52;")

// maxCopySequence is the longest OSC 52 the Scanner collects: a
// selection and MaxCopy bytes in base64.
var maxCopySequence = len(copyPrefix) + 16 + base64.StdEncoding.EncodedLen(MaxCopy)

// ParseCopy is the text an OSC 52's body, what follows "52;", puts on
// the clipboard. It is false for a read ("?"), which is never answered,
// for an empty payload, which asks some terminals to clear the
// clipboard, for a selection that isn't c, p, q, s or 0-7, for bad
// base64, for text that isn't UTF-8 and for more than MaxCopy.
func ParseCopy(body string) (string, bool) {
	sel, b64, ok := strings.Cut(body, ";")
	if !ok || strings.Trim(sel, "cpqs01234567") != "" || b64 == "" || b64 == "?" {
		return "", false
	}
	if base64.StdEncoding.DecodedLen(len(b64)) > MaxCopy+2 {
		return "", false
	}
	text, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(text) > MaxCopy || !utf8.Valid(text) {
		return "", false
	}
	return string(text), true
}

// isCopy says osc, a sequence being collected, is an OSC 52.
func isCopy(osc []byte) bool { return bytes.HasPrefix(osc, copyPrefix) }
