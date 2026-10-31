package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

func write(t *testing.T, src string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

const src = "# Title\n\nSome *prose*.\n\n```sh {#hello}\necho hello\n```\n\n```go\nfmt.Println(\"not run\")\n```\n\n```sh {#fail}\n(exit 3)\n```\n"

func TestCells(t *testing.T) {
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	cells := n.Cells()
	var kinds []string
	for _, c := range cells {
		if c.Block < 0 {
			kinds = append(kinds, "prose")
		} else {
			kinds = append(kinds, c.ID)
		}
	}
	if got, want := strings.Join(kinds, " "), "prose prose hello prose fail"; got != want {
		t.Fatalf("cells = %s, want %s", got, want)
	}
	if !strings.Contains(cells[1].HTML, "<em>prose</em>") {
		t.Errorf("prose HTML = %q", cells[1].HTML)
	}
	if !strings.Contains(cells[3].HTML, "not run") {
		t.Errorf("the go block should be drawn as prose: %+v", cells[3])
	}
}

func TestRunAndSave(t *testing.T) {
	file := write(t, src)
	var mu sync.Mutex
	var out strings.Builder
	n, err := Open(file, Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == "stdout" {
			out.WriteString(e.Data)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunFrom(0); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello\n" {
		t.Errorf("streamed stdout = %q", got)
	}
	cells := n.Cells()
	if c := cells[2]; !c.Ran || c.Failed || !strings.Contains(c.Output, "hello") {
		t.Errorf("hello cell = %+v", c)
	}
	if c := cells[4]; !c.Failed || c.Status != 3 {
		t.Errorf("fail cell = %+v", c)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	if !strings.Contains(string(saved), "```output {for=hello exit=0}\nhello\n```") {
		t.Errorf("saved file:\n%s", saved)
	}
}

// TestSaveNamesBlocks checks that saving writes an id into each unnamed
// block, keeping its result, and that cells show it by line.
func TestSaveNamesBlocks(t *testing.T) {
	file := write(t, "# T\n\n```sh\necho hi\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunFrom(0); err != nil {
		t.Fatal(err)
	}
	if c := n.Cells()[1]; c.ID != "block-1" || c.Name != "line 3" {
		t.Errorf("cell before saving: id %q, name %q", c.ID, c.Name)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	c := n.Cells()[1]
	if !doc.GeneratedID(c.ID) || c.Name != "line 3" || !c.Ran || !strings.Contains(c.Output, "hi") {
		t.Errorf("cell after saving = %+v", c)
	}
	saved, _ := os.ReadFile(file)
	if want := "```sh {#" + c.ID + "}\necho hi\n```\n\n```output {for=" + c.ID + " exit=0}\nhi\n```\n"; !strings.HasSuffix(string(saved), want) {
		t.Errorf("saved file:\n%s", saved)
	}
}

func TestConfirm(t *testing.T) {
	file := write(t, "```sh {#ask confirm=\"Really?\"}\necho yes\n```\n")
	var asked string
	n, err := Open(file, Options{Confirm: func(i int, q string) run.Answer {
		asked = q
		return run.Skip
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunFrom(0); err != nil {
		t.Fatal(err)
	}
	if asked != "Really?" {
		t.Errorf("asked %q", asked)
	}
	if n.Cells()[0].Ran {
		t.Error("a skipped block ran")
	}

	// Running the block itself is the answer.
	asked = ""
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if asked != "" {
		t.Errorf("RunBlock asked %q", asked)
	}
	if !n.Cells()[0].Ran {
		t.Error("RunBlock didn't run the block")
	}
}

func TestSections(t *testing.T) {
	file := write(t, "```sh {#os}\nOS=Plan9\n```\n\n"+
		"<!-- mdsh:if {{#os/env(OS)}}==Linux -->\n## Linux\n\n```sh {#linux}\nuname -r\n```\n<!-- mdsh:end -->\n\n"+
		"<!-- mdsh:if {{#os/env(OS)}}==Plan9 -->\n## Plan 9\n\n```sh {#plan9}\necho glenda\n```\n<!-- mdsh:end -->\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ids := func() string {
		var s []string
		for _, c := range n.Cells() {
			if c.Block >= 0 {
				s = append(s, c.ID)
			} else if strings.Contains(c.HTML, "<h2>") {
				s = append(s, "h2")
			}
		}
		return strings.Join(s, " ")
	}
	if got, want := ids(), "os"; got != want {
		t.Errorf("before running: cells %q, want %q", got, want)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if got, want := ids(), "os h2 plan9"; got != want {
		t.Errorf("after running: cells %q, want %q", got, want)
	}
}

func TestDirtyAndChanged(t *testing.T) {
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if n.Dirty() || n.Changed() {
		t.Fatalf("a file just opened is neither dirty nor changed")
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if !n.Dirty() {
		t.Errorf("running a block should make the notebook dirty")
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if n.Dirty() || n.Changed() {
		t.Errorf("after Save the notebook is dirty %v, changed %v", n.Dirty(), n.Changed())
	}
	if err := os.WriteFile(file, []byte(src+"\nMore.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !n.Changed() {
		t.Errorf("Changed should notice the file being written")
	}
}

func TestSaveAs(t *testing.T) {
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "copy.md")
	if err := n.SaveAs(other); err != nil {
		t.Fatal(err)
	}
	if n.File() != other {
		t.Errorf("File = %s, want %s", n.File(), other)
	}
	got, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "```output {for=hello exit=0}\nhello\n```") {
		t.Errorf("the copy lacks the output:\n%s", got)
	}
	if orig, _ := os.ReadFile(file); string(orig) != src {
		t.Errorf("SaveAs changed the original file:\n%s", orig)
	}
	// The shells now start in the new folder.
	var mu sync.Mutex
	var out strings.Builder
	n2, err := Open(write(t, "```sh {#pwd}\npwd\n```\n"), Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == "stdout" {
			out.WriteString(e.Data)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n2.Close()
	dir := t.TempDir()
	if err := n2.SaveAs(filepath.Join(dir, "pwd.md")); err != nil {
		t.Fatal(err)
	}
	if err := n2.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if real, _ := filepath.EvalSymlinks(dir); strings.TrimSpace(out.String()) != dir && strings.TrimSpace(out.String()) != real {
		t.Errorf("pwd = %q, want %s", out.String(), dir)
	}
}

func TestRunOptions(t *testing.T) {
	var mu sync.Mutex
	var out strings.Builder
	n, err := Open(write(t, "```sh {#args}\necho \"$1-$2\"\n```\n\n```sh {#two}\necho two\n```\n"), Options{
		Run: RunOptions{Args: []string{"a", "b"}},
		OnEvent: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			if e.Kind == "stdout" {
				out.WriteString(e.Data)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.SetRunOptions(RunOptions{Args: []string{"c", "d"}, Only: "args"}); err != nil {
		t.Fatal(err)
	}
	if c := n.Cells(); !c[0].Ran {
		t.Errorf("output from before the change should be kept")
	}
	if err := n.RunFrom(0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got, want := out.String(), "a-b\nc-d\n"; got != want {
		t.Errorf("output = %q, want %q (only=args leaves out two)", got, want)
	}
	if n.RunOptions().Only != "args" {
		t.Errorf("RunOptions = %+v", n.RunOptions())
	}
}

// Source runs in the shell before the first block, as --source does.
func TestRunOptionsSource(t *testing.T) {
	var mu sync.Mutex
	var out strings.Builder
	n, err := Open(write(t, "```sh\ngreet\n```\n"), Options{
		Run: RunOptions{Source: "greet() { echo hi from source; }"},
		OnEvent: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			if e.Kind == "stdout" {
				out.WriteString(e.Data)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got, want := out.String(), "hi from source\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// A background block returns at once and shows as running; the
// notebook checks on it while nothing runs and fills in its output when
// it ends.
func TestBackgroundBlock(t *testing.T) {
	src := "```sh {#bg background}\nsleep 0.3; echo from bg\n```\n\n```sh {#fg}\necho fg\n```\n"
	var mu sync.Mutex
	var kinds []string
	done := make(chan struct{}, 1)
	n, err := Open(write(t, src), Options{OnEvent: func(e Event) {
		mu.Lock()
		kinds = append(kinds, fmt.Sprintf("%s %d", e.Kind, e.Block))
		mu.Unlock()
		if e.Kind == "done" && e.Block == 0 {
			done <- struct{}{}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if c := blockCell(t, n.Cells(), "bg"); !c.Running || c.Ran {
		t.Errorf("bg cell after RunBlock: %+v; want running", c)
	}
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the background block never ended")
	}
	// The cell is filled in once reap has folded the result in.
	deadline := time.Now().Add(5 * time.Second)
	for {
		c := blockCell(t, n.Cells(), "bg")
		if !c.Running && strings.Contains(c.Output, "from bg") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bg cell after it ended: %+v", c)
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if got, want := strings.Join(kinds, ","), "background 0,started 1,stdout 1,done 1,done 0"; got != want {
		t.Errorf("events = %s, want %s", got, want)
	}
}

// Closing a notebook kills a background block still running, without
// waiting for it.
func TestBackgroundClose(t *testing.T) {
	n, err := Open(write(t, "```sh {#bg background}\nsleep 60\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	n.Close()
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Close took %s", d)
	}
}

func blockCell(t *testing.T, cells []Cell, id string) Cell {
	t.Helper()
	for _, c := range cells {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no cell %s", id)
	return Cell{}
}

func TestClearOutputs(t *testing.T) {
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunFrom(0); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if err := n.ClearOutputs(); err != nil {
		t.Fatal(err)
	}
	if got := n.Text(); got != src {
		t.Errorf("text after ClearOutputs:\n%s\nwant:\n%s", got, src)
	}
	for _, c := range n.Cells() {
		if c.Block >= 0 && (c.Ran || c.Failed || c.Output != "") {
			t.Errorf("cell after ClearOutputs = %+v", c)
		}
	}
	if !n.Dirty() {
		t.Errorf("ClearOutputs should leave the notebook to be saved")
	}
	// The shells started again: a block runs, and its output comes back.
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if c := n.Cells()[2]; !c.Ran || !strings.Contains(c.Output, "hello") {
		t.Errorf("hello cell after running again = %+v", c)
	}
}

// After Save As in the same folder, assets go to the new file's assets
// folder, and the ones made before are copied there with their links.
func TestSaveAsAssets(t *testing.T) {
	const pics = "```sh {#old out=screenshot}\necho old\n```\n\n```sh {#now out=screenshot}\necho now\n```\n\n```sh {#later out=screenshot}\necho later\n```\n"
	file := write(t, pics)
	dir := filepath.Dir(file)
	// old ran and was saved before this session.
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	n.Close()

	n, err = Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "copy.md")
	if err := n.SaveAs(other); err != nil {
		t.Fatal(err)
	}
	if err := n.RunBlock(2); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "now", "later"} {
		link := fmt.Sprintf("![%s](copy.md.assets/%s.svg)", id, id)
		if !strings.Contains(string(got), link) {
			t.Errorf("the copy lacks %s:\n%s", link, got)
		}
		if _, err := os.Stat(filepath.Join(dir, "copy.md.assets", id+".svg")); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "doc.md.assets", "later.svg")); err == nil {
		t.Error("a block run after Save As wrote to the old assets folder")
	}
	orig, _ := os.ReadFile(file)
	if !strings.Contains(string(orig), "![old](doc.md.assets/old.svg)") || strings.Contains(string(orig), "copy.md") {
		t.Errorf("SaveAs changed the original file:\n%s", orig)
	}
}

// Names lists the files in the assets folder, for res:// completion.
func TestNamesAssets(t *testing.T) {
	file := write(t, src)
	os.WriteFile(filepath.Join(filepath.Dir(file), "notes.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(file), ".env"), nil, 0o644)
	for _, f := range []string{"a.png", "sub/b.md", ".hidden", ".git/x"} {
		p := filepath.Join(file+".assets", filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	names := n.Names()
	if got := fmt.Sprint(names.Assets); got != "[a.png sub/b.md]" {
		t.Errorf("assets = %s", got)
	}
	if names.AssetsDir != "doc.md.assets" {
		t.Errorf("assets dir = %s", names.AssetsDir)
	}
	// Files beside the document leave out the document, hidden files and
	// the assets folder.
	if got := fmt.Sprint(names.Files); got != "[notes.txt]" {
		t.Errorf("files = %s", got)
	}
}

// An include of a file a block writes shows it after the block runs, and
// a typed include shows its file's lines without a run.
func TestIncludeFresh(t *testing.T) {
	file := write(t, "```sh {#shot out=screenshot}\necho hi\n```\n\n```svg {<include file=res://shot.svg}\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "```svg {<include file=res://shot.svg}\n<svg") {
		t.Errorf("include not read after the run:\n%s", n.Text())
	}

	os.WriteFile(filepath.Join(filepath.Dir(file), "n.txt"), []byte("typed in\n"), 0o644)
	if err := n.SetText(n.Text() + "\n```text {<include file=n.txt}\n```\n"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range n.Cells() {
		found = found || strings.Contains(c.HTML, "typed in")
	}
	if !found {
		t.Errorf("typed include not shown: %+v", n.Cells())
	}
}

func TestDepend(t *testing.T) {
	file := write(t, "```sh {#build confirm}\necho built\n```\n\n```sh {#deploy depends=build}\necho deploy\n```\n")
	var asked []string
	n, err := Open(file, Options{
		Confirm: func(i int, q string) run.Answer {
			asked = append(asked, fmt.Sprintf("confirm %d", i))
			return run.Stop
		},
		Depend: func(i, of int, q string) run.Answer {
			asked = append(asked, fmt.Sprintf("depend %d of %d: %s", i, of, q))
			return run.Go
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	// Saying to run build answers its confirm too.
	if want := []string{"depend 0 of 1: deploy depends on this block, which hasn't run yet. Run it first?"}; !slices.Equal(asked, want) {
		t.Errorf("asked %q, want %q", asked, want)
	}
	if c := n.Cells(); !c[0].Ran || !c[1].Ran {
		t.Errorf("build ran %v, deploy ran %v", c[0].Ran, c[1].Ran)
	}
}
