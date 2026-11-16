// Package rawhtml lets the HTML written in a document's markdown through
// to the viewers, cleaned. CommonMark allows raw HTML, such as a link
// written as <a href="...">, <kbd>, <sub> or <details>; goldmark in its
// safe mode leaves it all out. Here each piece of raw HTML goes through
// an allowlist instead, as GitHub cleans a README: text formatting,
// links, lists, tables and the like stay, with only the attributes that
// can't run anything or change the page around them.
//
// Images and image maps (<img usemap>, <map>, <area>) stay too, but what
// an image loads is up to the viewer, through Options.Image, as it is
// for a markdown image.
//
// What never gets through: scripts, styles and event handlers; forms,
// frames and embeds; id, class and style attributes, and name anywhere
// but on a <map>, which could reach the viewer's own elements and CSS;
// and links and image sources other than http:, https:, mailto: and
// relative ones.
package rawhtml

import (
	"bytes"
	"io"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// policy is the allowlist. It is built once and is safe to share.
var policy = newPolicy()

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	// http:, https:, mailto: and relative links only, and only ones that
	// parse.
	p.AllowStandardURLs()
	// The viewers open a link themselves; rel="nofollow" is for crawlers.
	p.RequireNoFollowOnLinks(false)
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("cite").OnElements("blockquote", "q", "del", "ins")
	p.AllowAttrs("title").Globally()
	p.AllowAttrs("lang").Matching(regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)).Globally()
	p.AllowAttrs("dir").Matching(regexp.MustCompile(`(?i)^(rtl|ltr|auto)$`)).Globally()
	p.AllowAttrs("align").Matching(regexp.MustCompile(`(?i)^(left|center|right|justify)$`)).OnElements("p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "td", "th")
	p.AllowAttrs("open").Matching(regexp.MustCompile(`(?i)^(|open)$`)).OnElements("details")
	p.AllowAttrs("start").Matching(bluemonday.Integer).OnElements("ol")
	p.AllowAttrs("reversed").Matching(regexp.MustCompile(`(?i)^(|reversed)$`)).OnElements("ol")
	p.AllowAttrs("colspan", "rowspan").Matching(bluemonday.Integer).OnElements("td", "th")
	p.AllowAttrs("datetime").Matching(bluemonday.ISO8601).OnElements("time", "del", "ins")
	// Images, and image maps: a map's name is what an image's usemap
	// names, and each area is a link.
	p.AllowAttrs("src").OnElements("img")
	p.AllowAttrs("alt").Matching(bluemonday.Paragraph).OnElements("img", "area")
	p.AllowAttrs("width", "height").Matching(bluemonday.NumberOrPercent).OnElements("img")
	p.AllowAttrs("align").Matching(bluemonday.ImageAlign).OnElements("img")
	p.AllowAttrs("usemap").Matching(regexp.MustCompile(`^#[A-Za-z][A-Za-z0-9_-]*$`)).OnElements("img")
	p.AllowAttrs("name").Matching(regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)).OnElements("map")
	p.AllowAttrs("href").OnElements("area")
	p.AllowAttrs("shape").Matching(regexp.MustCompile(`(?i)^(rect|circle|poly|default)$`)).OnElements("area")
	p.AllowAttrs("coords").Matching(regexp.MustCompile(`^[0-9., ]*$`)).OnElements("area")
	p.AllowElements(
		"a", "abbr", "b", "bdi", "br", "cite", "code", "dfn", "em", "i", "kbd", "mark",
		"q", "rp", "rt", "ruby", "s", "samp", "small", "span", "strike", "strong", "sub",
		"sup", "time", "u", "var", "wbr", "del", "ins",
		"p", "div", "blockquote", "pre", "hr", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "dl", "dt", "dd", "details", "summary", "figure", "figcaption",
		"table", "caption", "thead", "tbody", "tfoot", "tr", "th", "td",
		"img", "map", "area",
	)
	return p
}

// Clean returns s with everything the allowlist doesn't allow taken out.
// Images are left as they are written; Options.Image is what decides
// what they load.
func Clean(s []byte) []byte {
	return policy.SanitizeBytes(s)
}

// Options are the viewer's say over raw HTML.
type Options struct {
	// Image is given each <img>'s src and alt, cleaned, and returns the
	// src the image should load. When it returns "", the image is left
	// out, and replace, HTML the viewer trusts, such as a note that the
	// image wasn't loaded, is put in its place. Nil leaves out every
	// image.
	Image func(src, alt string) (load string, replace string)
}

// Option renders a document's raw HTML cleaned, in place of goldmark's
// "raw HTML omitted". Pass it to goldmark.WithRendererOptions; the
// renderer must not be in unsafe mode, which this replaces.
func Option(o Options) renderer.Option {
	return renderer.WithNodeRenderers(util.Prioritized(nodeRenderer{o}, 100))
}

// write writes raw cleaned, with its images as o.Image has them.
func (o Options) write(w io.Writer, raw []byte) {
	clean := Clean(raw)
	if !bytes.Contains(clean, []byte("<img")) {
		_, _ = w.Write(clean)
		return
	}
	z := html.NewTokenizer(bytes.NewReader(clean))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return
		}
		raw := z.Raw()
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			_, _ = w.Write(raw)
			continue
		}
		t := z.Token()
		if t.Data != "img" {
			_, _ = w.Write(raw)
			continue
		}
		var src, alt string
		for _, a := range t.Attr {
			switch a.Key {
			case "src":
				src = a.Val
			case "alt":
				alt = a.Val
			}
		}
		load, replace := "", ""
		if o.Image != nil && src != "" {
			load, replace = o.Image(src, alt)
		}
		if load == "" {
			_, _ = io.WriteString(w, replace)
			continue
		}
		for i := range t.Attr {
			if t.Attr[i].Key == "src" {
				t.Attr[i].Val = load
			}
		}
		_, _ = io.WriteString(w, t.String())
	}
}

type nodeRenderer struct{ o Options }

func (r nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, r.renderRaw)
	reg.Register(ast.KindHTMLBlock, r.renderBlock)
}

// renderRaw writes an inline piece of HTML, such as <a href="..."> or
// </a>, cleaned. Each tag is a node of its own, cleaned alone: a tag the
// allowlist drops leaves its closing tag behind, which browsers ignore.
func (r nodeRenderer) renderRaw(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	var raw []byte
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		raw = append(raw, seg.Value(src)...)
	}
	r.o.write(w, raw)
	return ast.WalkSkipChildren, nil
}

// renderBlock writes an HTML block, its lines and its closing line,
// cleaned as one.
func (r nodeRenderer) renderBlock(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	var raw []byte
	for i := 0; i < n.Lines().Len(); i++ {
		line := n.Lines().At(i)
		raw = append(raw, line.Value(src)...)
	}
	if n.HasClosure() {
		raw = append(raw, n.ClosureLine.Value(src)...)
	}
	r.o.write(w, raw)
	return ast.WalkContinue, nil
}
