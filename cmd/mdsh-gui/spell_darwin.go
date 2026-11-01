package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

// registerSpellDefaults makes WebKit check spelling as you type unless
// the user turned it off: WKWebView reads WebContinuousSpellCheckingEnabled
// from the app's defaults and treats a missing key as off, so the
// spellcheck attribute alone underlines nothing.
static void registerSpellDefaults(void) {
	[[NSUserDefaults standardUserDefaults] registerDefaults:@{@"WebContinuousSpellCheckingEnabled": @YES}];
}
*/
import "C"

// init runs before the webview is made, which reads the default once.
func init() { C.registerSpellDefaults() }
