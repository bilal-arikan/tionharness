package db

import (
	"fmt"
	"os"
	"path/filepath"
)

// durableAtomicWriteBytes is reserved for CLI recovery records and debug
// compaction checkpoints whose correctness must survive power loss. The wider
// store keeps its existing atomicWriteBytes behaviour and cost profile.
func durableAtomicWriteBytes(path string, data []byte, perm os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".durable-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := tmp.Close(); retErr == nil && closeErr != nil {
				retErr = closeErr
			}
		}
		if removeErr := os.Remove(tmpPath); retErr == nil && removeErr != nil && !os.IsNotExist(removeErr) {
			retErr = removeErr
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true
	if err := durableReplace(tmpPath, path); err != nil {
		return fmt.Errorf("durable replace: %w", err)
	}
	return nil
}
