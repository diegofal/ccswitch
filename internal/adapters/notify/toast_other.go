//go:build !windows

package notify

import "errors"

// toast exists only on Windows; Notify uses a command elsewhere.
func toast(string, string) error { return errors.New("toasts are Windows only") }
