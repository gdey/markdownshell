package nvim

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct {
		line string
		want Version
	}{
		{"NVIM v0.11.4", Version{0, 11, 4}},
		{"NVIM v0.12.0-dev-1234+g5678abc", Version{0, 12, 0}},
		{"NVIM v0.10.0\r", Version{0, 10, 0}},
	} {
		got, err := parseVersion(tc.line)
		if err != nil || got != tc.want {
			t.Errorf("parseVersion(%q) = %v, %v; want %v", tc.line, got, err, tc.want)
		}
	}
	if _, err := parseVersion("VIM - Vi IMproved 9.1"); err == nil {
		t.Error("parseVersion of Vim's version: no error")
	}
}

func TestLess(t *testing.T) {
	if !(Version{0, 9, 5}).Less(MinVersion) {
		t.Error("0.9.5 is not older than the minimum")
	}
	if (Version{0, 10, 0}).Less(MinVersion) || (Version{1, 0, 0}).Less(MinVersion) {
		t.Error("0.10.0 or 1.0.0 is older than the minimum")
	}
}

// testNvim is the Neovim the tests run, from $MDSH_TEST_NVIM or found as
// the app would.
func testNvim(t *testing.T) string {
	t.Helper()
	f, err := Find(os.Getenv("MDSH_TEST_NVIM"))
	if err != nil {
		t.Skip("no Neovim to test with: ", err)
	}
	return f.Path
}

// wait waits for a value on c.
func wait[T any](t *testing.T, c chan T, what string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("no %s", what)
		panic("unreachable")
	}
}

func TestEditor(t *testing.T) {
	path := testNvim(t)
	redraws := make(chan [][]any, 100)
	changed := make(chan string, 10)
	written := make(chan string, 10)
	exited := make(chan error, 1)
	e, err := Start(Options{
		Path:    path,
		Cols:    40,
		Rows:    10,
		Name:    "mdsh://test.md",
		Text:    "# Title\n\nfirst line\n",
		Clean:   true,
		Redraw:  func(ev [][]any) { redraws <- ev },
		Changed: func(text string) { changed <- text },
		Write:   func(text string) { written <- text },
		Exit:    func(err error) { exited <- err },
	})
	if err != nil {
		t.Fatal(err)
	}

	// The grid shows the text.
	deadline := time.After(10 * time.Second)
	for screen := ""; !strings.Contains(screen, "first line"); {
		select {
		case ev := <-redraws:
			if _, err := json.Marshal(ev); err != nil {
				t.Fatal("redraw events don't go to JSON: ", err)
			}
			screen += gridText(ev)
		case <-deadline:
			t.Fatalf("the grid never showed the text; got %q", screen)
		}
	}

	if err := e.Input("Goadded<Esc>"); err != nil {
		t.Fatal(err)
	}
	if got, want := wait(t, changed, "change"), "# Title\n\nfirst line\nadded\n"; got != want {
		t.Errorf("changed text = %q, want %q", got, want)
	}

	// u undoes the edit, not the text the buffer started with.
	if err := e.Input("u"); err != nil {
		t.Fatal(err)
	}
	if got, want := wait(t, changed, "undo"), "# Title\n\nfirst line\n"; got != want {
		t.Errorf("text after u = %q, want %q", got, want)
	}

	if err := e.Input(":w<CR>"); err != nil {
		t.Fatal(err)
	}
	if got, want := wait(t, written, ":w"), "# Title\n\nfirst line\n"; got != want {
		t.Errorf("written text = %q, want %q", got, want)
	}

	// Text set from mdsh-gui isn't sent back as a change.
	if err := e.SetText("# Title\n\nnew\n"); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := e.v.ExecLua("return vim.api.nvim_buf_get_lines(0, 0, -1, false)", &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != "# Title\n\nnew" {
		t.Errorf("buffer after SetText = %q", got)
	}
	select {
	case text := <-changed:
		t.Errorf("SetText sent a change back: %q", text)
	case <-time.After(500 * time.Millisecond):
	}

	if err := e.Close(); err != nil {
		t.Log("close: ", err)
	}
	wait(t, exited, "exit")
}

// gridText is the text of the grid_line events in ev.
func gridText(ev [][]any) string {
	var b strings.Builder
	for _, e := range ev {
		if e[0] != "grid_line" {
			continue
		}
		for _, args := range e[1:] {
			a, _ := args.([]any)
			if len(a) < 4 {
				continue
			}
			cells, _ := a[3].([]any)
			for _, c := range cells {
				cell, _ := c.([]any)
				if len(cell) == 0 {
					continue
				}
				s, _ := cell[0].(string)
				n := 1
				if len(cell) == 3 {
					if r, ok := cell[2].(int64); ok {
						n = int(r)
					}
				}
				b.WriteString(strings.Repeat(s, n))
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}
