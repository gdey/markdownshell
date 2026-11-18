package notebook

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termrec"
)

// Where a recording's player is left is written to its fence, or its tty
// block, as frame=N, and steps one after another undo as one.
func TestSetCastFrame(t *testing.T) {
	cast := `{"version":2,"width":20,"height":2}` + "\n" + `[0.1,"o","a"]` + "\n" + `[1,"o","b"]` + "\n"
	file := write(t, "# Rec\n\n```cast\n"+cast+"```\n\n```sh {#t tty}\necho hi\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.SetCastFrame(-1, cast, 1); err != nil {
		t.Fatal(err)
	}
	if err := n.SetCastFrame(-1, "\n"+cast, 2); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```cast {frame=2}\n") {
		t.Errorf("fence:\n%s", text)
	}
	var html strings.Builder
	for _, c := range n.Cells() {
		html.WriteString(c.HTML)
	}
	if !strings.Contains(html.String(), `#frame=2">`) {
		t.Errorf("the player doesn't open at frame 2: %s", html.String())
	}
	if err := n.SetCastFrame(1, "", 3); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```sh {#t tty frame=3}\n") {
		t.Errorf("tty block:\n%s", text)
	}
	if _, err := n.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Undo(); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```cast\n") {
		t.Errorf("two frame steps didn't undo as one:\n%s", text)
	}
	// 0 is the last frame, which needs no attribute.
	if err := n.SetCastFrame(-1, cast, 2); err != nil {
		t.Fatal(err)
	}
	if err := n.SetCastFrame(-1, cast, 0); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```cast\n") {
		t.Errorf("frame= kept for the last frame:\n%s", text)
	}
	if err := n.SetCastFrame(-1, "nope", 1); err == nil {
		t.Error("no error for a recording no fence holds")
	}
	if err := n.SetCastFrame(0, "", 1); err == nil {
		t.Error("no error for frame= on a fence that isn't a tty block")
	}
}

// testCast is a recording of two frames, "first" and then "second".
func testCast(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	err := termrec.WriteCast(&b, termrec.Recording{Cols: 10, Rows: 2, Events: []termrec.Event{
		{At: 0, Data: []byte("first")},
		{At: time.Second, Data: []byte("\x1b[2J\x1b[Hsecond")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A tty block's recording can be swapped for the text of a frame, or a
// GIF of it, and Undo puts it back.
func TestReplaceCast(t *testing.T) {
	region := "<!-- mdsh:output for=\"t\" -->\n![t](doc.md.assets/t.cast#frame=1)\n<!-- /mdsh:output -->\n"
	src := "# Rec\n\n```sh {#t tty frame=1}\ntop\n```\n\n" + region + "\nafter\n"
	file := write(t, src)
	assets := filepath.Join(filepath.Dir(file), "doc.md.assets")
	os.Mkdir(assets, 0o755)
	if err := os.WriteFile(filepath.Join(assets, "t.cast"), testCast(t), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if err := n.ReplaceCast(0, "text", 1); err != nil {
		t.Fatal(err)
	}
	want := "# Rec\n\n```sh {#t tty frame=1}\ntop\n```\n\n```output {for=t}\nfirst\n```\n\nafter\n"
	if got := string(n.d.Src); got != want {
		t.Errorf("text:\ngot  %q\nwant %q", got, want)
	}
	if !n.d.Blocks[0].HasOutput() {
		t.Error("the text isn't the block's output")
	}
	if _, err := n.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := string(n.d.Src); got != src {
		t.Fatalf("undo:\ngot  %q\nwant %q", got, src)
	}

	if err := n.ReplaceCast(0, "gif", 0); err != nil {
		t.Fatal(err)
	}
	if got, want := string(n.d.Src), strings.Replace(src, "t.cast#", "t.gif#", 1); got != want {
		t.Errorf("gif:\ngot  %q\nwant %q", got, want)
	}
	if data, err := os.ReadFile(filepath.Join(assets, "t.gif")); err != nil || !bytes.HasPrefix(data, []byte("GIF8")) {
		t.Errorf("no GIF beside the cast: %v", err)
	}
	if err := n.ReplaceCast(0, "gif", 0); err == nil {
		t.Error("no error with no recording left to swap")
	}
}

// An inline recording becomes an inline GIF; one nested in a quote keeps
// the quote; and a block that ran this session writes the swap back on
// save, with its exit status.
func TestReplaceCastInline(t *testing.T) {
	uri := "data:" + run.CastMIME + ";base64," + base64.StdEncoding.EncodeToString(testCast(t))
	src := "> ```sh {#t tty embed=inline}\n> top\n> ```\n>\n> <!-- mdsh:output for=\"t\" -->\n> ![recording](" + uri + ")\n> <!-- /mdsh:output -->\n\n```sh {#plain}\necho\n```\n"
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if err := n.ReplaceCast(0, "gif", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(n.d.Src), "> ![recording](data:image/gif;base64,") {
		t.Errorf("gif:\n%s", n.d.Src)
	}
	n.Undo()

	n.results[0] = &run.Result{Block: n.d.Blocks[0], Ran: true, Status: 2, Image: uri}
	n.sess.SetResult(0, n.results[0])
	if err := n.ReplaceCast(0, "text", 0); err != nil {
		t.Fatal(err)
	}
	want := "> ```sh {#t tty embed=inline}\n> top\n> ```\n>\n> ```output {for=t exit=2}\n> second\n> ```\n\n```sh {#plain}\necho\n```\n"
	if got := string(n.d.Src); got != want {
		t.Errorf("text:\ngot  %q\nwant %q", got, want)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != want {
		t.Errorf("saved:\ngot  %q\nwant %q", got, want)
	}

	if err := n.ReplaceCast(1, "text", 0); err == nil {
		t.Error("no error for a block that isn't a tty block")
	}
	if err := n.ReplaceCast(0, "png", 0); err == nil {
		t.Error("no error for a kind it can't make")
	}
}
