package fonts

import (
	"slices"
	"testing"
)

func TestTidyFonts(t *testing.T) {
	got := tidyFonts([]string{"menlo", ".AppleSystemUIFont", "Fira Code", "Bad;Name", "Fira Code", "Andale Mono", " Menlo2 "})
	want := []string{"Andale Mono", "Fira Code", "menlo", "Menlo2"}
	if !slices.Equal(got, want) {
		t.Errorf("tidyFonts = %q, want %q", got, want)
	}
}

func TestWithNerdFonts(t *testing.T) {
	all := []string{"Helvetica", "JetBrainsMono Nerd Font", "JetBrainsMono Nerd Font Mono", "Menlo", "Symbols Nerd Font Mono"}
	got := withNerdFonts(all, []string{"Menlo", "JetBrainsMono Nerd Font Mono"})
	want := []string{"JetBrainsMono Nerd Font", "JetBrainsMono Nerd Font Mono", "Menlo"}
	if !slices.Equal(got, want) {
		t.Errorf("withNerdFonts = %q, want %q", got, want)
	}
}

// Where fc-list runs, it lists some fonts; where it doesn't, none.
func TestSystemFonts(t *testing.T) {
	f := systemFonts()
	for _, m := range f.Mono {
		if !slices.Contains(f.All, m) {
			t.Errorf("mono font %q not in All", m)
		}
	}
	t.Logf("%d fonts, %d monospace", len(f.All), len(f.Mono))
}
