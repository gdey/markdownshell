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
