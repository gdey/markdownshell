package doc

import (
	"strings"
	"testing"
)

func TestFrontMatterFix(t *testing.T) {
	for _, tt := range []struct {
		name, fm, want string
	}{
		{"title", "title: Setup: first steps\n", "title: \"Setup: first steps\"\n"},
		{"two", "title: a: b\ndescription: c: \"d\"\n", "title: \"a: b\"\ndescription: \"c: \\\"d\\\"\"\n"},
		{"nested", "mdsh:\n  shell: x\nsummary: Why: this\n", "mdsh:\n  shell: x\nsummary: \"Why: this\"\n"},
		{"ends in colon", "title: Steps: \nx: y\n", "title: \"Steps:\"\nx: y\n"},
		{"crlf", "title: a: b\r\n", "title: \"a: b\"\r\n"},
	} {
		src := "---\n" + tt.fm + "---\n\n```sh\necho hi\n```\n"
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if tt.name == "ends in colon" {
			// It parses already: nothing to fix.
			if d.FrontMatterErr == nil {
				if _, ok := d.frontMatterFix(); ok {
					t.Errorf("%s: a fix for front matter that parses", tt.name)
				}
				continue
			}
		}
		f, ok := d.frontMatterFix()
		if !ok {
			t.Errorf("%s: no fix for %v", tt.name, d.FrontMatterErr)
			continue
		}
		if want := "---\n" + tt.want + "---\n\n```sh\necho hi\n```\n"; string(f.Src) != want {
			t.Errorf("%s: fixed to %q, want %q", tt.name, f.Src, want)
		}
		if f.Warning != d.FrontMatterErr.Error() || f.Label == "" || !strings.Contains(f.Detail, "quotes") {
			t.Errorf("%s: fix = %+v", tt.name, f)
		}
		nd, _ := Parse(f.Src)
		if nd.FrontMatterErr != nil {
			t.Errorf("%s: still %v", tt.name, nd.FrontMatterErr)
		}
		// Fixes lists it.
		if fs := d.Fixes(); len(fs) == 0 || fs[0].Warning != f.Warning {
			t.Errorf("%s: Fixes() = %+v", tt.name, fs)
		}
	}
}

func TestFrontMatterNoFix(t *testing.T) {
	for _, fm := range []string{
		// Not about a colon.
		"mdsh:\n  on_error: maybe\n",
		"title: [unclosed\n",
		// A comment after the colon would end up in the value.
		"title: a: b # note\n",
		// Text in a block scalar is left alone, and the error is
		// elsewhere.
		"description: |\n  a: b\nmdsh: [\n",
	} {
		d, err := Parse([]byte("---\n" + fm + "---\n"))
		if err != nil {
			t.Fatal(err)
		}
		if d.FrontMatterErr == nil {
			t.Errorf("%q parses", fm)
			continue
		}
		if f, ok := d.frontMatterFix(); ok {
			t.Errorf("%q: fix %q", fm, f.Src)
		}
	}
}
