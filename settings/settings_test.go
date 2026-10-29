package settings

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
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
