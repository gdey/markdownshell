package main

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
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
		return a.app.Browser.OpenURL(target)
	case linkDoc:
		return a.openIn(t.window(), target)
	}
	return a.app.Browser.OpenFile(target)
}

// buildLinkMenu makes the right-click menu of a link in the rendered
// view, in place of the web view's, whose Open Link would load the link
// over the app's page. The frontend gives the link's href as the
// menu's data.
func (a *App) buildLinkMenu() {
	m := application.NewContextMenu("link")
	m.Add("Open Link").OnClick(func(c *application.Context) {
		t := a.currentTab()
		if t == nil {
			return
		}
		if err := a.openLink(t, menuHref(c)); err != nil {
			var win application.Window
			if w := t.window(); w != nil {
				win = w.win
			}
			a.showError(win, err)
		}
	})
	m.Add("Copy Link").OnClick(func(c *application.Context) {
		href := menuHref(c)
		if t := a.currentTab(); t != nil {
			if _, target, err := linkTarget(t.file(), href); err == nil {
				href = target
			}
		}
		a.app.Clipboard.SetText(href)
	})
	m.Update()
}

// menuHref is the href the frontend gave a link's menu, as a quoted CSS
// string.
func menuHref(c *application.Context) string {
	s := strings.TrimSpace(c.ContextMenuData())
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s
}
