package providers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// prepareShadowHome creates a turn-local CODEX_HOME while sharing the base
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
	dst := filepath.Join(dir, "auth.json")
	if err := os.Link(src, dst); err == nil {
		return dir, cleanup, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return dir, cleanup, nil
	}

	in, err := os.Open(src)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		cleanup()
		return "", func() {}, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", func() {}, closeErr
	}
	return dir, cleanup, nil
}
