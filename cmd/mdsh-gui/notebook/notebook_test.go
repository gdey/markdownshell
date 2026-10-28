package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	if got, want := strings.Join(kinds, " "), "prose hello prose fail"; got != want {
		t.Fatalf("cells = %s, want %s", got, want)
	}
	if !strings.Contains(cells[0].HTML, "<em>prose</em>") {
		t.Errorf("prose HTML = %q", cells[0].HTML)
	}
	if !strings.Contains(cells[2].HTML, "not run") {
		t.Errorf("the go block should be drawn with the prose: %q", cells[2].HTML)
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
	if c := cells[1]; !c.Ran || c.Failed || !strings.Contains(c.Output, "hello") {
		t.Errorf("hello cell = %+v", c)
	}
	if c := cells[3]; !c.Failed || c.Status != 3 {
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
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if asked != "Really?" {
		t.Errorf("asked %q", asked)
	}
	if n.Cells()[0].Ran {
		t.Error("a skipped block ran")
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
