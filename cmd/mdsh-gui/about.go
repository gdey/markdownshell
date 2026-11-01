package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/zshrc"
)

// version is set by release builds with -ldflags "-X main.version=...",
// as build/darwin/app.sh and build/linux/appimage.sh do. Other builds
// take it from the build info.
var version = ""

// build is what the binary knows about itself.
type build struct {
	Version  string // the release version, module version or "dev"
	Commit   string // the vcs revision, shortened
	Time     string // when the commit was made, as the vcs recorded it
	Modified bool   // the tree had uncommitted changes
}

// buildInfo reads the version from the release build's version, else
// from bi: the module version for a go install build, and the vcs
// revision, time and modified flag when the build had them.
func buildInfo(version string, bi *debug.BuildInfo) build {
	b := build{Version: version}
	if bi == nil {
		if b.Version == "" {
			b.Version = "dev"
		}
		return b
	}
	if b.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		b.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Commit = s.Value
			if len(b.Commit) > 12 {
				b.Commit = b.Commit[:12]
			}
		case "vcs.time":
			b.Time = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	if b.Version == "" {
		b.Version = "dev"
	}
	return b
}

// String is the version as --version prints it: the version, then the
// commit and its time in parentheses when known, with "modified" after
// the commit when the tree had uncommitted changes.
func (b build) String() string {
	s := b.Version
	var extra []string
	if b.Commit != "" {
		c := b.Commit
		if b.Modified {
			c += " modified"
		}
		extra = append(extra, c)
	}
	if b.Time != "" {
		extra = append(extra, b.Time)
	}
	if len(extra) > 0 {
		s += " (" + strings.Join(extra, ", ") + ")"
	}
	return s
}

// thisBuild is the build of the running binary.
func thisBuild() build {
	bi, _ := debug.ReadBuildInfo()
	return buildInfo(version, bi)
}

// About is what the Settings window's About tab shows.
type About struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	Modified  bool   `json:"modified"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	// Home is for showing paths as ~/….
	Home string `json:"home"`
	// SettingsFile is the JSON file the settings live in.
	SettingsFile string `json:"settingsFile"`
	// Zshrc is the .zshrc the zsh wrapper setting edits.
	Zshrc string `json:"zshrc"`
	Tools []Tool `json:"tools"`
	Links []Link `json:"links"`
}

// Tool is a program mdsh-gui uses, and whether it was found on PATH.
type Tool struct {
	Name    string `json:"name"`
	Note    string `json:"note"`    // what it's for
	Path    string `json:"path"`    // where it was found, or ""
	Version string `json:"version"` // what --version said, or ""
}

// Link is a place to read more.
type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// The places About links to.
const (
	docsURL    = "https://gdey.github.io/markdownshell/"
	repoURL    = "https://github.com/gdey/markdownshell"
	releaseURL = repoURL + "/releases"
	changesURL = repoURL + "/blob/main/CHANGELOG.md"
	issueURL   = repoURL + "/issues/new"
)

// About gathers the version, build, paths and tools for the About tab.
func (a *App) About() About {
	b := thisBuild()
	home, _ := os.UserHomeDir()
	zsh, _ := zshrc.Path()
	ab := About{
		Version:      b.Version,
		Commit:       b.Commit,
		BuildTime:    b.Time,
		Modified:     b.Modified,
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Home:         home,
		SettingsFile: a.settings.Path(),
		Zshrc:        zsh,
		Links: []Link{
			{"Documentation", docsURL},
			{"Source code", repoURL},
			{"Releases", releaseURL},
			{"Changelog", changesURL},
			{"Report a problem", issueURL},
		},
	}
	tools := []Tool{
		{Name: "zsh", Note: "runs zsh blocks, and sh blocks in a file that has zsh blocks"},
		{Name: "sh", Note: "runs sh blocks"},
		{Name: "mdsh", Note: "the command line tool, which the zsh wrapper and Create Report's command use"},
	}
	if shell := a.settings.Get().Shell; shell != "" {
		tools = append([]Tool{{Name: shell, Note: "the shell for every file, from Settings"}}, tools...)
	}
	for _, t := range tools {
		ab.Tools = append(ab.Tools, lookTool(t))
	}
	return ab
}

// lookTool finds t on PATH and asks it for its version.
func lookTool(t Tool) Tool {
	path, err := exec.LookPath(t.Name)
	if err != nil {
		return t
	}
	t.Path = path
	t.Version = toolVersion(path)
	return t
}

// toolVersion is the first line program prints for --version, or "" if
// it has none or takes too long. sh is skipped: dash has no --version
// and bash's is several lines about bash, not sh.
func toolVersion(program string) string {
	if strings.HasSuffix(program, "/sh") {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, program, "--version").Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line
}

// OpenURL opens a web address in the browser, for the About tab's links.
func (a *App) OpenURL(u string) error {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return errors.New("not a web address: " + u)
	}
	return a.app.Browser.OpenURL(u)
}
