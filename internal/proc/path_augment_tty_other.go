//go:build !darwin && !freebsd && !netbsd && !openbsd && !linux

package proc

// stdinIsTerminal is only consulted on macOS/Linux/BSD (AugmentPATH is a no-op
// on Windows); elsewhere report "not a terminal" so the login-shell PATH is read.
func stdinIsTerminal() bool { return false }
