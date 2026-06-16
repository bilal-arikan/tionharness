// Package config loads runtime configuration from environment variables
// and resolves data/workspace directories.
package config

import (
	"os"
	"path/filepath"
)

// Config holds resolved runtime settings.
type Config struct {
	Addr         string // HTTP listen address, e.g. "127.0.0.1:8080"
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
		// Loopback-only by default: the desktop app and dev frontend reach the
		// API over localhost, and binding to 127.0.0.1 avoids the Windows
		// Firewall "allow inbound" prompt that a 0.0.0.0 bind triggers on every
		// rebuild. Set SWARMGO_ADDR (e.g. ":8090" or "0.0.0.0:8090") to expose it.
		Addr:            envOr("SWARMGO_ADDR", "127.0.0.1:8080"),
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
