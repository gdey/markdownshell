package rawhtml

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// testImage loads local images from /doc, and puts a note in place of
// remote ones, as mdsh-gui does.
func testImage(src, alt string) (string, string) {
	if strings.Contains(src, "://") {
		return "", "<em>not loaded: " + alt + "</em>"
	}
	return "/doc/" + src, ""
}

func render(t *testing.T, md string) string {
	t.Helper()
	gm := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(Option(Options{Image: testImage})))
	var buf bytes.Buffer
	if err := gm.Convert([]byte(md), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// What CommonMark lets a document write stays.
func TestKept(t *testing.T) {
	for _, tc := range []struct{ md, want string }{
		{`see <a href="https://example.com/x?a=1&amp;b=2">the site</a> now`, `<p>see <a href="https://example.com/x?a=1&amp;b=2">the site</a> now</p>`},
		{`<a href="other.md#setup" title="Setup">setup</a>`, `<a href="other.md#setup" title="Setup">setup</a>`},
		{`<a href="#install">up</a>`, `<a href="#install">up</a>`},
		{`<a href="mailto:me@example.com">mail</a>`, `<a href="mailto:me@example.com">mail</a>`},
		{`press <kbd>Ctrl</kbd>+<kbd>C</kbd>`, `press <kbd>Ctrl</kbd>+<kbd>C</kbd>`},
		{`H<sub>2</sub>O and x<sup>2</sup>`, `H<sub>2</sub>O and x<sup>2</sup>`},
		{"line<br>break", "line<br>break"},
		{"<details open>\n<summary>More</summary>\n\nhidden\n\n</details>\n", "<details open=\"\">\n<summary>More</summary>"},
		{"<p align=\"center\">\n  centered\n</p>\n", "<p align=\"center\">\n  centered\n</p>"},
		{`<img src="plot.png" alt="A plot" width="300">`, `<img src="/doc/plot.png" alt="A plot" width="300">`},
		{`<a href="big.png"><img src="small.png" alt="x"></a>`, `<a href="big.png"><img src="/doc/small.png" alt="x"></a>`},
		{`<img src="https://example.com/p.png" alt="Logo">`, `<em>not loaded: Logo</em>`},
		{"<img src=\"plan.png\" usemap=\"#rooms\" alt=\"Plan\">\n<map name=\"rooms\">\n<area shape=\"rect\" coords=\"0,0,10,10\" href=\"kitchen.md\" alt=\"Kitchen\">\n<area shape=\"circle\" coords=\"20,20,5\" href=\"https://example.com\" alt=\"Web\">\n</map>\n",
			"<img src=\"/doc/plan.png\" usemap=\"#rooms\" alt=\"Plan\">\n<map name=\"rooms\">\n<area shape=\"rect\" coords=\"0,0,10,10\" href=\"kitchen.md\" alt=\"Kitchen\">\n<area shape=\"circle\" coords=\"20,20,5\" href=\"https://example.com\" alt=\"Web\">\n</map>"},
	} {
		if got := render(t, tc.md); !strings.Contains(got, tc.want) {
			t.Errorf("%q:\n got %q\nwant %q in it", tc.md, got, tc.want)
		}
	}
}

// What could run, load, or reach the page around it goes.
func TestDropped(t *testing.T) {
	for _, md := range []string{
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="JaVaScRiPt:alert(1)">x</a>`,
		`<a href="java&#x09;script:alert(1)">x</a>`,
		`<a href="vbscript:x">x</a>`,
		`<a href="data:text/html,<script>alert(1)</script>">x</a>`,
		`<a href="file:///etc/passwd">x</a>`,
		`<a href="https://example.com" onclick="alert(1)">x</a>`,
		`<a href="https://example.com" target="_top">x</a>`,
		`<span onmouseover="alert(1)">x</span>`,
		`<span style="position:fixed;inset:0">x</span>`,
		`<span id="doc" class="run-section" name="n">x</span>`,
		`<img src="javascript:alert(1)">`,
		`<img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=">`,
		`<img src="file:///etc/passwd.png">`,
		`<img src=x onerror=alert(1) style="width:100vw" class=c id=i name=n>`,
		`<img src=x usemap="javascript:alert(1)">`,
		`<map name="x y" id="doc"><area href="javascript:alert(1)" onclick="x()" target=_top></map>`,
		`<map name="m"><area href="data:text/html,x"></map>`,
		"<script>alert(1)</script>\n",
		"<style>body{display:none}</style>\n",
		`<iframe src="https://example.com"></iframe>`,
		`<object data="x"></object>`,
		`<embed src="x">`,
		`<form action="https://example.com"><input name=a><button>go</button></form>`,
		`<svg onload=alert(1)><circle/></svg>`,
		`<math><mi xlink:href="javascript:alert(1)">x</mi></math>`,
		`<meta http-equiv="refresh" content="0;url=https://example.com">`,
		`<base href="https://example.com/">`,
		`<link rel=stylesheet href="https://example.com/x.css">`,
		"<!-- a comment -->\n",
		`<textarea>x</textarea>`,
	} {
		got := strings.ToLower(render(t, md))
		if handler.MatchString(got) {
			t.Errorf("%q: event handler kept: %q", md, got)
		}
		for _, bad := range []string{"javascript", "vbscript", "data:", "file:", "style=", "<style", "<script", "<iframe", "<object", "<embed", "<form", "<input", "<button", "<svg", "<math", "<meta", "<base", "<link", "<textarea", "target=", "id=", "class=", `name="n"`, `name="x y"`, "<!--"} {
			if strings.Contains(got, bad) {
				t.Errorf("%q: %q kept: %q", md, bad, got)
			}
		}
	}
}

var handler = regexp.MustCompile(`\son[a-z]+\s*=`)

// A markdown link with a script: URL is still blanked, as goldmark does.
func TestMarkdownLinkUnchanged(t *testing.T) {
	if got := render(t, "[x](javascript:alert(1))"); strings.Contains(got, "javascript") {
		t.Errorf("got %q", got)
	}
}
