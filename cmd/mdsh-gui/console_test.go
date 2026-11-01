package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestConsoleKeepsTheLast(t *testing.T) {
	c := newConsole()
	for i := range consoleMax + 5 {
		c.add(time.Now(), slog.LevelInfo, fmt.Sprint(i))
	}
	got := c.list()
	if len(got) != consoleMax || got[0].Message != "5" || got[len(got)-1].Seq != consoleMax+5 {
		t.Errorf("kept %d, first %q, last seq %d", len(got), got[0].Message, got[len(got)-1].Seq)
	}
	c.clear()
	if n := len(c.list()); n != 0 {
		t.Errorf("after clear, %d entries", n)
	}
	if e := c.add(time.Now(), slog.LevelWarn, "after"); e.Seq != consoleMax+6 {
		t.Errorf("seq after clear = %d", e.Seq)
	}
}

func TestConsoleSend(t *testing.T) {
	c := newConsole()
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan ConsoleEntry, 1)
	done := make(chan struct{})
	go func() {
		c.send(ctx, func(e ConsoleEntry) { got <- e })
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
