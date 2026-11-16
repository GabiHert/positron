package menubar

import "sync"

// AppHandlers hear about the app window from macOS.
type AppHandlers struct {
	// Visible is called with false when the app is hidden (the window was closed or Cmd+H) and
	// with true when it shows again.
	Visible func(visible bool)
	// Reopen is called when the person clicks the Dock icon or opens the app again.
	Reopen func()
}

var (
	appMu       sync.Mutex
	appHandlers AppHandlers
)

// WatchApp routes the app's hide, unhide and reopen to h. Call it once the app has launched.
func WatchApp(h AppHandlers) {
	appMu.Lock()
	appHandlers = h
	appMu.Unlock()
	appWatch()
}

// SetHideDockWhenClosed sets whether the Dock icon goes away while the window is closed.
func SetHideDockWhenClosed(hide bool) { appSetHideDock(hide) }

// ShowDockIcon brings the Dock icon back; call it before showing the window.
func ShowDockIcon() { appShow() }

func appVisible(v bool) {
	appMu.Lock()
	fn := appHandlers.Visible
	appMu.Unlock()
	if fn != nil {
		go fn(v)
	}
}

func appReopen() {
	appMu.Lock()
	fn := appHandlers.Reopen
	appMu.Unlock()
	if fn != nil {
		go fn()
	}
}
