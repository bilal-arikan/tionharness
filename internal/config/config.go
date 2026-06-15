// Package config loads runtime configuration from environment variables
// and resolves data/workspace directories.
package config

import (
	"os"
	"path/filepath"
)

// Config holds resolved runtime settings.
type Config struct {
	Addr         string // HTTP listen address, e.g. ":8080"
	DataDir      string // persistent state directory
	WorkspaceDir string // task workspace root
	AccessKey    string // dashboard auth token (optional)

	// Provider keys (loaded lazily; empty means unset).
	AnthropicAPIKey string
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dataDir := envOr("SWARMGO_DATA_DIR", filepath.Join(home, ".swarmgo"))
	workspaceDir := envOr("SWARMGO_WORKSPACE_DIR", filepath.Join(dataDir, "workspace"))

	cfg := &Config{
		Addr:            envOr("SWARMGO_ADDR", ":8080"),
		DataDir:         dataDir,
		WorkspaceDir:    workspaceDir,
		AccessKey:       os.Getenv("ACCESS_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
	}

	// Ensure base directories exist.
	for _, dir := range []string{cfg.DataDir, cfg.WorkspaceDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// DBPath returns the SQLite database file path inside the data directory.
func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "swarmgo.db")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
