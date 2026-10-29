package report

import (
	"bytes"
	"encoding/base64"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// exitRE finds an output fence's exit status.
var exitRE = regexp.MustCompile(`\bexit=(\d+)`)

// page lays md out as a standalone HTML page. Raw HTML in md is left
// out, as GitHub leaves out what it doesn't allow, and local images are
// put inside the page as data: URIs, so the file can be sent on its own.
func page(md []byte, opts Options) ([]byte, error) {
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(images{opts.Dir}, 100))),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(fences{}, 100))),
	)
	var body bytes.Buffer
	if err := gm.Convert(stripFrontMatter(md), &body); err != nil {
		return nil, err
	}
	title := opts.Title
	if title == "" {
		title = "mdsh report"
	}
	var b bytes.Buffer
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<meta name=\"generator\" content=\"mdsh\">\n")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>\n<style>\n" + css + "</style>\n</head>\n<body>\n<main>\n")
	b.Write(body.Bytes())
	b.WriteString("</main>\n</body>\n</html>\n")
	return b.Bytes(), nil
}

// css styles the page like the PDF: runnable blocks in a light box
// captioned with their info string, output in a dark one, stderr in red.
const css = `body { margin: 0; background: #fff; color: #1e1e1e; font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; }
main { max-width: 860px; margin: 0 auto; padding: 32px 16px 64px; }
h1, h2 { border-bottom: 1px solid #d8d8dc; padding-bottom: .25em; }
a { color: #0a5bd3; }
code, pre { font: 13px/1.45 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
:not(pre) > code { background: #f0f0f3; border-radius: 4px; padding: .1em .3em; }
pre { margin: 0; padding: 10px 12px; overflow-x: auto; }
figure { margin: 1em 0; border-radius: 6px; overflow: hidden; background: #f4f4f6; }
figcaption { padding: 6px 12px 0; color: #8a8a8e; font: 12px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
figure.output { background: #1e1e1e; color: #d4d4d4; }
figure.stderr { background: #3c1414; color: #ffc8c8; }
figure.output figcaption { color: #ff8a8a; }
table { border-collapse: collapse; margin: 1em 0; }
th, td { border: 1px solid #d8d8dc; padding: 4px 10px; }
th { background: #f4f4f6; }
blockquote { margin: 1em 0; padding: 0 1em; border-left: 4px solid #d8d8dc; color: #555; }
img { max-width: 100%; }
@media print { main { max-width: none; padding: 0; } figure { break-inside: avoid; } }
`

// fences draws fenced code blocks as figures, captioned like the PDF.
type fences struct{}

func (fences) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		f := n.(*ast.FencedCodeBlock)
		lang := string(f.Language(src))
		info := ""
		if f.Info != nil {
			info = strings.TrimSpace(string(f.Info.Segment.Value(src)))
		}
		class, caption := "block", info
		var body bytes.Buffer
		for i := 0; i < f.Lines().Len(); i++ {
			s := f.Lines().At(i)
			body.Write(s.Value(src))
		}
		if lang == "output" {
			class, caption = "output", ""
			if strings.Contains(info, "stream=stderr") {
				class = "output stderr"
			}
			// Only a failing status is worth a caption.
			if m := exitRE.FindStringSubmatch(info); m != nil && m[1] != "0" {
				caption = "exit " + m[1]
			}
			if strings.TrimSpace(body.String()) == "" {
				body.Reset()
			}
		}
		w.WriteString(`<figure class="` + class + `">`)
		if caption != "" {
			w.WriteString("<figcaption>" + html.EscapeString(caption) + "</figcaption>")
		}
		w.WriteString("<pre><code")
		if lang != "" && lang != "output" {
			w.WriteString(` class="language-` + html.EscapeString(lang) + `"`)
		}
		w.WriteString(">" + html.EscapeString(body.String()) + "</code></pre></figure>\n")
		return ast.WalkSkipChildren, nil
	})
}

// images puts local images into the page as data: URIs, read from dir.
// Remote links and files it can't read are left as they are.
type images struct{ dir string }

func (t images) Transform(root *ast.Document, _ text.Reader, _ parser.Context) {
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if img, ok := n.(*ast.Image); ok && entering {
			if uri := t.inline(string(img.Destination)); uri != "" {
				img.Destination = []byte(uri)
			}
		}
		return ast.WalkContinue, nil
	})
}

func (t images) inline(dest string) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return ""
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.dir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	typ := ""
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		typ = "image/png"
	case bytes.HasPrefix(data, []byte("\xff\xd8")):
		typ = "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		// A recording's #frame= is for still pages; here it plays.
		typ = "image/gif"
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) > 12 && string(data[8:12]) == "WEBP":
		typ = "image/webp"
	case bytes.Contains(data[:min(len(data), 512)], []byte("<svg")):
		typ = "image/svg+xml"
	default:
		return ""
	}
	return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data)
}
