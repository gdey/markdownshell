package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestConsoleHandler(t *testing.T) {
	var out bytes.Buffer
	c := newConsole()
	log := slog.New(newConsoleHandler(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelWarn}), c))

	log.Debug("too quiet")
	log.Info("[AssetFileServerFS] Handling request", "url", "/style.css")
	log.Info("Platform Info:", "os", "macOS 15", "arch", "arm64")
	log.Error("Binding call failed: line 96: block git-cleanup: depends= names unknown block \"git-siwtch_parent\"")
	log.With("tab", 2).WithGroup("run").Warn("slow", "block", "build", slog.Group("env", "shell", "zsh"))

	got := c.list()
	want := []struct{ level, msg string }{
		{"info", "Platform Info: os=\"macOS 15\" arch=arm64"},
		{"error", `Binding call failed: line 96: block git-cleanup: depends= names unknown block "git-siwtch_parent"`},
		{"warn", "slow tab=2 run.block=build run.env.shell=zsh"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Level != w.level || got[i].Message != w.msg || got[i].Seq != i+1 {
			t.Errorf("entry %d = %+v, want seq %d %s %q", i, got[i], i+1, w.level, w.msg)
		}
	}
	// The next handler still gets what it asks for, and only that.
	if s := out.String(); strings.Contains(s, "Platform") || !strings.Contains(s, "Binding call failed") || !strings.Contains(s, "run.block=build") {
		t.Errorf("next handler got:\n%s", s)
	}
}

// A "file" and a "line" say where an entry is about, rather than
// being part of its text.
func TestConsoleWhere(t *testing.T) {
	c := newConsole()
	log := slog.New(newConsoleHandler(slog.NewTextHandler(io.Discard, nil), c))

	log.Warn("block a: depends= names unknown block", "file", "/x/a.md", "line", 3, "id", "a")
	log.With("file", "/x/b.md").Warn("near", "line", uint(7))
	log.Warn("no file", "line", 9)
	log.WithGroup("g").Warn("grouped", "file", "/x/c.md", "line", 2)
	log.Warn("odd types", "file", 4, "line", "5")

	want := []ConsoleEntry{
		{Message: "block a: depends= names unknown block id=a", File: "/x/a.md", Line: 3},
		{Message: "near", File: "/x/b.md", Line: 7},
		{Message: "no file line=9"},
		{Message: "grouped g.file=/x/c.md g.line=2"},
		{Message: "odd types file=4 line=5"},
	}
	got := c.list()
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if g := got[i]; g.Message != w.Message || g.File != w.File || g.Line != w.Line {
			t.Errorf("entry %d = %q %q:%d, want %q %q:%d", i, g.Message, g.File, g.Line, w.Message, w.File, w.Line)
		}
	}
}

// A "fix" attribute names a warning's fix; resolve strikes out the
// warnings it names, once, and says so on the stale channel.
func TestConsoleFixAndResolve(t *testing.T) {
	c := newConsole()
	log := slog.New(newConsoleHandler(slog.NewTextHandler(io.Discard, nil), c))
	log.Warn("output copy for unknown block", "file", "/x/a.md", "line", 7, "fix", "Delete the copy")
	log.Warn("output copy for unknown block", "file", "/x/b.md", "line", 7)
	log.Warn("no file", "fix", "Delete it")

	got := c.list()
	if got[0].Fix != "Delete the copy" || got[1].Fix != "" {
		t.Errorf("fixes %q, %q", got[0].Fix, got[1].Fix)
	}
	if got[2].Fix != "" || got[2].Message != `no file fix="Delete it"` {
		t.Errorf("a fix with no file: %+v", got[2])
	}
	if seqs := c.resolve("/x/a.md", "output copy for unknown block"); len(seqs) != 1 || seqs[0] != got[0].Seq {
		t.Errorf("resolved %v, want [%d]", seqs, got[0].Seq)
	}
	if seqs := <-c.stale; len(seqs) != 1 {
		t.Errorf("sent %v", seqs)
	}
	if seqs := c.resolve("/x/a.md", "output copy for unknown block"); seqs != nil {
		t.Errorf("resolved %v again", seqs)
	}
	got = c.list()
	if !got[0].Stale || got[1].Stale {
		t.Errorf("stale: %v, %v", got[0].Stale, got[1].Stale)
	}
}

func TestConsoleKeepsTheLast(t *testing.T) {
	c := newConsole()
	for i := range consoleMax + 5 {
		c.add(ConsoleEntry{Time: time.Now(), Level: "info", Message: fmt.Sprint(i)})
	}
	got := c.list()
	if len(got) != consoleMax || got[0].Message != "5" || got[len(got)-1].Seq != consoleMax+5 {
		t.Errorf("kept %d, first %q, last seq %d", len(got), got[0].Message, got[len(got)-1].Seq)
	}
	c.clear()
	if n := len(c.list()); n != 0 {
		t.Errorf("after clear, %d entries", n)
	}
	if e := c.add(ConsoleEntry{Time: time.Now(), Level: "warn", Message: "after"}); e.Seq != consoleMax+6 {
		t.Errorf("seq after clear = %d", e.Seq)
	}
}

func TestConsoleSend(t *testing.T) {
	c := newConsole()
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan ConsoleEntry, 1)
	done := make(chan struct{})
	go func() {
		c.send(ctx, func(e ConsoleEntry) { got <- e }, func([]int) {})
		close(done)
	}()
	slog.New(newConsoleHandler(slog.DiscardHandler, c)).Error("boom", "err", errors.New("bad depends"))
	select {
	case e := <-got:
		if e.Message != `boom err="bad depends"` || e.Level != "error" {
			t.Errorf("sent %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("nothing sent")
	}
	cancel()
	<-done
}
