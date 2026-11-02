package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/ui"
)

// The app carries the mdsh command line tool: mdsh.app has it beside
// mdsh-gui in Contents/MacOS, and the AppImage at usr/bin/mdsh. Settings
// › Shell (and on macOS the app menu) puts it on PATH: on macOS as a
// symlink in /usr/local/bin to the binary inside the app, so it updates
// and goes away with the app; from an AppImage as a copy in ~/.local/bin,
// since the image is a file whose name changes with every release. See
// docs/design/cli-in-app.md.

// CLI is where the command line tool stands, for Settings › Shell.
type CLI = ui.CLI

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
	// unquarantine clears the quarantine flag on the app, so a
	// download's helper can run from a terminal. Nil when not needed.
	unquarantine func() error
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
	if h.unquarantine != nil {
		_ = h.unquarantine() // best effort: a terminal shows macOS's own message if it's still set
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
		return h.admin(fmt.Sprintf("mkdir -p %s && ln -sfn %s %s",
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

// repair is the launch-time check, which asks nothing: on macOS it
// points a broken link back at this app when it can write the folder;
// from an AppImage it copies again when the copy is another version's.
func (h *cliHost) repair() error {
	c := h.state()
	switch {
	case h.copy && c.Installed:
		v, _ := os.ReadFile(h.marker())
		if strings.TrimSpace(string(v)) == h.version {
			return nil
		}
		return h.copyIn()
	case !h.copy && c.Broken != "":
		if err := os.Remove(h.link); err != nil {
			return err
		}
		return os.Symlink(h.bundled, h.link)
	}
	return nil
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
func (g *gui) CLIState() CLI {
	return g.cli.state()
}

// InstallCLI puts the command line tool on PATH, and returns where it
// stands after.
func (g *gui) InstallCLI() (CLI, error) {
	err := g.cli.install()
	g.updateCLIMenu()
	return g.cli.state(), err
}

// UninstallCLI takes the command line tool off PATH, when it's this
// app's, and returns where it stands after.
func (g *gui) UninstallCLI() (CLI, error) {
	err := g.cli.uninstall()
	g.updateCLIMenu()
	return g.cli.state(), err
}
