package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termrec"
)

func testCast(t *testing.T) []byte {
	t.Helper()
	rec := termrec.Recording{Cols: 20, Rows: 3, Events: []termrec.Event{
		{At: 0, Data: []byte("one\r\n")},
		{At: 500 * time.Millisecond, Data: []byte("two\r\n")},
		{At: time.Second, Data: []byte("three\r\n")},
	}}
	var b bytes.Buffer
	if err := termrec.WriteCast(&b, rec); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCastLoad(t *testing.T) {
	data := testCast(t)
	file := filepath.Join(t.TempDir(), "a.cast")
	os.WriteFile(file, data, 0o644)
	var c casts
	for _, src := range []string{
		"data:" + run.CastMIME + ";base64," + base64.StdEncoding.EncodeToString(data) + "#frame=2",
		"/doc" + file + "?v=1#frame=2",
	} {
		info, err := c.load(src)
		if err != nil {
			t.Fatalf("%s: %v", src[:20], err)
		}
		if len(info.Delays) != 3 || info.Frame != 2 || info.Width == 0 || info.Height == 0 || info.Cols != 20 {
			t.Errorf("%s: info = %+v, want 3 frames, frame 2", src[:20], info)
		}
		p, err := c.frame(info.ID, 1)
		if err != nil || !bytes.HasPrefix(p, []byte("\x89PNG")) {
			t.Errorf("frame 1: %v", err)
		}
		if g, err := c.gif(info.ID); err != nil || !bytes.HasPrefix(g, []byte("GIF8")) {
			t.Errorf("gif: %v", err)
		}
	}
	if _, err := c.load("/doc/etc/passwd"); err == nil {
		t.Error("loaded a file that isn't a .cast")
	}
	if _, err := c.frame("nope", 1); err == nil {
		t.Error("frame of an unknown recording")
	}
}
