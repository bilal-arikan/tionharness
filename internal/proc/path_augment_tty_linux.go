//go:build linux

package proc

import "golang.org/x/sys/unix"

// stdinIsTerminal reports whether stdin is a real terminal. A ModeCharDevice
// check is not enough: /dev/null is a character device too.
func stdinIsTerminal() bool {
	_, err := unix.IoctlGetTermios(0, unix.TCGETS)
	return err == nil
}
