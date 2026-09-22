package decider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// configFileName is the decider's settings document in the data directory. It
// lives apart from settings.json on purpose: the decider is a separate layer
// with a nested, per-authority shape. It holds no secret (a model's own key
// lives encrypted in decider/models.json).
const configFileName = "decider.json"

// configVersion 2 moved the connection (backend, provider account, model) out
// into decision models and renamed "sites" to "authorities".
const configVersion = 2

// ConfigPath returns where the config lives under dataDir.
func ConfigPath(dataDir string) string {
	return filepath.Join(dataDir, configFileName)
}

// configFile is the on-disk shape, readable in both versions.
type configFile struct {
	Version      int                        `json:"version"`
	Enabled      bool                       `json:"enabled"`
	DefaultModel string                     `json:"defaultModel,omitempty"`
	Authorities  map[string]AuthorityConfig `json:"authorities,omitempty"`

	// Version 1: one connection for every site.
	Backend            string                     `json:"backend,omitempty"`
	ProviderInstanceID string                     `json:"providerInstanceId,omitempty"`
	Model              string                     `json:"model,omitempty"`
	TimeoutMs          int                        `json:"timeoutMs,omitempty"`
	Sites              map[string]AuthorityConfig `json:"sites,omitempty"`
}

// legacyConnection is a version-1 config's connection, from which the first
// decision model is created.
type legacyConnection struct {
	Backend            string
	ProviderInstanceID string
	Model              string
	TimeoutMs          int
}

// loadConfig reads the config from dataDir. A missing file yields
// DefaultConfig; a corrupt one is an error so it is never silently replaced. A
// version-1 file is converted, and its connection returned so the caller can
// turn it into the first decision model.
func loadConfig(dataDir string) (Config, *legacyConnection, error) {
	data, err := os.ReadFile(ConfigPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil, nil
	}
	if err != nil {
		return DefaultConfig(), nil, err
	}
	var f configFile
	if err := json.Unmarshal(data, &f); err != nil {
		return DefaultConfig(), nil, fmt.Errorf("parse %s: %w", configFileName, err)
	}
	c := Config{Enabled: f.Enabled, DefaultModel: f.DefaultModel, Authorities: f.Authorities}
	var legacy *legacyConnection
	if f.Version < configVersion {
		if c.Authorities == nil {
			c.Authorities = f.Sites
		}
		legacy = &legacyConnection{Backend: f.Backend, ProviderInstanceID: f.ProviderInstanceID, Model: f.Model, TimeoutMs: f.TimeoutMs}
	}
	return c.Normalized(), legacy, nil
}

// saveConfig writes the config to dataDir atomically (temp file + rename).
func saveConfig(dataDir string, c Config) error {
	c = c.Normalized()
	data, err := json.MarshalIndent(configFile{
		Version:      configVersion,
		Enabled:      c.Enabled,
		DefaultModel: c.DefaultModel,
		Authorities:  c.Authorities,
	}, "", "  ")
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

// seedModel is the decision model created on first run: from a version-1
// config's connection, or Jev through the first OpenRouter account (the
// out-of-the-box choice) when there was no config at all. It borrows provider
// credentials, exactly like the version-1 connection did.
func seedModel(legacy *legacyConnection) ModelInput {
	in := ModelInput{
		ID:          "DM1",
		Label:       "Jev · OpenRouter",
		Backend:     OpenRouterBackendID,
		Enabled:     true,
		Model:       JevModel,
		Credentials: CredentialsProvider,
	}
	if legacy == nil {
		return in
	}
	if legacy.Backend != "" {
		if b, ok := Lookup(legacy.Backend); ok {
			in.Backend = legacy.Backend
			if legacy.Backend != OpenRouterBackendID {
				in.Label = b.Manifest().Label
			}
		}
	}
	in.ProviderInstanceID = legacy.ProviderInstanceID
	if legacy.Model != "" {
		in.Model = legacy.Model
	}
	in.TimeoutMs = legacy.TimeoutMs
	return in
}
