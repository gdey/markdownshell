//go:build !darwin

package main

import (
	"errors"
	"time"
)

// errNoGIFClipboard is returned by copyGIF where mdsh-gui can only put
// text on the clipboard.
var errNoGIFClipboard = errors.New("copying a GIF needs macOS")

// copyGIF puts data, an animated GIF, on the clipboard: only on macOS.
func copyGIF([]byte) error { return errNoGIFClipboard }

// copySecret puts text, a secret, on the clipboard, and takes it off
// again after d if it is still there. Only macOS has a way to keep it
// out of clipboard managers' history.
func copySecret(text string, cb clipboardText, d time.Duration) error {
	if !cb.SetText(text) {
		return errors.New("couldn't set the clipboard")
	}
	time.AfterFunc(d, func() {
		if now, ok := cb.Text(); ok && now == text {
			cb.SetText("")
		}
	})
	return nil
}
