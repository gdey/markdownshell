package ui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/notebook"
)

// linkKind is what following a link does.
type linkKind int

const (
	linkWeb   linkKind = iota // open it in the browser
	linkDoc                   // open the markdown file in a tab
	linkImage                 // show the image full size in the window
	linkFile                  // show the file in the file manager, or open it as the system would
)

// linkImages are the pictures a link shows full size in the window, as
// a click on an image in the document does.
var linkImages = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".avif", ".apng", ".bmp"}

// linkTarget is where href, a link in the document file, goes: a web
// address, or a file, which a relative href names from file's folder.
// A link to a heading in the same document has no target; the frontend
// follows those.
func linkTarget(file, href string) (linkKind, string, error) {
	u, err := url.Parse(href)
	if err != nil {
		return 0, "", fmt.Errorf("link %q: %w", href, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return linkWeb, u.String(), nil
	case "file":
	case "":
		if u.Host != "" {
			return linkWeb, "https:" + u.String(), nil
		}
	default:
		return 0, "", fmt.Errorf("link %q: mdsh can't open %s: links", href, u.Scheme)
	}
	if u.Path == "" {
		return 0, "", fmt.Errorf("link %q: no file to open", href)
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		if file == "" {
			return 0, "", fmt.Errorf("link %q: the document isn't saved", href)
		}
		path = filepath.Join(filepath.Dir(file), path)
	}
	path = filepath.Clean(path)
	switch ext := strings.ToLower(filepath.Ext(path)); {
	case ext == ".md" || ext == ".markdown":
		return linkDoc, path, nil
	case slices.Contains(linkImages, ext):
		return linkImage, path, nil
	}
	return linkFile, path, nil
}

// OpenLink follows href, a link in the tab's document: a web address
// opens in the browser, a markdown file in a new tab beside this one,
// and an image beside the document is shown full size in the window:
// OpenLink returns the address the window loads it from. Any other file
// is shown in the file manager, unless Settings › Files lists its type
// to open with the system's app. Opening it would run a .command, .app
// or script beside the document on one click, past every question a
// block asks (finding 6 of the v1.4.0 security review).
func (a *App) OpenLink(ctx context.Context, href string) (string, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return "", err
	}
	return a.openLink(t, href)
}

func (a *App) openLink(t *docTab, href string) (string, error) {
	kind, target, err := linkTarget(t.file(), href)
	if err != nil {
		return "", err
	}
	switch kind {
	case linkWeb:
		return "", a.host.OpenURL(target)
	case linkDoc:
		return "", a.openIn(t.window(), target)
	case linkImage:
		if real, err := filepath.EvalSymlinks(target); err == nil && a.underDoc(real) {
			return notebook.DocURL(real), nil
		}
	}
	s := a.settings.Get()
	if s.OpenType(target) && openable(target) {
		return "", a.host.OpenFile(target)
	}
	return "", a.host.Reveal(target)
}

// openable reports whether path may be opened with the system's app
// for its type: a regular file without the exec bit, not a folder or an
// app bundle, whatever its name says.
func openable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 == 0
}
