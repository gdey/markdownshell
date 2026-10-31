package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func TestNameBlocks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	// block-1's picture was saved by an earlier run; block-2 runs now.
	src := "```sh {out=screenshot}\necho old\n```\n\n" +
		"<!-- mdsh:output for=\"block-1\" -->\n![](d.md.assets/block-1.svg)\n<!-- /mdsh:output -->\n\n" +
		"```sh {out=screenshot}\necho new\n```\n\n" +
		"```sh {#keep}\ntrue\n```\n"
	os.MkdirAll(filepath.Join(dir, "d.md.assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "d.md.assets", "block-1.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(dir, "d.md.assets", "block-10.svg"), []byte("<svg/>"), 0o644)
	opts := run.Options{File: file, Dir: dir, Only: "block-2"}
	d, results := runDoc(t, src, opts)
	if results[1].Image != "d.md.assets/block-2.svg" {
		t.Fatalf("image = %q", results[1].Image)
	}

	renames, err := run.NameBlocks(d, results, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(renames) != 2 || !doc.GeneratedID(d.Blocks[0].ID) || !doc.GeneratedID(d.Blocks[1].ID) || d.Blocks[2].ID != "keep" {
		t.Fatalf("renames %v, ids %s %s %s", renames, d.Blocks[0].ID, d.Blocks[1].ID, d.Blocks[2].ID)
	}
	one, two := d.Blocks[0].ID, d.Blocks[1].ID
	for _, id := range []string{one, two} {
		if _, err := os.Stat(filepath.Join(dir, "d.md.assets", id+".svg")); err != nil {
			t.Errorf("%s.svg: %v", id, err)
		}
	}
	for _, name := range []string{"block-1.svg", "block-2.svg"} {
		if _, err := os.Stat(filepath.Join(dir, "d.md.assets", name)); err == nil {
			t.Errorf("%s is still there", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "d.md.assets", "block-10.svg")); err != nil {
		t.Errorf("block-10.svg was moved: %v", err)
	}
	if results[1].Image != "d.md.assets/"+two+".svg" {
		t.Errorf("image = %q", results[1].Image)
	}
	out := string(render.Markdown(d, results, render.Options{WriteBack: true}))
	for _, want := range []string{
		"```sh {#" + one + " out=screenshot}",
		"<!-- mdsh:output for=\"" + one + "\" -->\n![",
		"](d.md.assets/" + one + ".svg)",
		"<!-- mdsh:output for=\"" + two + "\" -->\n![screenshot](d.md.assets/" + two + ".svg)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("saved document lacks %q:\n%s", want, out)
		}
	}
}
