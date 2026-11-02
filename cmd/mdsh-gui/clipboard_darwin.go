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
