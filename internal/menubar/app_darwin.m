#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include <stdatomic.h>

// Implemented in Go (app_darwin.go).
extern void positronAppVisible(int visible);
extern void positronAppReopen(void);

static atomic_int hideDockWhenClosed; // 1: no Dock icon while the window is closed
static atomic_int watching;

// positronShouldHandleReopen is added to Wails' app delegate, which has no such method: a click on
// the Dock icon, or opening the app again from Finder, shows the window.
static BOOL positronShouldHandleReopen(id self, SEL _cmd, NSApplication *app, BOOL hasVisibleWindows) {
	positronAppReopen();
	return NO;
}

static void setDockIcon(BOOL show) {
	NSApplicationActivationPolicy want = show ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory;
	if (NSApp.activationPolicy != want) {
		[NSApp setActivationPolicy:want];
	}
}

// PositronAppWatch reports when the app is hidden (the window closed with the red button or
// Cmd+W, or Cmd+H) and shown again. Call it once the app has launched.
void PositronAppWatch(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (atomic_exchange(&watching, 1)) {
			return;
		}
		NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
		[nc addObserverForName:NSApplicationDidHideNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
			positronAppVisible(0);
			if (atomic_load(&hideDockWhenClosed)) {
				// Leaving the Dock un-hides the app, so take its windows off screen first. The
				// app then stays out of sight until Go shows it (Dock icon, menu bar, opening it
				// again); no unhide notification comes in that case.
				for (NSWindow *w in NSApp.windows) {
					[w orderOut:nil];
				}
				setDockIcon(NO);
			}
		}];
		[nc addObserverForName:NSApplicationDidUnhideNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
			setDockIcon(YES);
			positronAppVisible(1);
		}];
		id delegate = NSApp.delegate;
		SEL reopen = @selector(applicationShouldHandleReopen:hasVisibleWindows:);
		if (delegate != nil && ![delegate respondsToSelector:reopen]) {
			class_addMethod([delegate class], reopen, (IMP)positronShouldHandleReopen, "B@:@B");
		}
	});
}

// PositronAppSetHideDock sets whether the Dock icon goes away while the window is closed.
void PositronAppSetHideDock(int hide) {
	atomic_store(&hideDockWhenClosed, hide ? 1 : 0);
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!hide) {
			setDockIcon(YES);
		} else if (NSApp.isHidden) {
			setDockIcon(NO);
		}
	});
}

// PositronAppShow brings the Dock icon back before the window shows.
void PositronAppShow(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		setDockIcon(YES);
	});
}
