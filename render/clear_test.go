package render

import (
	"testing"

	"github.com/gdey/markdownshell/doc"
)

func TestClear(t *testing.T) {
	// What mdsh -w wrote after a run of a document with output after its
	// blocks, nested ones too, copies and placeholders.
	ran := "# T\n\n```sh {#hello}\necho hello; X=42\n```\n\n```output {for=hello exit=0}\nhello\n```\n\n" +
		"Value: {{#hello/env(X):\"42\"}} status {{$?:\"0\"}} home {{$HOME:\"/root\"}}\n\n" +
		"- item\n\n  ```sh {#md out=markdown}\n  echo '**bold**'\n  ```\n\n  <!-- mdsh:output for=\"md\" -->\n  **bold**\n  <!-- /mdsh:output -->\n\n" +
		"> ```sh {#err}\n> echo oops >&2; exit 2\n> ```\n>\n> ```output {for=err exit=2 exited}\n> ```\n>\n> ```output {for=err stream=stderr}\n> oops\n> ```\n\n" +
		"Copy:\n\n```output {for=hello exit=0}\nhello\n```\n\n<!-- mdsh:output for=\"md\" -->\n**bold**\n<!-- /mdsh:output -->\n\n" +
		"> ```output {for=err stream=stderr}\n> oops\n> ```\n"
	// The env placeholder keeps its value: it is the caller's, not a
	// block's, and every save writes it again anyway.
	want := "# T\n\n```sh {#hello}\necho hello; X=42\n```\n\n" +
		"Value: {{#hello/env(X)}} status {{$?}} home {{$HOME:\"/root\"}}\n\n" +
		"- item\n\n  ```sh {#md out=markdown}\n  echo '**bold**'\n  ```\n\n" +
		"> ```sh {#err}\n> echo oops >&2; exit 2\n> ```\n\n" +
		"Copy:\n\n```output {for=hello}\n```\n\n<!-- mdsh:output for=\"md\" -->\n<!-- /mdsh:output -->\n\n" +
		"> ```output {for=err stream=stderr}\n> ```\n"
	d, err := doc.Parse([]byte(ran))
	if err != nil {
		t.Fatal(err)
	}
	got := string(Clear(d))
	if got != want {
		t.Fatalf("Clear:\n%s\nwant:\n%s", got, want)
	}
	if err := d.SetSource([]byte(got)); err != nil {
		t.Fatal(err)
	}
	if again := string(Clear(d)); again != got {
		t.Errorf("Clear of a cleared document changed it:\n%s", again)
	}
}

func TestClearKeeps(t *testing.T) {
	// An include's lines, and a copy of no block.
	src := "```sh {<include file=a.sh}\necho hi\n```\n\n```output {for=nobody}\nold\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Clear(d)); got != src {
		t.Errorf("Clear changed what it should keep:\n%s", got)
	}
}
