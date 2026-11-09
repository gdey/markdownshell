package ui

// CLI is where the command line tool stands, for Settings › Shell. The
// desktop app carries mdsh and can put it on PATH; see
// docs/design/cli-in-app.md.
type CLI struct {
	// Bundled is the mdsh this app carries, or "" when this build
	// carries none (go run, go install, a .deb, Windows, a browser).
	Bundled string `json:"bundled"`
	// Link is where Install puts it.
	Link string `json:"link"`
	// Copy is whether Install copies Bundled to Link (an AppImage)
	// rather than linking to it (macOS).
	Copy bool `json:"copy"`
	// Installed is whether Link is this app's: a symlink to Bundled, or
	// a copy with the marker beside it.
	Installed bool `json:"installed"`
	// Broken is where Link points when it's a symlink into an mdsh.app
	// that isn't this one, such as after the app moved.
	Broken string `json:"broken"`
	// Linked is another mdsh on PATH that is this app's, such as the
	// Homebrew cask's link, which comes first on PATH.
	Linked string `json:"linked"`
	// Other is an mdsh on PATH that isn't this app's, such as the
	// formula's. Install won't put a second one beside it.
	Other string `json:"other"`
	// InPath is whether Link's folder is on PATH.
	InPath bool `json:"inPath"`
	// Home is for showing paths as ~/….
	Home string `json:"home"`
}

// CLIState says where the command line tool stands.
func (a *App) CLIState() CLI {
	return a.host.CLIState()
}

// InstallCLI puts the command line tool on PATH, and returns where it
// stands after.
func (a *App) InstallCLI() (CLI, error) {
	return a.host.InstallCLI()
}

// UninstallCLI takes the command line tool off PATH, when it's this
// app's, and returns where it stands after.
func (a *App) UninstallCLI() (CLI, error) {
	return a.host.UninstallCLI()
}
