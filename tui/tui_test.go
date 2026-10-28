package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// newModel writes src to a file and opens it in a model 80x40 big.
func newModel(t *testing.T, src string, opts Options) (*Model, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	opts.Run.File = file
	opts.Run.Environ = []string{"PATH=" + os.Getenv("PATH")}
	opts.Style = "notty"
	if opts.Save == nil {
		opts.Save = func(b []byte) error { return os.WriteFile(file, b, 0o644) }
	}
	m, err := New(d, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	return m, file
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m.Update(msg)
	}
}

// pump feeds run events to m until the run ends or asks for an answer.
func pump(t *testing.T, m *Model) {
	t.Helper()
	for {
		select {
		case msg := <-m.events:
			m.Update(msg)
			switch msg.(type) {
			case runDoneMsg:
				return
			case confirmMsg:
				if m.prompt != nil {
					return
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for the run")
		}
	}
}

func read(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const twoBlocks = "# Title\n\n```sh {#one}\nX=hi; echo $X\n```\n\nSome prose.\n\n```sh {#two}\necho \"$X again\"\n```\n"

func TestRunBlockSavesOutput(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{AutoSave: true})
	press(m, "enter")
	pump(t, m)
	if v := m.View(); !strings.Contains(v, "hi") || !strings.Contains(v, "✓") {
		t.Errorf("view doesn't show the output:\n%s", v)
	}
	got := read(t, file)
	if !strings.Contains(got, "```output {for=one exit=0}\nhi\n```") {
		t.Errorf("output not saved:\n%s", got)
	}
	if strings.Contains(got, "for=two") {
		t.Errorf("block two was saved without running:\n%s", got)
	}

	// The env lives on, so block two sees X.
	press(m, "j", "enter")
	pump(t, m)
	if got := read(t, file); !strings.Contains(got, "```output {for=two exit=0}\nhi again\n```") {
		t.Errorf("block two didn't share the env:\n%s", got)
	}
}

func TestRerunReplacesOutput(t *testing.T) {
	src := "```sh {#count}\nN=$((${N:-0}+1)); echo run $N\n```\n"
	m, file := newModel(t, src, Options{AutoSave: true})
	press(m, "enter")
	pump(t, m)
	press(m, "enter")
	pump(t, m)
	got := read(t, file)
	if !strings.Contains(got, "run 2") || strings.Contains(got, "run 1") {
		t.Errorf("rerun didn't replace the output:\n%s", got)
	}
	if strings.Count(got, "```output") != 1 {
		t.Errorf("want one output fence:\n%s", got)
	}
}

func TestRunFromHere(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{AutoSave: true})
	press(m, "a")
	pump(t, m)
	got := read(t, file)
	if !strings.Contains(got, "for=one") || !strings.Contains(got, "for=two") {
		t.Errorf("a didn't run both blocks:\n%s", got)
	}
}

func TestConfirmAsksInTUI(t *testing.T) {
	src := "```sh {#danger confirm=\"Really?\"}\necho boom\n```\n"
	for _, tc := range []struct {
		key  string
		want bool
	}{{"r", true}, {"s", false}, {"enter", false}} {
		m, file := newModel(t, src, Options{AutoSave: true})
		press(m, "enter")
		pump(t, m)
		if m.prompt == nil {
			t.Fatalf("%s: no prompt", tc.key)
		}
		if v := m.View(); !strings.Contains(v, "Really?") {
			t.Errorf("%s: prompt not shown:\n%s", tc.key, v)
		}
		press(m, tc.key)
		pump(t, m)
		if got := strings.Contains(read(t, file), "for=danger"); got != tc.want {
			t.Errorf("%s: ran = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestStepOnlyUnsaved(t *testing.T) {
	src := "```sh {#old}\necho new old\n```\n\n```output {for=old exit=0}\nsaved\n```\n\n```sh {#fresh}\necho fresh\n```\n"
	m, file := newModel(t, src, Options{AutoSave: true, Run: run.Options{Step: true}})
	press(m, "a")
	pump(t, m)
	press(m, "o") // skips old; fresh has no saved output, so it is asked about
	pump(t, m)
	if m.prompt == nil || m.prompt.block.ID != "fresh" {
		t.Fatal("not asked about fresh")
	}
	press(m, "r")
	pump(t, m)
	got := read(t, file)
	if !strings.Contains(got, "{for=old exit=0}\nsaved\n") || !strings.Contains(got, "{for=fresh exit=0}\nfresh\n") {
		t.Errorf("o didn't skip only the saved block:\n%s", got)
	}
}

func TestManualSave(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{})
	press(m, "enter")
	pump(t, m)
	if strings.Contains(read(t, file), "for=one") {
		t.Fatal("saved without w")
	}
	press(m, "q")
	if !strings.Contains(m.status, "not saved") {
		t.Errorf("q with unsaved output didn't warn: %q", m.status)
	}
	press(m, "w")
	if !strings.Contains(read(t, file), "for=one") {
		t.Error("w didn't save")
	}
}

func TestSaveWontClobberEdits(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{})
	press(m, "enter")
	pump(t, m)
	later := time.Now().Add(time.Hour)
	if err := os.WriteFile(file, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(file, later, later)
	press(m, "w")
	if read(t, file) != "edited\n" {
		t.Fatal("overwrote a file changed on disk")
	}
	press(m, "w")
	if !strings.Contains(read(t, file), "for=one") {
		t.Error("second w didn't overwrite")
	}
}

func TestIncludeNumbered(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("l1\nl2\nl3\n"), 0o644)
	src := "```text {<include file=a.txt from=2 number_on}\n```\n"
	file := filepath.Join(dir, "doc.md")
	os.WriteFile(file, []byte(src), 0o644)
	d, _ := doc.Parse([]byte(src))
	m, err := New(d, Options{Run: run.Options{File: file}, Style: "notty"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if v := m.View(); !strings.Contains(v, "2  l2") || !strings.Contains(v, "3  l3") {
		t.Errorf("include not numbered:\n%s", v)
	}
}

func TestKeysTypedTogether(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{AutoSave: true})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jr")})
	pump(t, m)
	if got := read(t, file); strings.Contains(got, "for=one") || !strings.Contains(got, "for=two") {
		t.Errorf("jr didn't run block two:\n%s", got)
	}
}

func TestSavedOutputShown(t *testing.T) {
	src := "```sh {#old}\necho x\n```\n\n```output {for=old exit=0}\nfrom last time\n```\n"
	m, _ := newModel(t, src, Options{})
	v := m.View()
	if !strings.Contains(v, "from last time") || strings.Contains(v, "for=old") {
		t.Errorf("saved output not shown without its fence:\n%s", v)
	}
}

func TestMarkdownOutputAndPlaceholders(t *testing.T) {
	src := "```sh {#a out=markdown}\nX=42\nprintf '| Name | Value |\\n| --- | --- |\\n| x | %s |\\n' \"$X\"\n```\n\nX is {{#a/env(X)}} here.\n"
	m, _ := newModel(t, src, Options{})
	press(m, "enter")
	pump(t, m)
	v := m.View()
	if strings.Contains(v, "| x | 42 |") || !strings.Contains(v, "Value") {
		t.Errorf("out=markdown output not drawn as markdown:\n%s", v)
	}
	if !strings.Contains(v, "X is 42 here.") {
		t.Errorf("placeholder not filled in:\n%s", v)
	}
}

func TestTypeNewBlock(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{AutoSave: true})
	press(m, "a")
	pump(t, m)
	press(m, "i")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(`echo "$X typed"`)})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	pump(t, m)
	got := read(t, file)
	want := "```sh\necho \"$X typed\"\n```\n\n```output {for=block-3 exit=0}\nhi typed\n```\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("new block not appended and run:\n%s", got)
	}
	if m.cur() != 2 {
		t.Errorf("cursor on block %d, want the new one", m.cur())
	}

	// esc cancels.
	press(m, "i")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("echo no")})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.d.Blocks) != 3 || m.typing {
		t.Errorf("esc added a block or kept typing")
	}
}
