package report

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"

	"github.com/gdey/markdownshell/termrec"
)

// png1 is a 1x1 PNG.
var png1, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

const src = `---
title: t
---
# Report

Value: {{#a}}.

` + "```sh {#a}\necho hi\n```\n\n```sh {#b}\necho bad >&2; exit 3\n```\n\n![shot](shot.png) ![web](https://example.com/x.png)\n\n<script>alert(1)</script>\n\nSee <a href=\"https://example.com/\" onclick=\"x()\">the site</a>.\n\n<img src=\"shot.png\" alt=\"raw\" width=\"50\">\n"

func results(t *testing.T, d *doc.Document) []*run.Result {
	t.Helper()
	rs := make([]*run.Result, len(d.Blocks))
	for i, b := range d.Blocks {
		rs[i] = &run.Result{Block: b, Ran: true}
	}
	rs[0].Stdout = []byte("hi\n")
	rs[1].Stderr, rs[1].Status = []byte("bad\n"), 3
	return rs
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "shot.png"), png1, 0o644)
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	rs := results(t, d)

	md, err := Render(d, rs, Options{HideCode: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(md); strings.Contains(s, "echo hi") || !strings.Contains(s, "```output {for=a exit=0}\nhi\n```") {
		t.Errorf("markdown:\n%s", s)
	}

	page, err := Render(d, rs, Options{Format: HTML, Dir: dir, Title: "a <b>"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	for _, want := range []string{
		"<title>a &lt;b&gt;</title>",
		`<figure class="block"><figcaption>sh {#a}</figcaption><pre><code class="language-sh">echo hi`,
		`<figure class="output"><pre><code>hi`,
		`<figure class="output"><figcaption>exit 3</figcaption>`,
		`<figure class="output stderr"><pre><code>bad`,
		`src="data:image/png;base64,`,
		`src="https://example.com/x.png"`,
		`See <a href="https://example.com/">the site</a>.`,
		`<img src="data:image/png;base64,`,
		`alt="raw" width="50">`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "title: t") || strings.Contains(s, "<script>") || strings.Contains(s, "onclick") {
		t.Errorf("page has front matter or raw HTML:\n%s", s)
	}

	out, err := Render(d, rs, Options{Format: PDF, Dir: dir})
	if err != nil || !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Errorf("pdf: %v, %q", err, out[:min(len(out), 10)])
	}

	if _, err := Render(d, rs, Options{Format: "docx"}); err == nil {
		t.Error("docx: no error")
	}
}

func TestFormatOf(t *testing.T) {
	for path, want := range map[string]Format{"a.md": Markdown, "A.HTML": HTML, "x.htm": HTML, "r.pdf": PDF, "t.txt": ""} {
		if got, ok := FormatOf(path); got != want || ok != (want != "") {
			t.Errorf("FormatOf(%q) = %q, %v", path, got, ok)
		}
	}
	for _, f := range Formats {
		if got, _ := FormatOf("x" + f.Ext()); got != f {
			t.Errorf("%s: Ext %q gives %q", f, f.Ext(), got)
		}
	}
	if Title("/a/b/notes.md") != "notes" || Title("") != "" {
		t.Error("Title")
	}
}

// A tty block's cast goes into the page as a GIF made from it, so it
// plays without a script.
func TestInlineCast(t *testing.T) {
	dir := t.TempDir()
	var cast bytes.Buffer
	termrec.WriteCast(&cast, termrec.Recording{Cols: 10, Rows: 2, Events: []termrec.Event{{At: 0, Data: []byte("hi")}}})
	os.WriteFile(filepath.Join(dir, "r.cast"), cast.Bytes(), 0o644)
	if got := newImages(Options{Dir: dir}).inline("r.cast#frame=1"); !strings.HasPrefix(got, "data:image/gif;base64,") {
		t.Errorf("inline = %.40q, want a GIF data: URI", got)
	}
}

// A cast fence, a recording kept in the document, plays in the page as
// a GIF made from it, not as its JSON. One that isn't a recording shows
// as code.
func TestCastFence(t *testing.T) {
	var cast bytes.Buffer
	termrec.WriteCast(&cast, termrec.Recording{Cols: 10, Rows: 2, Events: []termrec.Event{{At: 0, Data: []byte("hi")}}})
	out, err := page([]byte("```cast {frame=1}\n"+cast.String()+"```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`<figure class="cast"><img alt="recording" src="data:image/gif;base64,`)) {
		t.Errorf("no GIF for the cast fence:\n%s", out)
	}
	if bytes.Contains(out, []byte("&quot;version&quot;")) {
		t.Error("the cast's JSON is in the page")
	}
	out, err = page([]byte("```cast\nnot a recording\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`<code class="language-cast">not a recording`)) {
		t.Errorf("a cast fence that isn't a recording isn't code:\n%s", out)
	}
}

// An image outside the document's folder stays a link, unless
// AllowOutside.
func TestInlineOutside(t *testing.T) {
	top := t.TempDir()
	dir := filepath.Join(top, "doc")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(top, "x.gif"), []byte("GIF89a"), 0o644)
	if got := newImages(Options{Dir: dir}).inline("../x.gif"); got != "" {
		t.Errorf("inline outside = %.40q, want it left as a link", got)
	}
	if got := newImages(Options{Dir: dir, AllowOutside: true}).inline("../x.gif"); !strings.HasPrefix(got, "data:image/gif") {
		t.Errorf("inline with AllowOutside = %.40q, want a data: URI", got)
	}
}

// An out=file output links its file from the caption, by the document's
// assets folder as seen from where the page is written.
func TestSpillLink(t *testing.T) {
	md := []byte("```output {for=b exit=2 truncated <include file=res://my build.log match=^FAIL}\nFAIL x\n```\n")
	md = []byte(strings.Replace(string(md), "res://my build.log", `"res://my build.log"`, 1))
	out, err := page(md, Options{File: "/w/docs/d.md", Out: "/w/out/r.html"})
	if err != nil {
		t.Fatal(err)
	}
	want := `<figcaption>exit 2 · <a href="../docs/d.md.assets/my%20build.log">my build.log</a> (cut at 16 MB)</figcaption>`
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("no link to the log:\n%s", out)
	}
	out, _ = page([]byte("```output {for=b exit=0 <include file=logs/b.txt}\nok\n```\n"), Options{File: "d.md"})
	if !bytes.Contains(out, []byte(`<figcaption><a href="logs/b.txt">logs/b.txt</a></figcaption>`)) {
		t.Errorf("no link to logs/b.txt:\n%s", out)
	}
}
