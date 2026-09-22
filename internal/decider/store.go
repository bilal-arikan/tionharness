package decider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// configFileName is the decider's own settings document in the data directory.
// It lives apart from settings.json on purpose: the decider is a separate layer
// with a nested, per-site shape, and it holds no secret (credentials stay on the
// provider instance it points at).
const configFileName = "decider.json"

// ConfigPath returns where the config lives under dataDir.
func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, configFileName)
}

// LoadConfig reads the config from dataDir. A missing file yields
// DefaultConfig; a corrupt one is an error so it is never silently replaced.
func LoadConfig(dataDir string) (Config, error) {
	data, err := os.ReadFile(ConfigPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return DefaultConfig(), fmt.Errorf("parse %s: %w", configFileName, err)
	}
	return c.Normalized(), nil
}

// SaveConfig writes the config to dataDir atomically (temp file + rename).
func SaveConfig(dataDir string, c Config) error {
	data, err := json.MarshalIndent(c.Normalized(), "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(ConfigPath(dataDir), data)
}

// writeFileAtomic writes data via a temp file and a rename so a crash never
// leaves a half-written file behind.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
