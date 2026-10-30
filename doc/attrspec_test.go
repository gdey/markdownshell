package doc

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// TestBlockAttrsMatchSpec keeps BlockAttrs in step with the attribute
// table in docs/spec.md and the include and textbox attributes it
// describes.
func TestBlockAttrsMatchSpec(t *testing.T) {
	spec, err := os.ReadFile("../docs/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	table := regexp.MustCompile("(?s)Block attributes in v1:\n\n(.*?)\n\n").FindSubmatch(spec)
	if table == nil {
		t.Fatal("no block attribute table in docs/spec.md")
	}
	var want []string
	for _, m := range regexp.MustCompile("(?m)^\\| `([#a-z_-]+)").FindAllSubmatch(table[1], -1) {
		name := string(m[1])
		if name == "#id" {
			name = "#"
		}
		want = append(want, name)
	}
	want = append(want, IncludeAttr, "file", "from", "to", "number_on", "lines", "mode", "placeholder", "var")
	var got []string
	for _, s := range BlockAttrs {
		got = append(got, s.Name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("BlockAttrs = %q\nthe spec has %q", got, want)
	}
	for _, a := range notForIncludes {
		if s, ok := LookupAttr(a); !ok || s.Include {
			t.Errorf("%s should be a runnable block's attribute", a)
		}
	}
}

func TestAttrCheck(t *testing.T) {
	for _, c := range []struct {
		name, value string
		set, ok     bool
	}{
		{"skip", "", false, true},
		{"skip", "yes", true, false},
		{"confirm", "", false, true},
		{"confirm", "Really?", true, true},
		{"run", "{{$DEBUG}}", true, true},
		{"run", "{{$DEBUG", true, false},
		{"run", "", false, false},
		{"run", "once", true, true},
		{"show", "once", true, false},
		{"out", "markdown", true, true},
		{"out", "image:plots/a.png", true, true},
		{"out", "image:", true, false},
		{"out", "html", true, false},
		{"timeout", "30s", true, true},
		{"timeout", "30", true, false},
		{"frame", "3", true, true},
		{"frame", "x", true, false},
		{"#", "", true, false},
		{"number_on", "off", true, true},
	} {
		s, ok := LookupAttr(c.name)
		if !ok {
			t.Fatalf("no spec for %s", c.name)
		}
		if err := s.Check(c.value, c.set); (err == nil) != c.ok {
			t.Errorf("%s=%q (set %v): err = %v, want ok %v", c.name, c.value, c.set, err, c.ok)
		}
	}
}

func TestInfoEdit(t *testing.T) {
	in := SplitInfo(`sh {#build env=build x-tool="a b" confirm}`)
	if in.Lang != "sh" || len(in.Tokens) != 4 {
		t.Fatalf("SplitInfo = %+v", in)
	}
	in.Set("env", "deploy", false)
	in.Set("confirm", `Ship "it"?`, false)
	in.Set("timeout", "5m", false)
	in.Set("#", "ship", false)
	in.Del("x-nothing")
	if got, want := in.String(), `sh {#ship env=deploy x-tool="a b" confirm="Ship \"it\"?" timeout=5m}`; got != want {
		t.Errorf("edited = %s\nwant     %s", got, want)
	}
	lang, attrs := ParseInfo(in.String())
	if lang != "sh" || attrs["#"] != "ship" || attrs["confirm"] != `Ship "it"?` || attrs["x-tool"] != "a b" {
		t.Errorf("ParseInfo of the edit = %s %v", lang, attrs)
	}
	in.Del("confirm")
	in.Del("x-tool")
	in.Del("env")
	in.Del("timeout")
	in.Del("#")
	if got := in.String(); got != "sh" {
		t.Errorf("all removed = %q", got)
	}
	in = SplitInfo("sh")
	in.Set("skip", "", true)
	in.Set("#", "a", false)
	if got := in.String(); got != "sh {#a skip}" {
		t.Errorf("added = %q", got)
	}
}
