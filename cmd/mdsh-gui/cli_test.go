package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeApp makes an mdsh.app with a helper under dir, and returns the
// helper's path.
func fakeApp(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "mdsh.app", "Contents", "MacOS", "mdsh")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho mdsh 1.4.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCLILink(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "usr-local-bin")
	h := &cliHost{bundled: fakeApp(t, filepath.Join(tmp, "Applications")), link: filepath.Join(bin, "mdsh"), pathDirs: []string{bin}}
	if c := h.state(); c.Installed || c.Other != "" || c.Broken != "" || !c.InPath {
		t.Fatalf("before install: %+v", c)
	}
	if err := h.install(); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(h.link); target != h.bundled {
		t.Errorf("link points at %q, want %q", target, h.bundled)
	}
	if !h.state().Installed {
		t.Error("not installed after install")
	}
	if err := h.uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(h.link); !os.IsNotExist(err) {
		t.Errorf("link still there after uninstall: %v", err)
	}
}

func TestCLIMovedApp(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	os.MkdirAll(bin, 0o755)
	old := filepath.Join(tmp, "Downloads", "mdsh.app", "Contents", "MacOS", "mdsh")
	h := &cliHost{bundled: fakeApp(t, filepath.Join(tmp, "Applications")), link: filepath.Join(bin, "mdsh"), pathDirs: []string{bin}}
	if err := os.Symlink(old, h.link); err != nil {
		t.Fatal(err)
	}
	if c := h.state(); c.Broken != old || c.Installed || c.Other != "" {
		t.Fatalf("moved app: %+v", c)
	}
	changed, err := h.repair()
	if err != nil {
		t.Fatal(err)
	}
	if !h.state().Installed {
		t.Error("repair didn't point the link at this app")
	}
	if !strings.Contains(changed, old) {
		t.Errorf("repair said %q, want the old target named", changed)
	}
}

// TestCLIOtherApp checks that launching an mdsh.app doesn't take over a
// link into another mdsh.app that is still there, such as the installed
// app's while this copy runs from a mounted .dmg, even after the
// installed app has launched and repaired its own link (finding 7 of
// the v1.4.0 security review, and finding 1 of the v1.5.0 one, where
// every copy shared one record of "where this app has stood").
func TestCLIOtherApp(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	os.MkdirAll(bin, 0o755)
	link := filepath.Join(bin, "mdsh")
	installed := &cliHost{bundled: fakeApp(t, filepath.Join(tmp, "Applications")), link: link, pathDirs: []string{bin}}
	if err := installed.install(); err != nil {
		t.Fatal(err)
	}
	// The installed app launches: its own link is left as it is.
	if changed, err := installed.repair(); err != nil || changed != "" {
		t.Fatalf("installed app's repair = %q, %v", changed, err)
	}
	// Then a copy launches from a mounted .dmg.
	dmg := &cliHost{bundled: fakeApp(t, filepath.Join(tmp, "Volumes", "mdsh-dmg")), link: link, pathDirs: []string{bin}}
	if c := dmg.state(); c.Broken != installed.bundled || !c.OtherApp {
		t.Errorf("state = %+v, want another app", c)
	}
	for range 2 {
		changed, err := dmg.repair()
		if err != nil || changed != "" {
			t.Fatalf("dmg copy's repair = %q, %v", changed, err)
		}
		if target, _ := os.Readlink(link); target != installed.bundled {
			t.Fatalf("the dmg copy's launch moved the link to %q", target)
		}
	}
	// Install, asked for, is what moves it.
	if err := dmg.install(); err != nil {
		t.Fatal(err)
	}
	if !dmg.state().Installed {
		t.Error("Install didn't point the link at the copy")
	}
	// And the installed app, launched again, doesn't take it back.
	if changed, err := installed.repair(); err != nil || changed != "" {
		t.Fatalf("installed app's repair after the copy's Install = %q, %v", changed, err)
	}
	if target, _ := os.Readlink(link); target != dmg.bundled {
		t.Errorf("the installed app's launch moved the link back to %q", target)
	}
}

func TestOlderVersion(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"1.4.0", "1.5.0", true},
		{"1.5.0", "1.4.0", false},
		{"1.5.0", "1.5.0", false},
		{"1.4.9", "1.10.0", true},
		{"1.5.0-rc.1", "1.5.0", true},
		{"1.5.0", "1.5.0-rc.1", false},
		{"1.5.0-rc.1", "1.5.0-rc.2", true},
		{"0.0.0-dev.12", "1.4.0", true},
		{"", "1.4.0", true},
		{"1.4.0", "", false},
		{"junk", "other", false},
	} {
		if got := olderVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("olderVersion(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCLIOtherOnPath(t *testing.T) {
	tmp := t.TempDir()
	brew, bin := filepath.Join(tmp, "homebrew"), filepath.Join(tmp, "bin")
	os.MkdirAll(brew, 0o755)
	formula := filepath.Join(brew, "mdsh")
	os.WriteFile(formula, []byte("#!/bin/sh\n"), 0o755)
	h := &cliHost{bundled: fakeApp(t, tmp), link: filepath.Join(bin, "mdsh"), pathDirs: []string{brew, bin}}
	if c := h.state(); c.Other != formula {
		t.Fatalf("Other = %q, want the formula's %q", c.Other, formula)
	}
	if err := h.install(); err == nil || !strings.Contains(err.Error(), formula) {
		t.Errorf("install beside the formula: %v, want an error naming it", err)
	}
	if _, err := os.Lstat(h.link); !os.IsNotExist(err) {
		t.Error("install linked anyway")
	}
	// A plain file at the link is someone else's too, and uninstall
	// leaves it be.
	os.Remove(formula)
	os.MkdirAll(bin, 0o755)
	os.WriteFile(h.link, []byte("#!/bin/sh\n"), 0o755)
	if c := h.state(); c.Other != h.link || c.Installed {
		t.Errorf("a file at the link: %+v", c)
	}
	if err := h.uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.link); err != nil {
		t.Error("uninstall removed a file that wasn't this app's")
	}
}

// The Homebrew cask links its own mdsh into Homebrew's bin, which
// comes first on PATH: install has nothing to do.
func TestCLICaskLink(t *testing.T) {
	tmp := t.TempDir()
	brew, bin := filepath.Join(tmp, "homebrew"), filepath.Join(tmp, "bin")
	os.MkdirAll(brew, 0o755)
	h := &cliHost{bundled: fakeApp(t, tmp), link: filepath.Join(bin, "mdsh"), pathDirs: []string{brew, bin}}
	cask := filepath.Join(brew, "mdsh")
	os.Symlink(h.bundled, cask)
	if c := h.state(); c.Linked != cask || c.Other != "" {
		t.Fatalf("cask: %+v", c)
	}
	if err := h.install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(h.link); !os.IsNotExist(err) {
		t.Error("install linked a second one beside the cask's")
	}
}

func TestCLICopy(t *testing.T) {
	tmp := t.TempDir()
	bundled := filepath.Join(tmp, "appdir", "usr", "bin", "mdsh")
	os.MkdirAll(filepath.Dir(bundled), 0o755)
	os.WriteFile(bundled, []byte("v1"), 0o755)
	bin := filepath.Join(tmp, "home", ".local", "bin")
	h := &cliHost{bundled: bundled, link: filepath.Join(bin, "mdsh"), copy: true, version: "1.4.0"}
	if c := h.state(); c.InPath || c.Installed {
		t.Fatalf("before install: %+v", c)
	}
	if err := h.install(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(h.link); string(b) != "v1" {
		t.Errorf("copy holds %q", b)
	}
	if fi, _ := os.Stat(h.link); fi.Mode()&0o111 == 0 {
		t.Errorf("copy isn't executable: %v", fi.Mode())
	}
	if m, _ := os.ReadFile(h.marker()); strings.TrimSpace(string(m)) != "1.4.0" {
		t.Errorf("marker holds %q", m)
	}
	if !h.state().Installed {
		t.Error("not installed after install")
	}

	// The same version: repair leaves it. A new one: repair copies. An
	// older one, such as an old AppImage opened again: repair leaves the
	// newer copy alone.
	os.WriteFile(bundled, []byte("v2"), 0o755)
	h.repair()
	if b, _ := os.ReadFile(h.link); string(b) != "v1" {
		t.Errorf("repair at the same version copied: %q", b)
	}
	h.version = "1.5.0"
	changed, err := h.repair()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(h.link); string(b) != "v2" {
		t.Errorf("repair at a new version left %q", b)
	}
	if !strings.Contains(changed, "1.4.0 to 1.5.0") {
		t.Errorf("repair said %q", changed)
	}
	os.WriteFile(bundled, []byte("old"), 0o755)
	h.version = "1.3.0"
	if changed, err := h.repair(); err != nil || changed != "" {
		t.Errorf("repair from an older AppImage = %q, %v", changed, err)
	}
	if b, _ := os.ReadFile(h.link); string(b) != "v2" {
		t.Errorf("an older AppImage replaced the copy: %q", b)
	}
	h.version = "1.5.0"
	if left, _ := filepath.Glob(filepath.Join(bin, ".mdsh.tmp*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}

	if err := h.uninstall(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{h.link, h.marker()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still there after uninstall", p)
		}
	}

	// An mdsh there without the marker isn't this app's.
	os.WriteFile(h.link, []byte("mine"), 0o755)
	if c := h.state(); c.Installed || c.Other != h.link {
		t.Errorf("unmarked copy: %+v", c)
	}
	if err := h.install(); err == nil {
		t.Error("install overwrote an mdsh that isn't this app's")
	}
}

func TestCLINoneBundled(t *testing.T) {
	h := &cliHost{link: filepath.Join(t.TempDir(), "mdsh")}
	if err := h.install(); err != errNoCLI {
		t.Errorf("install = %v, want errNoCLI", err)
	}
}

// adminScript's command runs as root, so a path with spaces, quotes and
// backslashes must reach sh as one word, through AppleScript's string.
func TestAdminScript(t *testing.T) {
	path := `/Users/j/My "Apps"/it's \here/mdsh.app/Contents/MacOS/mdsh`
	script := adminScript("printf %s " + shellQuote(path))
	const pre, post = `do shell script "`, `" with administrator privileges`
	if !strings.HasPrefix(script, pre) || !strings.HasSuffix(script, post) {
		t.Fatalf("script = %s", script)
	}
	// Read the string literal back as AppleScript does.
	lit := strings.TrimSuffix(strings.TrimPrefix(script, pre), post)
	var cmd strings.Builder
	for i := 0; i < len(lit); i++ {
		switch c := lit[i]; {
		case c == '\\':
			i++
			cmd.WriteByte(lit[i])
		case c == '"':
			t.Fatalf("unescaped quote in %s", lit)
		default:
			cmd.WriteByte(c)
		}
	}
	out, err := exec.Command("sh", "-c", cmd.String()).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != path {
		t.Errorf("sh saw %q, want %q", out, path)
	}
}
