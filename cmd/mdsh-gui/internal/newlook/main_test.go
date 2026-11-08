package main

import (
	"os"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/looks"
)

// The new look's palettes come from style.css; if its layout changes,
// newlook must change with it.
func TestLookCSS(t *testing.T) {
	style, err := os.ReadFile("../../frontend/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css, err := lookCSS(string(style), "moss", "Moss", 50, "1.6")
	if err != nil {
		t.Fatal(err)
	}
	l, err := looks.Parse([]byte(css))
	if err != nil || l.ID != "moss" || l.Name != "Moss" || l.Order != 50 || l.Since != "1.6" || l.Tokens != "--moss-" {
		t.Errorf("front matter = %+v, %v", l, err)
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
