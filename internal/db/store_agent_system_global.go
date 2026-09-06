package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// systemAgentOverridesFile is the app-level document holding the user's edits to
// the BUILT-IN system agents, keyed by SystemKey.
const systemAgentOverridesFile = "system-agents.json"

// GlobalSystemAgentOverrides is the app-level customisation layer for built-in
// system agents.
//
// A built-in row is owned by the compiled registry and re-imposed on every boot
// (see EnsureSystemAgents), which is what keeps it identical in every workspace.
// That left "customise" no place to write, so it derived a per-workspace copy —
// one extra roster row per role, per workspace, and a change that did not apply
// anywhere else.
//
// This layer is the place to write. It stores, per SystemKey, the inheritable
// fields the user changed; EnsureSystemAgents applies them ON TOP of the
// canonical definition, so the edited built-in IS what every workspace seeds and
// re-imposes. Editing the built-in therefore creates no copy and reaches the
// whole installation, while a field the user never touched still follows the
// compiled registry across upgrades.
//
// One instance is shared by every workspace DB, so it carries its own lock and
// its own atomic write — deliberately NOT part of DB's lock.
type GlobalSystemAgentOverrides struct {
	path string

	mu sync.RWMutex
	m  map[string]SystemAgentOverride
}

// SystemAgentOverride is one role's stored customisation: the edited values plus
// the keys that are actually pinned. Only a key listed in Fields is applied, so
// an untouched field keeps following the compiled definition.
type SystemAgentOverride struct {
	// Fields lists the inheritable field keys the user pinned (InheritableFieldKeys).
	Fields []string `json:"fields"`
	// Values carries the edited agent values. Only the units named by Fields are
	// read from it; every other field is a stale cache and must be ignored.
	Values Agent `json:"values"`
	// Name is the built-in's display name when the user renamed it. Empty keeps
	// the compiled name. Name is not an inheritable field, so it is stored apart.
	Name string `json:"name,omitempty"`
	// UpdatedAt is when the customisation was last written.
	UpdatedAt int64 `json:"updatedAt,omitempty"`
}

// OpenGlobalSystemAgentOverrides loads <dataDir>/system-agents.json.
//
// An absent file is normal (nothing customised yet) and yields an empty layer. A
// file that exists but cannot be read or parsed is an error: it means real
// customisations on disk would be silently dropped back to the built-in
// defaults, and the caller decides whether to run without the layer.
func OpenGlobalSystemAgentOverrides(dataDir string) (*GlobalSystemAgentOverrides, error) {
	g := &GlobalSystemAgentOverrides{
		path: filepath.Join(dataDir, systemAgentOverridesFile),
		m:    map[string]SystemAgentOverride{},
	}
	var m map[string]SystemAgentOverride
	if err := readJSONFile(g.path, &m); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return g, nil
		}
		return nil, fmt.Errorf("read %s: %w", g.path, err)
	}
	for k, v := range m {
		v.Fields = normalizeOverrides(v.Fields)
		g.m[k] = v
	}
	return g, nil
}

// Path returns the absolute path of the app-global document on disk.
func (g *GlobalSystemAgentOverrides) Path() string { return g.path }

// Get returns the stored customisation for a role, if any.
func (g *GlobalSystemAgentOverrides) Get(systemKey string) (SystemAgentOverride, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ov, ok := g.m[systemKey]
	return ov, ok
}

// Keys returns every customised role, sorted, so callers can iterate stably.
func (g *GlobalSystemAgentOverrides) Keys() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]string, 0, len(g.m))
	for k := range g.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Put stores a role's customisation: `values` is the edited agent and `fields`
// names the units to pin. An empty field list with no name clears the role back
// to the compiled definition.
func (g *GlobalSystemAgentOverrides) Put(systemKey string, fields []string, values Agent, name string) error {
	fields = normalizeOverrides(fields)
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(fields) == 0 && name == "" {
		if _, ok := g.m[systemKey]; !ok {
			return nil
		}
		delete(g.m, systemKey)
		return atomicWriteJSON(g.path, g.m)
	}
	// Keep only the pinned units, so a rolled-back field cannot be resurrected
	// later by a stale cached value sitting in the document.
	var stored Agent
	for _, f := range inheritableFields {
		if overrideSet(fields)[f.Key] {
			f.copy(&stored, &values)
		}
	}
	g.m[systemKey] = SystemAgentOverride{Fields: fields, Values: stored, Name: name, UpdatedAt: now()}
	return atomicWriteJSON(g.path, g.m)
}

// Clear drops a role's customisation, so it follows the compiled definition
// again. Reports whether anything was stored.
func (g *GlobalSystemAgentOverrides) Clear(systemKey string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.m[systemKey]; !ok {
		return false, nil
	}
	delete(g.m, systemKey)
	return true, atomicWriteJSON(g.path, g.m)
}

// apply lays a role's customisation over the canonical definition, returning the
// row the built-in should carry. Only pinned units are taken from the override;
// everything else stays canonical, so an untouched field keeps tracking the
// compiled registry across upgrades.
func (ov SystemAgentOverride) apply(canonical Agent) Agent {
	out := canonical
	set := overrideSet(ov.Fields)
	values := ov.Values
	for _, f := range inheritableFields {
		if set[f.Key] {
			f.copy(&out, &values)
		}
	}
	if ov.Name != "" {
		out.Name = ov.Name
	}
	out.Overrides = normalizeOverrides(ov.Fields)
	return out
}

// SetGlobalSystemAgentOverrides attaches the shared app-level layer to this
// workspace DB. Wired by the workspace manager right before EnsureSystemAgents;
// a DB with no layer attached seeds the plain compiled defaults.
func (d *DB) SetGlobalSystemAgentOverrides(g *GlobalSystemAgentOverrides) {
	d.globalSysAgentsMu.Lock()
	defer d.globalSysAgentsMu.Unlock()
	d.globalSysAgents = g
}

// GlobalSystemAgentOverrideLayer returns the attached layer, or nil.
func (d *DB) GlobalSystemAgentOverrideLayer() *GlobalSystemAgentOverrides {
	d.globalSysAgentsMu.RLock()
	defer d.globalSysAgentsMu.RUnlock()
	return d.globalSysAgents
}

// canonicalFor renders the row a built-in should carry for this definition: the
// compiled values with the app-global customisation, if any, laid over them.
func (d *DB) canonicalFor(def SystemAgentDefinition) Agent {
	canonical := def.canonicalAgent()
	g := d.GlobalSystemAgentOverrideLayer()
	if g == nil {
		return canonical
	}
	if ov, ok := g.Get(def.SystemKey); ok {
		return ov.apply(canonical)
	}
	return canonical
}

// rememberSystemDefinitions records the compiled registry passed to
// EnsureSystemAgents, so a later edit can resolve the definition behind a role.
func (d *DB) rememberSystemDefinitions(defs []SystemAgentDefinition) {
	d.systemDefsMu.Lock()
	defer d.systemDefsMu.Unlock()
	d.systemDefs = make(map[string]SystemAgentDefinition, len(defs))
	for _, def := range defs {
		d.systemDefs[def.SystemKey] = def
	}
}

// systemDefinition returns the compiled definition behind a system role.
func (d *DB) systemDefinition(systemKey string) (SystemAgentDefinition, bool) {
	d.systemDefsMu.RLock()
	defer d.systemDefsMu.RUnlock()
	def, ok := d.systemDefs[systemKey]
	return def, ok
}

// compiledCanonicalFor renders a role's row from the compiled registry ALONE,
// with no customisation laid over it — the baseline an edit is compared against.
func (d *DB) compiledCanonicalFor(systemKey string) Agent {
	def, ok := d.systemDefinition(systemKey)
	if !ok {
		return Agent{}
	}
	return def.canonicalAgent()
}
