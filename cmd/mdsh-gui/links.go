package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/wailsapp/wails/v3/pkg/application"
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
		return "", a.app.Browser.OpenURL(target)
	case linkDoc:
		return "", a.openIn(t.window(), target)
	case linkImage:
		if real, err := filepath.EvalSymlinks(target); err == nil && a.underDoc(real) {
			return notebook.DocURL(real), nil
		}
	}
	s := a.settings.Get()
	if s.OpenType(target) && openable(target) {
		return "", a.app.Browser.OpenFile(target)
	}
	return "", a.app.Env.OpenFileManager(target, true)
}

// openable reports whether path may be opened with the system's app
// for its type: a regular file without the exec bit, not a folder or an
// app bundle, whatever its name says.
func openable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 == 0
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
		u, err := a.openLink(t, menuHref(c))
		if err != nil {
			var win application.Window
			if w := t.window(); w != nil {
				win = w.win
			}
			a.showError(win, err)
		} else if u != "" {
			t.emit("view-image", u)
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
