package doc

import (
	_ "embed"
	"fmt"
	"reflect"
	"slices"
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
	d, err := Parse([]byte("---\nmdsh:\n  on_error: maybe\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.FrontMatterErr == nil {
		t.Fatal("want FrontMatterErr")
	}
}

func TestFrontMatterErrorText(t *testing.T) {
	for _, tt := range []struct{ fm, want string }{
		{"title: a: b\n", "front matter: mapping values are not allowed in this context"},
		{"a: 1\n b: 2\n", "front matter: line 3: mapping values are not allowed in this context"},
		{"title: x\ntitle: y\n", `front matter: line 3: mapping key "title" already defined at line 2`},
	} {
		d, err := Parse([]byte("---\n" + tt.fm + "---\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(d.FrontMatterErr); got != tt.want {
			t.Errorf("%q: %s, want %s", tt.fm, got, tt.want)
		}
	}
	// Lines count from the top of the file, #! line and all.
	d, _ := Parse([]byte("#!/usr/bin/env mdsh\n---\na: 1\n b: 2\n---\n"))
	if got, want := fmt.Sprint(d.FrontMatterErr), "front matter: line 4: mapping values are not allowed in this context"; got != want {
		t.Errorf("after #!: %s, want %s", got, want)
	}
}

// Front matter that doesn't parse doesn't stop the document parsing:
// it keeps its lines, with the error and the default options.
func TestParseBadFrontMatter(t *testing.T) {
	src := "---\ntitle: Setup: first steps\nmdsh:\n  on_error: stop\n---\n\n# Hi\n\n```sh\necho hi\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if d.FrontMatterErr == nil || !strings.Contains(d.FrontMatterErr.Error(), "front matter: ") {
		t.Fatalf("FrontMatterErr = %v", d.FrontMatterErr)
	}
	if want := strings.Index(src, "\n\n# Hi") + 1; d.FrontMatter != (Span{0, want}) {
		t.Errorf("FrontMatter = %+v, want {0 %d}", d.FrontMatter, want)
	}
	if d.Options.OnError != "" {
		t.Errorf("Options = %+v, want the defaults", d.Options)
	}
	if len(d.Blocks) != 1 || d.Blocks[0].Body != "echo hi\n" {
		t.Errorf("Blocks = %+v", d.Blocks)
	}
}

func TestParseTerminal(t *testing.T) {
	d, err := Parse([]byte("---\nmdsh:\n  terminal:\n    cols: 120\n    rows: 40\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Options.Terminal; got.Cols != 120 || got.Rows != 40 {
		t.Errorf("Terminal = %+v", got)
	}
	if d, _ := Parse([]byte("---\nmdsh:\n  terminal:\n    rows: -1\n---\n")); d.FrontMatterErr == nil {
		t.Error("rows -1: want FrontMatterErr")
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
		"once": true, // the runner checks for output
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
		"```sh {#a show=once}\n```\n",
		"<!-- mdsh:if once -->\n<!-- mdsh:end -->\n",
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestParseFolds(t *testing.T) {
	src := "# A\n<!-- mdsh:collapse -->\n## B\n  <!-- mdsh:collapse {{$X}}!=yes -->\n<!-- mdsh:collapsed -->\n```sh {#a collapse collapse-output={{$?}}==0}\n<!-- mdsh:collapse -->\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Folds) != 2 {
		t.Fatalf("folds = %+v", d.Folds)
	}
	if f := d.Folds[0]; string(d.Src[f.Span.Start:f.Span.End]) != "<!-- mdsh:collapse -->\n" || f.Cond.Ref != nil || f.Cond.Value != "true" {
		t.Errorf("fold 0 = %+v", f)
	}
	if f := d.Folds[1]; f.Cond.Ref == nil || f.Cond.Ref.Var != "X" || f.Cond.Op != "!=" {
		t.Errorf("fold 1 = %+v", f)
	}
	if c, ok := d.Blocks[0].Cond("collapse"); !ok || !c.Eval(nil) {
		t.Errorf("collapse = %+v, %v", c, ok)
	}
	for _, bad := range []string{
		"<!-- mdsh:collapse nope -->\n",
		"<!-- mdsh:collapse once -->\n",
		"```sh {#a collapse=nope}\n```\n",
		"```sh {#a collapse-output=once}\n```\n",
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

func TestCopies(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\na\n```\n\n" +
		"```output {for=a exit=0}\na\n```\n\n" +
		"```sh {#b out=markdown}\necho b\n```\n\n" +
		"```output {for=a stream=stderr}\noops\n```\n\n" +
		"- Table:\n\n  <!-- mdsh:output for=\"b\" -->\n  b\n  <!-- /mdsh:output -->\n\n" +
		"> ```output {for=nope}\n> x\n> ```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Copies) != 4 {
		t.Fatalf("copies = %d, want 4", len(d.Copies))
	}
	want := []Copy{
		{For: "a"},
		{For: "a", Stream: "stderr"},
		{For: "b", Region: true, Prefix: "  "},
		{For: "nope", Prefix: "> "},
	}
	for i, c := range d.Copies {
		got := *c
		got.Span = Span{}
		if got != want[i] {
			t.Errorf("copy %d = %+v, want %+v", i, got, want[i])
		}
	}
	if s := string(src[d.Copies[2].Span.Start:d.Copies[2].Span.End]); !strings.HasPrefix(s, "  <!--") || !strings.HasSuffix(s, "-->\n") {
		t.Errorf("region copy span = %q", s)
	}
	// The real output is still the block's own.
	if s := string(src[d.Blocks[0].Output.Start:d.Blocks[0].Output.End]); s != "\n```output {for=a exit=0}\na\n```\n" {
		t.Errorf("a's output = %q", s)
	}
	if len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], `"nope"`) {
		t.Errorf("warnings = %q", d.Warnings)
	}
}

func TestTextboxFit(t *testing.T) {
	src := "```textbox {#a lines=fit var=v}\none\ntwo\nthree\n```\n\n```textbox {#b lines=3}\n```\n\n" +
		"```textbox {#c lines=fit var=v}\none\ntwo\nthree\n```\n\n```textbox {#d var=v}\none\ntwo\nthree\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := d.Block("a").Textbox(), (Textbox{Text: "one\ntwo\nthree", Lines: 1, Fit: true}); got != want {
		t.Errorf("a = %+v, want %+v", got, want)
	}
	if got := d.Block("b").Textbox(); got.Fit || got.Lines != 3 {
		t.Errorf("b = %+v", got)
	}
	// A fit textbox is a copy of another fit one, not of one with lines=1.
	if d.Block("c").VarTaken != "" || !strings.Contains(d.Block("d").VarTaken, "lines= differs") {
		t.Errorf("c: %q; d: %q", d.Block("c").VarTaken, d.Block("d").VarTaken)
	}
}

func TestBackgroundNotTTY(t *testing.T) {
	_, err := Parse([]byte("```sh {#a background tty}\nx\n```\n"))
	if err == nil || !strings.Contains(err.Error(), "background") {
		t.Errorf("err = %v, want background and tty refused", err)
	}
}

func TestDuplicateVar(t *testing.T) {
	src := "```textbox {#a var=env}\n```\n\n```choice {#b var=env}\n- [ ] x\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := `var=env is already bound by "a" on line 1, and this isn't a copy of it (it is a choice, not a textbox), so it is ignored`
	if b := d.Block("b"); b.VarTaken != want || !slices.Equal(d.Warnings, []string{"line 4: " + want}) {
		t.Errorf("VarTaken = %q, warnings %q", b.VarTaken, d.Warnings)
	}
	if _, _, ok := d.Block("b").Var(); ok {
		t.Error("b binds env too")
	}
	if name, _, ok := d.Block("a").Var(); !ok || name != "MDSH_VAR_env" {
		t.Errorf("a binds %q, %v", name, ok)
	}
	// var= on other blocks binds nothing, so it doesn't clash.
	if _, err := Parse([]byte("```textbox {var=env}\n```\n```sh {var=env}\n```\n")); err != nil {
		t.Error(err)
	}
}

func TestMirrorVar(t *testing.T) {
	doc := func(a, b, c, d string) string {
		return "```choice {#a mode=radio var=env}\n" + a + "```\n\n" +
			"> ```choice {#b mode=radio var=env}\n" + strings.ReplaceAll(b, "- ", "> - ") + "> ```\n\n" +
			"```choice {#c mode=multi var=env}\n" + c + "```\n\n" +
			"```choice {#d mode=radio var=env}\n" + d + "```\n\n" +
			"```textbox {#t var=host}\none\n```\n\n```textbox {#u var=host}\none\n```\n"
	}
	const prod, staging = "- [ ] staging\n- [x] prod\n", "- [x] staging\n- [ ] prod\n"
	src := doc(prod, staging, prod, prod)
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"a": "", "b": "a", "c": "", "d": "a", "t": "", "u": "t"} {
		if got := d.Block(id).MirrorOf; got != want {
			t.Errorf("%s mirrors %q, want %q", id, got, want)
		}
	}
	if d.Block("c").VarTaken == "" {
		t.Error("c, a multi, mirrors a radio")
	}
	wantWarn := []string{
		`line 6: var=env: this copy of "a" on line 1 has a different value, so it is set to that one's`,
		`line 11: var=env is already bound by "a" on line 1, and this isn't a copy of it (mode= differs), so it is ignored`,
	}
	if !slices.Equal(d.Warnings, wantWarn) {
		t.Errorf("warnings %q\nwant %q", d.Warnings, wantWarn)
	}
	if _, _, ok := d.Block("b").Var(); ok {
		t.Error("b binds env too")
	}
	if got := d.Mirrors(d.Block("b")); len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "d" {
		t.Errorf("Mirrors(b) = %v", got)
	}

	// Loading puts every copy at the first's value.
	fixed, ok := d.MatchMirrors()
	if !ok || string(fixed) != doc(prod, prod, prod, prod) {
		t.Fatalf("MatchMirrors = %v %q", ok, fixed)
	}
	if err := d.SetSource(fixed); err != nil {
		t.Fatal(err)
	}
	if len(d.Warnings) != 1 {
		t.Errorf("warnings after matching: %q", d.Warnings)
	}
	if _, ok := d.MatchMirrors(); ok {
		t.Error("MatchMirrors changed matched copies")
	}

	// Setting one sets them all, but not the one that isn't a copy.
	if got, want := string(d.SetChoice(d.Block("d"), []bool{true, false})), doc(staging, staging, prod, staging); got != want {
		t.Errorf("SetChoice:\n%s\nwant:\n%s", got, want)
	}
	got := string(d.SetText(d.Block("t"), "two\n```"))
	if strings.Count(got, "````textbox") != 2 || strings.Count(got, "two\n```\n````") != 2 {
		t.Errorf("SetText:\n%s", got)
	}
}

func TestHTTPBlockAttrs(t *testing.T) {
	for _, attr := range []string{"env=x", "share=A", "tty", "background", "out=markdown"} {
		_, err := Parse([]byte("```http {#a " + attr + "}\nGET https://example.com\n```\n"))
		if err == nil || !strings.Contains(err.Error(), "an http block takes no") {
			t.Errorf("%s: err = %v", attr, err)
		}
	}
	d, err := Parse([]byte("```http {#a}\nGET https://example.com/{{#b/env(X)}}\n[Captures]\nt: secret jsonpath $.t\nu: jsonpath $.u\n```\n```sh {#b}\nX=1\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if refs := d.Refs(); len(refs) != 1 || refs[0] != (Ref{BlockID: "b", Var: "X"}) {
		t.Errorf("refs = %v", refs)
	}
	if a := d.Block("a"); !a.SecretVar("t") || a.SecretVar("u") {
		t.Errorf("SecretVar is wrong")
	}
	if _, err := Parse([]byte("```http {#a}\nGET https://example.com/{{#nope/env(X)}}\n```\n")); err == nil {
		t.Errorf("a placeholder for an unknown block parsed")
	}
}

func TestResponseRefs(t *testing.T) {
	d, err := Parse([]byte("```http {#login}\nGET https://example.com\n```\n\nToken {{#login/json($.items[0]['id']):\"7\"}}, type {{#login/header(Content-Type)}}, status {{#login/status}}.\n\n```sh {#next run={{#login/status}}==200}\necho\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Ref{
		{BlockID: "login", Fn: FnJSON, Var: "$.items[0]['id']"},
		{BlockID: "login", Fn: FnHeader, Var: "Content-Type"},
		{BlockID: "login", Fn: FnStatus},
	}
	for i, p := range d.Placeholders {
		if p.Ref != want[i] {
			t.Errorf("placeholder %d = %+v, want %+v", i, p.Ref, want[i])
		}
		if got := p.Ref.String(); !strings.Contains(string(d.Src), strings.TrimSuffix(got, "}}")) {
			t.Errorf("String() = %s", got)
		}
	}
	if d.Placeholders[0].Value != "7" {
		t.Errorf("stored value = %q", d.Placeholders[0].Value)
	}
	if c, _ := d.Block("next").Cond("run"); c.Ref == nil || *c.Ref != want[2] {
		t.Errorf("run= ref = %v", c.Ref)
	}
	for _, src := range []string{
		"```sh {#a}\necho\n```\n{{#a/status}}\n",
		"```http {#a}\nGET https://x\n```\n{{#a/json($.x[)}}\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q) didn't fail", src)
		}
	}
}
