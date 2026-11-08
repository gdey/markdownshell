package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/trust"
)

const sourceSrc = "---\nmdsh:\n  env:\n    source: [./setup.sh]\n---\n```sh {#a}\necho a\n```\n"

// sourceModel opens sourceSrc beside a setup.sh that touches a marker.
func sourceModel(t *testing.T, st *trust.Store) (m *Model, file, marker string) {
	t.Helper()
	m, file = newModel(t, sourceSrc, Options{AutoSave: true, Trust: st})
	marker = filepath.Join(filepath.Dir(file), "SOURCED")
	if err := os.WriteFile(filepath.Join(filepath.Dir(file), "setup.sh"), []byte("touch "+marker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return m, file, marker
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestSourcesAskBeforeRun(t *testing.T) {
	for _, tc := range []struct {
		key           string
		ran, sourced  bool
		remembered    bool
		rememberedYes bool
	}{
		{"y", true, true, true, true},
		{"n", true, false, true, false},
		{"s", false, false, false, false},
		{"enter", false, false, false, false},
	} {
		st := trust.Open(filepath.Join(t.TempDir(), "sources.json"))
		m, file, marker := sourceModel(t, st)
		press(m, "enter")
		if m.srcAsk == nil {
			t.Fatalf("%s: not asked", tc.key)
		}
		if v := m.View(); !strings.Contains(v, "setup.sh") || !strings.Contains(v, "y yes") {
			t.Errorf("%s: question doesn't show the file:\n%s", tc.key, v)
		}
		if m.running || exists(marker) {
			t.Fatalf("%s: ran before the answer", tc.key)
		}
		press(m, tc.key)
		if m.running {
			pump(t, m)
		}
		if got := strings.Contains(read(t, file), "for=a"); got != tc.ran {
			t.Errorf("%s: ran = %v, want %v", tc.key, got, tc.ran)
		}
		if exists(marker) != tc.sourced {
			t.Errorf("%s: sourced = %v, want %v", tc.key, exists(marker), tc.sourced)
		}
		paths := []string{filepath.Join(filepath.Dir(file), "setup.sh")}
		allow, ok := st.Lookup(file, paths)
		if ok != tc.remembered || allow != tc.rememberedYes {
			t.Errorf("%s: remembered %v %v, want %v %v", tc.key, ok, allow, tc.remembered, tc.rememberedYes)
		}
	}
}

func TestSourcesAskBeforeHistory(t *testing.T) {
	m, _, marker := sourceModel(t, nil)
	press(m, "h")
	if m.srcAsk == nil || m.hist.open {
		t.Fatalf("History opened without asking (asked %v, open %v)", m.srcAsk != nil, m.hist.open)
	}
	if exists(marker) {
		t.Fatal("opening History sourced setup.sh before the answer")
	}
	press(m, "y")
	if !m.hist.open || !exists(marker) {
		t.Errorf("after yes: open %v, sourced %v", m.hist.open, exists(marker))
	}
}

func TestSourcesRemembered(t *testing.T) {
	st := trust.Open(filepath.Join(t.TempDir(), "sources.json"))
	m, file, marker := sourceModel(t, st)
	if err := st.Remember(file, []string{filepath.Join(filepath.Dir(file), "setup.sh")}, true); err != nil {
		t.Fatal(err)
	}
	press(m, "enter")
	if m.srcAsk != nil {
		t.Fatal("asked again for a remembered file")
	}
	pump(t, m)
	if !exists(marker) {
		t.Error("remembered yes didn't source setup.sh")
	}
}

func TestKeepEnd(t *testing.T) {
	if got := keepEnd("/a/very/long/folder/setup.sh", 12); got != "…er/setup.sh" {
		t.Errorf("keepEnd = %q", got)
	}
	if got := keepEnd("setup.sh", 12); got != "setup.sh" {
		t.Errorf("short = %q", got)
	}
}
