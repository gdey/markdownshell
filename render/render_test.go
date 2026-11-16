package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termrec"
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

func TestTerminalEscapesCode(t *testing.T) {
	src := "```sh\necho a\x1b[2K\n```\n\ntext\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(Markdown(d, nil, Options{Terminal: true})), "```sh\necho a^[[2K\n```\n\ntext\n"; got != want {
		t.Errorf("terminal:\ngot  %q\nwant %q", got, want)
	}
	if got := string(Markdown(d, nil, Options{Terminal: true, WriteBack: true})); got != src {
		t.Errorf("write-back changed the code: %q", got)
	}
}

func TestNoSaveWriteBack(t *testing.T) {
	src := "```sh {#a nosave}\necho a\n```\n\n```output {for=a exit=0}\nold\n```\n\n" +
		"Saw {{#a/env(X):\"stored\"}} and {{#b/env(Y):\"was\"}}.\n\n" +
		"```output {for=a}\nold copy\n```\n\n" +
		"```sh {#b}\necho b\n```\n\n```output {for=b exit=0 nosave}\nold b\n```\n\n" +
		"```output {for=b}\nrefilled copy\n```\n\n" +
		"```sh {#c}\necho c\n```\n\n```output {for=c exit=0}\nold c\n```\n\n" +
		"```output {for=c nosave}\nkept copy\n```\n\n" +
		"```output {for=c}\nrefilled copy\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("new a\n"), Vars: map[string]string{"X": "fresh"}},
		{Block: d.Blocks[1], Ran: true, Stdout: []byte("new b\n"), Vars: map[string]string{"Y": "now"}},
		{Block: d.Blocks[2], Ran: true, Stdout: []byte("new c\n")},
	}
	// a is nosave: its output, its copies and its placeholders keep what
	// the file holds. b's output fence is nosave, so that is kept while
	// its placeholder is filled; c's copy marked nosave is kept while
	// the other is refilled.
	want := "```sh {#a nosave}\necho a\n```\n\n```output {for=a exit=0}\nold\n```\n\n" +
		"Saw {{#a/env(X):\"stored\"}} and {{#b/env(Y):\"now\"}}.\n\n" +
		"```output {for=a}\nold copy\n```\n\n" +
		"```sh {#b}\necho b\n```\n\n```output {for=b exit=0 nosave}\nold b\n```\n\n" +
		"```output {for=b exit=0}\nnew b\n```\n\n" +
		"```sh {#c}\necho c\n```\n\n```output {for=c exit=0}\nnew c\n```\n\n" +
		"```output {for=c nosave}\nkept copy\n```\n\n" +
		"```output {for=c exit=0}\nnew c\n```\n"
	if got := string(Markdown(d, results, Options{WriteBack: true})); got != want {
		t.Errorf("write-back:\ngot  %q\nwant %q", got, want)
	}
	// Printing shows the run as it is.
	printed := string(Markdown(d, results, Options{}))
	if !strings.Contains(printed, "new a\n") || !strings.Contains(printed, "new b\n") || !strings.Contains(printed, "Saw fresh and now.") || strings.Contains(printed, "old copy") {
		t.Errorf("printed: %q", printed)
	}
}

// TestSecretWriteBack checks that secret implies nosave: write-back
// keeps what the file holds after a secret block, and adds no output
// where it had none.
func TestSecretWriteBack(t *testing.T) {
	src := "```sh {#token secret}\necho hunter2\n```\n\n```sh {#plain}\necho hi\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("hunter2\n")},
		{Block: d.Blocks[1], Ran: true, Stdout: []byte("hi\n")},
	}
	want := src + "\n```output {for=plain exit=0}\nhi\n```\n"
	if got := string(Markdown(d, results, Options{WriteBack: true})); got != want {
		t.Errorf("write-back:\ngot  %q\nwant %q", got, want)
	}
}

// On a terminal, as with mdsh -n, a cast fence shows the text of its
// frame=, or of the last frame, not the recording's JSON; written back,
// it is left as it is. Nested in a quote, the text keeps the quote.
func TestTerminalCastFence(t *testing.T) {
	var cast bytes.Buffer
	termrec.WriteCast(&cast, termrec.Recording{Cols: 10, Rows: 2, Events: []termrec.Event{
		{At: 0, Data: []byte("first")},
		{At: time.Second, Data: []byte("\x1b[2J\x1b[Hsecond")},
	}})
	quote := func(s string) string {
		return "> " + strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", "\n> ") + "\n"
	}
	for _, tt := range []struct{ src, want string }{
		{"```cast\n" + cast.String() + "```\n", "```cast\nsecond\n```\n"},
		{"```cast {frame=1}\n" + cast.String() + "```\n", "```cast {frame=1}\nfirst\n```\n"},
		{quote("```cast\n" + cast.String() + "```"), "> ```cast\n> second\n> ```\n"},
		{"```cast\nnot a recording\n```\n", "```cast\nnot a recording\n```\n"},
	} {
		d, err := doc.Parse([]byte(tt.src))
		if err != nil {
			t.Fatal(err)
		}
		if got := string(Markdown(d, nil, Options{Terminal: true})); got != tt.want {
			t.Errorf("terminal:\ngot  %q\nwant %q", got, tt.want)
		}
		if got := string(Markdown(d, nil, Options{Terminal: true, WriteBack: true})); got != tt.src {
			t.Errorf("write-back changed the cast: %q", got)
		}
	}
}

// A tty block's result whose recording was swapped for the text of a
// frame writes that text as its output, not nothing.
func TestRecordingAsText(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#a tty}\ntop\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := &run.Result{Block: d.Blocks[0], Ran: true, Stdout: []byte("load 1.0")}
	if got, want := Output(r, false), "```output {for=a exit=0}\nload 1.0\n```\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	r.Image = "a.gif"
	if got, want := Output(r, false), "<!-- mdsh:output for=\"a\" -->\n![a](a.gif)\n<!-- /mdsh:output -->\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestMaskSecrets checks that a report prints a secret block's code and
// output, run or saved, as (secret), and leaves other blocks alone
// (finding 9 of the v1.4.0 security review).
func TestMaskSecrets(t *testing.T) {
	src := "# T\n\n```sh {#tok secret}\nvault read token\n```\n\n```sh {#old secret}\ncat key\n```\n\n```output {for=old exit=0}\nsaved-key\n```\n\n```sh {#none secret}\ntrue\n```\n\n```sh {#plain}\necho hi\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("s3cr3t\n")},
		nil,
		{Block: d.Blocks[2], Ran: true},
		{Block: d.Blocks[3], Ran: true, Stdout: []byte("hi\n")},
	}
	want := "# T\n\n```sh {#tok secret}\n(secret)\n```\n\n```output {for=tok}\n(secret)\n```\n\n```sh {#old secret}\n(secret)\n```\n\n```output {for=old}\n(secret)\n```\n\n```sh {#none secret}\n(secret)\n```\n\n```sh {#plain}\necho hi\n```\n\n```output {for=plain exit=0}\nhi\n```\n"
	if got := string(Markdown(d, results, Options{MaskSecrets: true})); got != want {
		t.Errorf("masked:\ngot  %q\nwant %q", got, want)
	}
	got := string(Markdown(d, results, Options{}))
	for _, s := range []string{"s3cr3t", "vault read token", "saved-key"} {
		if !strings.Contains(got, s) {
			t.Errorf("unmasked lacks %q:\n%s", s, got)
		}
	}
	// Write-back never masks: the file keeps its code.
	if got := string(Markdown(d, results, Options{MaskSecrets: true, WriteBack: true})); !strings.Contains(got, "vault read token") {
		t.Errorf("write-back masked the code:\n%s", got)
	}
}

// TestAtStamp checks that a run's start time is written as at= on its
// output: on the stdout fence or region, else on the stderr fence; and
// that a result with no start time is written without one.
func TestAtStamp(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\n```sh {#m out=markdown}\necho m\n```\n\n```sh {#e show-stderr}\necho e >&2\n```\n\n```sh {#both show-stderr}\necho o; echo e >&2; false\n```\n\n```sh {#old}\necho old\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 13, 19, 8, 0, 0, time.UTC)
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Start: at, Stdout: []byte("a\n")},
		{Block: d.Blocks[1], Ran: true, Start: at, Stdout: []byte("*m*\n")},
		{Block: d.Blocks[2], Ran: true, Start: at, Stderr: []byte("e\n")},
		{Block: d.Blocks[3], Ran: true, Start: at, Stdout: []byte("o\n"), Stderr: []byte("e\n"), Status: 1},
		{Block: d.Blocks[4], Ran: true, Stdout: []byte("old\n")},
	}
	got := string(Markdown(d, results, Options{WriteBack: true}))
	for _, want := range []string{
		"```output {for=a exit=0 at=2026-11-13T19:08:00Z}\na\n```\n",
		"<!-- mdsh:output for=\"m\" at=\"2026-11-13T19:08:00Z\" -->\n*m*\n<!-- /mdsh:output -->\n",
		"```output {for=e stream=stderr at=2026-11-13T19:08:00Z}\ne\n```\n",
		"```output {for=both exit=1 at=2026-11-13T19:08:00Z}\no\n```\n\n```output {for=both stream=stderr}\ne\n```\n",
		"```output {for=old exit=0}\nold\n```\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// The stamped file parses back with each output still its block's.
	again, err := doc.Parse([]byte(got))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Copies) != 0 {
		t.Errorf("stamped outputs read as %d copies", len(again.Copies))
	}
	for _, b := range again.Blocks {
		if !b.HasOutput() {
			t.Errorf("%s lost its output", b.ID)
		}
	}
}
