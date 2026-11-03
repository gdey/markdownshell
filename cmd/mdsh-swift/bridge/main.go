// Command mdsh-swift-bridge serves mdsh-gui's notebook to the SwiftUI
// app, mdsh-swift, as JSON lines on stdin and stdout (see rpc.go).
//
//	mdsh-swift-bridge [--settings FILE]
//
// The app starts it once and keeps it for as long as it runs. Every
// document the app opens is a notebook here, with its own shells; the
// app only draws them. All the parsing, running, editing and saving is
// the same Go code mdsh-gui and mdsh use, so the Swift app adds no
// logic or file format of its own.
//
// Log lines go to the app as "console" events, and to stderr.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

func main() {
	settingsFile := flag.String("settings", "", "the settings `file` (default mdsh/swift.json in the config folder)")
	flag.Parse()
	path := *settingsFile
	if path == "" {
		var err error
		if path, err = defaultSettings(); err != nil {
			fmt.Fprintln(os.Stderr, "mdsh-swift-bridge:", err)
			os.Exit(1)
		}
	}
	store, err := settings.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdsh-swift-bridge:", err)
		os.Exit(1)
	}
	c := newConn(os.Stdout)
	slog.SetDefault(slog.New(newConsoleHandler(c, slog.NewTextHandler(os.Stderr, nil))))
	b := newBridge(c, store)
	if err := serve(context.Background(), os.Stdin, c, b.methods(), b.closeAll); err != nil {
		fmt.Fprintln(os.Stderr, "mdsh-swift-bridge:", err)
		os.Exit(1)
	}
}

// defaultSettings is mdsh/swift.json in the user's config folder, beside
// mdsh-gui's gui.json. The two apps keep their own, so one running
// doesn't write over the other's recent files and window places.
func defaultSettings() (string, error) {
	gui, err := settings.DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(gui), "swift.json"), nil
}
