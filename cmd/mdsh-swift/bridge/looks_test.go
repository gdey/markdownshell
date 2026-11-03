package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// notYet are mdsh-gui looks mdsh-swift doesn't draw yet.
var notYet = map[string]bool{}

// lookField is a string field in a look.css's TOML front matter. The
// test reads them with a pattern rather than package looks, so the
// bridge needs no TOML module of its own.
var lookField = regexp.MustCompile(`(?m)^(id|name|description|label|sprockets|rules|shadows) = ("(?:[^"\\]|\\.)*")$`)

// TestLooksAgree checks that Theme.swift names each of mdsh-gui's looks
// with the name, description and texture labels its look.css gives,
// since Settings › Look shows them.
func TestLooksAgree(t *testing.T) {
	files, err := filepath.Glob("../../mdsh-gui/frontend/looks/*/look.css")
	if err != nil || len(files) == 0 {
		t.Fatalf("no looks: %v", err)
	}
	src, err := os.ReadFile("../Sources/MdshSwift/Theme.swift")
	if err != nil {
		t.Fatal(err)
	}
	theme := string(src)
	for _, f := range files {
		css, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		front, _, ok := strings.Cut(strings.TrimPrefix(string(css), "/* +++\n"), "+++")
		if !ok {
			t.Fatalf("%s: no front matter", f)
		}
		var id string
		var want []string
		for _, m := range lookField.FindAllStringSubmatch(front, -1) {
			v, err := strconv.Unquote(m[2])
			if err != nil {
				t.Fatalf("%s: %s: %v", f, m[1], err)
			}
			if m[1] == "id" {
				id = v
			} else {
				want = append(want, v)
			}
		}
		if notYet[id] {
			continue
		}
		if id != "" && !slices.Contains(lookIDs, id) {
			t.Errorf("look %q: not in the bridge's lookIDs, so Settings would drop it", id)
		}
		if len(want) < 2 {
			t.Fatalf("%s: found only %q", f, want)
		}
		for _, w := range want {
			if !strings.Contains(theme, strconv.Quote(w)) {
				t.Errorf("look %q: Theme.swift lacks %q", id, w)
			}
		}
	}
}
