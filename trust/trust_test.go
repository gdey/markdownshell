package trust_test

import (
	"path/filepath"
	"testing"

	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/trust"
)

func TestAsk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mdsh", "sources.json")
	st := trust.Open(path)
	asked := 0
	answer := run.Go
	ask := func([]string) run.Answer { asked++; return answer }
	approve := st.Ask("/d/doc.md", ask)

	srcs := []string{"/d/setup.sh"}
	if a := approve(srcs); a != run.Go || asked != 1 {
		t.Fatalf("first: %v, asked %d", a, asked)
	}
	// Remembered, in a new store on the same file as another front end
	// would open.
	if a := trust.Open(path).Ask("/d/doc.md", nil)(srcs); a != run.Go {
		t.Errorf("remembered: %v, want Go", a)
	}
	// Another document isn't covered.
	if a := trust.Open(path).Ask("/d/other.md", nil)(srcs); a != run.Stop {
		t.Errorf("other file: %v, want Stop", a)
	}
	// A changed list asks again, and a Skip is remembered as no.
	answer = run.Skip
	more := []string{"/d/setup.sh", "/home/me/.evil"}
	if a := approve(more); a != run.Skip || asked != 2 {
		t.Errorf("changed list: %v, asked %d", a, asked)
	}
	if a := approve(more); a != run.Skip || asked != 2 {
		t.Errorf("remembered no: %v, asked %d", a, asked)
	}
	// Stop isn't remembered.
	answer = run.Stop
	if a := st.Ask("/d/third.md", ask)(srcs); a != run.Stop || asked != 3 {
		t.Errorf("stop: %v, asked %d", a, asked)
	}
	if _, ok := st.Lookup("/d/third.md", srcs); ok {
		t.Error("Stop was remembered")
	}
}

func TestNilStore(t *testing.T) {
	var st *trust.Store
	if a := st.Ask("/d/doc.md", func([]string) run.Answer { return run.Go })([]string{"x"}); a != run.Go {
		t.Errorf("nil store: %v", a)
	}
}
