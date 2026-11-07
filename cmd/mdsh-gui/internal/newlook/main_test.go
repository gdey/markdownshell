package main

import (
	"os"
	"strings"
	"testing"
)

// The new look's palettes come from style.css; if its layout changes,
// newlook must change with it.
func TestLookCSS(t *testing.T) {
	style, err := os.ReadFile("../../frontend/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css, err := lookCSS(string(style), "moss", "Moss")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`:root[data-look="moss"][data-scheme="dark"]`, "--bg:", "--term-bg:", "--font-ui:", "--moss-side:"} {
		if !strings.Contains(css, want) {
			t.Errorf("new look.css has no %s", want)
		}
	}
}

func TestNextMinor(t *testing.T) {
	if got := maxSince("1.9", "1.10"); got != "1.10" {
		t.Errorf("maxSince = %s", got)
	}
	if got, _ := nextMinor("1.10"); got != "1.11" {
		t.Errorf("nextMinor = %s", got)
	}
}
