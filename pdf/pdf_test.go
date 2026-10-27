package pdf

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/textimg"
)

func pngBytes(t *testing.T, w int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, 20))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.png"), pngBytes(t, 40), 0o644)
	os.WriteFile(filepath.Join(dir, "shot.svg"), textimg.SVG("$ ls\nfile"), 0o644)
	os.WriteFile(filepath.Join(dir, "other.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644)
	md := "---\nmdsh:\n  shell: sh\n---\n" +
		"# Title ünïcode\n\nSome *em*, **strong**, `code`, ~~gone~~, [link](https://example.com), <https://x.org>.\n\n" +
		"> quote\n\n- a\n  - nested\n- [x] done\n\n3. three\n4. four\n\n| a | b |\n|:-|-:|\n| 1 | 2 |\n\n---\n\n" +
		"```sh {#a}\necho " + strings.Repeat("long ", 40) + "\n```\n\n" +
		"```output {for=a exit=1}\nhi\n```\n\n```output {for=a stream=stderr}\nbad\n```\n\n" +
		"<!-- mdsh:output for=b -->\n![a](a.png) ![s](shot.svg) ![o](other.svg) ![m](missing.png) ![r](https://x.org/r.png)\n" +
		"![d](data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t, 30)) + ")\n<!-- /mdsh:output -->\n"
	out, err := Render([]byte(md), Options{Dir: dir, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", out[:min(len(out), 20)])
	}
	// a.png and the data: URI are drawn; the SVGs, missing and remote
	// images are not. fpdf stores identical images once, so they differ.
	if n := bytes.Count(out, []byte("/Subtype /Image")); n != 2 {
		t.Errorf("%d images in the PDF, want 2", n)
	}
}

func TestRenderPages(t *testing.T) {
	out, err := Render([]byte(strings.Repeat("A paragraph.\n\n", 200)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(out, []byte("/Type /Page\n")); n < 2 {
		t.Errorf("%d pages, want several", n)
	}
}

func TestStripFrontMatter(t *testing.T) {
	for in, want := range map[string]string{
		"---\na: 1\n---\n# x\n": "# x\n",
		"---\na: 1\n...\nbody":  "body",
		"---\nnot closed\n":     "---\nnot closed\n",
		"# x\n---\n":            "# x\n---\n",
	} {
		if got := string(stripFrontMatter([]byte(in))); got != want {
			t.Errorf("stripFrontMatter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNumberedGutter(t *testing.T) {
	if got := numberedGutter([]string{" 9  a", "10  b"}, 9); got != 4 {
		t.Errorf("gutter = %d, want 4", got)
	}
	if got := numberedGutter([]string{"9  a", "b"}, 9); got != 0 {
		t.Errorf("gutter = %d for unnumbered lines, want 0", got)
	}
}

func TestIncludeLabel(t *testing.T) {
	for inc, want := range map[doc.Include]string{
		{File: "a.go"}:                 "a.go",
		{File: "a.go", From: 4, To: 6}: "a.go, lines 4–6",
		{File: "a.go", From: 8}:        "a.go, from line 8",
		{File: "a.go", To: 3}:          "a.go, lines 1–3",
	} {
		if got := includeLabel(inc); got != want {
			t.Errorf("includeLabel(%+v) = %q, want %q", inc, got, want)
		}
	}
}

func TestRenderInclude(t *testing.T) {
	md := "```go {<include file=a.go from=9 to=10 number_on}\n 9  a\n10  b\n```\n"
	if _, err := Render([]byte(md), Options{}); err != nil {
		t.Fatal(err)
	}
}
