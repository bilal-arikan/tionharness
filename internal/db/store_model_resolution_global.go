package db

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GlobalModelResolutions is the app-level twin of a workspace's model-resolution
// map: the same "asked for X, got Y" facts, kept once per installation instead
// of once per workspace.
//
// It exists because the facts are a property of the MACHINE (which claude-cli is
// installed, what Anthropic currently serves behind "opus"), not of a workspace.
// Without it every new workspace starts blind and shows "Varsayılan" for the
// empty claude-cli model id until it has completed a turn of its own, even
// though another workspace on the same machine already learned the answer.
//
// One instance is shared by every workspace DB, so it carries its own lock and
// its own atomic write — it is deliberately NOT part of DB's lock.
type GlobalModelResolutions struct {
	path string

	mu sync.RWMutex
	m  map[string]ModelResolution
}

// OpenGlobalModelResolutions loads <dataDir>/model-resolutions.json.
//
// An absent file is normal (first boot) and yields an empty store. A file that
// exists but cannot be read or parsed is an error: it means real state on disk
// is being ignored, and the caller decides whether to run without the layer.
func OpenGlobalModelResolutions(dataDir string) (*GlobalModelResolutions, error) {
	g := &GlobalModelResolutions{
		path: filepath.Join(dataDir, modelResolutionsFile),
		m:    map[string]ModelResolution{},
	}
	var m map[string]ModelResolution
	if err := readJSONFile(g.path, &m); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return g, nil
		}
		return nil, fmt.Errorf("read %s: %w", g.path, err)
	}
	for k, v := range m {
		g.m[k] = v
	}
	return g, nil
}

// Path returns the absolute path of the app-global document on disk.
func (g *GlobalModelResolutions) Path() string { return g.path }

// Note records a resolution, mirroring NoteModelResolution's semantics: nothing
// to learn is a no-op, and a re-confirmation writes no file.
func (g *GlobalModelResolutions) Note(provider, requested, resolved string) error {
	provider = strings.TrimSpace(provider)
	requested = strings.TrimSpace(requested)
	resolved = strings.TrimSpace(resolved)
	if provider == "" || resolved == "" || resolved == requested {
		return nil
	}

	key := modelResolutionKey(provider, requested)
	g.mu.Lock()
	defer g.mu.Unlock()
	if cur, ok := g.m[key]; ok && cur.Resolved == resolved {
		return nil
	}
	g.m[key] = ModelResolution{
		Provider:  provider,
		Requested: requested,
		Resolved:  resolved,
		SeenAt:    time.Now(),
	}
	return atomicWriteJSON(g.path, g.m)
}

// Resolved returns the concrete model any workspace on this machine last
// observed behind a requested id, or "" when none ever has.
func (g *GlobalModelResolutions) Resolved(provider, requested string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.m[modelResolutionKey(provider, requested)].Resolved
}

// SetGlobalModelResolutions attaches the shared app-level store to this
// workspace DB. Wired by the workspace manager right after Open; a DB with no
// store attached simply keeps the workspace-local behaviour.
func (d *DB) SetGlobalModelResolutions(g *GlobalModelResolutions) {
	d.globalModelResMu.Lock()
	defer d.globalModelResMu.Unlock()
	d.globalModelRes = g
}

func (d *DB) globalModelResolutions() *GlobalModelResolutions {
	d.globalModelResMu.RLock()
	defer d.globalModelResMu.RUnlock()
	return d.globalModelRes
}

// GlobalResolvedModelFor is the app-global fallback for ResolvedModelFor: it
// answers from what ANY workspace on this machine has observed. Read-side only
// — ResolvedModelFor stays strictly workspace-local so a workspace never claims
// to have seen a model it has not.
func (d *DB) GlobalResolvedModelFor(provider, requested string) string {
	g := d.globalModelResolutions()
	if g == nil {
		return ""
	}
	return g.Resolved(provider, requested)
}

// noteGlobalModelResolution mirrors a freshly learned resolution into the
// app-global store. Best-effort: a failure here must not fail the turn that
// learned the fact, but it is logged rather than dropped.
func (d *DB) noteGlobalModelResolution(provider, requested, resolved string) {
	g := d.globalModelResolutions()
	if g == nil {
		return
	}
	if err := g.Note(provider, requested, resolved); err != nil {
		slog.Warn("app-global model resolution write failed",
			"component", "db", "path", g.Path(),
			"provider", provider, "requested", requested, "resolved", resolved,
			"error", err)
	}
}
