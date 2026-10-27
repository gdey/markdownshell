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
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/termrec"
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

func TestLoadGIFFrame(t *testing.T) {
	dir := t.TempDir()
	data, err := termrec.GIF(termrec.Recording{Cols: 10, Rows: 2, Events: []termrec.Event{
		{At: 0, Data: []byte("first")},
		{At: time.Second, Data: []byte("\x1b[2J\x1b[Hsecond")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "s.gif"), data, 0o644)
	r := &renderer{dir: dir}
	for _, tt := range []struct {
		dest  string
		frame int
	}{{"s.gif", 0}, {"s.gif#frame=1", 1}, {"s.gif#frame=2", 2}, {"s.gif#frame=9", 0}} {
		got, typ := r.load(tt.dest)
		if typ != "PNG" {
			t.Fatalf("%s: type %q, want a PNG of one frame", tt.dest, typ)
		}
		img, err := png.Decode(bytes.NewReader(got))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := termrec.Frame(data, tt.frame)
		if !sameImage(img, double(want)) {
			t.Errorf("%s: not frame %d", tt.dest, tt.frame)
		}
	}
	first, _ := termrec.Frame(data, 1)
	if last, _ := termrec.Frame(data, 0); sameImage(first, last) {
		t.Fatal("frames are the same; the test can't tell them apart")
	}
}

func sameImage(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				return false
			}
		}
	}
	return true
}
