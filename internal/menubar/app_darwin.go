//go:build darwin

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
void PositronAppWatch(void);
void PositronAppSetHideDock(int hide);
void PositronAppShow(void);
*/
import "C"

func appWatch() { C.PositronAppWatch() }
func appShow()  { C.PositronAppShow() }
func appSetHideDock(hide bool) {
	h := 0
	if hide {
		h = 1
	}
	C.PositronAppSetHideDock(C.int(h))
}

//export positronAppVisible
func positronAppVisible(visible C.int) { appVisible(visible != 0) }

//export positronAppReopen
func positronAppReopen() { appReopen() }
