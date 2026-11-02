package collection

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestWriteNew(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "rocket.svg", "<svg/>")
	s := Settings{Title: " Deploys ", Description: "Ship it.", Icon: "rocket.svg", Color: "#3b82f6", Tags: []string{"Ops", "#prod", "ops"}}
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(filepath.Join(dir, FileName))
	want := "title: Deploys\ndescription: Ship it.\nicon: rocket.svg\ncolor: '#3b82f6'\ntags: [ops, prod]\n"
	if string(src) != want {
		t.Errorf("wrote\n%s\nwant\n%s", src, want)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Deploys" || got.Icon != "rocket.svg" || got.Color != "#3b82f6" || !slices.Equal(got.Tags, []string{"ops", "prod"}) {
		t.Errorf("Read = %+v", got)
	}
}

func TestWriteKeeps(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, FileName, "# Our deploys.\ntitle: Old # the name\nowner: ops-team\ncolor: red\ntags: a, b\n")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, Settings{Title: "New", Description: "yes", Tags: []string{"c"}}); err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(path)
	want := "# Our deploys.\ntitle: New # the name\nowner: ops-team\ntags: [c]\ndescription: yes\n"
	if string(src) != want {
		t.Errorf("wrote\n%s\nwant\n%s", src, want)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestWriteEmptyRemoves(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, FileName, "title: x\n")
	if err := Write(dir, Settings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	// Nothing to write and no file: still no file.
	if err := Write(dir, Settings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file made: %v", err)
	}
	// Other keys keep the file.
	write(t, dir, FileName, "owner: me\ntitle: x\n")
	if err := Write(dir, Settings{}); err != nil {
		t.Fatal(err)
	}
	if src, _ := os.ReadFile(path); string(src) != "owner: me\n" {
		t.Errorf("wrote %q", src)
	}
}

func TestWriteRefuses(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		s    Settings
		want string
	}{
		{Settings{Icon: "missing.png"}, "not found"},
		{Settings{Icon: "a long text icon"}, "short text icon"},
		{Settings{Background: "../out.png"}, "../out.png"},
		{Settings{Color: "red; x"}, "color"},
	} {
		err := Write(dir, tc.s)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Write(%+v) = %v, want %q", tc.s, err, tc.want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Errorf("a refused Write made the file")
	}
	// A file that doesn't parse isn't overwritten.
	path := write(t, dir, FileName, "title: [\n")
	if err := Write(dir, Settings{Title: "x"}); err == nil {
		t.Error("Write over a broken file worked")
	}
	if _, err := Read(dir); err == nil {
		t.Error("Read of a broken file worked")
	}
	if src, _ := os.ReadFile(path); string(src) != "title: [\n" {
		t.Errorf("broken file changed to %q", src)
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	if !Writable(dir) {
		t.Error("new folder not writable")
	}
	if Writable(filepath.Join(dir, "nope")) {
		t.Error("missing folder writable")
	}
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permissions don't bind here")
	}
	path := write(t, dir, FileName, "title: x\n")
	os.Chmod(path, 0o444)
	if Writable(dir) {
		t.Error("folder with a read-only .mdsh.yaml writable")
	}
	os.Chmod(path, 0o644)
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	if Writable(dir) {
		t.Error("read-only folder writable")
	}
}

func TestImages(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.png", "notes.md", "img/b.SVG", ".git/c.png", "x/y/z/deep.png", "x/y/ok.jpg"} {
		write(t, dir, n, "")
	}
	got := Images(dir, 10)
	want := []string{"a.png", "img/b.SVG", "x/y/ok.jpg"}
	if !slices.Equal(got, want) {
		t.Errorf("Images = %q, want %q", got, want)
	}
	if got := Images(dir, 1); len(got) != 1 {
		t.Errorf("Images(1) = %q", got)
	}
}
