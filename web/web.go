// Package web holds the notebook frontend that mdsh-gui shows in its
// windows. The TypeScript is in src, bundled into bundle by go generate
// in cmd/mdsh-gui.
package web

import (
	"embed"
	"io/fs"

	"github.com/gdey/markdownshell/looks"
)

// FS is the frontend's files, as the pages load them. looks holds the
// looks, whose CSS is bundled into bundle/looks.css.
//
//go:embed index.html window.html settings.html changelog.html cheatsheet.html style.css looks bundle vendor/codemirror.js vendor/xterm.js vendor/xterm.css
var FS embed.FS

// Looks are the looks in looks; see package looks.
var Looks = loadLooks()

func loadLooks() []looks.Look {
	dir, err := fs.Sub(FS, "looks")
	if err != nil {
		panic(err)
	}
	all, err := looks.Load(dir)
	if err != nil {
		panic("web/looks: " + err.Error())
	}
	return all
}
