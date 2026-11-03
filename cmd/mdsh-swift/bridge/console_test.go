package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// TestConsoleFix checks that a warning's "fix" attribute goes into the
// entry's Fix when it is about a file, and stays in the message when it
// isn't.
func TestConsoleFix(t *testing.T) {
	var out bytes.Buffer
	h := newConsoleHandler(newConn(&out), slog.NewTextHandler(&bytes.Buffer{}, nil))
	log := slog.New(h)
	log.Warn("an output copy for a block that isn't there", "file", "/x.md", "line", 3, "fix", "Delete it")
	log.Warn("nowhere", "fix", "Delete it")
	var got []consoleEntry
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var e struct {
			Event string       `json:"event"`
			Data  consoleEntry `json:"data"`
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		got = append(got, e.Data)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if g := got[0]; g.Fix != "Delete it" || g.File != "/x.md" || g.Line != 3 || strings.Contains(g.Message, "fix=") {
		t.Errorf("entry about a file: %+v", g)
	}
	if g := got[1]; g.Fix != "" || !strings.Contains(g.Message, `fix="Delete it"`) {
		t.Errorf("entry about no file: %+v", g)
	}
}
