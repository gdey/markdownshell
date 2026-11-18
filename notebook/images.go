package notebook

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/docroot"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// DocPrefix starts the URL of a file beside a document, as the window's
// asset server serves it: DocPrefix, then the file's absolute path.
const DocPrefix = "/doc"

// DocURL is the URL the window loads the file at path from. The file's
// modification time is added, so an image a run writes again is loaded
// again.
func DocURL(path string) string {
	u := url.URL{Path: DocPrefix + filepath.ToSlash(path)}
	if fi, err := os.Stat(path); err == nil {
		u.RawQuery = fmt.Sprintf("v=%d", fi.ModTime().UnixNano())
	}
	return u.String()
}

// docImages points images with a relative link, such as an out=image
// block's output or ![](plot.png) in the prose, at the file beside the
// document. Images from elsewhere, such as https: or file: links, are
// not loaded: drawing them would tell whoever serves them that the file
// was opened, and placeholders in the link could send them the user's
// environment. A note with the alt text and host is drawn instead. Data:
// URIs and absolute paths, which the window serves itself, are left as
// they are.
type docImages struct{ n *Notebook }

// Transform runs while Cells holds n.mu, so it reads n.file directly.
func (t docImages) Transform(root *ast.Document, reader text.Reader, _ parser.Context) {
	dir := filepath.Dir(t.n.file)
	type note struct {
		img   *ast.Image
		where string
	}
	var notes []note
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := node.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		link := string(img.Destination)
		if u, where := imageURL(dir, link); u != "" {
			img.Destination = []byte(u)
		} else if where != "" {
			notes = append(notes, note{img, where})
		}
		return ast.WalkContinue, nil
	})
	for _, r := range notes {
		r.img.Parent().ReplaceChild(r.img.Parent(), r.img, notLoaded(r.img, reader.Source(), r.where))
	}
}

// imageURL is where the window loads the image link names, in a
// document in dir, from: the window's /doc route for a file in dir or
// under it. For an image it doesn't load, it returns "" and where it is,
// for the note: "from" a host for one from another site, or "outside"
// for a file outside dir, or hidden in it, which the window's /doc route
// doesn't serve. A data: URI, which the window shows itself, and a link
// that is neither, come back as they are.
func imageURL(dir, link string) (u, where string) {
	if path, ok := localImage(dir, link); ok {
		if !insideDir(dir, path) {
			return "", "outside"
		}
		u := DocURL(path)
		// A recording's #frame=N is the frame its player opens at.
		if _, frag, ok := strings.Cut(link, "#"); ok {
			u += "#" + frag
		}
		return u, ""
	}
	if host, ok := remoteImage(link); ok {
		return "", "from " + host
	}
	return link, ""
}

// localImage is the file an image link in a document in dir names: a
// relative path, which is beside the document, an absolute one, or a
// file: URL.
func localImage(dir, link string) (string, bool) {
	if path, ok := beside(dir, link); ok {
		return path, true
	}
	u, err := url.Parse(link)
	if err != nil || u.Path == "" || !strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	if (u.Scheme == "" && u.Host == "" && !strings.HasPrefix(link, "//")) || (strings.EqualFold(u.Scheme, "file") && (u.Host == "" || u.Host == "localhost")) {
		return filepath.FromSlash(u.Path), true
	}
	return "", false
}

// insideDir reports whether path is in dir or under it, and not hidden
// there, as the window's /doc route serves it.
func insideDir(dir, path string) bool {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	_, err := docroot.Resolve(dir, path)
	return err == nil
}

// remoteImage reports whether link is an image the window would fetch
// from somewhere other than itself, and the host it names, if any.
func remoteImage(link string) (string, bool) {
	u, err := url.Parse(link)
	if err != nil {
		// A link Go can't parse may still be one the webview fetches.
		return "", true
	}
	switch {
	case strings.EqualFold(u.Scheme, "data"):
		return "", false
	case u.Scheme != "" || u.Host != "" || strings.HasPrefix(link, "//"):
		return u.Hostname(), true
	}
	return "", false
}

// notLoaded is the note drawn in place of img, an image the window
// doesn't load by itself: one from another site, or a file outside the
// document's folder. One the window may load when asked carries its link
// and alt text as data-src and data-alt, for the note's Load button.
func notLoaded(img *ast.Image, src []byte, where string) ast.Node {
	alt := string(nodeText(img, src))
	link := string(img.Destination)
	e := ast.NewEmphasis(1)
	e.SetAttributeString("class", []byte("remote-image"))
	e.SetAttributeString("title", []byte(notLoadedTitle(where)))
	if loadable(link, where) {
		e.SetAttributeString("data-src", []byte(link))
		e.SetAttributeString("data-alt", []byte(alt))
	}
	e.AppendChild(e, ast.NewString([]byte(notLoadedText(alt, where))))
	return e
}

// loadable reports whether link, an image the window didn't load, is one
// it may load when the user asks: an http: or https: address with a
// host, or a file outside the document's folder.
func loadable(link, where string) bool {
	if where == "outside" {
		return true
	}
	u, err := url.Parse(link)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// notLoadedTitle is the note's tooltip.
func notLoadedTitle(where string) string {
	if where == "outside" {
		return "mdsh-gui doesn't load images from outside the document's folder until you ask"
	}
	return "mdsh-gui doesn't load images from other sites until you ask"
}

// notLoadedText is what the note for an image says.
func notLoadedText(alt, where string) string {
	what := "Image not loaded"
	if alt = strings.TrimSpace(alt); alt != "" {
		what += ": " + alt
	}
	switch {
	case where == "outside":
		what += " (outside this document's folder)"
	case where != "from ":
		what += " (" + where + ")"
	}
	return what
}

// rawImage is what an <img> written in the document's HTML loads, by
// the rules docImages holds a markdown image to, with the same note in
// place of one it doesn't load. It runs while Cells holds n.mu.
func (n *Notebook) rawImage(src, alt string) (string, string) {
	u, where := imageURL(filepath.Dir(n.file), src)
	if u != "" || where == "" {
		return u, ""
	}
	data := ""
	if loadable(src, where) {
		data = ` data-src="` + html.EscapeString(src) + `" data-alt="` + html.EscapeString(alt) + `"`
	}
	return "", `<em class="remote-image" title="` + html.EscapeString(notLoadedTitle(where)) + `"` + data + `>` + html.EscapeString(notLoadedText(alt, where)) + "</em>"
}

// nodeText is the plain text under n, such as an image's alt text.
func nodeText(n ast.Node, src []byte) []byte {
	var b []byte
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b = append(b, c.Segment.Value(src)...)
		case *ast.String:
			b = append(b, c.Value...)
		}
		return ast.WalkContinue, nil
	})
	return b
}

// beside is the file a relative link in the document at dir names.
func beside(dir, link string) (string, bool) {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	return filepath.Join(dir, filepath.FromSlash(u.Path)), true
}

func (n *Notebook) imageTransformer() parser.Option {
	return parser.WithASTTransformers(util.Prioritized(docImages{n}, 500))
}

// OutsideImage is the file link, an image in the document at file,
// names when that is outside the document's folder, or hidden in it: an
// image the window shows only when the user asks.
func OutsideImage(file, link string) (string, bool) {
	dir := filepath.Dir(file)
	path, ok := localImage(dir, link)
	if !ok || insideDir(dir, path) {
		return "", false
	}
	return path, true
}
