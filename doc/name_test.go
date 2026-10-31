package doc

import (
	"fmt"
	"strings"
	"testing"
)

func TestNameBlocks(t *testing.T) {
	src := "# T\n\n" +
		"```sh\necho hi\n```\n\n" +
		"```output {for=block-1 exit=0}\nhi\n```\n\n" +
		"It said {{#block-1/?:\"0\"}}.\n\n" +
		"<!-- mdsh:if {{#block-1/ran?}} -->\n" +
		"```sh {run={{#block-1/?}}==0 out=image}\nprintf img\n```\n\n" +
		"<!-- mdsh:output for=\"block-2\" -->\n![](d.md.assets/block-2.png)\n<!-- /mdsh:output -->\n" +
		"<!-- mdsh:end -->\n\n" +
		"```go\nx\n```\n\n" +
		"```sh {#named}\ntrue\n```\n\n" +
		"- ```sh\n  echo listed\n  ```\n\n" +
		"```sh {tty}\nsh\n```\n\n" +
		"```tape {for=block-6}\nType \"{{#block-1/?}}\"\n```\n\n" +
		"Again:\n\n<!-- mdsh:output for=\"block-2\" -->\n![](d.md.assets/block-2.png)\n<!-- /mdsh:output -->\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	blocks := append([]*Block(nil), d.Blocks...)
	n := 0
	renames, err := d.NameBlocks(func(b *Block) bool { return b.Lang == "sh" }, func() string {
		n++
		return fmt.Sprintf("block-x%d", n)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Rename{{"block-1", "block-x1"}, {"block-2", "block-x2"}, {"block-5", "block-x3"}, {"block-6", "block-x4"}}
	if fmt.Sprint(renames) != fmt.Sprint(want) {
		t.Errorf("renames = %v, want %v", renames, want)
	}
	wantSrc := "# T\n\n" +
		"```sh {#block-x1}\necho hi\n```\n\n" +
		"```output {for=block-x1 exit=0}\nhi\n```\n\n" +
		"It said {{#block-x1/?:\"0\"}}.\n\n" +
		"<!-- mdsh:if {{#block-x1/ran?}} -->\n" +
		"```sh {#block-x2 run={{#block-x1/?}}==0 out=image}\nprintf img\n```\n\n" +
		"<!-- mdsh:output for=\"block-x2\" -->\n![](d.md.assets/block-x2.png)\n<!-- /mdsh:output -->\n" +
		"<!-- mdsh:end -->\n\n" +
		"```go\nx\n```\n\n" +
		"```sh {#named}\ntrue\n```\n\n" +
		"- ```sh {#block-x3}\n  echo listed\n  ```\n\n" +
		"```sh {#block-x4 tty}\nsh\n```\n\n" +
		"```tape {for=block-x4}\nType \"{{#block-x1/?}}\"\n```\n\n" +
		"Again:\n\n<!-- mdsh:output for=\"block-x2\" -->\n![](d.md.assets/block-x2.png)\n<!-- /mdsh:output -->\n"
	if string(d.Src) != wantSrc {
		t.Errorf("src =\n%s\nwant\n%s", d.Src, wantSrc)
	}
	for i, b := range d.Blocks {
		if b != blocks[i] {
			t.Errorf("block %d lost its identity", i)
		}
	}
	if !d.Blocks[1].HasOutput() || len(d.Copies) != 1 || d.Copies[0].For != "block-x2" {
		t.Errorf("output or copy lost: %v, %d copies", d.Blocks[1].HasOutput(), len(d.Copies))
	}

	// Named blocks stay as they are.
	if renames, err := d.NameBlocks(func(b *Block) bool { return b.Lang == "sh" }, NewBlockID); err != nil || renames != nil {
		t.Errorf("second naming: %v, %v", renames, err)
	}
}

func TestNewBlockID(t *testing.T) {
	a, b := NewBlockID(), NewBlockID()
	if a == b || !GeneratedID(a) || !strings.HasPrefix(a, "block-") || len(a) != len("block-")+36 {
		t.Errorf("ids %q, %q", a, b)
	}
	for _, id := range []string{"block-3", "build", "block-xyz"} {
		if GeneratedID(id) {
			t.Errorf("GeneratedID(%q) = true", id)
		}
	}
}

func TestLabel(t *testing.T) {
	d, err := Parse([]byte("```sh\n```\n\n```sh {#build}\n```\n\n```sh {#" + NewBlockID() + "}\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range d.Blocks {
		got = append(got, d.Label(b))
	}
	if want := "at line 1|build|at line 7"; strings.Join(got, "|") != want {
		t.Errorf("labels = %q, want %q", strings.Join(got, "|"), want)
	}
}
