package doc

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Document {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// spot is the boundary at the start of the line holding text.
func spot(t *testing.T, d *Document, text string) int {
	t.Helper()
	i := strings.Index(string(d.Src), text)
	if i < 0 {
		t.Fatalf("no %q", text)
	}
	i = strings.LastIndex(string(d.Src[:i]), "\n") + 1
	for _, b := range d.Boundaries() {
		if b == i {
			return i
		}
	}
	t.Fatalf("%q isn't at a boundary; boundaries %v", text, d.Boundaries())
	return 0
}

func TestBoundaries(t *testing.T) {
	src := "---\ntitle: t\n---\n# T\n\nIntro.\n\n" +
		"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
		"- item\n\n  ```sh {#n}\n  true\n  ```\n\n  more\n\n" +
		"> quote\n>\n> still\n\n" +
		"<!-- mdsh:output for=\"a\" -->\n![](x.png)\n\nmore\n<!-- /mdsh:output -->\n\nEnd.\n"
	d := mustParse(t, src)
	var got []string
	for _, b := range d.Boundaries() {
		line, _, _ := strings.Cut(src[b:], "\n")
		got = append(got, line)
	}
	want := []string{"# T", "Intro.", "```sh {#a}", "- item", "> quote", `<!-- mdsh:output for="a" -->`, "End.", ""}
	if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", want) {
		t.Errorf("boundaries at\n%q\nwant\n%q", got, want)
	}
}

func TestMoveBlock(t *testing.T) {
	src := "# T\n\nIntro.\n\n" +
		"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
		"Between.\n" +
		"```sh {#b tty}\nsh\n```\n" +
		"```tape {for=b}\nType \"exit\"\n```\n\n" +
		"```sh {#c}\necho c\n```\n\n\n\nEnd.\n"
	tests := []struct {
		name, block, to, want string
		order                 string
	}{{
		name:  "down past prose",
		block: "a", to: "End.",
		want: "# T\n\nIntro.\n\n" +
			"Between.\n" +
			"```sh {#b tty}\nsh\n```\n" +
			"```tape {for=b}\nType \"exit\"\n```\n\n" +
			"```sh {#c}\necho c\n```\n\n\n\n" +
			"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\nEnd.\n",
		order: "[1 2 3 0]",
	}, {
		name:  "to the end of the file",
		block: "a", to: "",
		want: "# T\n\nIntro.\n\n" +
			"Between.\n" +
			"```sh {#b tty}\nsh\n```\n" +
			"```tape {for=b}\nType \"exit\"\n```\n\n" +
			"```sh {#c}\necho c\n```\n\n\n\nEnd.\n\n" +
			"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n",
		order: "[1 2 3 0]",
	}, {
		name:  "up, with its tape, out of a tight paragraph",
		block: "b", to: "Intro.",
		want: "# T\n\n" +
			"```sh {#b tty}\nsh\n```\n" +
			"```tape {for=b}\nType \"exit\"\n```\n\n" +
			"Intro.\n\n" +
			"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
			"Between.\n\n" +
			"```sh {#c}\necho c\n```\n\n\n\nEnd.\n",
		order: "[1 2 0 3]",
	}, {
		name:  "to the top",
		block: "c", to: "# T",
		want: "```sh {#c}\necho c\n```\n\n# T\n\nIntro.\n\n" +
			"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
			"Between.\n" +
			"```sh {#b tty}\nsh\n```\n" +
			"```tape {for=b}\nType \"exit\"\n```\n\n" +
			"End.\n",
		order: "[3 0 1 2]",
	}, {
		name:  "where it is",
		block: "a", to: "```sh {#a}",
		want:  src,
		order: "[0 1 2 3]",
	}, {
		name:  "right after itself",
		block: "a", to: "Between.",
		want:  src,
		order: "[0 1 2 3]",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, src)
			blocks := append([]*Block(nil), d.Blocks...)
			at := len(d.Src)
			if tt.to != "" {
				at = spot(t, d, tt.to)
			}
			order, err := d.MoveBlock(d.Block(tt.block), at)
			if err != nil {
				t.Fatal(err)
			}
			if string(d.Src) != tt.want {
				t.Errorf("src =\n%s\nwant\n%s", d.Src, tt.want)
			}
			if fmt.Sprint(order) != tt.order {
				t.Errorf("order = %v, want %s", order, tt.order)
			}
			for j, i := range order {
				if d.Blocks[j] != blocks[i] {
					t.Errorf("block %d isn't the old block %d", j, i)
				}
			}
			if !d.Block("a").HasOutput() {
				t.Error("a lost its output")
			}
			if d.Tape(d.Block("b")) == nil {
				t.Error("b lost its tape")
			}
		})
	}
}

func TestMoveBlockRefuses(t *testing.T) {
	src := "Intro.\n\n" +
		"- item\n\n  ```sh {#n}\n  true\n  ```\n\n" +
		"```sh\necho unnamed\n```\n\n```output {for=block-2 exit=0}\nunnamed\n```\n\n" +
		"```sh {#x}\necho x\n```\n\n" +
		"<!-- mdsh:if {{$X}} -->\n\nIn.\n\n<!-- mdsh:end -->\n"
	d := mustParse(t, src)
	if _, err := d.MoveBlock(d.Block("n"), 0); err == nil || !strings.Contains(err.Error(), "top-level") {
		t.Errorf("nested block: err = %v", err)
	}
	if _, err := d.MoveBlock(d.Block("block-2"), 0); err == nil || !strings.Contains(err.Error(), "name the document's blocks") {
		t.Errorf("unnamed block with output: err = %v", err)
	}
	if _, err := d.MoveBlock(d.Block("x"), spot(t, d, "item")+3); err == nil {
		t.Error("move to a spot inside a list worked")
	}
	if _, err := d.MoveSpan(Span{spot(t, d, "<!-- mdsh:if"), spot(t, d, "In.")}, 0); err == nil || !strings.Contains(err.Error(), "split") {
		t.Errorf("half a section: err = %v", err)
	}
	if string(d.Src) != src {
		t.Errorf("a refused move changed the source:\n%s", d.Src)
	}
}

func TestMoveSpan(t *testing.T) {
	src := "# One\n\nFirst.\n\n```sh {#a}\necho a\n```\n\n" +
		"## Sub\n\nSub text.\n\n" +
		"# Two\n\n<!-- mdsh:if {{$X}} -->\n```sh {#b}\necho b\n```\n<!-- mdsh:end -->\n"
	d := mustParse(t, src)
	order, err := d.MoveSpan(Span{0, spot(t, d, "# Two")}, len(d.Src))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Two\n\n<!-- mdsh:if {{$X}} -->\n```sh {#b}\necho b\n```\n<!-- mdsh:end -->\n\n" +
		"# One\n\nFirst.\n\n```sh {#a}\necho a\n```\n\n## Sub\n\nSub text.\n"
	if string(d.Src) != want {
		t.Errorf("src =\n%s\nwant\n%s", d.Src, want)
	}
	if fmt.Sprint(order) != "[1 0]" {
		t.Errorf("order = %v", order)
	}
	// A whole section moves with what's in it.
	if _, err := d.MoveSpan(Span{spot(t, d, "<!-- mdsh:if"), spot(t, d, "# One")}, len(d.Src)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(d.Src), "Sub text.\n\n<!-- mdsh:if {{$X}} -->\n```sh {#b}\necho b\n```\n<!-- mdsh:end -->\n") {
		t.Errorf("src =\n%s", d.Src)
	}
}

func TestMoveUnnamedWithoutRefs(t *testing.T) {
	// Unnamed blocks nothing refers to may renumber, as code blocks
	// mdsh doesn't run do.
	d := mustParse(t, "```go\nx\n```\n\n```py\ny\n```\n")
	if _, err := d.MoveBlock(d.Blocks[0], len(d.Src)); err != nil {
		t.Fatal(err)
	}
	if d.Blocks[0].Lang != "py" || d.Blocks[0].ID != "block-1" {
		t.Errorf("blocks %s %s", d.Blocks[0].Lang, d.Blocks[0].ID)
	}

	// One that depends= names may not.
	d = mustParse(t, "```sh\ntrue\n```\n\n```sh\nfalse\n```\n\n```sh {#x depends=block-1}\ntrue\n```\n")
	if _, err := d.MoveBlock(d.Blocks[0], len(d.Src)); err == nil || !strings.Contains(err.Error(), "name the document's blocks") {
		t.Errorf("unnamed block that depends= names: err = %v", err)
	}
}

func TestOrderProblems(t *testing.T) {
	sh := func(b *Block) bool { return b.Lang == "sh" }
	src := "```sh {#detect}\necho\n```\n\n" +
		"```sh {#use run={{#detect/?}}==0}\necho\n```\n\n" +
		"```sh {#after run={{$?}}==0}\necho\n```\n\n" +
		"```choice {#c1 var=env}\na\nb\n```\n\n" +
		"```choice {#c2 var=env}\nb\na\n```\n\n" +
		"OS is {{#detect/env(OS)}}.\n\n" +
		"<!-- mdsh:if {{$X}} -->\n\n```sh {#inside}\ntrue\n```\n\n<!-- mdsh:end -->\n"
	d := mustParse(t, src)
	before := d.OrderProblems(sh)
	if p := NewProblems(before, before); p != nil {
		t.Errorf("no change, problems %v", p)
	}
	move := func(id, to string) []string {
		t.Helper()
		at := len(d.Src)
		if to != "" {
			at = spot(t, d, to)
		}
		before := d.OrderProblems(sh)
		if _, err := d.MoveBlock(d.Block(id), at); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, p := range NewProblems(before, d.OrderProblems(sh)) {
			got = append(got, p.Text)
		}
		return got
	}
	check := func(got []string, want ...string) {
		t.Helper()
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("new problems\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}

	check(move("detect", "OS is"),
		"run={{#detect/?}}==0 on #use reads {{#detect/?}}, which comes after it, so it hasn't run yet",
	)
	check(move("detect", "```sh {#use"))
	check(move("use", "```sh {#detect"),
		"run={{#detect/?}}==0 on #use reads {{#detect/?}}, which comes after it, so it hasn't run yet",
		"run={{$?}}==0 on #after reads {{$?}} from the block above it, which is now #detect")
	check(move("c2", "```choice {#c1"), "#c2 now sets var=env, since it is the first input that binds it")
	check(move("inside", "```sh {#detect"), "#inside is outside every mdsh:if section")
	check(move("use", "<!-- mdsh:end"), "#use is inside mdsh:if {{$X}}, so it is left out when that is false")
}

func TestMoveEverywhere(t *testing.T) {
	src := "#!/usr/bin/env mdsh\n---\ntitle: t\n---\n```sh {#first}\ntrue\n```\n# T\nText right under.\n" +
		"```sh {#a}\necho a\n```\n```output {for=a}\na\n```\n" +
		"| x | y |\n|---|---|\n| 1 | 2 |\n\n" +
		"```include {file=x.sh}\n```\n\n" +
		"- list\n- more\n\n" +
		"<!-- mdsh:if {{$X}} -->\n```sh {#b}\necho b\n```\n<!-- mdsh:end -->\n" +
		"```sh {#c}\necho c\n```"
	base := mustParse(t, src)
	for i, b := range base.Blocks {
		for _, at := range base.Boundaries() {
			d := mustParse(t, src)
			order, err := d.MoveBlock(d.Blocks[i], at)
			if err != nil {
				t.Errorf("move %s to %d: %v", b.ID, at, err)
				continue
			}
			if !strings.HasPrefix(string(d.Src), "#!/usr/bin/env mdsh\n---\ntitle: t\n---\n") || strings.Contains(string(d.Src), "\n\n\n") {
				t.Errorf("move %s to %d:\n%s", b.ID, at, d.Src)
			}
			if !d.Block("a").HasOutput() {
				t.Errorf("move %s to %d: a lost its output", b.ID, at)
			}
			// And back again.
			back := 0
			for j, k := range order {
				if k == i {
					back = j
				}
			}
			var to int
			if i+1 < len(base.Blocks) {
				to = d.Blocks[slices.Index(order, i+1)].Span.Start
			} else {
				to = len(d.Src)
			}
			if _, err := d.MoveBlock(d.Blocks[back], to); err != nil {
				t.Errorf("move %s back: %v", b.ID, err)
			}
		}
	}
}

func TestMoveSection(t *testing.T) {
	src := "Intro.\n\n# One\n\nA.\n\n## One.a\n\n```sh {#a}\necho a\n```\n\n" +
		"# Two\n\nB.\n\nSetext\n------\n\nC.\n"
	d := mustParse(t, src)
	var hs []string
	for _, h := range d.Headings() {
		line, _, _ := strings.Cut(src[h.Span.Start:], "\n")
		hs = append(hs, fmt.Sprintf("%d %s", h.Level, line))
	}
	if got := strings.Join(hs, "|"); got != "1 # One|2 ## One.a|1 # Two|2 Setext" {
		t.Errorf("headings %s", got)
	}
	span, level, err := d.Section(spot(t, d, "## One.a"))
	if err != nil || level != 2 || src[span.End:span.End+5] != "# Two" {
		t.Errorf("section %v %d %v", span, level, err)
	}
	// ## One.a can't go mid-section or before a deeper heading.
	if _, err := d.MoveSection(spot(t, d, "## One.a"), spot(t, d, "C.")); err == nil {
		t.Error("moved into the middle of a section")
	}
	// It can go at the end, making it Two's last subsection.
	if _, err := d.MoveSection(spot(t, d, "## One.a"), len(d.Src)); err != nil {
		t.Fatal(err)
	}
	want := "Intro.\n\n# One\n\nA.\n\n# Two\n\nB.\n\nSetext\n------\n\nC.\n\n## One.a\n\n```sh {#a}\necho a\n```\n"
	if string(d.Src) != want {
		t.Errorf("src =\n%s\nwant\n%s", d.Src, want)
	}
	// # Two goes before # One, subsections and all.
	if _, err := d.MoveSection(spot(t, d, "# Two"), spot(t, d, "# One")); err != nil {
		t.Fatal(err)
	}
	want = "Intro.\n\n# Two\n\nB.\n\nSetext\n------\n\nC.\n\n## One.a\n\n```sh {#a}\necho a\n```\n\n# One\n\nA.\n"
	if string(d.Src) != want {
		t.Errorf("src =\n%s\nwant\n%s", d.Src, want)
	}
}
