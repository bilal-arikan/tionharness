//go:build !windows

package db

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func durableReplace(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

func durableRemove(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

// syncDir fsyncs a directory. Some filesystems (exFAT/SMB volumes on macOS, some
// FUSE/9p mounts on Linux) cannot fsync a directory and answer EINVAL/ENOTSUP;
// the rename or remove has already happened, so that is not a failed write.
func syncDir(dir *os.File) error {
	err := dir.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EBADF) {
		return nil
	}
	return err
}
