// Package web holds the notebook frontend that mdsh-gui shows in its
// windows. The TypeScript is in src, bundled into bundle by go generate
// in cmd/mdsh-gui.
package web

import "embed"

// FS is the frontend's files, as the pages load them.
//
//go:embed index.html window.html settings.html style.css bundle vendor/codemirror.js vendor/xterm.js vendor/xterm.css
var FS embed.FS
