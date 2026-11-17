package trust_test

import (
	"fmt"
	"path/filepath"
	"slices"
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

// Allowed lists the documents last allowed, and Forget drops an
// answer so the document is asked again.
func TestAllowedForget(t *testing.T) {
	st := trust.Open(filepath.Join(t.TempDir(), "outside.json"))
	if err := st.Remember("/a.md", []string{"../x"}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.Remember("/b.md", []string{"../y"}, false); err != nil {
		t.Fatal(err)
	}
	if got := st.Allowed(); !slices.Equal(got, []string{"/a.md"}) {
		t.Errorf("Allowed = %q, want /a.md", got)
	}
	if err := st.Forget("/a.md"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Lookup("/a.md", []string{"../x"}); ok {
		t.Error("/a.md still answered after Forget")
	}
	if allow, ok := st.Lookup("/b.md", []string{"../y"}); !ok || allow {
		t.Errorf("/b.md = %v, %v; want refused", allow, ok)
	}
	if got := (*trust.Store)(nil).Allowed(); got != nil {
		t.Errorf("nil store: Allowed = %q", got)
	}
}

// Add grows a document's list an item at a time; Has finds them, for
// that document only, and Forget drops the list.
func TestAddHas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "images.json")
	st := trust.Open(path)
	if st.Has("/a.md", "https://x/1.png") {
		t.Fatal("empty store has an item")
	}
	for _, item := range []string{"https://x/1.png", "https://x/2.png", "https://x/1.png"} {
		if err := st.Add("/a.md", item); err != nil {
			t.Fatal(err)
		}
	}
	if !st.Has("/a.md", "https://x/1.png") || !st.Has("/a.md", "https://x/2.png") {
		t.Error("lost an item")
	}
	if st.Has("/b.md", "https://x/1.png") {
		t.Error("another document has the item")
	}
	// A new store on the same file, as another front end would open it.
	if !trust.Open(path).Has("/a.md", "https://x/2.png") {
		t.Error("not kept on disk")
	}
	for i := range trust.MaxItems + 5 {
		st.Add("/a.md", fmt.Sprintf("https://x/%d.png", i+10))
	}
	if st.Has("/a.md", "https://x/2.png") {
		t.Error("oldest item kept past trust.MaxItems")
	}
	if err := st.Forget("/a.md"); err != nil || st.Has("/a.md", "https://x/20.png") {
		t.Errorf("forget: %v", err)
	}
	var nilStore *trust.Store
	if nilStore.Has("/a.md", "x") || nilStore.Add("/a.md", "x") != nil {
		t.Error("nil store")
	}
}
