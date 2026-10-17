package settings

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The HTTP provider kinds (anthropic, openrouter, deepseek, minimax, zai,
// openai-compat) were removed on 2026-10-13 (_Docs/95 §7); their API keys now
// belong in opencode's own credential store, keyed by opencode's provider id.
// This file copies those keys out of a legacy providers.json instance into
// the app-global opencode home so the user does not have to re-enter them.

// openCodeVendorForLegacy maps a removed kind's instance to the opencode
// provider id that takes its key, or "" when there is no counterpart.
func openCodeVendorForLegacy(p ProviderInstance) string {
	switch p.KindID {
	case "anthropic":
		return "anthropic"
	case "openrouter":
		return "openrouter"
	case "deepseek", "deepseek-anthropic":
		return "deepseek"
	case "minimax", "minimax-anthropic":
		return "minimax"
	case "zai":
		return "zai"
	case "openai-compat":
		if strings.Contains(strings.ToLower(p.Config["baseUrl"]), "openrouter.ai") {
			return "openrouter"
		}
	}
	return ""
}

// openCodeAuthEntry is the shape `opencode auth login` writes for an API key.
type openCodeAuthEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// ExportLegacyKeysToOpenCode copies the API key of every instance whose kind
// is no longer registered (registered reports a live kind) into the opencode
// auth store under <dataDir>/opencode-home. A vendor that already holds a
// credential there is never overwritten, so the step is idempotent and a key
// the user entered through `opencode auth login` wins. It returns the vendors
// it added. The legacy instances themselves are left in place for the user to
// delete.
func (s *ProviderStore) ExportLegacyKeysToOpenCode(dataDir string, registered func(kindID string) bool) ([]string, error) {
	pending := map[string]string{}
	for _, p := range s.List() {
		if registered(p.KindID) {
			continue
		}
		vendor := openCodeVendorForLegacy(p)
		if vendor == "" {
			continue
		}
		if _, dup := pending[vendor]; dup {
			continue
		}
		if key := s.Secret(p.ID, "key"); key != "" {
			pending[vendor] = key
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}

	path := filepath.Join(legacyDefaultConfigDir(dataDir, "opencode-cli"), "data", "opencode", "auth.json")
	entries := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("opencode auth store %s is not a JSON object: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read opencode auth store: %w", err)
	}

	var added []string
	for vendor, key := range pending {
		if cur, ok := entries[vendor]; ok && strings.TrimSpace(string(cur)) != "" && string(cur) != "null" {
			continue
		}
		enc, _ := json.Marshal(openCodeAuthEntry{Type: "api", Key: key})
		entries[vendor] = enc
		added = append(added, vendor)
	}
	if len(added) == 0 {
		return nil, nil
	}
	sort.Strings(added)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create opencode auth dir: %w", err)
	}
	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return nil, fmt.Errorf("write opencode auth store: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("replace opencode auth store: %w", err)
	}
	slog.Info("copied legacy provider API keys into the opencode auth store", "path", path, "vendors", added)
	return added, nil
}
