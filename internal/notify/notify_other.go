//go:build !darwin

package notify

import "errors"

// Post is only available on macOS.
func Post(Notification) error { return errors.New("notifications need macOS") }
