package termstatus

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseCopy(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for _, c := range []struct {
		body, want string
		ok         bool
	}{
		{"c;" + b64("hi\n"), "hi\n", true},
		{"p;" + b64("x"), "x", true},
		{"s0;" + b64("x"), "x", true},
		{";" + b64("x"), "x", true},
		{"c;" + b64("héllo ✓"), "héllo ✓", true},
		{"c;?", "", false},                  // a read is never answered
		{"c;", "", false},                   // clearing leaves the clipboard be
		{"no semicolon", "", false},         //
		{"z;" + b64("x"), "", false},        // not a selection
		{"c;not base64!", "", false},        //
		{"c;" + b64("\xff\xfe"), "", false}, // not UTF-8
		{"c;" + b64(strings.Repeat("x", MaxCopy+1)), "", false},
		{"c;" + b64(strings.Repeat("x", MaxCopy)), strings.Repeat("x", MaxCopy), true},
	} {
		got, ok := ParseCopy(c.body)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseCopy(%.30q) = %.30q, %v; want %.30q, %v", c.body, got, ok, c.want, c.ok)
		}
	}
}

func TestScannerCopy(t *testing.T) {
	var out bytes.Buffer
	var got []string
	sc := NewScanner(&out, nil, nil)
	sc.Clipboard = func(s string) { got = append(got, s) }
	big := strings.Repeat("y", 100000) // past MaxSequence, so collected only as an OSC 52
	in := "a\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("one")) + "\x07b" +
		"\x1b]52;c;?\x07" + // a read: nothing
		"\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(big)) + "\x1b\\c"
	for i := 0; i < len(in); i += 7 {
		sc.Write([]byte(in[i:min(i+7, len(in))]))
	}
	if out.String() != in {
		t.Error("stream changed")
	}
	if len(got) != 2 || got[0] != "one" || got[1] != big {
		t.Errorf("copied %d: %.20q", len(got), got)
	}
	// Without Clipboard, an OSC 52 is passed on and not collected past
	// MaxSequence.
	got = nil
	sc.Clipboard = nil
	sc.Write([]byte("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("two")) + "\x07"))
	if len(got) != 0 || sc.osc != nil {
		t.Errorf("without Clipboard: %q, collecting %d", got, len(sc.osc))
	}
}
