package doc

import (
	_ "embed"
	"reflect"
	"strings"
	"testing"
)

func TestParseInfo(t *testing.T) {
	tests := []struct {
		info  string
		lang  string
		attrs Attrs
	}{
		{"sh", "sh", Attrs{}},
		{"", "", Attrs{}},
		{"sh {#build env=build share=A,B skip}", "sh", Attrs{"#": "build", "env": "build", "share": "A,B", "skip": ""}},
		{"sh{#x}", "sh", Attrs{"#": "x"}},
		{"{.sh #x}", "sh", Attrs{"#": "x"}},
		{`sh {#x title="a b \"c\""}`, "sh", Attrs{"#": "x", "title": `a b "c"`}},
	}
	for _, tt := range tests {
		lang, attrs := ParseInfo(tt.info)
		if lang != tt.lang || !reflect.DeepEqual(attrs, tt.attrs) {
			t.Errorf("ParseInfo(%q) = %q, %v; want %q, %v", tt.info, lang, attrs, tt.lang, tt.attrs)
		}
	}
}

func TestAttrsList(t *testing.T) {
	a := Attrs{"share": "A, B,", "none": ""}
	if got := a.List("share"); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("List(share) = %v", got)
	}
	if got := a.List("none"); got == nil || len(got) != 0 {
		t.Errorf("List(none) = %#v, want empty non-nil", got)
	}
	if got := a.List("missing"); got != nil {
		t.Errorf("List(missing) = %#v, want nil", got)
	}
}

//go:embed testdata/sample.md
var sample string

func TestParse(t *testing.T) {
	d, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if d.Shebang != "#!/usr/bin/env mdsh" {
		t.Errorf("Shebang = %q", d.Shebang)
	}
	if d.Options.Shell != "/bin/bash" || d.Options.OnError != "stop" || !reflect.DeepEqual(d.Options.Env.Share, []string{"PATH"}) {
		t.Errorf("Options = %+v", d.Options)
	}
	if len(d.Blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(d.Blocks))
	}

	a := d.Blocks[0]
	if a.ID != "a" || a.Lang != "sh" || a.Body != "echo a\n" {
		t.Errorf("block a = %+v", a)
	}
	if got := string(d.Src[a.Output.Start:a.Output.End]); got != "\n```output {for=a exit=0}\na\n```\n" {
		t.Errorf("a output = %q", got)
	}

	b := d.Blocks[1]
	if b.ID != "block-2" || b.Body != "echo '```'\n" {
		t.Errorf("block 2 = %+v", b)
	}
	if got := string(d.Src[b.Output.Start:b.Output.End]); got != "\n<!-- mdsh:output for=\"block-2\" -->\n| x |\n<!-- /mdsh:output -->\n" {
		t.Errorf("block-2 output = %q", got)
	}

	g := d.Blocks[2]
	if g.Lang != "go" || g.Body != "x := 1\n" {
		t.Errorf("indented block = %+v", g)
	}
	if g.Output.Start != g.Output.End {
		t.Errorf("go block should have no output")
	}
}

func TestParseDuplicateID(t *testing.T) {
	_, err := Parse([]byte("```sh {#a}\n```\n```sh {#a}\n```\n"))
	if err == nil {
		t.Fatal("want error for duplicate id")
	}
}

func TestParseUnclosed(t *testing.T) {
	d, err := Parse([]byte("```sh\necho hi\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 1 || d.Blocks[0].Body != "echo hi\n" || d.Blocks[0].Span.End != len(d.Src) {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
}

func TestParseNoFrontMatter(t *testing.T) {
	// A thematic break with no closing line is not front matter.
	d, err := Parse([]byte("---\n# hi\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.FrontMatter.End != 0 {
		t.Errorf("FrontMatter = %+v", d.FrontMatter)
	}
}

func TestParseBadOnError(t *testing.T) {
	if _, err := Parse([]byte("---\nmdsh:\n  on_error: maybe\n---\n")); err == nil {
		t.Fatal("want error")
	}
}

func TestPlaceholders(t *testing.T) {
	src := "```sh {#setup}\nexport G=hi\n```\n\n" +
		"Say {{#setup/env(G)}} and {{#setup/env(G):\"old \\\"q\\\"\"}}, not `{{#setup/env(G)}}`.\n\n" +
		"```sh\necho {{#setup/env(G)}}\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Placeholders) != 2 {
		t.Fatalf("got %d placeholders, want 2: %+v", len(d.Placeholders), d.Placeholders)
	}
	p0, p1 := d.Placeholders[0], d.Placeholders[1]
	if p0.BlockID != "setup" || p0.Var != "G" || p0.HasValue {
		t.Errorf("p0 = %+v", p0)
	}
	if !p1.HasValue || p1.Value != `old "q"` {
		t.Errorf("p1 = %+v", p1)
	}
	if got := p1.Source("a\"b\\c\nd"); got != `{{#setup/env(G):"a\"b\\c\nd"}}` {
		t.Errorf("Source = %s", got)
	}
}

func TestPlaceholderUnknownBlock(t *testing.T) {
	if _, err := Parse([]byte("{{#nope/env(X)}}\n")); err == nil {
		t.Fatal("want error")
	}
}

func TestPlaceholdersOff(t *testing.T) {
	d, err := Parse([]byte("---\nmdsh:\n  interpolate: false\n---\n{{#nope/env(X)}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Placeholders) != 0 {
		t.Errorf("placeholders = %v", d.Placeholders)
	}
}

func TestParseCond(t *testing.T) {
	look := func(r Ref) (string, bool) {
		return map[Ref]string{{Var: "ON"}: "1", {Var: "?"}: "0", {Var: "err?"}: "0", {BlockID: "d", Var: "?"}: "2", {BlockID: "d", Var: "err?"}: "2", {Var: "ran?"}: "true", {BlockID: "d", Var: "ran?"}: "true", {Var: "OFF"}: "no", {BlockID: "d", Var: "OS"}: "Linux"}[r], true
	}
	for s, want := range map[string]bool{
		"": true, "true": true, "false": false, "0": false,
		"{{$ON}}": true, "{{$OFF}}": false, "{{$MISSING}}": false, "!{{$OFF}}": true,
		"{{#d/env(OS)}}==Linux": true, "{{#d/env(OS)}}==Darwin": false, "{{#d/env(OS)}}!=Darwin": true,
		"{{$?}}==0": true, "{{$?}}": false, "{{#d/?}}!=0": true,
		"{{$err?}}": false, "{{#d/err?}}": true, "{{$ran?}}": true, "!{{#d/ran?}}": false,
	} {
		c, err := ParseCond(s)
		if err != nil {
			t.Errorf("ParseCond(%q): %v", s, err)
			continue
		}
		if got := c.Eval(look); got != want {
			t.Errorf("%q = %v, want %v", s, got, want)
		}
	}
	for _, bad := range []string{"DEBUG", "{{DEBUG}}", "!{{$X}}==1", "maybe", "{{#d/??}}", "{{$??}}", "{{$ran}}?", "{{#d/env(ran?)}}"} {
		if _, err := ParseCond(bad); err == nil {
			t.Errorf("ParseCond(%q) should fail", bad)
		}
	}
}

func TestParseSections(t *testing.T) {
	src := "a\n<!-- mdsh:if {{$X}} -->\nb\n<!-- mdsh:if !{{$Y}} -->\nc\n<!-- mdsh:end -->\n<!-- mdsh:end -->\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Sections) != 2 || d.Sections[0].End.Start <= d.Sections[1].End.Start {
		t.Fatalf("sections = %+v", d.Sections)
	}
	for _, bad := range []string{
		"<!-- mdsh:if {{$X}} -->\n",
		"<!-- mdsh:end -->\n",
		"<!-- mdsh:if X -->\n<!-- mdsh:end -->\n",
		"```sh {#a show=nope}\n```\n",
		"```sh {#a run={{#zz/env(X)}}}\n```\n",
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestCallerPlaceholder(t *testing.T) {
	d, err := Parse([]byte("hi {{$USER}} and {{$HOME:\"/h\"}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Placeholders) != 2 || d.Placeholders[0].Var != "USER" || d.Placeholders[0].BlockID != "" || d.Placeholders[1].Value != "/h" {
		t.Fatalf("placeholders = %+v %+v", d.Placeholders[0], d.Placeholders[1])
	}
	if got := d.Placeholders[1].Source("/x"); got != `{{$HOME:"/x"}}` {
		t.Errorf("Source = %s", got)
	}
}

func TestStatusPlaceholder(t *testing.T) {
	d, err := Parse([]byte("```sh {#a}\n```\n{{$?}} {{#a/?:\"3\"}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Placeholders) != 2 {
		t.Fatalf("placeholders = %+v", d.Placeholders)
	}
	p0, p1 := d.Placeholders[0], d.Placeholders[1]
	if p0.Ref != (Ref{Var: Status}) || p1.Ref != (Ref{BlockID: "a", Var: Status}) || p1.Value != "3" {
		t.Errorf("got %+v, %+v", p0, p1)
	}
	if got := p1.Source("0"); got != `{{#a/?:"0"}}` {
		t.Errorf("Source = %s", got)
	}
	if _, err := Parse([]byte("{{#zz/?}}\n")); err == nil {
		t.Error("want error for unknown block")
	}
}

func TestParseNested(t *testing.T) {
	tests := []struct {
		name, src      string
		body, prefix   string
		lead, spanText string
	}{
		{"bullet", "- ```sh\n  echo a\n  ```\n", "echo a\n", "  ", "- ", "- ```sh\n  echo a\n  ```\n"},
		{"ordered later line", "1. Run:\n\n   ```sh\n   echo a\n    b\n   ```\n", "echo a\n b\n", "   ", "", "   ```sh\n   echo a\n    b\n   ```\n"},
		{"nested list", "- a\n  - b\n\n    ```sh\n    echo b\n    ```\n", "echo b\n", "    ", "", "    ```sh\n    echo b\n    ```\n"},
		{"quote", "> ```sh\n> echo a\n>\n> ```\n", "echo a\n\n", "> ", "", "> ```sh\n> echo a\n>\n> ```\n"},
		{"quote no space", ">```sh\n>echo a\n>```\n", "echo a\n", ">", "", ">```sh\n>echo a\n>```\n"},
		{"quote list", "> 1. ```sh\n>    echo a\n>    ```\n", "echo a\n", "> " + "   ", "> 1. ", "> 1. ```sh\n>    echo a\n>    ```\n"},
		{"list quote", "1. x\n\n   > ```sh\n   > echo a\n   > ```\n", "echo a\n", "   > ", "", "   > ```sh\n   > echo a\n   > ```\n"},
		{"quote ends fence", "> ```sh\n> echo a\nafter\n", "echo a\n", "> ", "", "> ```sh\n> echo a\n"},
		{"item ends fence", "- ```sh\n  echo a\nafter\n", "echo a\n", "  ", "- ", "- ```sh\n  echo a\n"},
	}
	for _, tt := range tests {
		d, err := Parse([]byte(tt.src))
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if len(d.Blocks) != 1 {
			t.Errorf("%s: got %d blocks, want 1", tt.name, len(d.Blocks))
			continue
		}
		b := d.Blocks[0]
		if b.Lang != "sh" || b.Body != tt.body || b.Prefix != tt.prefix || b.Lead != tt.lead {
			t.Errorf("%s: got lang %q body %q prefix %q lead %q", tt.name, b.Lang, b.Body, b.Prefix, b.Lead)
		}
		if got := string(d.Src[b.Span.Start:b.Span.End]); got != tt.spanText {
			t.Errorf("%s: span = %q, want %q", tt.name, got, tt.spanText)
		}
	}
}

func TestParseNotNested(t *testing.T) {
	for _, src := range []string{
		// Indented code inside a list item.
		"- a\n\n      ```sh\n      echo a\n      ```\n",
		// The list item ended, so this is indented code.
		"- a\n\nb\n\n    ```sh\n    x\n    ```\n",
		// Not a list marker.
		"1.5 ```sh\n",
	} {
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Blocks) != 0 {
			t.Errorf("Parse(%q) found %+v", src, d.Blocks[0])
		}
	}
}

func TestParseNestedOutput(t *testing.T) {
	src := "- ```sh {#a}\n  echo a\n  ```\n\n  ```output {for=a exit=0}\n  a\n  ```\n" +
		"> ```sh {#b}\n> echo b\n> ```\n>\n> <!-- mdsh:output for=\"b\" -->\n> | b |\n> <!-- /mdsh:output -->\n" +
		"\n```sh {#c}\n```\n\n  ```output {for=c exit=0}\n  ```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(d.Blocks))
	}
	want := []string{
		"\n  ```output {for=a exit=0}\n  a\n  ```\n",
		">\n> <!-- mdsh:output for=\"b\" -->\n> | b |\n> <!-- /mdsh:output -->\n",
		"\n  ```output {for=c exit=0}\n  ```\n", // indent 2 is still the top level
	}
	for i, b := range d.Blocks {
		if got := string(d.Src[b.Output.Start:b.Output.End]); got != want[i] {
			t.Errorf("block %s output = %q, want %q", b.ID, got, want[i])
		}
	}
}

func TestTapes(t *testing.T) {
	src := "```sh {#a tty}\nx\n```\n\n```tape {for=a}\nEnter\n```\n\n```sh\ny\n```\n\n```tape {for=b <include file=b.tape}\n```\n\n```sh {#b tty}\nz\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Blocks[2].ID; got != "block-2" {
		t.Errorf("block after a tape is %q, want block-2", got)
	}
	if tp := d.Tape(d.Block("a")); tp == nil || tp.Body != "Enter\n" {
		t.Errorf("tape for a = %+v", tp)
	}
	if tp := d.Tape(d.Block("b")); tp == nil || !tp.IsInclude() {
		t.Errorf("tape for b = %+v", tp)
	}
	if d.Tape(d.Block("block-2")) != nil {
		t.Error("block-2 has a tape")
	}

	for _, bad := range []string{
		"```tape {for=nope}\n```\n",
		"```sh {#a}\nx\n```\n```tape {for=a}\n```\n",
		"```sh {#a tty}\nx\n```\n```tape {for=a}\n```\n```tape {for=a}\n```\n",
		"```sh {#a tty}\nx\n```\n```tape {#t for=a}\n```\n",
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) = nil error", bad)
		}
	}
}

func TestTapeRefs(t *testing.T) {
	src := "```sh {#s}\nP=x\n```\n\n```sh {#a tty}\nx\n```\n\n```tape {for=a}\nType \"{{$USER}}:{{#s/env(P)}}\"\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []Ref{{Var: "USER"}, {BlockID: "s", Var: "P"}}
	if got := d.Refs(); !reflect.DeepEqual(got, want) {
		t.Errorf("Refs() = %v, want %v", got, want)
	}
	if _, err := Parse([]byte(strings.Replace(src, "#s/", "#nope/", 1))); err == nil {
		t.Error("a tape naming an unknown block parsed")
	}
	lookup := func(r Ref) (string, bool) {
		return map[string]string{"USER": "j", "P": `p"w`}[r.Var], r.Var != "MISSING"
	}
	got, err := Expand(`Type "{{$USER}}" {{#s/env(P):"old"}}`, lookup)
	if err != nil || got != `Type "j" p"w` {
		t.Errorf("Expand = %q, %v", got, err)
	}
	if _, err := Expand("{{$MISSING}}", lookup); err == nil {
		t.Error("Expand of an unknown ref succeeded")
	}
}
