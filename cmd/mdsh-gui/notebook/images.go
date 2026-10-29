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
// document. Links with a scheme, data: URIs and absolute paths are left
// as they are.
type docImages struct{ n *Notebook }

// Transform runs while Cells holds n.mu, so it reads n.file directly.
func (t docImages) Transform(root *ast.Document, _ text.Reader, _ parser.Context) {
	dir := filepath.Dir(t.n.file)
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := node.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		if path, ok := beside(dir, string(img.Destination)); ok {
			img.Destination = []byte(DocURL(path))
		}
		return ast.WalkContinue, nil
	})
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
