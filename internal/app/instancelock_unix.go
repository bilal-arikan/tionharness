//go:build !windows

package app

import (
	"os"
	"syscall"
)

// instanceLock holds the open lock file whose advisory flock guards the data dir.
type instanceLock struct{ f *os.File }

// acquireInstanceLock takes a non-blocking exclusive flock on the lock file. The
// advisory lock is bound to the open file description and released when the fd is
// closed OR the process exits, so a crash never leaves a stale lock. A second
// process (or a second open in this one) gets EWOULDBLOCK while it is held.
func acquireInstanceLock(path string) (*instanceLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return &instanceLock{f: f}, nil
}

// release drops the flock and closes the file. Idempotent.
func (l *instanceLock) release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
