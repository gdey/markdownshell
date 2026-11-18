//go:build darwin && cgo

package fonts

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>

// fontFamilies returns the installed font families, one a line, each
// after "m " if a member of it is fixed pitch and "p " if not. The
// caller frees it.
static char *fontFamilies(void) {
	@autoreleasepool {
		NSFontManager *fm = [NSFontManager sharedFontManager];
		NSMutableString *out = [NSMutableString string];
		for (NSString *family in [fm availableFontFamilies]) {
			BOOL mono = NO;
			for (NSArray *m in [fm availableMembersOfFontFamily:family]) {
				if (m.count > 3 && ([m[3] unsignedIntegerValue] & NSFixedPitchFontMask)) {
					mono = YES;
					break;
				}
			}
			[out appendFormat:@"%@ %@\n", mono ? @"m" : @"p", family];
		}
		return strdup([out UTF8String]);
	}
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

// systemFonts asks AppKit's font manager for the installed families.
func systemFonts() Fonts {
	cs := C.fontFamilies()
	defer C.free(unsafe.Pointer(cs))
	var all, mono []string
	for _, line := range strings.Split(C.GoString(cs), "\n") {
		kind, name, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		all = append(all, name)
		if kind == "m" {
			mono = append(mono, name)
		}
	}
	all = tidyFonts(all)
	return Fonts{All: all, Mono: withNerdFonts(all, mono)}
}
