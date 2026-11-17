package notebook

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const earlierDoc = "```sh {#n history=3}\necho 4\n```\n\n" +
	"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\n" +
	"```output {for=n exit=0 at=2026-11-16T22:20:03Z history=1}\n3\n```\n\n" +
	"```output {for=n exit=7 at=2026-11-16T22:20:02Z history=2}\n2\n```\n\nAfter.\n"

// A block's earlier runs go to its cell, newest first, apart from its
// current output.
func TestEarlierCells(t *testing.T) {
	n, err := Open(write(t, earlierDoc), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var c *Cell
	for _, x := range n.Cells() {
		if x.Block == 0 {
			c = &x
		}
	}
	if c == nil {
		t.Fatal("no cell for the block")
	}
	if strings.Contains(c.Output, ">3") || !strings.Contains(c.Output, "4") {
		t.Errorf("output holds more than the current run: %s", c.Output)
	}
	if len(c.Earlier) != 2 {
		t.Fatalf("earlier runs: %+v", c.Earlier)
	}
	e1, e2 := c.Earlier[0], c.Earlier[1]
	if e1.K != 1 || e1.At != "2026-11-16T22:20:03Z" || e1.Failed || !strings.Contains(e1.Output, "3") {
		t.Errorf("run 1 back: %+v", e1)
	}
	if e2.K != 2 || e2.Exit != 7 || !e2.Failed || !strings.Contains(e2.Output, "2") {
		t.Errorf("run 2 back: %+v", e2)
	}
}

// Clear earlier runs keeps the current output, and deleting one earlier
// run numbers the rest again. Undo puts each back.
func TestDropEarlier(t *testing.T) {
	n, err := Open(write(t, earlierDoc), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.DeleteEarlierRun(0, 1); err != nil {
		t.Fatal(err)
	}
	want := "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\n" +
		"```output {for=n exit=7 at=2026-11-16T22:20:02Z history=1}\n2\n```\n\nAfter.\n"
	if got := n.Text(); got != want {
		t.Errorf("after deleting run 1 back:\n%s\nwant\n%s", got, want)
	}
	if err := n.DeleteEarlierRun(0, 5); err == nil {
		t.Error("deleting a run the block hasn't kept: no error")
	}
	if err := n.ClearEarlierRuns(0); err != nil {
		t.Fatal(err)
	}
	want = "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\nAfter.\n"
	if got := n.Text(); got != want {
		t.Errorf("after clearing the earlier runs:\n%s\nwant\n%s", got, want)
	}
	if what, err := n.Undo(); err != nil || what != "clear earlier runs" {
		t.Errorf("undo: %q, %v", what, err)
	}
	if what, err := n.Undo(); err != nil || what != "delete earlier run" || n.Text() != earlierDoc {
		t.Errorf("undo: %q, %v:\n%s", what, err, n.Text())
	}
}

// Clearing the earlier runs of a block that ran this session doesn't
// push its run down again when the document is written.
func TestDropEarlierAfterRun(t *testing.T) {
	n, err := Open(write(t, earlierDoc), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(n.Text(), "history="); got != 4 { // the fence's and three runs back
		t.Fatalf("after a run, %d history=:\n%s", got, n.Text())
	}
	if err := n.ClearEarlierRuns(0); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if got := noAt(n.Text()); got != "```sh {#n history=3}\necho 4\n```\n\n```output {for=n exit=0}\n4\n```\n\nAfter.\n" {
		t.Errorf("after clearing and saving:\n%s", got)
	}
}

// Saving deletes the image files of earlier runs cleared since the last
// save, and only those.
func TestDropEarlierFiles(t *testing.T) {
	src := "```sh {#p history=2 out=image}\necho\n```\n\n" +
		"<!-- mdsh:output for=\"p\" at=\"2026-11-16T22:20:04Z\" -->\n![p](d.md.assets/p.svg)\n<!-- /mdsh:output -->\n\n" +
		"<!-- mdsh:output for=\"p\" at=\"2026-11-16T22:20:03Z\" history=\"1\" -->\n![p](d.md.assets/p.2026-11-16T222003Z.svg)\n<!-- /mdsh:output -->\n"
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(dir, "d.md.assets")
	os.Mkdir(assets, 0o755)
	for _, f := range []string{"p.svg", "p.2026-11-16T222003Z.svg"} {
		os.WriteFile(filepath.Join(assets, f), []byte("<svg/>"), 0o644)
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.ClearEarlierRuns(0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(assets, "p.2026-11-16T222003Z.svg")); err != nil {
		t.Error("clearing deleted the file before saving, so Undo can't bring it back")
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(assets, "p.2026-11-16T222003Z.svg")); err == nil {
		t.Error("saving kept the cleared run's file")
	}
	if _, err := os.Stat(filepath.Join(assets, "p.svg")); err != nil {
		t.Error("saving deleted the current output's file")
	}
}

// Taking history= off a block warns, in the console, that the next
// write drops its earlier runs.
func TestTrimWarning(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	n, err := Open(write(t, strings.Replace(earlierDoc, " history=3}", " history=1}", 1)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "dropped 1 earlier run of block n: its history= keeps 1") {
		t.Errorf("console:\n%s", buf.String())
	}
	if strings.Contains(n.Text(), "history=2") {
		t.Errorf("saving kept run 2 back:\n%s", n.Text())
	}
}
