package notebook

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gdey/markdownshell/run"
)

// The History pane's prompt starts the shell, so it asks about the
// env.source files first, and starts none without a yes (security
// review v1.4.0, finding 1).
func TestPromptAsksBeforeSourcing(t *testing.T) {
	file := write(t, "---\nmdsh:\n  env:\n    source: [./setup.sh]\n---\n```sh {#a}\necho a\n```\n")
	dir := filepath.Dir(file)
	marker := filepath.Join(dir, "SOURCED")
	if err := os.WriteFile(filepath.Join(dir, "setup.sh"), []byte("touch "+marker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourced := func() bool { _, err := os.Stat(marker); return err == nil }
	var asked [][]string
	answer := run.Stop
	n, err := Open(file, Options{Sources: func(paths []string) run.Answer {
		asked = append(asked, paths)
		return answer
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if _, err := n.SetHistory(true, false); err != nil {
		t.Fatal(err)
	}
	if len(asked) > 0 || sourced() {
		t.Fatalf("opening the file asked %d times, sourced %v; want neither yet", len(asked), sourced())
	}
	if _, err := n.Prompt(); !errors.Is(err, run.ErrSourcesNotApproved) {
		t.Errorf("Prompt after Cancel: %v, want ErrSourcesNotApproved", err)
	}
	if sourced() {
		t.Fatal("the prompt sourced setup.sh after Cancel")
	}
	answer = run.Go
	if _, err := n.Prompt(); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{filepath.Join(dir, "setup.sh")}, {filepath.Join(dir, "setup.sh")}}; !slices.EqualFunc(asked, want, slices.Equal) || !sourced() {
		t.Errorf("asked %q, sourced %v; want %q and sourced", asked, sourced(), want)
	}
}
