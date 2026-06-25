package market

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// registriesFile holds the configured remote registries (global market dir).
const registriesFile = "registries.json"

// remoteCacheDirName is the subdir (inside the global market dir) where fetched
// registry indexes are cached. scanDir skips directories, so this never pollutes
// the local pack catalog.
const remoteCacheDirName = ".remote-cache"

// installedLedgerFile records, per workspace, the version last installed for each
// pack id, so the UI can flag available updates. Lives in the write (workspace) dir.
const installedLedgerFile = "installed.json"

// --- Registry configuration (global, shared across workspaces) ---

func (s *Store) registriesPath() string {
	if s.globalDir == "" {
		return ""
	}
	return filepath.Join(s.globalDir, registriesFile)
}

// ListRegistries returns the configured remote registries (sorted by name).
func (s *Store) ListRegistries() []Registry {
	regs := s.loadRegistries()
	sort.Slice(regs, func(i, j int) bool {
		return strings.ToLower(regs[i].Name) < strings.ToLower(regs[j].Name)
	})
	return regs
}

// loadRegistries reads registries.json (absent → empty list).
func (s *Store) loadRegistries() []Registry {
	path := s.registriesPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var regs []Registry
	if json.Unmarshal(data, &regs) != nil {
		return nil
	}
	return regs
}

// saveRegistries persists the registry list atomically.
func (s *Store) saveRegistries(regs []Registry) error {
	path := s.registriesPath()
	if path == "" {
		return fmt.Errorf("no global market directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(regs, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AddRegistry adds (or re-enables/renames) a registry by URL and returns the list.
func (s *Store) AddRegistry(name, url string) ([]Registry, error) {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("registry URL must be http(s)")
	}
	if strings.TrimSpace(name) == "" {
		name = url
	}
	regs := s.loadRegistries()
	for i := range regs {
		if strings.EqualFold(regs[i].URL, url) {
			regs[i].Name = name
			regs[i].Enabled = true
			if err := s.saveRegistries(regs); err != nil {
				return nil, err
			}
			return s.ListRegistries(), nil
		}
	}
	regs = append(regs, Registry{Name: name, URL: url, Enabled: true, AddedAt: time.Now().Unix()})
	if err := s.saveRegistries(regs); err != nil {
		return nil, err
	}
	return s.ListRegistries(), nil
}

// RemoveRegistry drops a registry by URL and clears its cached index.
func (s *Store) RemoveRegistry(url string) ([]Registry, error) {
	url = strings.TrimSpace(url)
	regs := s.loadRegistries()
	out := make([]Registry, 0, len(regs))
	for _, r := range regs {
		if strings.EqualFold(r.URL, url) {
			continue
		}
		out = append(out, r)
	}
	if err := s.saveRegistries(out); err != nil {
		return nil, err
	}
	// Drop its cached index, then rebuild the in-memory remote catalog.
	if dir := s.remoteCacheDir(); dir != "" {
		_ = os.Remove(filepath.Join(dir, cacheFileName(url)))
	}
	s.Reload()
	return s.ListRegistries(), nil
}

// --- Remote index cache ---

func (s *Store) remoteCacheDir() string {
	if s.globalDir == "" {
		return ""
	}
	return filepath.Join(s.globalDir, remoteCacheDirName)
}

// cacheFileName derives a stable cache filename for a registry URL.
func cacheFileName(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:8]) + ".json"
}

// cachedIndex pairs a fetched index with the registry it came from (persisted in
// the cache so the merged catalog survives restarts without re-fetching).
type cachedIndex struct {
	RegistryName string        `json:"registryName"`
	URL          string        `json:"url"`
	Index        RegistryIndex `json:"index"`
}

// RefreshRemote fetches every enabled registry's index, caches it, and rebuilds
// the in-memory remote catalog. Per-registry errors are collected and returned
// together; a failing registry never blocks the others.
func (s *Store) RefreshRemote(ctx context.Context) error {
	dir := s.remoteCacheDir()
	if dir == "" {
		return fmt.Errorf("no global market directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var errs []string
	for _, r := range s.loadRegistries() {
		if !r.Enabled {
			continue
		}
		idx, err := fetchIndex(ctx, r.URL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.Name, err))
			continue
		}
		entry := cachedIndex{RegistryName: r.Name, URL: r.URL, Index: idx}
		data, mErr := json.MarshalIndent(entry, "", "  ")
		if mErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.Name, mErr))
			continue
		}
		_ = os.WriteFile(filepath.Join(dir, cacheFileName(r.URL)), data, 0o644)
	}
	s.Reload() // rebuild remote map from the refreshed cache
	if len(errs) > 0 {
		return fmt.Errorf("some registries failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// loadRemoteCache reads every cached registry index and builds the id→Pack remote
// map. Only registries still present and enabled in registries.json contribute,
// so a removed/disabled registry's stale cache is ignored.
func (s *Store) loadRemoteCache() map[string]Pack {
	out := map[string]Pack{}
	dir := s.remoteCacheDir()
	if dir == "" {
		return out
	}
	enabled := map[string]bool{}
	for _, r := range s.loadRegistries() {
		if r.Enabled {
			enabled[strings.ToLower(r.URL)] = true
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, rErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rErr != nil {
			continue
		}
		var ci cachedIndex
		if json.Unmarshal(data, &ci) != nil {
			continue
		}
		if !enabled[strings.ToLower(ci.URL)] {
			continue // registry removed/disabled — skip its stale cache
		}
		for _, pe := range ci.Index.Packs {
			if pe.ID == "" || pe.Kind == "" || pe.URL == "" {
				continue
			}
			out[pe.ID] = entryToPack(pe, ci.RegistryName)
		}
	}
	return out
}

// --- Install ledger (per workspace) ---

func (s *Store) ledgerPath() string {
	if s.ledgerDir == "" {
		return ""
	}
	return filepath.Join(s.ledgerDir, installedLedgerFile)
}

// InstalledVersions returns the recorded packID→version map for this workspace.
func (s *Store) InstalledVersions() map[string]string {
	out := map[string]string{}
	path := s.ledgerPath()
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// RecordInstall stamps a pack id with the version just installed (best-effort).
func (s *Store) RecordInstall(id, version string) {
	path := s.ledgerPath()
	if path == "" || id == "" {
		return
	}
	led := s.InstalledVersions()
	if version == "" {
		version = "0.0.0"
	}
	led[id] = version
	data, err := json.MarshalIndent(led, "", "  ")
	if err != nil {
		return
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// UpdateAvailable reports whether the catalog version of id is newer than the
// recorded installed version (false when never installed or up to date).
func (s *Store) UpdateAvailable(id, catalogVersion string) bool {
	led := s.InstalledVersions()
	cur, ok := led[id]
	if !ok {
		return false
	}
	return compareVersions(catalogVersion, cur) > 0
}
