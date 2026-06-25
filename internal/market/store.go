package market

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// packFileSuffix is the extension every pack file carries.
const packFileSuffix = ".swarmpack.json"

// tier pairs a directory with the source label its packs carry. A tier with an
// fsys is read from an embedded FS (bundled); otherwise from the OS dir.
type tier struct {
	dir    string
	source Source
}

// Store resolves and caches marketplace packs from the three tiers (bundled →
// global → workspace). Safe for concurrent use. The cache holds manifests only;
// payloads are read from disk on demand so the catalog stays lean.
type Store struct {
	tiers []tier

	mu     sync.RWMutex
	loaded bool
	byID   map[string]Pack // id -> resolved (highest-priority) LOCAL pack manifest
	order  []string        // local ids, display order (sorted by name)

	// remote holds pack manifests resolved from cached remote registry indexes,
	// keyed by id. Local packs override remote ones of the same id in List().
	remote map[string]Pack

	// globalDir is the only local pack tier and the data-dir-level market dir; it
	// holds the pack files, registries.json and the remote-index cache. Publish
	// writes here.
	globalDir string
	// ledgerDir is the per-workspace root where the install ledger (installed.json)
	// lives — installs are per-workspace, so update status is tracked per workspace.
	ledgerDir string
}

// New builds a store over a single local pack tier — the global market dir
// (<DataDir>/market) — plus remote registries. There is no bundled (embedded) or
// per-workspace pack tier: packs live only in the global dir and remote sources.
// ledgerDir is the per-workspace root where the install ledger is kept. Any dir
// may be empty/missing.
func New(globalDir, ledgerDir string) *Store {
	var tiers []tier
	if globalDir != "" {
		tiers = append(tiers, tier{globalDir, SourceGlobal})
	}
	return &Store{
		tiers:     tiers,
		byID:      map[string]Pack{},
		remote:    map[string]Pack{},
		globalDir: globalDir,
		ledgerDir: ledgerDir,
	}
}

// ensure lazily loads the catalog on first use.
func (s *Store) ensure() {
	s.mu.RLock()
	done := s.loaded
	s.mu.RUnlock()
	if done {
		return
	}
	s.Reload()
}

// Reload re-scans every tier, rebuilding the catalog (manifests only).
func (s *Store) Reload() {
	byID := map[string]Pack{}
	for _, t := range s.tiers { // ascending priority: later overrides earlier
		for _, p := range scanDir(t) {
			byID[p.ID] = p
		}
	}
	order := make([]string, 0, len(byID))
	for id := range byID {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := byID[order[i]], byID[order[j]]
		if strings.EqualFold(a.Name, b.Name) {
			return a.ID < b.ID
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	remote := s.loadRemoteCache()

	s.mu.Lock()
	s.byID = byID
	s.order = order
	s.remote = remote
	s.loaded = true
	s.mu.Unlock()
}

// scanDir reads every <dir>/*.swarmpack.json and parses its manifest. Malformed
// or mis-tagged files are skipped silently (a bad file never breaks the catalog).
func scanDir(t tier) []Pack {
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		return nil // missing/inaccessible dir → no packs
	}
	var out []Pack
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), packFileSuffix) {
			continue
		}
		path := filepath.Join(t.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p Pack
		if json.Unmarshal(data, &p) != nil {
			continue
		}
		if p.Schema != SchemaV1 || p.ID == "" || p.Kind == "" {
			continue // not a well-formed pack
		}
		if p.Name == "" {
			p.Name = p.ID
		}
		// Manifest-only in the catalog: payload is re-read on Get.
		p.Payload = Payload{}
		p.Source = t.source
		p.Path = path
		out = append(out, p)
	}
	return out
}

// List returns the resolved pack manifests in display order: local packs first
// (workspace>global>bundled), then remote packs whose id is not shadowed by a
// local one. Sorted by name within each group.
func (s *Store) List() []Pack {
	s.ensure()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Pack, 0, len(s.order)+len(s.remote))
	for _, id := range s.order {
		out = append(out, s.byID[id])
	}
	rem := make([]Pack, 0, len(s.remote))
	for id, p := range s.remote {
		if _, shadowed := s.byID[id]; shadowed {
			continue
		}
		rem = append(rem, p)
	}
	sort.Slice(rem, func(i, j int) bool {
		if strings.EqualFold(rem[i].Name, rem[j].Name) {
			return rem[i].ID < rem[j].ID
		}
		return strings.ToLower(rem[i].Name) < strings.ToLower(rem[j].Name)
	})
	return append(out, rem...)
}

// ListKind returns only packs of the given kind, in display order.
func (s *Store) ListKind(kind string) []Pack {
	out := []Pack{}
	for _, p := range s.List() {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	return out
}

// Get returns a pack WITH its payload, read fresh from disk so edits are picked
// up and the (heavier) payload never sits in the catalog cache.
func (s *Store) Get(id string) (Pack, bool) {
	s.ensure()
	s.mu.RLock()
	meta, ok := s.byID[id]
	rmeta, rok := s.remote[id]
	s.mu.RUnlock()
	if !ok {
		// Not local — try a remote pack: download its payload lazily (optional
		// sha256 verification happens inside fetchPayload).
		if !rok {
			return Pack{}, false
		}
		// A source-ref pack (directory-site bridge) has no payload to download — its
		// install runs the ingest pipeline against SourceRef.URL. Return the manifest.
		if rmeta.SourceRef != nil {
			return rmeta, true
		}
		full, err := fetchPayload(context.Background(), rmeta.remoteURL, rmeta.remoteSHA)
		if err != nil {
			return Pack{}, false
		}
		full.Source = SourceRemote
		full.RegistryName = rmeta.RegistryName
		return full, true
	}
	data, err := os.ReadFile(meta.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload() // stale catalog entry — drop it
		}
		return Pack{}, false
	}
	var full Pack
	if json.Unmarshal(data, &full) != nil {
		return Pack{}, false
	}
	full.Source = meta.Source
	full.Path = meta.Path
	return full, true
}

// Publish writes a pack to the global market dir and reloads the catalog. The
// file is named <id>.swarmpack.json; an existing pack with the same id is
// overwritten (re-publish updates in place).
func (s *Store) Publish(p Pack) (Pack, error) {
	if s.globalDir == "" {
		return Pack{}, fmt.Errorf("no writable market directory")
	}
	if p.ID == "" || p.Kind == "" {
		return Pack{}, fmt.Errorf("pack id and kind are required")
	}
	p.Schema = SchemaV1
	if err := os.MkdirAll(s.globalDir, 0o755); err != nil {
		return Pack{}, fmt.Errorf("create market dir: %w", err)
	}
	path := filepath.Join(s.globalDir, safeFileName(p.ID)+packFileSuffix)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Pack{}, fmt.Errorf("marshal pack: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Pack{}, fmt.Errorf("write pack: %w", err)
	}
	s.Reload()
	out, _ := s.Get(p.ID)
	return out, nil
}

// Import writes a raw pack JSON (e.g. pasted by the user) into the writable tier
// after validating the envelope. Returns the stored pack.
func (s *Store) Import(raw []byte) (Pack, error) {
	var p Pack
	if err := json.Unmarshal(raw, &p); err != nil {
		return Pack{}, fmt.Errorf("invalid pack JSON: %w", err)
	}
	if p.Schema != SchemaV1 {
		return Pack{}, fmt.Errorf("unsupported pack schema %q (want %q)", p.Schema, SchemaV1)
	}
	if p.ID == "" || p.Kind == "" {
		return Pack{}, fmt.Errorf("pack id and kind are required")
	}
	return s.Publish(p)
}

// safeFileName strips path separators and other unsafe chars from a pack id so
// it can be used as a filename.
func safeFileName(id string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_", " ", "-")
	return strings.Trim(r.Replace(id), "._-")
}
