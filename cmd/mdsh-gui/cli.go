package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The app carries the mdsh command line tool: mdsh.app has it beside
// mdsh-gui in Contents/MacOS, and the AppImage at usr/bin/mdsh. Settings
// › Shell (and on macOS the app menu) puts it on PATH: on macOS as a
// symlink in /usr/local/bin to the binary inside the app, so it updates
// and goes away with the app; from an AppImage as a copy in ~/.local/bin,
// since the image is a file whose name changes with every release. See
// docs/design/cli-in-app.md.

// CLI is where the command line tool stands, for Settings › Shell.
type CLI struct {
	// Bundled is the mdsh this app carries, or "" when this build
	// carries none (go run, go install, a .deb, Windows).
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
	// OtherApp is whether Broken is still there: another mdsh.app, such
	// as an older release's, rather than where this one used to be.
	OtherApp bool `json:"otherApp"`
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

// cliHost is what the CLI's install needs to know about the platform,
// so the logic can be tested anywhere.
type cliHost struct {
	bundled string // the mdsh inside the app, or ""
	link    string // where install puts it
	copy    bool   // install copies bundled, with a marker, instead of linking
	version string // the app's version, written to the marker
	// pathDirs are the folders a shell looks in, in order.
	pathDirs []string
	// admin runs a shell command as root, after asking for a password,
	// for a link folder the user can't write. Nil when there's no way.
	admin func(cmd string) error
}

// marker is the file beside a copied mdsh that says this app put it
// there, holding the version it copied.
func (h *cliHost) marker() string {
	return filepath.Join(filepath.Dir(h.link), ".mdsh-from-appimage")
}

// isAppHelper reports whether target is the mdsh inside some mdsh.app.
func isAppHelper(target string) bool {
	return strings.HasSuffix(target, ".app/Contents/MacOS/mdsh")
}

// state looks at Link and PATH.
func (h *cliHost) state() CLI {
	home, _ := os.UserHomeDir()
	c := CLI{Bundled: h.bundled, Link: h.link, Copy: h.copy, Home: home}
	if h.bundled == "" {
		return c
	}
	c.InPath = slices.Contains(h.pathDirs, filepath.Dir(h.link))
	if h.copy {
		if _, err := os.Stat(h.link); err == nil {
			if _, err := os.Stat(h.marker()); err == nil {
				c.Installed = true
			} else {
				c.Other = h.link
			}
		}
	} else if target, err := os.Readlink(h.link); err == nil {
		switch {
		case target == h.bundled:
			c.Installed = true
		case isAppHelper(target):
			c.Broken = target
			_, err := os.Stat(target)
			c.OtherApp = err == nil
		default:
			c.Other = h.link
		}
	} else if _, err := os.Lstat(h.link); err == nil {
		c.Other = h.link
	}
	// The first other mdsh on PATH: it would run instead of Link when
	// its folder comes first, and beside it Link would be a second copy.
	bundled, _ := filepath.EvalSymlinks(h.bundled)
	for _, dir := range h.pathDirs {
		p := filepath.Join(dir, "mdsh")
		if p == h.link {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
			continue
		}
		if real, _ := filepath.EvalSymlinks(p); real != "" && real == bundled {
			if c.Linked == "" {
				c.Linked = p
			}
			continue
		}
		if c.Other == "" {
			c.Other = p
		}
		break
	}
	return c
}

// errNoCLI is Install's error for a build that carries no mdsh.
var errNoCLI = errors.New("this build of mdsh-gui doesn't carry the mdsh command line tool")

// errCanceled is macOS's administrator prompt's Cancel.
var errCanceled = errors.New("canceled")

// install puts the CLI on PATH.
func (h *cliHost) install() error {
	c := h.state()
	switch {
	case h.bundled == "":
		return errNoCLI
	case c.Linked != "":
		return nil // the cask's link, or another, already runs this app's mdsh
	case c.Other != "":
		return fmt.Errorf("there is already an mdsh at %s, which isn't this app's; remove it first, or keep using it", c.Other)
	}
	if h.copy {
		return h.copyIn()
	}
	err := os.MkdirAll(filepath.Dir(h.link), 0o755)
	if err == nil {
		err = os.Remove(h.link)
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}
	if err == nil {
		err = os.Symlink(h.bundled, h.link)
	}
	if errors.Is(err, fs.ErrPermission) && h.admin != nil {
		err = h.admin(fmt.Sprintf("mkdir -p %s && ln -sfn %s %s",
			shellQuote(filepath.Dir(h.link)), shellQuote(h.bundled), shellQuote(h.link)))
	}
	return err
}

// copyIn copies the bundled mdsh to Link through a temporary file, so a
// running mdsh is never half written, then writes the marker.
func (h *cliHost) copyIn() error {
	dir := filepath.Dir(h.link)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src, err := os.Open(h.bundled)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(dir, ".mdsh.tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(h.marker(), []byte(h.version+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), h.link)
}

// uninstall takes Link away, only when it's this app's (or a broken
// link into an mdsh.app).
func (h *cliHost) uninstall() error {
	c := h.state()
	if !c.Installed && c.Broken == "" {
		return nil
	}
	if h.copy {
		if err := os.Remove(h.link); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return os.Remove(h.marker())
	}
	err := os.Remove(h.link)
	if errors.Is(err, fs.ErrPermission) && h.admin != nil {
		return h.admin("rm -f " + shellQuote(h.link))
	}
	return err
}

// repair is the launch-time check, which asks nothing, and says what it
// changed, for the Console. On macOS it points a link into an mdsh.app
// back at this app when the app moved, which is when the link's target
// is gone. A link into another mdsh.app that is still there, such as the
// installed app's while this copy runs from a mounted .dmg, an older
// release's, or a fork's build, is left alone: launching that copy once
// must not make it the mdsh every terminal runs; Settings › Shell shows
// it, and Install takes it over when asked. Nothing remembered decides
// this, since the settings are the user's and every copy of the app
// shares them. From an AppImage it copies again only when the copy is an
// older version's, never a newer one's. (Finding 7 of the v1.4.0
// security review, and finding 1 of the v1.5.0 one.)
func (h *cliHost) repair() (changed string, err error) {
	c := h.state()
	switch {
	case h.copy && c.Installed:
		v, _ := os.ReadFile(h.marker())
		old := strings.TrimSpace(string(v))
		if !olderVersion(old, h.version) {
			return "", nil
		}
		if err := h.copyIn(); err != nil {
			return "", err
		}
		return fmt.Sprintf("updated %s from %s to %s", h.link, old, h.version), nil
	case !h.copy && c.Broken != "":
		if c.OtherApp {
			return "", nil
		}
		if err := os.Remove(h.link); err != nil {
			return "", err
		}
		if err := os.Symlink(h.bundled, h.link); err != nil {
			return "", err
		}
		return fmt.Sprintf("pointed %s at this app; it pointed at %s", h.link, c.Broken), nil
	}
	return "", nil
}

// olderVersion reports whether version a is older than b, both as
// X.Y.Z with an optional -pre part, which is older than X.Y.Z itself.
// A version that doesn't parse is older than one that does, and two
// that don't aren't ordered.
func olderVersion(a, b string) bool {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	switch {
	case !oka || !okb:
		return !oka && okb
	}
	for i := range 3 {
		if pa.n[i] != pb.n[i] {
			return pa.n[i] < pb.n[i]
		}
	}
	switch {
	case pa.pre == pb.pre:
		return false
	case pa.pre == "":
		return false
	case pb.pre == "":
		return true
	}
	return pa.pre < pb.pre
}

type semver struct {
	n   [3]int
	pre string
}

func parseVersion(s string) (semver, bool) {
	var v semver
	core, pre, _ := strings.Cut(strings.TrimPrefix(s, "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.n[i] = n
	}
	v.pre = pre
	return v, true
}

// shellQuote quotes s for sh as one word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// adminScript is the AppleScript that runs cmd as root after macOS's
// administrator password prompt. cmd runs as root, so every path in it
// must already be quoted with shellQuote; this escapes it again for an
// AppleScript string literal.
func adminScript(cmd string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `do shell script "` + r.Replace(cmd) + `" with administrator privileges`
}

// envPathDirs are the folders on PATH, in order.
func envPathDirs() []string {
	var dirs []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d != "" && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// CLIState says where the command line tool stands.
func (a *App) CLIState() CLI {
	return a.cli.state()
}

// InstallCLI puts the command line tool on PATH, and returns where it
// stands after.
func (a *App) InstallCLI() (CLI, error) {
	err := a.cli.install()
	a.updateCLIMenu()
	return a.cli.state(), err
}

// UninstallCLI takes the command line tool off PATH, when it's this
// app's, and returns where it stands after.
func (a *App) UninstallCLI() (CLI, error) {
	err := a.cli.uninstall()
	a.updateCLIMenu()
	return a.cli.state(), err
}
