package pureraw

/*
// Keep the binary runnable on older macOS: PureRAW 6 itself needs 13.3.
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AppKit -mmacosx-version-min=13.0
#import <AppKit/AppKit.h>
#include <stdlib.h>

// requestQuit asks every running instance of the app to quit, like the Dock's
// Quit item. It needs no Automation permission.
static int requestQuit(const char *bundleID) {
	@autoreleasepool {
		NSString *bid = [NSString stringWithUTF8String:bundleID];
		int n = 0;
		for (NSRunningApplication *app in [NSRunningApplication runningApplicationsWithBundleIdentifier:bid]) {
			if ([app terminate]) {
				n++;
			}
		}
		return n;
	}
}
*/
import "C"

import "unsafe"

// requestQuit asks PureRAW to quit normally, which works even while a dialog
// is open and, unlike SIGTERM, lets it exit cleanly.
func requestQuit() int {
	id := C.CString(BundleID)
	defer C.free(unsafe.Pointer(id))
	return int(C.requestQuit(id))
}
