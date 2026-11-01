package run

import (
	"slices"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  a  b ", []string{"a", "b"}},
		{`'a b' "c d" e\ f`, []string{"a b", "c d", "e f"}},
		{`'it''s' "say \"hi\"" ''`, []string{"its", `say "hi"`, ""}},
		{`'\n' x\$y`, []string{`\n`, "x$y"}},
	} {
		got, err := SplitArgs(c.in)
		if err != nil {
			t.Errorf("SplitArgs(%q): %v", c.in, err)
			continue
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("SplitArgs(%q) = %q, want %q", c.in, got, c.want)
		}
		back, err := SplitArgs(JoinArgs(got))
		if err != nil || !slices.Equal(back, got) {
			t.Errorf("SplitArgs(JoinArgs(%q)) = %q, %v", got, back, err)
		}
	}
	for _, bad := range []string{`'a`, `"a`, `a\`} {
		if _, err := SplitArgs(bad); err == nil {
			t.Errorf("SplitArgs(%q) should fail", bad)
		}
	}
	if got := JoinArgs([]string{"plain", "two words", "it's"}); got != `plain 'two words' 'it'\''s'` {
		t.Errorf("JoinArgs = %s", got)
	}
}
