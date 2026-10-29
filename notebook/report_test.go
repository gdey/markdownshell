package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/report"
)

func TestReport(t *testing.T) {
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if ran, total := n.Ran(); ran != 1 || total != 2 {
		t.Errorf("Ran = %d of %d, want 1 of 2", ran, total)
	}

	// The window's output, not yet saved.
	md, err := n.MakeReport(Report{HideCode: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(md); strings.Contains(s, "echo hello") || !strings.Contains(s, "hello\n```") {
		t.Errorf("window report:\n%s", s)
	}
	// The saved file has none.
	md, _ = n.MakeReport(Report{Saved: true})
	if strings.Contains(string(md), "```output") {
		t.Errorf("saved report has output:\n%s", md)
	}

	dest := filepath.Join(filepath.Dir(file), "r.html")
	if err := n.WriteReport(Report{Format: report.HTML, Dest: dest}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); !strings.Contains(string(b), "<title>doc</title>") {
		t.Errorf("r.html:\n%s", b)
	}
}

func TestReportCommand(t *testing.T) {
	file := write(t, src)
	n, err := Open(file, Options{Run: RunOptions{Args: []string{"a b"}, Only: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	home := filepath.Dir(filepath.Dir(file))
	in := func(name string) string { return filepath.Join(filepath.Dir(file), name) }
	for _, tc := range []struct {
		r    Report
		want string
	}{
		{Report{Format: report.PDF, Ran: true, Dest: filepath.Join(home, "r.pdf")}, "mdsh --only hello --pdf ~/r.pdf doc.md 'a b'"},
		{Report{Format: report.HTML, Saved: true, HideCode: true, Dest: "/x y/r.html"}, "mdsh -n --hide-code --html '/x y/r.html' doc.md"},
		{Report{Format: report.Markdown, Ran: true, HideOutput: true, Dest: in("r.md")}, "mdsh --only hello --hide-output doc.md 'a b' > r.md"},
		{Report{Format: report.Markdown, Dest: "/r.md"}, "mdsh -n -o /r.md doc.md"},
	} {
		if got := n.ReportCommand(tc.r, home); got != tc.want {
			t.Errorf("%+v:\n got %s\nwant %s", tc.r, got, tc.want)
		}
	}
}

// TestReportMode checks that a report's conditions see $MDSH_MODE
// report, while the window sees interactive.
func TestReportMode(t *testing.T) {
	src := "Kept.\n\n<!-- mdsh:if {{$MDSH_MODE}}!=report -->\nWindow only.\n<!-- mdsh:end -->\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var html string
	for _, c := range n.Cells() {
		html += c.HTML
	}
	if !strings.Contains(html, "Window only.") {
		t.Errorf("window: %s", html)
	}
	md, err := n.MakeReport(Report{})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(md); strings.Contains(s, "Window only.") || !strings.Contains(s, "Kept.") {
		t.Errorf("report:\n%s", s)
	}
}
