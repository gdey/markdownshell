package main

import (
	"bytes"
	"regexp"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	markdownshell "github.com/gdey/markdownshell"
)

// The cheat sheet is docs/cheatsheet.md as built into the app, shown
// read-only in a tab of its own beside the changelog, so every attribute,
// key and flag is a menu away, and matches this build.

const (
	// cheatsheetPage is the page a cheat sheet tab shows.
	cheatsheetPage = "/cheatsheet.html"
	// cheatsheetURL is the cheat sheet's address, which its tab gives
	// another app it's dragged out to.
	cheatsheetURL = "mdsh://doc/docs/cheatsheet.md"
)

// frontMatter is the YAML front matter at the top of the cheat sheet,
// which is the website's (its title, description and place in the docs
// section), not part of the page.
var frontMatter = regexp.MustCompile(`\A---\r?\n(?:.*\r?\n)*?---\r?\n`)

// cheatsheetHTML renders md, the cheat sheet, for its page, without its
// front matter. Raw HTML in it is left out.
func cheatsheetHTML(md []byte) (string, error) {
	md = frontMatter.ReplaceAll(md, nil)
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

var renderCheatsheet = sync.OnceValues(func() (string, error) {
	return cheatsheetHTML(markdownshell.Cheatsheet)
})

// Cheatsheet is the app's cheat sheet, rendered, for its tab.
func (a *App) Cheatsheet() (string, error) { return renderCheatsheet() }

// ShowCheatsheet brings the cheat sheet's tab forward, opening one as
// ShowChangelog does.
func (a *App) ShowCheatsheet() { a.showPage(cheatsheetPage) }
