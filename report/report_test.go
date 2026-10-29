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
)

// png1 is a 1x1 PNG.
var png1, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

const src = `---
title: t
---
# Report

Value: {{#a}}.

` + "```sh {#a}\necho hi\n```\n\n```sh {#b}\necho bad >&2; exit 3\n```\n\n![shot](shot.png) ![web](https://example.com/x.png)\n\n<script>alert(1)</script>\n"

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
	} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "title: t") || strings.Contains(s, "<script>") {
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
