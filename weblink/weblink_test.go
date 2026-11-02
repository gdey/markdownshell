package weblink

import "testing"

func TestFind(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"see https://example.com/a?b=c#d now", []string{"https://example.com/a?b=c#d"}},
		{"Listening on http://localhost:8080.", []string{"http://localhost:8080"}},
		{"(docs at https://go.dev/doc)", []string{"https://go.dev/doc"}},
		{"https://en.wikipedia.org/wiki/Go_(game)", []string{"https://en.wikipedia.org/wiki/Go_(game)"}},
		{"mail mailto:me@example.com, or", []string{"mailto:me@example.com"}},
		{`<a href="https://x.io/">`, []string{"https://x.io/"}},
		{"\x1b[32mhttps://x.io\x1b[0m", []string{"https://x.io"}},
		{"HTTPS://X.IO", []string{"HTTPS://X.IO"}},
		{"file:///etc/passwd javascript:alert(1) ftp://x.io", nil},
		{"http:// http:///path mailto:", nil},
		{"nohttps://x.io", nil},
	} {
		var got []string
		for _, sp := range Find(tc.in) {
			got = append(got, tc.in[sp[0]:sp[1]])
		}
		if len(got) != len(tc.want) {
			t.Errorf("Find(%q) = %q, want %q", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Find(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	}
}

func TestOSC8(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"go to https://x.io.", "go to \x1b]8;;https://x.io\x1b\\https://x.io\x1b]8;;\x1b\\."},
		{"no links", "no links"},
		{"\x1b]8;;https://a.io\x1b\\a\x1b]8;;\x1b\\ https://b.io", "\x1b]8;;https://a.io\x1b\\a\x1b]8;;\x1b\\ https://b.io"},
	} {
		if got := OSC8(tc.in); got != tc.want {
			t.Errorf("OSC8(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
