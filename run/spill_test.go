package run_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/docroot"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

// out=file writes stdout to <doc>.assets/<id>.log, and the output fence
// is an include of it: the first fold_output lines, or the window the
// fence's attributes ask for, which write-back keeps. A copy shows a
// window of its own, and an include above the block shows this run's
// file, not the last one's.
func TestSpill(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	src := "---\nmdsh:\n  fold_output: 2\n---\n" +
		"```text {<include file=res://build.log match=^FAIL}\nstale\n```\n\n" +
		"```sh {#build out=file}\nprintf 'a\\nb\\nc\\nFAIL x\\nd\\n'\necho oops >&2\n```\n\n" +
		"```output {for=build exit=0 <include file=res://build.log match=\"^FAIL\" before=1 lines=1 number_on}\nold\n```\n\n" +
		"Summary:\n\n```output {for=build until=c}\n```\n"
	d, results := runDoc(t, src, run.Options{Dir: dir, File: file, Now: fixedNow})
	r := results[1]
	if r.Spill == nil || r.Spill.File != "res://build.log" || r.Spill.Truncated {
		t.Fatalf("spill = %+v", r.Spill)
	}
	got, err := os.ReadFile(filepath.Join(dir, "d.md.assets", "build.log"))
	if err != nil || string(got) != "a\nb\nc\nFAIL x\nd\n" {
		t.Fatalf("build.log = %q, %v", got, err)
	}
	if inc := results[0]; inc.Err != nil || string(inc.Stdout) != "FAIL x\nd\n" {
		t.Errorf("include above = %q, %v", inc.Stdout, inc.Err)
	}
	out := noAt(string(render.Markdown(d, results, render.Options{WriteBack: true})))
	for _, want := range []string{
		"```text {<include file=res://build.log match=^FAIL}\nFAIL x\nd\n```\n",
		"```output {for=build exit=0 <include file=res://build.log match=^FAIL before=1 lines=1 number_on}\nc\nFAIL x\n```\n",
		"```output {for=build exit=0 <include file=res://build.log until=c}\na\nb\nc\n```\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("write-back missing %q:\n%s", want, out)
		}
	}
	printed := string(render.Markdown(d, results, render.Options{}))
	if !strings.Contains(printed, "3  c\n4  FAIL x\n") {
		t.Errorf("printed without numbers from the window:\n%s", printed)
	}

	// With no window attributes, the fence is the first fold_output
	// lines; running again with the same output leaves the file alone.
	d2, err := doc.Parse([]byte(strings.Replace(out, " match=^FAIL before=1 lines=1 number_on", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "d.md.assets", "build.log")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.Chtimes(log, old, old)
	d2, results2 := runDoc(t, string(d2.Src), run.Options{Dir: dir, File: file, Now: fixedNow})
	if info, _ := os.Stat(log); !info.ModTime().Equal(old) {
		t.Errorf("an unchanged log was written again")
	}
	if out := noAt(string(render.Markdown(d2, results2, render.Options{WriteBack: true}))); !strings.Contains(out, "```output {for=build exit=0 <include file=res://build.log}\na\nb\n```\n") {
		t.Errorf("default window:\n%s", out)
	}
}

// out=file:path writes there; a failing block's log is kept; output
// past docroot.MaxRead is cut at a line and the fence marked truncated;
// a nosave block writes no file.
func TestSpillPathCapNoSave(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	src := "```sh {#a out=file:logs/a.txt}\necho one\nfalse\n```\n\n" +
		"```sh {#big out=file}\nyes 0123456789abcdef | head -c " + strconv.Itoa(docroot.MaxRead+100) + "\n```\n\n" +
		"```sh {#n out=file nosave}\necho secretish\n```\n"
	os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	d, results := runDoc(t, src, run.Options{Dir: dir, File: file, Now: fixedNow})
	if b, err := os.ReadFile(filepath.Join(dir, "logs", "a.txt")); err != nil || string(b) != "one\n" {
		t.Errorf("a.txt = %q, %v", b, err)
	}
	big := results[1]
	if big.Spill == nil || !big.Spill.Truncated || len(big.Stdout) > docroot.MaxRead || !strings.HasSuffix(string(big.Stdout), "\n") {
		t.Errorf("big: spill %+v, %d bytes", big.Spill, len(big.Stdout))
	}
	if _, err := os.Stat(filepath.Join(dir, "d.md.assets", "n.log")); err == nil {
		t.Error("nosave block wrote its log")
	}
	if r := results[2]; r.Spill == nil || string(r.Stdout) != "secretish\n" {
		t.Errorf("nosave result = %+v", r)
	}
	out := noAt(string(render.Markdown(d, results, render.Options{WriteBack: true})))
	for _, want := range []string{
		"```output {for=a exit=1 <include file=logs/a.txt}\none\n```\n",
		"```output {for=big exit=0 truncated <include file=res://big.log}\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("write-back missing %q", want)
		}
	}
	if strings.Contains(out, "secretish\n```\n\n```output") || strings.Contains(out, "for=n ") {
		t.Errorf("nosave output written back:\n%s", out)
	}
}

func TestSpillParse(t *testing.T) {
	for src, want := range map[string]string{
		"```sh {#a out=file secret}\n```\n":                                            "out=file can't go with secret",
		"```sh {#a out=file tty}\n```\n":                                               "can't have out=file",
		"```sh {#a out=file}\n```\n\n```output {for=a <include file=x match=(}\n```\n": "output for a: match=",
		"```sh {#a}\n```\n\n```output {for=a <include file=x lines=2 until=y}\n```\n":  "both lines= and until=",
	} {
		if _, err := doc.Parse([]byte(src)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", src, err, want)
		}
	}
}

// history=N keeps an earlier run's log under the name HistoryAssetName
// gives it, and points that run's fence at it.
func TestSpillHistory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	src := "```sh {#a out=file history=1}\necho \"$RANDOM$RANDOM\"\n```\n"
	for range 2 {
		d, err := doc.Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		s, err := run.NewSession(d, run.Options{Dir: dir, File: file, Environ: []string{"PATH=" + os.Getenv("PATH")}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.RunBlock(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		src = string(render.Markdown(d, s.Results(), render.Options{WriteBack: true}))
		s.Close()
		time.Sleep(1100 * time.Millisecond)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "d.md.assets"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 {
		t.Fatalf("assets = %v", names)
	}
	rotated := names[0]
	if rotated == "a.log" {
		rotated = names[1]
	}
	if !strings.Contains(src, "<include file=res://"+rotated+" history=1}") {
		t.Errorf("history fence doesn't name %s:\n%s", rotated, src)
	}
}

// Naming an unnamed out=file block renames its log and points its output
// fence at the new name.
func TestSpillNameBlocks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "d.md")
	d, results := runDoc(t, "```sh {out=file}\necho hi\n```\n", run.Options{Dir: dir, File: file})
	src := render.Markdown(d, results, render.Options{WriteBack: true})
	if err := d.SetSource(src); err != nil {
		t.Fatal(err)
	}
	renames, err := run.NameBlocks(d, results, run.Options{Dir: dir, File: file})
	if err != nil || len(renames) != 1 {
		t.Fatalf("renames = %v, %v", renames, err)
	}
	id := renames[0].New
	if results[0].Spill.File != "res://"+id+".log" {
		t.Errorf("spill file = %s", results[0].Spill.File)
	}
	if !strings.Contains(string(d.Src), "<include file=res://"+id+".log}") {
		t.Errorf("source:\n%s", d.Src)
	}
	if _, err := os.Stat(filepath.Join(dir, "d.md.assets", id+".log")); err != nil {
		t.Error(err)
	}
}
