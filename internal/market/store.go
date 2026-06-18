package market

import (
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
	byID   map[string]Pack // id -> resolved (highest-priority) pack manifest
	order  []string        // ids, display order (sorted by name)

	// writeDir is the workspace tier dir; Publish writes here. Falls back to the
	// global dir when there is no workspace dir.
	writeDir string
}

// New builds a store over the global + workspace market tiers. Bundled packs are
// seeded into the global dir by EnsureDefaults (called before New), so a single
// global scan covers both bundled and global tiers. Any dir may be empty/missing.
func New(globalDir, workspaceDir string) *Store {
	var tiers []tier
	if globalDir != "" {
		tiers = append(tiers, tier{globalDir, SourceGlobal})
	}
	if workspaceDir != "" {
		tiers = append(tiers, tier{workspaceDir, SourceWorkspace})
	}
	writeDir := workspaceDir
	if writeDir == "" {
		writeDir = globalDir
	}
	return &Store{tiers: tiers, byID: map[string]Pack{}, writeDir: writeDir}
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

	s.mu.Lock()
	s.byID = byID
	s.order = order
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

// List returns the resolved pack manifests in display order.
func (s *Store) List() []Pack {
	s.ensure()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Pack, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.byID[id])
	}
	return out
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
	s.mu.RUnlock()
	if !ok {
		return Pack{}, false
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

// Publish writes a pack to the writable (workspace, else global) tier and
// reloads the catalog. The file is named <id>.swarmpack.json; an existing pack
// with the same id in that tier is overwritten (re-publish updates in place).
func (s *Store) Publish(p Pack) (Pack, error) {
	if s.writeDir == "" {
		return Pack{}, fmt.Errorf("no writable market directory")
	}
	if p.ID == "" || p.Kind == "" {
		return Pack{}, fmt.Errorf("pack id and kind are required")
	}
	p.Schema = SchemaV1
	if err := os.MkdirAll(s.writeDir, 0o755); err != nil {
		return Pack{}, fmt.Errorf("create market dir: %w", err)
	}
	path := filepath.Join(s.writeDir, safeFileName(p.ID)+packFileSuffix)
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
