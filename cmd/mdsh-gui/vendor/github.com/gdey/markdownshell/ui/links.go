package ui

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// linkKind is what following a link does.
type linkKind int

const (
	linkWeb  linkKind = iota // open it in the browser
	linkDoc                  // open the markdown file in a tab
	linkFile                 // open the file as the system would
)

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
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".md" || ext == ".markdown" {
		return linkDoc, path, nil
	}
	return linkFile, path, nil
}

// OpenLink follows href, a link in the tab's document: a web address
// opens in the browser, a markdown file in a new tab beside this one,
// and any other file as the system would open it.
func (a *App) OpenLink(ctx context.Context, href string) error {
	t, err := a.tab(ctx)
	if err != nil {
		return err
	}
	return a.openLink(t, href)
}

func (a *App) openLink(t *docTab, href string) error {
	kind, target, err := linkTarget(t.file(), href)
	if err != nil {
		return err
	}
	switch kind {
	case linkWeb:
		return a.host.OpenURL(target)
	case linkDoc:
		return a.openIn(t.window(), target)
	}
	return a.host.OpenFile(target)
}
