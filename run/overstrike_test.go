package run

import (
	"bytes"
	"testing"
)

func TestOverstrike(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain\n", "plain\n"},
		{"N\bNA\bAM\bME\bE\n", "NAME\n"},   // man bold
		{"_\bE_\bx\n", "Ex\n"},             // man underline
		{"_\b‘x\b’", "‘’"},                 // multibyte runes
		{"\bstart\n\bnext", "start\nnext"}, // nothing to erase
		{"|\b/\b-\bdone\n", "done\n"},      // spinner
	} {
		if got := string(Overstrike([]byte(tc.in))); got != tc.want {
			t.Errorf("Overstrike(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOverstrikeWriterSplit(t *testing.T) {
	in := "B\bBU\bUI\bIL\bLT\bTI\bIN\bN(1)\n_\bE_\bx_\bt ‘é’\n"
	for i := 0; i <= len(in); i++ {
		var buf bytes.Buffer
		w := &overstrikeWriter{w: &buf}
		w.Write([]byte(in[:i]))
		w.Write([]byte(in[i:]))
		w.Flush()
		if got, want := buf.String(), "BUILTIN(1)\nExt ‘é’\n"; got != want {
			t.Errorf("split at %d: got %q, want %q", i, got, want)
		}
	}
}
