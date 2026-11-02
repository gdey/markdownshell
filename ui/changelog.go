package ui

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	markdownshell "github.com/gdey/markdownshell"
)

// The changelog is CHANGELOG.md as built into the app, shown read-only
// in a tab of its own, so reading what changed needs no browser and
// says what changed in this build, not on the website's main branch.

const (
	// changelogPage is the page a changelog tab shows.
	changelogPage = "/changelog.html"
	// changelogURL is the changelog's address, which its tab gives
	// another app it's dragged out to.
	changelogURL = "mdsh://doc/CHANGELOG.md"
	// changelogImages is where the changelog's screenshots are served.
	changelogImages = "/changelog/"
)

var (
	// emptyUnreleased is an Unreleased section with nothing in it yet,
	// as just after a release; it is left out, as the website does.
	emptyUnreleased = regexp.MustCompile(`(?m)^## \[Unreleased\]\s*\n(## )`)
	// imageLinks are the changelog's links to its screenshots, which it
	// writes relative to the repository's top for GitHub.
	imageLinks = regexp.MustCompile(`\]\(docs/changelog/`)
)

// changelogHTML renders md, a changelog, for the changelog page. Raw
// HTML in it is left out, and its screenshots load from the app.
func changelogHTML(md []byte) (string, error) {
	md = emptyUnreleased.ReplaceAll(md, []byte("$1"))
	md = imageLinks.ReplaceAll(md, []byte("]("+changelogImages))
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	var b bytes.Buffer
	if err := gm.Convert(md, &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

var renderChangelog = sync.OnceValues(func() (string, error) {
	return changelogHTML(markdownshell.Changelog)
})

// Changelog is the app's changelog, rendered, for its tab.
func (a *App) Changelog() (string, error) { return renderChangelog() }

// imageTypes are the kinds of image the changelog shows, by extension.
var imageTypes = map[string]string{".png": "image/png", ".gif": "image/gif"}

// serveChangelogImage serves the changelog's screenshot name, and
// nothing else.
func serveChangelogImage(w http.ResponseWriter, r *http.Request, name string) {
	typ := imageTypes[path.Ext(name)]
	if strings.Contains(name, "/") || typ == "" {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(markdownshell.ChangelogImages, "docs/changelog/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

// ShowChangelog brings the changelog's tab forward, opening one if
// there is none (see showPage).
func (a *App) ShowChangelog() { a.showPage(changelogPage) }

// showPage brings the tab showing page forward, opening one if there is
// none (see placePage), and returns it.
func (a *App) showPage(page string) *docTab {
	if t := a.pageTab(page); t != nil {
		a.focusTab(t)
		return t
	}
	t := a.newPageTab(page)
	a.placePage(t)
	return t
}

// pageTab is the tab showing page, or nil.
func (a *App) pageTab(page string) *docTab {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, t := range a.tabs {
		if t.page == page {
			return t
		}
	}
	return nil
}

// placePage puts t, a new page tab, after the tab in front of the
// document window in front, and brings that window forward. With no
// document window open, t opens in a window of its own.
func (a *App) placePage(t *docTab) {
	w := a.current()
	if w == nil {
		w = a.lastWindow()
	}
	if w == nil {
		a.newWindow(t)
		return
	}
	a.mu.Lock()
	i := slices.Index(w.tabs, w.active) + 1
	a.mu.Unlock()
	a.moveTab(t, w, i)
	w.win.Focus()
}

// lastWindow is the document window opened last, or nil, for when no
// window is in front.
func (a *App) lastWindow() *docWindow {
	a.mu.Lock()
	defer a.mu.Unlock()
	var last *docWindow
	for id, w := range a.windows {
		if last == nil || id > last.win.ID() {
			last = w
		}
	}
	return last
}
