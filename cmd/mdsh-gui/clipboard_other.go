//go:build !darwin

package main

import "errors"

// errNoGIFClipboard is returned by copyGIF where mdsh-gui can only put
// text on the clipboard.
var errNoGIFClipboard = errors.New("copying a GIF needs macOS")

// copyGIF puts data, an animated GIF, on the clipboard: only on macOS.
func copyGIF([]byte) error { return errNoGIFClipboard }
