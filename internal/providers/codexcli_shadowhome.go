package providers

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const shadowHomeMaxAge = time.Hour

const shadowHomeOwnerFile = ".owner"

const processStartTimeTolerance = 2 * time.Second

type shadowHomeOwner struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
}

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
	if err := sweepShadowHomes(shadowRoot, time.Now()); err != nil {
		return "", cleanup, err
	}
	dir, err = os.MkdirTemp(shadowRoot, "turn-")
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	startedAt, ok := processStartTime(os.Getpid())
	if !ok {
		startedAt = time.Now()
	}
	if err := writeShadowHomeOwner(dir, startedAt); err != nil {
		cleanup()
		return "", func() {}, err
	}

	for _, name := range []string{"auth.json", "models_cache.json", "installation_id"} {
		if err := copyShadowHomeFile(base, dir, name); err != nil {
			cleanup()
			return "", func() {}, err
		}
	}
	return dir, cleanup, nil
}

func sweepShadowHomes(shadowRoot string, now time.Time) error {
	entries, err := os.ReadDir(shadowRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "turn-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) <= shadowHomeMaxAge {
			continue
		}
		active, err := shadowHomeIsActive(filepath.Join(shadowRoot, entry.Name()))
		if err != nil || active {
			continue
		}
		_ = os.RemoveAll(filepath.Join(shadowRoot, entry.Name()))
	}
	return nil
}

func writeShadowHomeOwner(dir string, startedAt time.Time) error {
	data, err := json.Marshal(shadowHomeOwner{PID: os.Getpid(), StartedAt: startedAt})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, shadowHomeOwnerFile), data, 0o600)
}

func shadowHomeIsActive(dir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, shadowHomeOwnerFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	var owner shadowHomeOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		return false, err
	}
	if owner.PID <= 0 || owner.StartedAt.IsZero() {
		return false, errors.New("invalid shadow home owner")
	}
	if !processIsAlive(owner.PID) {
		return false, nil
	}
	startedAt, ok := processStartTime(owner.PID)
	if !ok {
		// An unavailable start time is ambiguous. Preserve the directory rather
		// than risk deleting a live turn.
		return true, nil
	}
	return absDuration(startedAt.Sub(owner.StartedAt)) <= processStartTimeTolerance, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func copyShadowHomeFile(base, dir, name string) error {
	src := filepath.Join(base, name)
	in, err := os.Open(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, "."+name+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	if err := out.Chmod(0o600); err != nil {
		_ = out.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	dst := filepath.Join(dir, name)
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return nil
}
