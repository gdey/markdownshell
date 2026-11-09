package main

import (
	"runtime/debug"

	"github.com/gdey/markdownshell/ui"
)

// version is set by release builds with -ldflags "-X main.version=...",
// as build/darwin/app.sh and build/linux/appimage.sh do. Other builds
// take it from the build info.
var version = ""

// thisBuild is the build of the running binary.
func thisBuild() ui.Build {
	bi, _ := debug.ReadBuildInfo()
	return ui.BuildInfo(version, bi)
}
