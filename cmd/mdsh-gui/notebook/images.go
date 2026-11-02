package notebook

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
		img  *ast.Image
		host string
	}
	var remote []note
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := node.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		link := string(img.Destination)
		if path, ok := beside(dir, link); ok {
			img.Destination = []byte(DocURL(path))
		} else if host, ok := remoteImage(link); ok {
			remote = append(remote, note{img, host})
		}
		return ast.WalkContinue, nil
	})
	for _, r := range remote {
		r.img.Parent().ReplaceChild(r.img.Parent(), r.img, notLoaded(r.img, reader.Source(), r.host))
	}
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

// notLoaded is the note drawn in place of img, a remote image.
func notLoaded(img *ast.Image, src []byte, host string) ast.Node {
	what := "Image not loaded"
	if alt := strings.TrimSpace(string(nodeText(img, src))); alt != "" {
		what += ": " + alt
	}
	if host != "" {
		what += " (from " + host + ")"
	}
	e := ast.NewEmphasis(1)
	e.SetAttributeString("class", []byte("remote-image"))
	e.SetAttributeString("title", []byte("mdsh-gui doesn't load images from other sites"))
	e.AppendChild(e, ast.NewString([]byte(what)))
	return e
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
