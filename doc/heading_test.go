package doc

import (
	"testing"
)

func TestHeadings(t *testing.T) {
	src := "---\ntitle: x\n---\n# A\n\n```sh {#a}\necho a\n```\n\n## B\n\n- # not a section\n\n```sh {#b}\necho b\n```\n\nC\n-\n\n```sh {#c}\necho c\n```\n\n# D\n\n```sh {#d}\necho d\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	hs := d.Headings()
	type got struct {
		level    int
		text     string
		from, to int
	}
	var gs []got
	for _, h := range hs {
		from, to := d.BlocksIn(h.Span)
		line := string(d.Src[h.Span.Start : h.Span.Start+lineEnd(d.Src[h.Span.Start:], 0)])
		gs = append(gs, got{h.Level, line, from, to})
	}
	want := []got{
		{1, "# A\n", 0, 3},
		{2, "## B\n", 1, 2},
		{2, "C\n", 2, 3},
		{1, "# D\n", 3, 4},
	}
	if len(gs) != len(want) {
		t.Fatalf("headings = %+v, want %+v", gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Errorf("heading %d = %+v, want %+v", i, gs[i], want[i])
		}
	}
	if h, ok := SectionAt(hs, d.Blocks[2].Span.Start); !ok || h != hs[2] {
		t.Errorf("SectionAt(c) = %+v, %v; want C", h, ok)
	}
	if _, ok := SectionAt(hs, 0); ok {
		t.Error("SectionAt before every heading found one")
	}
}
