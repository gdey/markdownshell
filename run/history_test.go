package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func TestHistoryAssetName(t *testing.T) {
	for path, want := range map[string]string{
		"d.md.assets/s.svg":  "d.md.assets/s.2026-11-13T190800Z.svg",
		"plots/sales.v2.png": "plots/sales.v2.2026-11-13T190800Z.png",
		"noext":              "noext.2026-11-13T190800Z",
	} {
		if got := run.HistoryAssetName(path, "2026-11-13T19:08:00Z"); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
	if got := run.HistoryAssetName("a/s.svg", ""); got != "a/s.earlier.svg" {
		t.Errorf("no time: got %s", got)
	}
}

// Each run of an image block with a history renames the last image to
// its earlier-run name before writing the new one, saving links the
// earlier run to it, and saving past the cap deletes the file of the run
// that fell off.
func TestHistoryAssets(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	assets := filepath.Join(dir, "d.md.assets")
	clock := time.Date(2026, 11, 13, 19, 8, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	text := "---\nmdsh:\n  env:\n    share: [N]\n---\n```sh {#s out=screenshot history=1}\necho \"$N\"\n```\n"
	step := func(n string) string {
		t.Helper()
		d, results := runDoc(t, text, run.Options{Dir: dir, File: file, Now: now, Environ: []string{"PATH=" + os.Getenv("PATH"), "N=" + n}})
		out := string(render.Markdown(d, results, render.Options{WriteBack: true}))
		if err := run.TidyHistoryAssets([]byte(text), []byte(out), run.Options{Dir: dir, File: file}); err != nil {
			t.Fatal(err)
		}
		text = out
		clock = clock.Add(time.Hour)
		return out
	}
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(assets, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return string(b)
	}

	step("one")
	if !strings.Contains(read("s.svg"), "one") {
		t.Fatal("first image not written")
	}
	out := step("two")
	if !strings.Contains(read("s.svg"), "two") || !strings.Contains(read("s.2026-11-13T190800Z.svg"), "one") {
		t.Errorf("second run: files not rotated")
	}
	if !strings.Contains(out, "](d.md.assets/s.2026-11-13T190800Z.svg)") || !strings.Contains(out, `history="1"`) {
		t.Errorf("second save:\n%s", out)
	}
	step("three")
	if !strings.Contains(read("s.svg"), "three") || !strings.Contains(read("s.2026-11-13T200800Z.svg"), "two") {
		t.Errorf("third run: files not rotated")
	}
	if _, err := os.Stat(filepath.Join(assets, "s.2026-11-13T190800Z.svg")); err == nil {
		t.Error("the run past the cap kept its file")
	}
}

// Only files mdsh writes for the block are renamed: a link the document
// points elsewhere is left alone.
func TestHistoryAssetsOwnOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.svg"), []byte("mine"), 0o644)
	src := "```sh {#s out=screenshot history}\necho hi\n```\n\n<!-- mdsh:output for=\"s\" at=\"2026-11-13T19:08:00Z\" -->\n![s](keep.svg)\n<!-- /mdsh:output -->\n"
	runDoc(t, src, run.Options{Dir: dir, File: filepath.Join(dir, "d.md")})
	if b, err := os.ReadFile(filepath.Join(dir, "keep.svg")); err != nil || string(b) != "mine" {
		t.Errorf("keep.svg = %q, %v", b, err)
	}
	if err := run.TidyHistoryAssets([]byte(src+"\n<!-- mdsh:output for=\"s\" history=\"1\" -->\n![s](keep.svg)\n<!-- /mdsh:output -->\n"), []byte(src), run.Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.svg")); err != nil {
		t.Errorf("tidy deleted a file it didn't name: %v", err)
	}
}
