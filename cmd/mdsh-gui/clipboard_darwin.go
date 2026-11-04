package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>

// copyGIF puts the GIF in data on the general pasteboard, as GIF data
// and as the file at path, so an app that takes images pastes the
// animation and Finder or a chat app pastes the file.
static int copyGIF(const void *data, int n, const char *path) {
	@autoreleasepool {
		NSPasteboardItem *item = [[NSPasteboardItem alloc] init];
		[item setData:[NSData dataWithBytes:data length:n] forType:@"com.compuserve.gif"];
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		[item setString:[url absoluteString] forType:NSPasteboardTypeFileURL];
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		return [pb writeObjects:@[item]] ? 1 : 0;
	}
}

// copyConcealed puts text on the general pasteboard marked concealed, so
// clipboard managers that follow nspasteboard.org leave it out of their
// history, and returns the pasteboard's change count after, or -1.
static long copyConcealed(const char *text) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		NSPasteboardItem *item = [[NSPasteboardItem alloc] init];
		[item setString:[NSString stringWithUTF8String:text] forType:NSPasteboardTypeString];
		[item setData:[NSData data] forType:@"org.nspasteboard.ConcealedType"];
		if (![pb writeObjects:@[item]]) return -1;
		return (long)[pb changeCount];
	}
}

// clearIf empties the general pasteboard if nothing has been put on it
// since change, its change count then.
static void clearIf(long change) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		if ([pb changeCount] == change) [pb clearContents];
	}
}
*/
import "C"

import (
	"errors"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

// copyGIF puts data, an animated GIF, on the clipboard. It is saved to a
// file in the temporary folder too, for apps that paste files.
func copyGIF(data []byte) error {
	path := filepath.Join(os.TempDir(), "mdsh-recording-"+time.Now().Format("20060102-150405")+".gif")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	p := C.CBytes(data)
	defer C.free(p)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	if C.copyGIF(p, C.int(len(data)), cpath) == 0 {
		return errors.New("the clipboard didn't take the GIF")
	}
	return nil
}

// copySecret puts text, a secret, on the clipboard marked concealed, and
// takes it off again after d unless something else has been copied
// since.
func copySecret(text string, _ clipboardText, d time.Duration) error {
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	change := C.copyConcealed(ctext)
	if change < 0 {
		return errors.New("the clipboard didn't take the text")
	}
	time.AfterFunc(d, func() { C.clearIf(change) })
	return nil
}
