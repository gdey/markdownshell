package run

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
)

// A lexer can put a newline and a brace in one token; the brace's line
// counts the newlines before it.
func TestBraceEndMultilineToken(t *testing.T) {
	ts := []tok{
		{chroma.Token{Type: chroma.Punctuation, Value: "{"}, 1},
		{chroma.Token{Type: chroma.Text, Value: "a\n\n}"}, 1},
	}
	if got := braceEnd(ts, 0); got != 3 {
		t.Errorf("braceEnd = %d, want 3", got)
	}
}
