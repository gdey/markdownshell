// Package markdownshell holds the files at the top of the repository
// that programs embed, such as the changelog and the cheat sheet
// mdsh-gui shows.
package markdownshell

import "embed"

// Changelog is CHANGELOG.md.
//
//go:embed CHANGELOG.md
var Changelog []byte

// Cheatsheet is docs/cheatsheet.md, the reference page mdsh-gui shows
// beside the changelog.
//
//go:embed docs/cheatsheet.md
var Cheatsheet []byte

// ChangelogImages are the screenshots and recordings the changelog shows, under
// docs/changelog, where its image links point.
//
//go:embed docs/changelog/*.png docs/changelog/*.gif
var ChangelogImages embed.FS
