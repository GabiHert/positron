// Package notify posts macOS notifications through UNUserNotificationCenter. Wails' own call
// always plays a sound and can't show a picture; this one can. Clicks still reach Wails'
// OnNotificationResponse, with UserInfo as the response's userInfo.
package notify

// Notification is one notification. A newer one with the same ID replaces it.
type Notification struct {
	ID       string
	Title    string
	Subtitle string
	Body     string
	// Thread groups notifications in Notification Center, like one chat's messages.
	Thread string
	Sound  bool
	// Image is a picture file shown with the notification; it is copied first, since macOS
	// takes the file it is given.
	Image    string
	UserInfo map[string]string
}
