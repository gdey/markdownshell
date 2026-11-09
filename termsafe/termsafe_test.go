package termsafe

import "testing"

func TestVisible(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"echo hi\n", "echo hi\n"},
		{"a\tb\n", "a\tb\n"},
		{"touch /tmp/x #\r\x1b[2K\n", "touch /tmp/x #^M^[[2K\n"},
		{"\x1b[1A\x1b[2K", "^[[1A^[[2K"},
		{"bell\a", "bell^G"},
		{"nul\x00", "nul^@"},
		{"del\x7f", "del^?"},
		{"csi\u009b2K", "csi<U+009B>2K"},
		{"if x ‮;nimda‬", "if x <U+202E>;nimda<U+202C>"},
		{"⁦iso⁩", "<U+2066>iso<U+2069>"},
		{"héllo ✓", "héllo ✓"},
		{"rm\u200b -rf", "rm<U+200B> -rf"},
		{"\ufeffecho", "<U+FEFF>echo"},
		{"bad\x9b2K", "bad<0x9B>2K"},
	} {
		if got := Visible(tc.in); got != tc.want {
			t.Errorf("Visible(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got, want := HasControl(tc.in), tc.in != tc.want; got != want {
			t.Errorf("HasControl(%q) = %v, want %v", tc.in, got, want)
		}
	}
}

func TestStyled(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain\n", "plain\n"},
		{"a\tb\r\nc\n", "a\tb\r\nc\n"},
		{"\x1b[1;31mred\x1b[0m \x1b[38:2:1:2:3mtc\x1b[m", "\x1b[1;31mred\x1b[0m \x1b[38:2:1:2:3mtc\x1b[m"},
		{"\x1b]52;c;ZXZpbAo=\x07hello", "^[]52;c;ZXZpbAo=^Ghello"},
		{"\x1b]0;title\x1b\\", "^[]0;title^[\\"},
		{"\x1b]8;;https://evil\x1b\\ok\x1b]8;;\x1b\\", "^[]8;;https://evil^[\\ok^[]8;;^[\\"},
		{"secret\r\x1b[2Kshown", "secret^M^[[2Kshown"},
		{"\x1b[1A\x1b[31", "^[[1A^[[31"},
		{"\x1b[?1049h", "^[[?1049h"},
		{"csi\u009b2K", "csi<U+009B>2K"},
		{"bad\x9b2K", "bad<0x9B>2K"},
		{"👨‍👩 ‮rtl", "👨‍👩 ‮rtl"},
		{"bell\a", "bell^G"},
	} {
		if got := Styled(tc.in); got != tc.want {
			t.Errorf("Styled(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSGROnly(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"$ ", "$ "},
		{"\x1b[32muser\x1b[0m $ ", "\x1b[32muser\x1b[0m $ "},
		{"\x1b]133;A\x07\x1b[1m%\x1b[m ", "\x1b[1m%\x1b[m "},
		{"\x1b]0;title\x1b\\$ ", "$ "},
		{"\x1b]52;c;ZXZpbAo=\x07$ ", "$ "},
		{"\x1b[2J\x1b[H$ ", "$ "},
		{"a\x1b=b\x07c\r", "abc"},
		{"\x1b]8;;https://x", ""},
		{"x\x1b", "x"},
	} {
		if got := SGROnly(tc.in); got != tc.want {
			t.Errorf("SGROnly(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
