package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// hardHint is a shortcut written out for both platforms in a string the
// page shows, as "cmd/ctrl+shift+H"; frontend/src/keys.ts writes each
// one for the platform instead.
var hardHint = regexp.MustCompile(`"[^"\n]*(?i:cmd/ctrl|ctrl/cmd)[^"\n]*"`)

func TestShortcutHintsAreWrittenForThePlatform(t *testing.T) {
	files, err := filepath.Glob("../../web/src/*.ts")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, "../../web/index.html", "../../web/window.html", "../../web/settings.html")
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range hardHint.FindAll(src, -1) {
			t.Errorf("%s: %s: use data-key or keys.ts's hinted", f, m)
		}
	}
}
