//go:build windows

package db

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func durableMove(from, to string, replace bool) error {
	fromPtr, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	return windows.MoveFileEx(fromPtr, toPtr, flags)
}

func durableReplace(from, to string) error {
	return durableMove(from, to, true)
}

func durableRemove(path string) error {
	tomb, err := os.CreateTemp(filepath.Dir(path), ".durable-retired-*")
	if err != nil {
		return err
	}
	tombPath := tomb.Name()
	if err := tomb.Close(); err != nil {
		_ = os.Remove(tombPath)
		return err
	}
	if err := os.Remove(tombPath); err != nil {
		return err
	}
	if err := durableMove(path, tombPath, false); err != nil {
		return err
	}
	return os.Remove(tombPath)
}
