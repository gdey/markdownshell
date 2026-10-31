package termrec

import "testing"

func TestText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"hi\r\n", "hi\n"},
		{"a\r\nb", "a\nb\n"},
		{"\x1b[1;32mgreen\x1b[0m\r\n", "green\n"},
		{"50%\r100%\r\n", "100%\n"},
		{"hello\rhi", "hillo\n"},
		{"hello\r\x1b[Khi", "hi\n"},
		{"abc\b\bX\r\n", "aXc\n"},
		{"\x1b]0;title\x07text\r\n\r\n\r\n", "text\n"},
		{"tab\there  \r\n", "tab\there\n"},
		{"héllo\r\n", "héllo\n"},
	} {
		rec := Recording{Events: []Event{{Data: []byte(tc.in)}}}
		if got := rec.Text(); got != tc.want {
			t.Errorf("Text(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFullScreen(t *testing.T) {
	split := Recording{Events: []Event{{Data: []byte("\x1b[?10")}, {Data: []byte("49hvim")}}}
	if !split.FullScreen() {
		t.Error("a split alternate-screen switch isn't full screen")
	}
	if (Recording{Events: []Event{{Data: []byte("\x1b[1mls\x1b[0m")}}}).FullScreen() {
		t.Error("plain output is full screen")
	}
}
