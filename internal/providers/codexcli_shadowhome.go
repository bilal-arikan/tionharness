package providers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// prepareShadowHome creates a turn-local CODEX_HOME with a snapshot of the base
// home's authentication state. An empty base preserves ambient ~/.codex use.
func prepareShadowHome(base string) (dir string, cleanup func(), err error) {
	cleanup = func() {}
	if base == "" {
		return "", cleanup, nil
	}

	shadowRoot := filepath.Join(base, ".shadow")
	if err := os.MkdirAll(shadowRoot, 0o700); err != nil {
		return "", cleanup, err
	}
	dir, err = os.MkdirTemp(shadowRoot, "turn-")
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	src := filepath.Join(base, "auth.json")
	in, err := os.Open(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dir, cleanup, nil
		}
		cleanup()
		return "", func() {}, err
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, ".auth-*.tmp")
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	if err := out.Chmod(0o600); err != nil {
		_ = out.Close()
		cleanup()
		return "", func() {}, err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		cleanup()
		return "", func() {}, copyErr
	}
	if syncErr != nil {
		cleanup()
		return "", func() {}, syncErr
	}
	if closeErr != nil {
		cleanup()
		return "", func() {}, closeErr
	}
	dst := filepath.Join(dir, "auth.json")
	if err := os.Rename(tmp, dst); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dir, cleanup, nil
}
