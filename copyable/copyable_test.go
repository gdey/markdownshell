package copyable

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}

func TestSplit(t *testing.T) {
	const start, end = "\x1b]1337;CopyToClipboard=\a", "\x1b]1337;EndCopy\a"
	tests := []struct {
		name string
		in   string
		want []Part
	}{
		{"plain", "hi\n\x1b[31mred\x1b[m\n", []Part{{Kind: Plain, Text: "hi\n\x1b[31mred\x1b[m\n"}}},
		{"region", "token: " + start + "abc123" + end + "\n", []Part{
			{Kind: Plain, Text: "token: "}, {Kind: Open, N: 1}, {Kind: Plain, Text: "abc123"}, {Kind: Close, N: 1}, {Kind: Plain, Text: "\n"},
		}},
		{"st ends it", "\x1b]1337;CopyToClipboard=general\x1b\\x\x1b]1337;EndCopy\x1b\\", []Part{
			{Kind: Open, N: 1}, {Kind: Plain, Text: "x"}, {Kind: Close, N: 1},
		}},
		{"unclosed", start + "abc", []Part{{Kind: Open, N: 1}, {Kind: Plain, Text: "abc"}, {Kind: Close, N: 1}}},
		{"stray end", "a" + end + "b", []Part{{Kind: Plain, Text: "ab"}}},
		{"reopened", start + "a" + start + "b" + end, []Part{
			{Kind: Open, N: 1}, {Kind: Plain, Text: "a"}, {Kind: Close, N: 1}, {Kind: Open, N: 2}, {Kind: Plain, Text: "b"}, {Kind: Close, N: 2},
		}},
		{"osc 52", "set\n" + osc52("secret") + "done\n", []Part{
			{Kind: Plain, Text: "set\n"}, {Kind: Hidden, Text: "secret", N: 1}, {Kind: Plain, Text: "done\n"},
		}},
		{"osc 52 unpadded", "\x1b]52;c;aGk\a", []Part{{Kind: Hidden, Text: "hi", N: 1}}},
		{"osc 52 query", "\x1b]52;c;?\a", nil},
		{"osc 52 clear", "\x1b]52;c;\a", nil},
		{"osc 52 not base64", "\x1b]52;c;!!\a", nil},
		{"osc 52 not utf-8", "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe}) + "\a", nil},
		{"other osc kept", "\x1b]0;title\aok", []Part{{Kind: Plain, Text: "\x1b]0;title\aok"}}},
		{"unterminated", "a\x1b]1337;CopyToClipboard=", []Part{{Kind: Plain, Text: "a\x1b]1337;CopyToClipboard="}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Split(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Split(%q)\n got %#v\nwant %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTexts(t *testing.T) {
	in := "a \x1b]1337;CopyToClipboard=\a\x1b[1mone\x1b[m\r\ntwo\x1b]1337;EndCopy\a b " + osc52("three") + "\n"
	want := []string{"one\ntwo", "three"}
	if got := Texts(in); !reflect.DeepEqual(got, want) {
		t.Errorf("Texts = %q, want %q", got, want)
	}
	if Texts("nothing") != nil {
		t.Error("Texts of plain output isn't nil")
	}
	if !Has(in) || Has("plain \x1b]0;x\a") {
		t.Error("Has")
	}
}
