package notebook

import (
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// applyEvents makes events on text, as the source pane does.
func applyEvents(t *testing.T, text string, events []LineEvent) string {
	t.Helper()
	for _, e := range events {
		got, err := applyLines([]byte(text), []LineEdit{{e.First, e.Last, e.Text}})
		if err != nil {
			t.Fatalf("event %+v on %q: %v", e, text, err)
		}
		text = string(got)
	}
	return text
}

func TestLineDiff(t *testing.T) {
	for _, c := range []struct {
		a, b        string
		first, last int
		text        string
	}{
		{"a\nb\nc\n", "a\nB\nc\n", 1, 2, "B\n"},
		{"a\nb\nc\n", "a\nc\n", 1, 2, ""},
		{"a\nc\n", "a\nb\nc\n", 1, 1, "b\n"},
		{"ab\n", "aXb\n", 0, 1, "aXb\n"},
		{"a\nb", "a\nbc", 1, 2, "bc"},
		{"a\nb", "a\nb\n", 1, 2, "b\n"},
		{"", "x\n", 0, 0, "x\n"},
		{"x\n", "", 0, 1, ""},
		{"x\nx\nx\n", "x\nx\nx\nx\n", 3, 3, "x\n"},
	} {
		f, l, text := lineDiff([]byte(c.a), []byte(c.b))
		if f != c.first || l != c.last || text != c.text {
			t.Errorf("lineDiff(%q, %q) = %d, %d, %q; want %d, %d, %q", c.a, c.b, f, l, text, c.first, c.last, c.text)
		}
	}
}

// TestLineDiffRandom checks that lineDiff's event takes a to b on random
// texts.
func TestLineDiffRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"a", "b", "\n", "\n\n", "xy", ""}
	text := func() string {
		var s strings.Builder
		for range r.IntN(12) {
			s.WriteString(pieces[r.IntN(len(pieces))])
		}
		return s.String()
	}
	for range 20000 {
		a, b := text(), text()
		f, l, s := lineDiff([]byte(a), []byte(b))
		if got := applyEvents(t, a, []LineEvent{{First: f, Last: l, Text: s}}); got != b {
			t.Fatalf("lineDiff(%q, %q) = %d, %d, %q, which makes %q", a, b, f, l, s, got)
		}
	}
}

func TestApplyLinesBad(t *testing.T) {
	for _, c := range []struct {
		src   string
		edits []LineEdit
	}{
		{"a\n", []LineEdit{{2, 2, "x\n"}}},
		{"a\nb\n", []LineEdit{{1, 2, ""}, {0, 1, ""}}},
		{"a\nb\n", []LineEdit{{0, 1, "x"}}},
		{"a", []LineEdit{{1, 1, "x\n"}}},
		{"a\n", []LineEdit{{1, 0, ""}}},
	} {
		if _, err := applyLines([]byte(c.src), c.edits); !errors.Is(err, ErrBadEdit) {
			t.Errorf("applyLines(%q, %v): %v, want ErrBadEdit", c.src, c.edits, err)
		}
	}
}

// TestEdit types into a notebook by line edits, as the source pane
// does, while other changes come in through SetText, as a run's output
// does, and checks that a pane catching up by the events since its
// version ends with Go's text.
func TestEdit(t *testing.T) {
	src := "# Title\n\nSome text.\n\n```sh\necho hi\n```\n\nMore text.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	text, v := n.TextAt()
	if text != src {
		t.Fatalf("TextAt = %q", text)
	}
	rep, err := n.Edit(v, []LineEdit{{2, 3, "Some new text.\n"}, {8, 9, "More.\nAnd more.\n"}})
	if err != nil || !rep.Applied {
		t.Fatalf("Edit: %+v, %v", rep, err)
	}
	want := "# Title\n\nSome new text.\n\n```sh\necho hi\n```\n\nMore.\nAnd more.\n"
	if got := n.Text(); got != want {
		t.Fatalf("after Edit: %q, want %q", got, want)
	}
	if got := applyEvents(t, src, n.LinesSince(v).Events); got != want {
		t.Errorf("events since %v make %q, want %q", v, got, want)
	}
	if rep.Version.Tick != v.Tick+2 {
		t.Errorf("Edit's version %v, want two ticks on from %v", rep.Version, v)
	}
	// Something else changes the file; an edit against the old version
	// is turned down with the events that catch it up.
	if ok, err := n.SetText(want, "# Title\n\nSome new text.\n\n```sh\necho hi\n```\n\nMore.\nAnd more.\nEnd.\n"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	stale, err := n.Edit(rep.Version, []LineEdit{{0, 1, "# Name\n"}})
	if err != nil || stale.Applied || stale.Reset {
		t.Fatalf("stale Edit: %+v, %v", stale, err)
	}
	if got := applyEvents(t, want, stale.Events); got != n.Text() {
		t.Errorf("events since %v make %q, want %q", rep.Version, got, n.Text())
	}
	if again, err := n.Edit(stale.Version, []LineEdit{{0, 1, "# Name\n"}}); err != nil || !again.Applied {
		t.Fatalf("Edit again: %+v, %v", again, err)
	}
	if got := n.Text(); !strings.HasPrefix(got, "# Name\n") || !strings.HasSuffix(got, "End.\n") {
		t.Errorf("after both: %q", got)
	}
	// Another document's version, or one past the kept events, is reset.
	if l := n.LinesSince(Version{Doc: v.Doc + 1000}); !l.Reset {
		t.Errorf("another document's version: %+v, want Reset", l)
	}
}

// TestEditKeepsOutput checks that a block whose code an edit doesn't
// touch keeps its output, as SetText does.
func TestEditKeepsOutput(t *testing.T) {
	n, err := Open(write(t, "Intro.\n\n```sh\necho hi\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	text, v := n.TextAt()
	if !strings.Contains(text, "hi\n") {
		t.Fatalf("no output in %q", text)
	}
	if rep, err := n.Edit(v, []LineEdit{{0, 1, "Intro, edited.\n"}}); err != nil || !rep.Applied {
		t.Fatal(rep, err)
	}
	if got := n.Text(); !strings.HasPrefix(got, "Intro, edited.\n") || !strings.Contains(got[len("Intro, edited.\n"):], "hi\n") {
		t.Errorf("after the edit: %q", got)
	}
}

func TestLineLogKeeps(t *testing.T) {
	l := newLineLog(nil)
	v := l.version()
	for range keepEvents + 10 {
		l.add(0, 0, "x\n")
	}
	if _, ok := l.since(v); ok {
		t.Error("since a version past the kept events: ok")
	}
	ev, ok := l.since(Version{Doc: l.doc, Tick: l.tick - 3})
	if !ok || len(ev) != 3 || ev[0].Tick != l.tick-2 {
		t.Errorf("since three ticks back: %v, %v", ev, ok)
	}
	l.add(0, 0, strings.Repeat("y", keepBytes+1))
	if _, ok := l.since(Version{Doc: l.doc, Tick: l.tick - 1}); ok {
		t.Error("an event bigger than keepBytes is kept")
	}
}

func TestBlocksAt(t *testing.T) {
	src := "# T\n\n```sh {#a}\necho a\n```\n\n```textbox {var=X #x}\nhi\n```\n\n```sh {#b depends=a,x,nope}\necho b\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	got, _ := n.BlocksAt()
	if want := SourceBlocks(src); !slices.Equal(got, want) {
		t.Errorf("BlocksAt = %v, want %v", got, want)
	}
	problems, _ := n.DependsAt()
	if want := []DependsProblem{{Line: 11, Name: "x", Input: true}, {Line: 11, Name: "nope"}}; !slices.Equal(problems, want) {
		t.Errorf("DependsAt = %v, want %v", problems, want)
	}
}
