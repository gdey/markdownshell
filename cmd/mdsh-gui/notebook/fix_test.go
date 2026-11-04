package notebook

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

// An output copy for a block that isn't there is logged with its fix;
// the fix deletes it, which resolves the warning, and Undo puts it back.
func TestFixWarning(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	src := "```sh {#a}\necho a\n```\n\nText.\n\n```output {for=nope}\nold\n```\n"
	file := write(t, src)
	var resolved []string
	n, err := Open(file, Options{Resolved: func(f, w string) {
		if f != file {
			t.Errorf("resolved in %q, want %q", f, file)
		}
		resolved = append(resolved, w)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	const warning = `output copy for unknown block "nope"`
	if want := `fix="Delete the copy"`; !strings.Contains(buf.String(), want) {
		t.Errorf("no %s in:\n%s", want, buf.String())
	}
	f, err := n.DescribeFix(warning)
	if err != nil || !strings.HasPrefix(f.Detail, "Delete the output copy at line 7.") {
		t.Fatalf("DescribeFix = %+v, %v", f, err)
	}
	if err := n.FixWarning(warning); err != nil {
		t.Fatal(err)
	}
	if got, want := string(n.d.Src), "```sh {#a}\necho a\n```\n\nText.\n"; got != want {
		t.Errorf("fixed:\n%s\nwant\n%s", got, want)
	}
	if !slices.Equal(resolved, []string{warning}) {
		t.Errorf("resolved %q", resolved)
	}
	if err := n.FixWarning(warning); err == nil {
		t.Error("fixed it twice")
	}
	if _, err := n.Undo(); err != nil {
		t.Fatal(err)
	}
	if string(n.d.Src) != src {
		t.Errorf("undone:\n%s", n.d.Src)
	}
	// Back, it is logged again, as new.
	if got := strings.Count(buf.String(), "unknown block"); got != 2 {
		t.Errorf("logged %d times:\n%s", got, buf.String())
	}
}
