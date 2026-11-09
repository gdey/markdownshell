package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mdsh", "gui.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := st.Get(); s.Autosave || len(s.Recent) > 0 {
		t.Fatalf("defaults = %+v", s)
	}
	if err := st.Update(func(s *Settings) { s.Autosave = true; s.Opened("/a/one.md") }); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := st2.Get()
	if !s.Autosave || !slices.Equal(s.Recent, []string{"/a/one.md"}) || s.LastFolder != "/a" {
		t.Errorf("read back %+v", s)
	}
}

// The settings name recent files, the shell and the Neovim to run, so
// only the user may read them, including a file an older version left
// 0644 (finding 12 of the v1.4.0 security review).
func TestStoreMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mdsh")
	path := filepath.Join(dir, "gui.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(s *Settings) { s.Opened("/a/one.md") }); err != nil {
		t.Fatal(err)
	}
	mode := func(p string) os.FileMode {
		t.Helper()
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Mode().Perm()
	}
	if m := mode(dir); m != 0o700 {
		t.Errorf("folder mode = %o, want 700", m)
	}
	if m := mode(path); m != 0o600 {
		t.Errorf("file mode = %o, want 600", m)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(s *Settings) { s.Autosave = true }); err != nil {
		t.Fatal(err)
	}
	if m := mode(path); m != 0o600 {
		t.Errorf("old 0644 file mode after a save = %o, want 600", m)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("folder holds %d files, want only gui.json", len(ents))
	}
}

// A gui.json kept elsewhere and linked in stays a link.
func TestStoreSymlink(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "dotfiles", "gui.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "mdsh", "gui.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, path); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(s *Settings) { s.Autosave = true }); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("gui.json is no longer a link: %v %v", fi, err)
	}
	st2, err := Open(real)
	if err != nil {
		t.Fatal(err)
	}
	if !st2.Get().Autosave {
		t.Error("the save didn't reach the linked file")
	}
}

func TestOpened(t *testing.T) {
	var s Settings
	for i := range MaxRecent + 2 {
		s.Opened(fmt.Sprintf("/d/%d.md", i))
	}
	s.Opened("/d/5.md")
	if len(s.Recent) != MaxRecent {
		t.Fatalf("len(Recent) = %d", len(s.Recent))
	}
	if s.Recent[0] != "/d/5.md" || s.Recent[1] != "/d/11.md" {
		t.Errorf("Recent = %v", s.Recent)
	}
	n := 0
	for _, f := range s.Recent {
		if f == "/d/5.md" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("/d/5.md is in Recent %d times", n)
	}
}

func TestStartDir(t *testing.T) {
	home, docs, fixed, last := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	for _, c := range []struct {
		name string
		s    Settings
		cwd  string
		docs string
		want string
	}{
		{"terminal", Settings{StartFolder: fixed}, "/work", docs, "/work"},
		{"fixed", Settings{StartFolder: fixed, LastFolder: last}, "/", docs, fixed},
		{"last", Settings{LastFolder: last}, "/", docs, last},
		{"first time", Settings{}, "/", docs, docs},
		{"no documents", Settings{}, "/", "", home},
		{"gone", Settings{StartFolder: "/no/such/dir"}, "", docs, docs},
	} {
		if got := c.s.StartDir(c.cwd, home, c.docs); got != c.want {
			t.Errorf("%s: StartDir = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestPlaced(t *testing.T) {
	var s Settings
	for i := range MaxPlaces + 2 {
		s.Placed(Place{File: fmt.Sprintf("/d/%d.md", i), Width: i})
	}
	if !s.Placed(Place{File: "/d/5.md", X: 10, Y: 20, Width: 800, Height: 600}) {
		t.Error("moving /d/5.md changed nothing")
	}
	if s.Placed(Place{File: "/d/5.md", X: 10, Y: 20, Width: 800, Height: 600}) {
		t.Error("placing /d/5.md where it was changed something")
	}
	if len(s.Places) != MaxPlaces {
		t.Fatalf("len(Places) = %d", len(s.Places))
	}
	if s.Places[1].File != fmt.Sprintf("/d/%d.md", MaxPlaces+1) {
		t.Errorf("Places[1] = %+v", s.Places[1])
	}
	p, ok := s.Place("/d/5.md")
	if !ok || p != (Place{File: "/d/5.md", X: 10, Y: 20, Width: 800, Height: 600}) {
		t.Errorf("Place(/d/5.md) = %+v, %v", p, ok)
	}
	if _, ok := s.Place("/d/0.md"); ok {
		t.Error("/d/0.md is still remembered past MaxPlaces")
	}
	n := 0
	for _, p := range s.Places {
		if p.File == "/d/5.md" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("/d/5.md is in Places %d times", n)
	}
}

func TestViewed(t *testing.T) {
	var s Settings
	if _, ok := s.FileView("/d/a.md"); ok {
		t.Error("a file never shown has a view")
	}
	for i := range MaxFileViews + 2 {
		s.Viewed(FileView{File: fmt.Sprintf("/d/%d.md", i), View: "rendered"})
	}
	v := FileView{File: "/d/5.md", View: "both", History: true}
	if !s.Viewed(v) {
		t.Error("changing /d/5.md's view changed nothing")
	}
	if s.Viewed(v) {
		t.Error("viewing /d/5.md as it was changed something")
	}
	if len(s.FileViews) != MaxFileViews {
		t.Fatalf("len(FileViews) = %d", len(s.FileViews))
	}
	if got, ok := s.FileView("/d/5.md"); !ok || got != v {
		t.Errorf("FileView(/d/5.md) = %+v, %v", got, ok)
	}
	if _, ok := s.FileView("/d/0.md"); ok {
		t.Error("/d/0.md is still remembered past MaxFileViews")
	}
	if got, _ := s.FileView("/d/6.md"); got.History {
		t.Error("History is open for a file it was never opened in")
	}
}

func TestStorePlaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	err = st.Update(func(s *Settings) {
		s.Placed(Place{File: "/a.md", X: 1, Y: 2, Width: 3, Height: 4})
	})
	if err != nil {
		t.Fatal(err)
	}
	got := st.Get()
	got.Places[0].Width = 99
	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []Settings{st.Get(), st2.Get()} {
		if p, _ := s.Place("/a.md"); p.Width != 3 {
			t.Errorf("read back %+v", s.Places)
		}
	}
}

func TestPin(t *testing.T) {
	var s Settings
	if !s.Pin("/d/a.md", true) || !s.Pin("/d/b.md", true) {
		t.Fatal("pinning a new file changed nothing")
	}
	if s.Pin("/d/a.md", true) {
		t.Error("pinning a pinned file changed something")
	}
	if !slices.Equal(s.Pinned, []string{"/d/a.md", "/d/b.md"}) {
		t.Errorf("Pinned = %v", s.Pinned)
	}
	c := s.clone()
	c.Pinned[0] = "/x.md"
	if s.Pinned[0] != "/d/a.md" {
		t.Error("clone shares Pinned")
	}
	if !s.Pin("/d/a.md", false) || s.Pin("/d/a.md", false) {
		t.Error("unpinning reported the wrong change")
	}
	if !slices.Equal(s.Pinned, []string{"/d/b.md"}) {
		t.Errorf("Pinned = %v", s.Pinned)
	}
}

func TestSecretShow(t *testing.T) {
	for secs, want := range map[int]time.Duration{0: SecretShowDefault, -5: SecretShowDefault, 15: 15 * time.Second, 300: 5 * time.Minute} {
		if got := (Settings{SecretShowSeconds: secs}).SecretShow(); got != want {
			t.Errorf("SecretShow with %d = %v, want %v", secs, got, want)
		}
	}
}
