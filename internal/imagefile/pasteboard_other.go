//go:build !darwin

package imagefile

import "errors"

// Pasteboard is only available on macOS.
func Pasteboard(string) error { return errors.New("copying images needs macOS") }
