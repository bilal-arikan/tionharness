package decider

import (
	"os"
	"path/filepath"
)

// appendJSONL shares disk writes while each journal owns its rotation policy.
// The caller holds its journal lock and passes a newline-terminated record.
func appendJSONL(path string, maxBytes int64, line []byte, rotate func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line)) > maxBytes {
		if err := rotate(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}
