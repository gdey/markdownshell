package tui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
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
		opts.Save = func(file string, b []byte) error { return os.WriteFile(file, b, 0o644) }
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
	// Saving names the new block.
	want := regexp.MustCompile("```sh \\{#(block-[0-9a-f-]{36})\\}\necho \"\\$X typed\"\n```\n\n```output \\{for=(block-[0-9a-f-]{36}) exit=0\\}\nhi typed\n```\n$")
	if g := want.FindStringSubmatch(got); g == nil || g[1] != g[2] {
		t.Errorf("new block not appended, run and named:\n%s", got)
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

func TestImagePreview(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0, 255, 0, 255}), image.Point{}, draw.Src)
	f, _ := os.Create(filepath.Join(dir, "g.png"))
	png.Encode(f, img)
	f.Close()

	src := "```sh {#pic out=image}\ncat g.png\n```\n"
	file := filepath.Join(dir, "doc.md")
	os.WriteFile(file, []byte(src), 0o644)
	open := func() *Model {
		d, _ := doc.Parse([]byte(read(t, file)))
		m, err := New(d, Options{
			Run:      run.Options{File: file, Environ: []string{"PATH=" + os.Getenv("PATH")}},
			Save:     func(file string, b []byte) error { return os.WriteFile(file, b, 0o644) },
			AutoSave: true, Images: true, Style: "notty",
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(m.Close)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		return m
	}
	m := open()
	press(m, "enter")
	pump(t, m)
	if v := m.View(); !strings.Contains(v, "image: doc.md.assets/pic.png") || !strings.Contains(v, "▀") {
		t.Errorf("no preview after the run:\n%s", v)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}); cmd == nil {
		t.Errorf("v didn't show the image: %s", m.status)
	}

	// Opened again, the saved image is previewed too.
	if v := open().View(); !strings.Contains(v, "saved output: doc.md.assets/pic.png") || !strings.Contains(v, "▀") {
		t.Errorf("no preview of the saved image:\n%s", v)
	}
}

func TestEditBlock(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{AutoSave: true})
	if m.editor() != "vi" {
		t.Errorf("editor = %q, want vi", m.editor())
	}
	m.opts.Run.Environ = append(m.opts.Run.Environ, "EDITOR=nano -w")
	if m.editor() != "nano -w" {
		t.Errorf("editor = %q, want $EDITOR", m.editor())
	}
	if press(m, "e"); m.status != "" {
		t.Fatalf("e: %s", m.status)
	}

	// The editor gets the whole block; what it leaves is put in the
	// document and saved.
	edit := func(i int, text string) {
		f := filepath.Join(t.TempDir(), "block.md")
		os.WriteFile(f, []byte(text), 0o644)
		m.Update(editedMsg{block: i, file: f})
	}
	edit(0, "```sh {#one}\nX=bye; echo $X\n```\n")
	got := read(t, file)
	if !strings.Contains(got, "```sh {#one}\nX=bye; echo $X\n```") || m.d.Blocks[0].Body != "X=bye; echo $X\n" {
		t.Errorf("edit not saved:\n%s", got)
	}
	press(m, "enter")
	pump(t, m)
	if got := read(t, file); !strings.Contains(got, "{for=one exit=0}\nbye\n") {
		t.Errorf("edited block didn't run:\n%s", got)
	}

	// Its attributes change too, and its saved output follows a new id.
	edit(0, "```sh {#greet timeout=5s}\nX=bye; echo $X\n```\n")
	got = read(t, file)
	if !strings.Contains(got, "```sh {#greet timeout=5s}\n") || !strings.Contains(got, "{for=greet exit=0}\nbye\n") || m.d.Blocks[0].ID != "greet" {
		t.Errorf("attributes not changed:\n%s", got)
	}

	edit(1, "```sh {#two}\necho\n")
	if !strings.Contains(m.status, "one closed fenced block") || m.d.Blocks[1].Body != "echo \"$X again\"\n" {
		t.Errorf("an unclosed fence: status %q, body %q", m.status, m.d.Blocks[1].Body)
	}
}

// A background block shows as running in the background, the run goes
// on, and its output comes in when a tick finds it ended.
func TestBackgroundBlock(t *testing.T) {
	src := "```sh {#bg background}\nsleep 0.3; echo from bg\n```\n\n```sh {#fg}\necho fg\n```\n"
	m, file := newModel(t, src, Options{AutoSave: true})
	press(m, "enter")
	pump(t, m)
	v := m.View()
	for _, want := range []string{"background", "running in the background…", "1 in background"} {
		if !strings.Contains(v, want) {
			t.Errorf("view doesn't show %q:\n%s", want, v)
		}
	}
	if m.running {
		t.Error("the run is still going; a background block shouldn't hold it")
	}

	// Another block runs meanwhile.
	press(m, "j", "enter")
	pump(t, m)
	if v := m.View(); !strings.Contains(v, "fg") {
		t.Errorf("fg didn't run:\n%s", v)
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(m.bg) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("the background block never ended")
		}
		m.Update(reapMsg{m})
		for len(m.events) > 0 {
			m.Update(<-m.events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	v = m.View()
	if !strings.Contains(v, "from bg") || strings.Contains(v, "in background") {
		t.Errorf("view after the block ended:\n%s", v)
	}
	if got := read(t, file); !strings.Contains(got, "```output {for=bg exit=0}\nfrom bg\n```") {
		t.Errorf("background output not saved:\n%s", got)
	}
}

func TestDependsAsksInTUI(t *testing.T) {
	src := "```sh {#deploy depends=build}\necho deploy\n```\n\n```sh {#build}\necho built\n```\n"
	for _, tc := range []struct {
		key        string
		dep, block bool
	}{{"r", true, true}, {"k", false, true}, {"s", false, false}, {"enter", false, false}} {
		m, file := newModel(t, src, Options{AutoSave: true})
		press(m, "enter")
		pump(t, m)
		if m.prompt == nil || m.prompt.block.ID != "build" {
			t.Fatalf("%s: not asked about build", tc.key)
		}
		if v := m.View(); !strings.Contains(v, "deploy depends on build") {
			t.Errorf("%s: prompt not shown:\n%s", tc.key, v)
		}
		if v := m.View(); !strings.Contains(v, "deploy depends on this block") {
			t.Errorf("%s: question not in build's card:\n%s", tc.key, v)
		}
		press(m, "o") // not an answer here
		if m.prompt == nil {
			t.Fatalf("%s: o answered a dependency's question", tc.key)
		}
		press(m, tc.key)
		pump(t, m)
		got := read(t, file)
		if dep, block := strings.Contains(got, "for=build"), strings.Contains(got, "for=deploy"); dep != tc.dep || block != tc.block {
			t.Errorf("%s: build ran %v, deploy ran %v; want %v, %v", tc.key, dep, block, tc.dep, tc.block)
		}
	}
}

func TestRunSection(t *testing.T) {
	src := "```sh {#pre}\necho pre\n```\n\n# A\n\n```sh {#a1}\necho a1\n```\n\n## B\n\n```sh {#b1}\necho b1\n```\n\n# C\n\n```sh {#c1}\necho c1\n```\n"
	m, file := newModel(t, src, Options{AutoSave: true})
	// On a1, s runs A: a1 and b1, in its subsection.
	press(m, "j", "s")
	pump(t, m)
	got := read(t, file)
	for id, want := range map[string]bool{"pre": false, "a1": true, "b1": true, "c1": false} {
		if ran := strings.Contains(got, "for="+id+" "); ran != want {
			t.Errorf("%s ran = %v, want %v:\n%s", id, ran, want, got)
		}
	}
	// pre is under no heading.
	press(m, "g", "s")
	if !strings.Contains(m.View(), "isn't under a heading") {
		t.Errorf("no status for a block under no heading:\n%s", m.View())
	}
}

func TestSaveAs(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{})
	press(m, "enter")
	pump(t, m)
	other := filepath.Join(filepath.Dir(file), "copy.md")
	press(m, "W")
	m.saveAs.SetValue("copy.md")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(read(t, other), "for=one") {
		t.Fatal("save as didn't write the output")
	}
	if strings.Contains(read(t, file), "for=one") {
		t.Error("save as wrote the old file")
	}
	if m.opts.Run.File != other || m.dirty {
		t.Errorf("after save as: file %s, dirty %v", m.opts.Run.File, m.dirty)
	}
	if msg, ok := cmd().(savedAsMsg); !ok || msg.file != other {
		t.Errorf("save as in the same folder sent %#v", msg)
	}

	// Another file that exists takes enter twice.
	press(m, "W")
	m.saveAs.SetValue(file)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.naming || strings.Contains(read(t, file), "for=one") {
		t.Fatal("save as overwrote a file without asking")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.naming || !strings.Contains(read(t, file), "for=one") {
		t.Error("enter again didn't overwrite")
	}
}

func TestFileChangedOnDisk(t *testing.T) {
	m, file := newModel(t, twoBlocks, Options{})
	later := time.Now().Add(time.Hour)
	os.WriteFile(file, []byte("edited\n"), 0o644)
	os.Chtimes(file, later, later)
	m.Update(checkMsg{m})
	if !m.changed || !strings.Contains(m.View(), "changed on disk") {
		t.Fatal("no notice for a file changed on disk")
	}
	press(m, "M")
	if m.changed || !m.dirty {
		t.Fatalf("keep mine: changed %v, dirty %v", m.changed, m.dirty)
	}
	press(m, "w")
	if !strings.Contains(read(t, file), "# Title") {
		t.Error("w after keep mine didn't overwrite")
	}

	os.WriteFile(file, []byte("edited again\n"), 0o644)
	os.Chtimes(file, later.Add(time.Hour), later.Add(time.Hour))
	m.Update(checkMsg{m})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Fatal("R didn't reload")
	}
	if msg, ok := cmd().(openMsg); !ok || msg.file != file {
		t.Errorf("R sent %#v", msg)
	}
}

// newApp opens the start screen on a folder holding two runbooks.
func newTestApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ndescription: |\n  The first\n  runbook.\n---\n"+twoBlocks), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "b.md"), []byte("```sh\necho b\n```\n"), 0o644)
	recent := filepath.Join(t.TempDir(), "tui.json")
	a, err := newApp(Config{Dir: dir, State: recent, Open: func(file string) (*doc.Document, Options, error) {
		src, err := os.ReadFile(file)
		if err != nil {
			return nil, Options{}, err
		}
		d, err := doc.Parse(src)
		return d, Options{
			Run:   run.Options{File: file, Environ: []string{"PATH=" + os.Getenv("PATH")}},
			Save:  func(file string, b []byte) error { return os.WriteFile(file, b, 0o644) },
			Style: "notty",
		}, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	return a, dir
}

// do sends msg to a, and what its command sends back, until one is not
// for the app itself.
func do(a *app, msg tea.Msg) {
	for msg != nil {
		_, cmd := a.Update(msg)
		msg = nil
		if cmd != nil {
			switch next := cmd().(type) {
			case openMsg, backMsg, savedAsMsg, viewMsg:
				msg = next
			}
		}
	}
}

func TestStartScreen(t *testing.T) {
	a, dir := newTestApp(t)
	view := a.View()
	for _, want := range []string{"This folder", "a.md", "2 blocks", "The first runbook.", "sub/", "b.md", "1 block"} {
		if !strings.Contains(view, want) {
			t.Errorf("start screen is missing %q:\n%s", want, view)
		}
	}
	for _, r := range "sub" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if v := a.View(); strings.Contains(v, "a.md") || !strings.Contains(v, "b.md") {
		t.Errorf("filter sub:\n%s", v)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	for _, r := range "first" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if v := a.View(); !strings.Contains(v, "a.md") || strings.Contains(v, "b.md") {
		t.Errorf("filter on the description:\n%s", v)
	}

	do(a, tea.KeyMsg{Type: tea.KeyEnter})
	if a.doc == nil || a.doc.opts.Run.File != filepath.Join(dir, "a.md") {
		t.Fatal("enter didn't open a.md")
	}
	if !a.doc.back {
		t.Error("q should go back to the start screen")
	}
	do(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if a.doc != nil {
		t.Fatal("q didn't go back to the start screen")
	}
	v := a.View()
	if i, j := strings.Index(v, "Recent"), strings.Index(v, "This folder"); i < 0 || j < i {
		t.Errorf("a.md isn't listed as recent first:\n%s", v)
	}
	if got := loadState(a.c.State).Recent; len(got) != 1 || got[0] != filepath.Join(dir, "a.md") {
		t.Errorf("recent files kept: %v", got)
	}
}

func TestSaveAsAnotherFolderReopens(t *testing.T) {
	a, dir := newTestApp(t)
	do(a, tea.KeyMsg{Type: tea.KeyEnter})
	m := a.doc
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	pump(t, m)
	press(m, "W")
	m.saveAs.SetValue(filepath.Join(dir, "sub", "c.md"))
	do(a, tea.KeyMsg{Type: tea.KeyEnter})
	if a.doc == m || a.doc.opts.Run.File != filepath.Join(dir, "sub", "c.md") {
		t.Fatal("save as into another folder didn't open the new file")
	}
	if !strings.Contains(a.View(), "hi") {
		t.Error("the new file doesn't show the output saved")
	}
}

func TestOutline(t *testing.T) {
	src := "---\ntitle: x\n# not a heading\n---\n# Ops\n\n## Disk ##\n\n```sh {#disk}\n# a comment, not a heading\necho freed\n```\n\n```text\n# nor this\n```\n\n#not-a-heading\n\n## Other\n\n```sh {#two}\nexit 3\n```\n"
	m, _ := newModel(t, src, Options{})
	var got []string
	for _, e := range m.entries() {
		if e.level > 0 {
			got = append(got, fmt.Sprintf("h%d %s", e.level, e.text))
		} else {
			got = append(got, "#"+m.d.Blocks[e.block].ID)
		}
	}
	if want := "h1 Ops|h2 Disk|#disk|h2 Other|#two"; strings.Join(got, "|") != want {
		t.Errorf("outline entries:\n got %s\nwant %s", strings.Join(got, "|"), want)
	}

	press(m, "j", "enter")
	pump(t, m)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	v := m.View()
	if !strings.Contains(v, "✗ #two") || !strings.Contains(v, "│") {
		t.Errorf("outline doesn't show #two failed:\n%s", v)
	}
	if msg, ok := cmd().(viewMsg); !ok || !msg.Outline {
		t.Errorf("o sent %#v", msg)
	}
}

func TestHideCodeAndOutput(t *testing.T) {
	m, _ := newModel(t, twoBlocks, Options{})
	press(m, "enter")
	pump(t, m)
	v := m.View()
	if !strings.Contains(v, "X=hi; echo $X") || !strings.Contains(v, "\n    hi") {
		t.Fatalf("code or output missing:\n%s", v)
	}
	press(m, "C")
	if v := m.View(); strings.Contains(v, "X=hi; echo $X") || !strings.Contains(v, "#one") {
		t.Errorf("C didn't hide the code but keep the block:\n%s", v)
	}
	press(m, "C", "O")
	if v := m.View(); !strings.Contains(v, "X=hi; echo $X") || strings.Contains(v, "\n    hi") {
		t.Errorf("O didn't hide the output:\n%s", v)
	}
}

func TestConfirmInCard(t *testing.T) {
	src := "```sh {#disk confirm=\"Free up disk space?\"}\necho freed\n```\n\nAfter.\n"
	m, _ := newModel(t, src, Options{})
	press(m, "enter")
	pump(t, m)
	v := m.View()
	q, code, after := strings.Index(v, "Free up disk space?"), strings.Index(v, "echo freed"), strings.Index(v, "After.")
	if q < 0 || !(code < q && q < after) {
		t.Errorf("the question isn't under the block's code:\n%s", v)
	}
	press(m, "r")
	pump(t, m)
}

func TestViewKept(t *testing.T) {
	a, _ := newTestApp(t)
	do(a, tea.KeyMsg{Type: tea.KeyEnter})
	do(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	if !loadState(a.c.State).View.HideCode {
		t.Fatal("the view wasn't kept")
	}
	do(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	do(a, tea.KeyMsg{Type: tea.KeyEnter})
	if !a.doc.shown.HideCode {
		t.Error("the next file didn't open in the view kept")
	}
}

func TestStatusBeatsLongPath(t *testing.T) {
	m, _ := newModel(t, twoBlocks, Options{})
	m.opts.Run.File = "/" + strings.Repeat("very-long-folder/", 8) + "doc.md"
	m.status = "the status"
	line := strings.SplitN(m.footer(), "\n", 2)[0]
	if !strings.Contains(line, "the status") || !strings.Contains(line, "/doc.md") {
		t.Errorf("status line: %q", line)
	}
}

func TestOutlineNamesUnnamedBlocksByLine(t *testing.T) {
	m, _ := newModel(t, "# T\n\n```sh\necho hi\n```\n", Options{View: View{Outline: true}})
	if v := m.View(); !strings.Contains(v, "block at line 3") {
		t.Errorf("outline:\n%s", v)
	}
}
