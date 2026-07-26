//go:build windows

package app

import "syscall"

// instanceLock holds the exclusive handle to the data-dir lock file. On Windows the
// lock is enforced by the share mode, not an advisory byte-range lock.
type instanceLock struct{ h syscall.Handle }

// acquireInstanceLock opens the lock file with dwShareMode = 0 (no sharing): while
// this handle is open, ANY other open of the same path — even from this process —
// fails with ERROR_SHARING_VIOLATION. The handle (and therefore the lock) is
// released by the OS when the process exits, so a crash never leaves a stale lock.
func acquireInstanceLock(path string) (*instanceLock, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(
		p,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // dwShareMode = 0 → exclusive; a concurrent open fails
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	return &instanceLock{h: h}, nil
}

// release closes the handle, freeing the lock. Idempotent.
func (l *instanceLock) release() error {
	if l == nil || l.h == syscall.InvalidHandle {
		return nil
	}
	err := syscall.CloseHandle(l.h)
	l.h = syscall.InvalidHandle
	return err
}
