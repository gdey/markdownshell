package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// --serve=stdio serves nothing until Settings turns it on.
func TestServeStdioOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := serveStdio(options{}); !errors.Is(err, errServeOff) {
		t.Errorf("serveStdio with the setting off = %v, want errServeOff", err)
	}
	// On, it gets past the setting; a missing file is the next thing
	// it stops at.
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg, "mdsh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "mdsh", "gui.json"), []byte(`{"serve": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := serveStdio(options{args: []string{filepath.Join(dir, "missing.md")}}); errors.Is(err, errServeOff) {
		t.Error("serveStdio with the setting on still says it's off")
	}
}
