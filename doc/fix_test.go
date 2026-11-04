package doc

import (
	"strings"
	"testing"
)

func TestFixes(t *testing.T) {
	src := "# T\n\n```sh {#a depends=gone,b}\necho a\n```\n\n```sh {#b depends=name}\necho b\n```\n\n```textbox {#name var=NAME}\nj\n```\n\nText.\n\n```output {for=nope}\nold\n```\n\nMore.\n\n```output {for=nope}\nolder\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	fixes := d.Fixes()
	if len(fixes) != 3 {
		t.Fatalf("got %d fixes, want 3: %+v", len(fixes), fixes)
	}
	// Every fix is for a warning the document has, and makes it go.
	for _, f := range fixes {
		found := false
		for _, w := range d.Warnings {
			if strings.HasSuffix(w, ": "+f.Warning) {
				found = true
			}
		}
		if !found {
			t.Errorf("fix for %q, which isn't in %q", f.Warning, d.Warnings)
		}
		nd, err := Parse(f.Src)
		if err != nil {
			t.Fatalf("%s: %v\n%s", f.Label, err, f.Src)
		}
		for _, w := range nd.Warnings {
			if strings.HasSuffix(w, ": "+f.Warning) {
				t.Errorf("%s left %q:\n%s", f.Label, w, f.Src)
			}
		}
		// And only it.
		others := 0
		for _, w := range d.Warnings {
			if !strings.HasSuffix(w, ": "+f.Warning) {
				others++
			}
		}
		if len(nd.Warnings) != others {
			t.Errorf("%s: warnings %q, from %q", f.Label, nd.Warnings, d.Warnings)
		}
	}
	if got, want := string(fixes[0].Src), "# T\n\n```sh {#a depends=gone,b}\necho a\n```\n\n```sh {#b depends=name}\necho b\n```\n\n```textbox {#name var=NAME}\nj\n```\n\nText.\n\nMore.\n"; got != want {
		t.Errorf("copies deleted:\n%s\nwant\n%s", got, want)
	}
	if want := "Delete the 2 output copies at lines 17 and 23. Its for=nope names no block that has output, so it shows nothing."; fixes[0].Detail != want {
		t.Errorf("detail %q, want %q", fixes[0].Detail, want)
	}
	if !strings.Contains(string(fixes[1].Src), "```sh {#a depends=b}\n") {
		t.Errorf("gone not taken out:\n%s", fixes[1].Src)
	}
	if !strings.Contains(string(fixes[2].Src), "```sh {#b}\n") || fixes[2].Detail != "Take depends=name off block b. name is a textbox, which never runs." {
		t.Errorf("depends= not taken off: %q\n%s", fixes[2].Detail, fixes[2].Src)
	}
}
