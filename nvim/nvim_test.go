package nvim

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/lsp"
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
	changed := make(chan Lines, 10)
	written := make(chan int, 10)
	exited := make(chan error, 1)
	e, err := Start(Options{
		Path:   path,
		Cols:   40,
		Rows:   10,
		Name:   "mdsh://test.md",
		Text:   "# Title\n\nfirst line\n",
		Clean:  true,
		Redraw: func(ev [][]any) { redraws <- ev },
		Lines:  func(l Lines) { changed <- l },
		Write:  func(seq int) { written <- seq },
		Exit:   func(err error) { exited <- err },
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
	// Typing sends the lines it changed, not the text.
	if got, want := wait(t, changed, "change"), (Lines{Seq: 1, First: 3, Last: 3, Text: "added\n"}); got != want {
		t.Errorf("changed lines = %+v, want %+v", got, want)
	}

	// u undoes the edit, not the text the buffer started with.
	if err := e.Input("u"); err != nil {
		t.Fatal(err)
	}
	if got, want := wait(t, changed, "undo"), (Lines{Seq: 2, First: 3, Last: 4, Text: ""}); got != want {
		t.Errorf("lines after u = %+v, want %+v", got, want)
	}

	// :w sends what was typed first, then says which send it follows.
	if err := e.Input("Ox<Esc>:w<CR>"); err != nil {
		t.Fatal(err)
	}
	if got, want := wait(t, changed, ":w's lines"), (Lines{Seq: 3, First: 2, Last: 2, Text: "x\n"}); got != want {
		t.Errorf("lines before :w = %+v, want %+v", got, want)
	}
	if got := wait(t, written, ":w"); got != 3 {
		t.Errorf(":w after seq %d, want 3", got)
	}

	// Lines set from mdsh-gui aren't sent back as a change.
	if ok, err := e.ApplyLines(3, []LineEdit{{First: 2, Last: 4, Text: "new\n"}}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	var got []string
	if err := e.v.ExecLua("return vim.api.nvim_buf_get_lines(0, 0, -1, false)", &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != "# Title\n\nnew" {
		t.Errorf("buffer after ApplyLines = %q", got)
	}
	select {
	case l := <-changed:
		t.Errorf("ApplyLines sent a change back: %+v", l)
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

// startTest starts a Neovim on text for a test, sending its events to
// events, and quits it when the test ends.
func startTest(t *testing.T, text string, events chan [2]string) *Editor {
	t.Helper()
	path := testNvim(t)
	e, err := Start(Options{
		Path:  path,
		Cols:  60,
		Rows:  20,
		Name:  "mdsh://test.md",
		Text:  text,
		Clean: true,
		Event: func(name, data string) {
			select {
			case events <- [2]string{name, data}:
			default:
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func bufLines(t *testing.T, e *Editor) string {
	t.Helper()
	var got []string
	if err := e.v.ExecLua("return vim.api.nvim_buf_get_lines(0, 0, -1, false)", &got); err != nil {
		t.Fatal(err)
	}
	return strings.Join(got, "\n")
}

// eventNamed waits for the event called name, skipping others.
func eventNamed(t *testing.T, events chan [2]string, name string) string {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev[0] == name {
				return ev[1]
			}
		case <-deadline:
			t.Fatalf("no %s event", name)
		}
	}
}

const runbook = "# Setup\n\nSome prose.\n\n```sh {#a}\necho a\n```\n\n## More\n\n```sh {#b depends=a}\necho b\n```\n"

func TestApplyLines(t *testing.T) {
	lines := make(chan Lines, 10)
	path := testNvim(t)
	e, err := Start(Options{Path: path, Cols: 60, Rows: 20, Name: "mdsh://test.md", Text: "one\ntwo\nthree\n", Clean: true, Lines: func(l Lines) { lines <- l }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	if ok, err := e.ApplyLines(0, []LineEdit{{First: 0, Last: 0, Text: "zero\n"}, {First: 2, Last: 3, Text: "THREE\nfour\n"}}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if got, want := bufLines(t, e), "zero\none\ntwo\nTHREE\nfour"; got != want {
		t.Errorf("after the edits: %q, want %q", got, want)
	}
	// Edits against an older seq change nothing.
	if ok, err := e.ApplyLines(0, []LineEdit{{First: 0, Last: 1, Text: "nope\n"}}); ok || err != nil {
		t.Errorf("an edit at an old seq: %v, %v", ok, err)
	}
	// Typing not yet sent is sent instead.
	if err := e.v.ExecLua(`vim.api.nvim_buf_set_lines(0, 1, 2, false, {"ONE"})`, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := e.ApplyLines(1, []LineEdit{{First: 0, Last: 1, Text: "nope\n"}}); ok || err != nil {
		t.Errorf("an edit with typing not sent: %v, %v", ok, err)
	}
	if got, want := wait(t, lines, "typing"), (Lines{Seq: 2, First: 1, Last: 2, Text: "ONE\n"}); got != want {
		t.Errorf("typing sent as %+v, want %+v", got, want)
	}
	if ok, err := e.ApplyLines(2, []LineEdit{{First: 4, Last: 5, Text: ""}}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if got, want := bufLines(t, e), "zero\nONE\ntwo\nTHREE"; got != want {
		t.Errorf("after the last edit: %q, want %q", got, want)
	}
	// A page catching up has the text and the seq.
	if text, seq, err := e.Snapshot(); text != "zero\nONE\ntwo\nTHREE\n" || seq != 3 || err != nil {
		t.Errorf("Snapshot = %q, %d, %v", text, seq, err)
	}
}

// TestTypedLines types in two places and checks that the one Lines it
// sends makes the text Neovim has of the text it had.
func TestTypedLines(t *testing.T) {
	lines := make(chan Lines, 10)
	path := testNvim(t)
	start := "a\nb\nc\nd\ne\nf\n"
	e, err := Start(Options{Path: path, Cols: 60, Rows: 20, Name: "mdsh://test.md", Text: start, Clean: true, Lines: func(l Lines) { lines <- l }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	for _, lua := range []string{
		`vim.api.nvim_buf_set_lines(0, 4, 5, false, {"E1", "E2"})`,
		`vim.api.nvim_buf_set_lines(0, 1, 2, false, {})`,
		`vim.api.nvim_buf_set_lines(0, 0, 0, false, {"top"})`,
	} {
		if err := e.v.ExecLua(lua, nil); err != nil {
			t.Fatal(err)
		}
	}
	l := wait(t, lines, "typing")
	old := strings.Split(strings.TrimSuffix(start, "\n"), "\n")
	got := append(append(append([]string{}, old[:l.First]...), splitLines(l.Text)...), old[l.Last:]...)
	if want := bufLines(t, e); strings.Join(got, "\n") != want {
		t.Errorf("%+v makes %q of the text, want %q", l, got, want)
	}
}

func TestCalls(t *testing.T) {
	events := make(chan [2]string, 100)
	e := startTest(t, runbook, events)
	call := func(fn string, args ...any) string {
		t.Helper()
		b, _ := json.Marshal(args)
		if args == nil {
			b = []byte("[]")
		}
		out, err := e.Call(fn, string(b))
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		return out
	}

	// jump puts the cursor on a line; state reports it, and how many
	// times the text was sent.
	call("jump", 6, 0, true, 0)
	var st struct {
		Seq  int    `json:"seq"`
		Mode string `json:"mode"`
		Line int    `json:"line"`
	}
	if err := json.Unmarshal([]byte(call("state")), &st); err != nil {
		t.Fatal(err)
	}
	if st.Line != 6 || st.Seq != 0 || st.Mode != "n" {
		t.Errorf("state = %+v", st)
	}
	if got := eventNamed(t, events, "cursor"); got != `{"line":6}` {
		t.Errorf("cursor event = %s", got)
	}

	// :RunBlock and a click on a run button ask mdsh-gui to run.
	call("set_runs", []int{5, 11}, []int{1})
	if err := e.Input(":RunBlock<CR>"); err != nil {
		t.Fatal(err)
	}
	if got := eventNamed(t, events, "run"); got != `{"what":"block"}` {
		t.Errorf("run event = %s", got)
	}
	var signs int
	if err := e.v.ExecLua(`return #vim.api.nvim_buf_get_extmarks(0, vim.api.nvim_get_namespaces().mdsh_runs, 0, -1, {})`, &signs); err != nil {
		t.Fatal(err)
	}
	if signs != 3 {
		t.Errorf("%d run buttons, want 3", signs)
	}
	call("jump", 1, 0, false, 0)
	if err := e.Mouse("left", "press", "", 4, 0); err != nil {
		t.Fatal(err)
	}
	if got := eventNamed(t, events, "run"); got != `{"line":5}` {
		t.Errorf("run event for a click = %s", got)
	}
	e.Mouse("left", "release", "", 4, 0)

	// Folds: a section folds to its last line, a fence to its closing one.
	var folds []int
	call("set_folds", []int{5, 9})
	if err := json.Unmarshal([]byte(call("folds")), &folds); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(folds) != "[5 9]" {
		t.Errorf("closed folds = %v, want [5 9]", folds)
	}
	var end int
	if err := e.v.ExecLua(`return vim.fn.foldclosedend(9)`, &end); err != nil {
		t.Fatal(err)
	}
	if end != 13 {
		t.Errorf("## More folds to line %d, want 13", end)
	}
	if err := e.v.ExecLua(`vim.cmd("normal! zR"); return vim.fn.foldlevel(11)`, &end); err != nil {
		t.Fatal(err)
	}
	if end != 3 {
		t.Errorf("foldlevel in b's fence = %d, want 3", end)
	}

	// The app's colors, and back to Neovim's own with the setting off.
	colors := map[string]any{"dark": true, "fg": "#e6edf3", "code_bg": "#151b23", "bg": "#0d1117", "accent": "#2f81f7"}
	call("set_colors", colors, true)
	var scheme struct {
		Name string `msgpack:"name"`
		Bg   int    `msgpack:"bg"`
		Back string `msgpack:"back"`
	}
	read := `return { name = vim.g.colors_name or "", bg = vim.api.nvim_get_hl(0, { name = "Normal" }).bg or -1, back = vim.o.background }`
	if err := e.v.ExecLua(read, &scheme); err != nil {
		t.Fatal(err)
	}
	if scheme.Name != "mdsh" || scheme.Bg != 0x151b23 || scheme.Back != "dark" {
		t.Errorf("with the app's colors: %+v", scheme)
	}
	colors["dark"] = false
	call("set_colors", colors, false)
	if err := e.v.ExecLua(read, &scheme); err != nil {
		t.Fatal(err)
	}
	if scheme.Name == "mdsh" || scheme.Bg == 0x151b23 || scheme.Back != "light" {
		t.Errorf("with the app's colors off: %+v", scheme)
	}

	call("set_spell", true)
	var spell bool
	if err := e.v.ExecLua(`return vim.wo.spell`, &spell); err != nil || !spell {
		t.Errorf("spell = %v, %v", spell, err)
	}
}

// TestMain runs the language server when a test's Neovim starts this
// test binary as it, as mdsh-gui runs itself with --lsp.
func TestMain(m *testing.M) {
	if os.Getenv("MDSH_TEST_LSP") == "1" {
		if err := lsp.Serve(os.Stdin, os.Stdout, ""); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLanguageServer(t *testing.T) {
	path := testNvim(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MDSH_TEST_LSP", "1")
	e, err := Start(Options{
		Path:  path,
		Cols:  60,
		Rows:  20,
		Name:  "mdsh://test.md",
		Text:  runbook + "\n```sh {#c depends=zz}\n```\n",
		Clean: true,
		LSP:   []string{exe},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })

	// The server attaches and reports the unknown depends= name.
	var got string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := e.v.ExecLua(`local d = vim.diagnostic.get(0); return #d > 0 and d[1].lnum .. ":" .. d[1].col .. " " .. d[1].message or ""`, &got); err != nil {
			t.Fatal(err)
		}
		if got != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if want := "14:18 There is no block zz, so depends= ignores it"; got != want {
		t.Errorf("diagnostic %q, want %q", got, want)
	}

	// Completion through the client, as omnifunc and plugins ask.
	var labels []string
	err = e.v.ExecLua(`
		local client = vim.lsp.get_clients({ name = "mdsh" })[1]
		local res = client.request_sync("textDocument/completion", {
			textDocument = { uri = vim.uri_from_bufnr(0) },
			position = { line = 10, character = 19 },
		}, 5000, 0)
		local out = {}
		for _, it in ipairs(res.result.items) do out[#out + 1] = it.label end
		return out`, &labels)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(labels) != "[a]" {
		t.Errorf("depends= completions %v, want [a]", labels)
	}
}

// TestSelection checks the right-click menu's Copy and Cut, which act on
// Neovim's selection: characterwise, linewise, in select mode, and none
// outside visual mode.
func TestSelection(t *testing.T) {
	e := startTest(t, "one two\nthree\nfour\n", make(chan [2]string, 100))
	sel := func(cut bool) string {
		t.Helper()
		out, err := e.Call("selection", fmt.Sprintf("[%v]", cut))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := sel(false); got != "null" {
		t.Errorf("in normal mode: %s", got)
	}
	e.Input("gg0wve")
	if got := sel(false); got != `"two"` {
		t.Errorf("characterwise: %s", got)
	}
	e.Input("<Esc>jVj")
	if got := sel(false); got != `"three\nfour\n"` {
		t.Errorf("linewise: %s", got)
	}
	e.Input("<Esc>gg0v<C-g>")
	if got := sel(true); got != `"o"` {
		t.Errorf("select mode: %s", got)
	}
	// Cut deletes it, as d does, leaving normal mode.
	deadline := time.Now().Add(5 * time.Second)
	for bufLines(t, e) != "ne two\nthree\nfour" {
		if time.Now().After(deadline) {
			t.Fatalf("after Cut: %q", bufLines(t, e))
		}
		time.Sleep(20 * time.Millisecond)
	}
	var mode string
	e.v.ExecLua(`return vim.api.nvim_get_mode().mode`, &mode)
	if mode != "n" {
		t.Errorf("mode after Cut = %q", mode)
	}
}
