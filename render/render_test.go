package render

import (
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

func TestFence(t *testing.T) {
	if got := Fence("output", "for=a", "x"); got != "```output {for=a}\nx\n```\n" {
		t.Errorf("got %q", got)
	}
	if got := Fence("output", "", "a ```` b\n"); got != "`````output\na ```` b\n`````\n" {
		t.Errorf("got %q", got)
	}
}

func TestHideFlags(t *testing.T) {
	src := "#!/usr/bin/env mdsh\n# T\n\n```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\nold\n```\n\n```go\nx\n```\n\nend\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("new\n")},
		{Block: d.Blocks[1], Skipped: true},
	}
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"all", Options{}, "# T\n\n```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\nnew\n```\n\n```go\nx\n```\n\nend\n"},
		{"hide code", Options{HideCode: true}, "# T\n\n```output {for=a exit=0}\nnew\n```\n\n```go\nx\n```\n\nend\n"},
		{"hide output", Options{HideOutput: true}, "# T\n\n```sh {#a}\necho a\n```\n\n```go\nx\n```\n\nend\n"},
		{"prose", Options{HideCode: true, HideOutput: true}, "# T\n\n```go\nx\n```\n\nend\n"},
	}
	for _, tt := range tests {
		if got := string(Markdown(d, results, tt.opts)); got != tt.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tt.name, got, tt.want)
		}
	}
	// Without results, existing output is kept.
	if got := string(Markdown(d, nil, Options{WriteBack: true})); got != src {
		t.Errorf("no results:\ngot  %q\nwant %q", got, src)
	}
}

func TestFoldLines(t *testing.T) {
	src := "# A\n<!-- mdsh:collapse -->\ntext\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Markdown(d, nil, Options{})); got != "# A\ntext\n" {
		t.Errorf("printed %q", got)
	}
	if got := string(Markdown(d, nil, Options{WriteBack: true})); got != src {
		t.Errorf("wrote back %q", got)
	}
}

func TestNested(t *testing.T) {
	src := "1. Run:\n\n   ```sh {#a}\n   echo a\n   ```\n\n> ```sh {#b}\n> echo b\n> ```\n\n- ```sh {#c}\n  echo c\n  ```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("a\n\nA\n")},
		{Block: d.Blocks[1], Ran: true, Stdout: []byte("b\n")},
		{Block: d.Blocks[2], Ran: true, Stdout: []byte("c\n")},
	}
	want := "1. Run:\n\n   ```sh {#a}\n   echo a\n   ```\n\n   ```output {for=a exit=0}\n   a\n\n   A\n   ```\n\n" +
		"> ```sh {#b}\n> echo b\n> ```\n>\n> ```output {for=b exit=0}\n> b\n> ```\n\n" +
		"- ```sh {#c}\n  echo c\n  ```\n\n  ```output {for=c exit=0}\n  c\n  ```\n"
	got := string(Markdown(d, results, Options{WriteBack: true}))
	if got != want {
		t.Fatalf("write-back:\ngot  %q\nwant %q", got, want)
	}

	// Writing back again replaces the output instead of adding more.
	d2, err := doc.Parse([]byte(got))
	if err != nil {
		t.Fatal(err)
	}
	for i := range results {
		results[i].Block = d2.Blocks[i]
	}
	if again := string(Markdown(d2, results, Options{WriteBack: true})); again != want {
		t.Errorf("second write-back:\ngot  %q\nwant %q", again, want)
	}

	// With code hidden, output takes the fence's place, list marker included.
	hidden := "1. Run:\n\n   ```output {for=a exit=0}\n   a\n\n   A\n   ```\n\n" +
		"> ```output {for=b exit=0}\n> b\n> ```\n\n" +
		"- ```output {for=c exit=0}\n  c\n  ```\n"
	if got := string(Markdown(d2, results, Options{HideCode: true})); got != hidden {
		t.Errorf("hide code:\ngot  %q\nwant %q", got, hidden)
	}
	// Same, with the output kept from the file.
	if got := string(Markdown(d2, nil, Options{HideCode: true})); got != hidden {
		t.Errorf("hide code, kept output:\ngot  %q\nwant %q", got, hidden)
	}
}

func TestPieces(t *testing.T) {
	src := "# T\n\n```sh {#a}\necho a\n```\n\nmid\n\n```sh {#b}\nfalse\n```\n\nend\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("a\n")},
		nil, // b hasn't finished
	}
	pieces := Pieces(d, results, Options{})
	want := []string{
		"# T\n\n", "```sh {#a}\necho a\n```\n", "\n```output {for=a exit=0}\na\n```\n",
		"\nmid\n\n", "```sh {#b}\nfalse\n```\n", "",
		"\nend\n",
	}
	if len(pieces) != len(want) {
		t.Fatalf("got %d pieces, want %d: %q", len(pieces), len(want), pieces)
	}
	for i := range want {
		if string(pieces[i]) != want[i] {
			t.Errorf("piece %d = %q, want %q", i, pieces[i], want[i])
		}
	}
}

func TestRecordingLink(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#a tty}\nx\n```\n\n```sh {#b tty frame=3}\nx\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"![a](d.md.assets/a.gif)\n", "![b](d.md.assets/b.gif#frame=3)\n"} {
		b := d.Blocks[i]
		r := &run.Result{Block: b, Ran: true, Image: "d.md.assets/" + b.ID + ".gif"}
		if got := Output(r, false); !strings.Contains(got, want) {
			t.Errorf("%s: output %q, want %q in it", b.ID, got, want)
		}
	}
}

func TestCopies(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\nold\n```\n\n" +
		"```sh {#b out=markdown}\necho b\n```\n\n" +
		"Summary:\n\n" +
		"```output {for=a exit=0}\nold\n```\n\n" +
		"```output {for=a stream=stderr}\nold err\n```\n\n" +
		"- B:\n\n  <!-- mdsh:output for=\"b\" -->\n  old b\n  <!-- /mdsh:output -->\n\n" +
		"```output {for=c exit=0}\nkept\n```\n\nend\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Status: 2, Stdout: []byte("new\n"), Stderr: []byte("new err\n")},
		{Block: d.Blocks[1], Ran: true, Stdout: []byte("| b |\n")},
	}
	want := "```sh {#a}\necho a\n```\n\n```output {for=a exit=2}\nnew\n```\n\n```output {for=a stream=stderr}\nnew err\n```\n\n" +
		"```sh {#b out=markdown}\necho b\n```\n\n<!-- mdsh:output for=\"b\" -->\n| b |\n<!-- /mdsh:output -->\n\n" +
		"Summary:\n\n" +
		"```output {for=a exit=2}\nnew\n```\n\n" +
		"```output {for=a stream=stderr}\nnew err\n```\n\n" +
		"- B:\n\n  <!-- mdsh:output for=\"b\" -->\n  | b |\n  <!-- /mdsh:output -->\n\n" +
		"```output {for=c exit=0}\nkept\n```\n\nend\n"
	if got := string(Markdown(d, results, Options{WriteBack: true})); got != want {
		t.Errorf("write-back:\ngot  %q\nwant %q", got, want)
	}
	// Blocks that didn't run keep their copies as they are.
	if got := string(Markdown(d, nil, Options{WriteBack: true})); got != src {
		t.Errorf("no results:\ngot  %q\nwant %q", got, src)
	}
	// Hidden output hides the copies too.
	if got := string(Markdown(d, results, Options{HideOutput: true})); strings.Contains(got, "new") || strings.Contains(got, "old") || !strings.Contains(got, "Summary:\n\n- B:\n\nend") {
		t.Errorf("hide output: %q", got)
	}
}

func TestEmptyCopy(t *testing.T) {
	src := "```sh {#a}\ntrue\n```\n\nA:\n\n```output {for=a exit=0}\nold\n```\n\n```output {for=a stream=stderr}\nold\n```\n\nend\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{{Block: d.Blocks[0], Ran: true}}
	// Write-back keeps empty copies, so a later run can fill them.
	want := "```sh {#a}\ntrue\n```\n\nA:\n\n```output {for=a exit=0}\n```\n\n```output {for=a stream=stderr}\n```\n\nend\n"
	if got := string(Markdown(d, results, Options{WriteBack: true})); got != want {
		t.Errorf("write-back:\ngot  %q\nwant %q", got, want)
	}
	// Printing leaves them out.
	if got, want := string(Markdown(d, results, Options{})), "```sh {#a}\ntrue\n```\n\nA:\n\nend\n"; got != want {
		t.Errorf("print:\ngot  %q\nwant %q", got, want)
	}
}
