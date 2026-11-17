package imagefile

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#import <AppKit/AppKit.h>

// board is a named pasteboard for tests; NULL is the clipboard.
static int positronCopyImage(const char *path, const char *board) {
	@autoreleasepool {
		NSString *p = [NSString stringWithUTF8String:path];
		NSImage *img = [[NSImage alloc] initWithContentsOfFile:p];
		if (img == nil) return 1;
		NSPasteboard *pb = board ? [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:board]] : [NSPasteboard generalPasteboard];
		[pb clearContents];
		return [pb writeObjects:@[img]] ? 0 : 2;
	}
}

static int positronBoardHasImage(const char *board) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:board]];
		BOOL ok = [pb canReadObjectForClasses:@[[NSImage class]] options:nil];
		[pb releaseGlobally];
		return ok ? 1 : 0;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Pasteboard puts the image file at path on the macOS clipboard as an image.
func Pasteboard(path string) error { return copyTo(path, "") }

func copyTo(path, board string) error {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	var cb *C.char
	if board != "" {
		cb = C.CString(board)
		defer C.free(unsafe.Pointer(cb))
	}
	switch C.positronCopyImage(cs, cb) {
	case 0:
		return nil
	case 1:
		return errors.New("macOS could not read this image")
	default:
		return errors.New("could not put the image on the clipboard")
	}
}

// boardHasImage reports whether the named pasteboard holds an image, then drops the pasteboard.
func boardHasImage(board string) bool {
	cb := C.CString(board)
	defer C.free(unsafe.Pointer(cb))
	return C.positronBoardHasImage(cb) == 1
}
