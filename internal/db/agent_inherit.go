package db

import (
	"errors"
	"fmt"
	"sort"
)

// Agent inheritance.
//
// An agent may name a ParentID. Every INHERITABLE field whose key is absent
// from the child's Overrides is read from the parent's effective value, and the
// parent may itself inherit, so a chain resolves root-first. Resolution happens
// at READ time (GetAgent, ListAgents, FindAgentBySystemKey, the value returned
// by every mutator); the on-disk row keeps the child's raw values, which for a
// non-overridden field are only a cache of what the parent held when the field
// was last touched.
//
// Identity fields — ID, Name, System/SystemKey, Disabled, Locked, ParentID,
// Overrides, CreatedBy, Deleted, timestamps — are never inherited.

var (
	// ErrAgentLocked is returned when a write targets a built-in (Locked) agent.
	// Callers may map it to an HTTP conflict; the supported alternative is to
	// derive a child and override fields there.
	ErrAgentLocked = errors.New("built-in agent is locked; derive a copy to customise it")
	// ErrAgentParentNotFound is returned when ParentID names no live agent.
	ErrAgentParentNotFound = errors.New("parent agent not found")
	// ErrAgentParentCycle is returned when an agent would inherit from itself or
	// from one of its own descendants.
	ErrAgentParentCycle = errors.New("agent cannot inherit from itself or one of its descendants")
	// ErrSystemRoleTaken is returned when a second ENABLED customisation would be
	// bound to the same system role; at most one child may serve a SystemKey.
	ErrSystemRoleTaken = errors.New("another enabled agent already customises this system role")
)

// maxInheritanceDepth bounds a resolution walk so a corrupted store (a cycle
// written by hand) can never hang a read. Cycles are rejected on write.
const maxInheritanceDepth = 32

// inheritableField is one unit of inheritance. A unit may span several struct
// fields when they are only meaningful together (provider kind + instance; the
// tool-access trio) so a child can never end up with half of a setting.
type inheritableField struct {
	Key   string
	copy  func(dst *Agent, src *Agent)
	equal func(a, b *Agent) bool
}

var inheritableFields = []inheritableField{
	{"soul",
		func(d, s *Agent) { d.Soul = s.Soul },
		func(a, b *Agent) bool { return a.Soul == b.Soul }},
	{"identity",
		func(d, s *Agent) { d.Identity = s.Identity },
		func(a, b *Agent) bool { return a.Identity == b.Identity }},
	{"provider",
		func(d, s *Agent) { d.Provider = s.Provider; d.ProviderInstanceID = s.ProviderInstanceID },
		// The instance id is backfilled from the kind at read time, so the kind is
		// the comparable part; an empty kind is the legacy "claude-cli" default.
		func(a, b *Agent) bool { return providerKindOrDefault(a.Provider) == providerKindOrDefault(b.Provider) }},
	{"model",
		func(d, s *Agent) { d.Model = s.Model },
		func(a, b *Agent) bool { return a.Model == b.Model }},
	{"thinkingLevel",
		func(d, s *Agent) { d.ThinkingLevel = s.ThinkingLevel },
		func(a, b *Agent) bool { return a.ThinkingLevel == b.ThinkingLevel }},
	{"nativeWebSearch",
		func(d, s *Agent) {
			d.NativeWebSearch = nil
			if s.NativeWebSearch != nil {
				v := *s.NativeWebSearch
				d.NativeWebSearch = &v
			}
		},
		func(a, b *Agent) bool { return a.NativeWebSearchEnabled() == b.NativeWebSearchEnabled() }},
	{"permissionMode",
		func(d, s *Agent) { d.PermissionMode = s.PermissionMode },
		func(a, b *Agent) bool { return a.PermissionMode == b.PermissionMode }},
	{"inboundPolicy",
		func(d, s *Agent) { d.InboundPolicy = s.InboundPolicy },
		func(a, b *Agent) bool { return a.InboundPolicy == b.InboundPolicy }},
	{"avatar",
		func(d, s *Agent) { d.Avatar = s.Avatar },
		func(a, b *Agent) bool { return a.Avatar == b.Avatar }},
	{"color",
		func(d, s *Agent) { d.Color = s.Color },
		func(a, b *Agent) bool { return a.Color == b.Color }},
	{"tools",
		func(d, s *Agent) {
			d.MCPEnabled = s.MCPEnabled
			d.ToolOverrides = s.ToolOverrides
			d.BlockedTools = s.BlockedTools
		},
		func(a, b *Agent) bool {
			return a.MCPEnabled == b.MCPEnabled && jsonTextOrDefault(a.ToolOverrides, "{}") == jsonTextOrDefault(b.ToolOverrides, "{}")
		}},
	{"allowedTools",
		func(d, s *Agent) { d.AllowedTools = s.AllowedTools },
		func(a, b *Agent) bool {
			return jsonTextOrDefault(a.AllowedTools, "[]") == jsonTextOrDefault(b.AllowedTools, "[]")
		}},
	{"skills",
		func(d, s *Agent) { d.Skills = append([]string{}, s.Skills...) },
		func(a, b *Agent) bool { return stringSlicesEqual(a.Skills, b.Skills) }},
	{"coordinatorMode",
		func(d, s *Agent) { d.CoordinatorMode = s.CoordinatorMode },
		func(a, b *Agent) bool { return a.CoordinatorMode == b.CoordinatorMode }},
	{"coordinatorWorkflow",
		func(d, s *Agent) { d.CoordinatorWorkflow = s.CoordinatorWorkflow },
		func(a, b *Agent) bool { return a.CoordinatorWorkflow == b.CoordinatorWorkflow }},
	{"coordinatorPrompt",
		func(d, s *Agent) { d.CoordinatorPrompt = s.CoordinatorPrompt },
		func(a, b *Agent) bool { return a.CoordinatorPrompt == b.CoordinatorPrompt }},
}

func providerKindOrDefault(kind string) string {
	if kind == "" {
		return "claude-cli"
	}
	return kind
}

func jsonTextOrDefault(text, def string) string {
	if text == "" {
		return def
	}
	return text
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// InheritableFieldKeys returns every override key, in display order.
func InheritableFieldKeys() []string {
	out := make([]string, 0, len(inheritableFields))
	for _, f := range inheritableFields {
		out = append(out, f.Key)
	}
	return out
}

// IsInheritableFieldKey reports whether key names an inheritable unit.
func IsInheritableFieldKey(key string) bool {
	for _, f := range inheritableFields {
		if f.Key == key {
			return true
		}
	}
	return false
}

// ValidateOverrideKeys rejects an unknown key so a typo in a client patch cannot
// be persisted as a silently ignored override.
func ValidateOverrideKeys(keys []string) error {
	for _, k := range keys {
		if !IsInheritableFieldKey(k) {
			return fmt.Errorf("unknown override field %q", k)
		}
	}
	return nil
}

// normalizeOverrides de-duplicates and orders keys canonically (display order)
// so the on-disk list is stable regardless of edit order.
func normalizeOverrides(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	set := overrideSet(keys)
	out := make([]string, 0, len(set))
	for _, f := range inheritableFields {
		if set[f.Key] {
			out = append(out, f.Key)
		}
	}
	return out
}

func overrideSet(keys []string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}

func withOverride(keys []string, key string) []string {
	if overrideSet(keys)[key] {
		return keys
	}
	return normalizeOverrides(append(append([]string{}, keys...), key))
}

func withoutOverride(keys []string, key string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != key {
			out = append(out, k)
		}
	}
	return normalizeOverrides(out)
}

// diffInheritable returns the keys whose values differ between a and b.
func diffInheritable(a, b Agent) []string {
	var out []string
	for _, f := range inheritableFields {
		if !f.equal(&a, &b) {
			out = append(out, f.Key)
		}
	}
	return out
}

// copyInheritable copies every inheritable unit from src into dst.
func copyInheritable(dst *Agent, src Agent) {
	for _, f := range inheritableFields {
		f.copy(dst, &src)
	}
}

// applyInheritance lays the child's OWN overrides over the parent's effective
// values. Identity fields come from the child untouched.
func applyInheritance(parent Agent, child Agent) Agent {
	out := child
	set := overrideSet(child.Overrides)
	for _, f := range inheritableFields {
		if !set[f.Key] {
			f.copy(&out, &parent)
		}
	}
	return out.backfillProviderInstance()
}

// resolveAgentLocked returns the EFFECTIVE agent: the parent chain folded
// root-first with each layer's overrides applied. Caller holds d.mu (either
// mode). A missing parent (or a cycle in a hand-edited store) ends the walk at
// the last row that could be found, so a read never fails — the child simply
// resolves against what exists.
func (d *DB) resolveAgentLocked(a Agent) Agent {
	if a.ParentID == "" {
		return a.backfillProviderInstance()
	}
	chain := []Agent{a}
	seen := map[string]bool{a.ID: true}
	cur := a
	for cur.ParentID != "" && len(chain) < maxInheritanceDepth {
		p, ok := d.agents[cur.ParentID]
		if !ok || seen[p.ID] {
			break
		}
		seen[p.ID] = true
		chain = append(chain, p)
		cur = p
	}
	eff := chain[len(chain)-1].backfillProviderInstance()
	for i := len(chain) - 2; i >= 0; i-- {
		eff = applyInheritance(eff, chain[i])
	}
	return eff
}

// materialize turns a resolved agent into an equivalent ROOT agent: every
// inheritable value is now owned, no parent, no overrides.
func materialize(resolved Agent) Agent {
	resolved.ParentID = ""
	resolved.Overrides = nil
	return resolved
}

// wouldCycleLocked reports whether making parentID the parent of childID would
// create a loop. Caller holds d.mu.
func (d *DB) wouldCycleLocked(childID, parentID string) bool {
	if childID == parentID {
		return true
	}
	cur := parentID
	for i := 0; i < maxInheritanceDepth && cur != ""; i++ {
		if cur == childID {
			return true
		}
		p, ok := d.agents[cur]
		if !ok {
			return false
		}
		cur = p.ParentID
	}
	return false
}

// checkParentLocked validates a prospective ParentID for childID (which may be
// "" for an agent that does not exist yet). Caller holds d.mu.
func (d *DB) checkParentLocked(childID, parentID string) error {
	if parentID == "" {
		return nil
	}
	p, ok := d.agents[parentID]
	if !ok || p.Deleted {
		return ErrAgentParentNotFound
	}
	if childID != "" && d.wouldCycleLocked(childID, parentID) {
		return ErrAgentParentCycle
	}
	return nil
}

// childrenLocked returns the live direct children of id, oldest first. Caller
// holds d.mu.
func (d *DB) childrenLocked(id string) []Agent {
	var out []Agent
	for _, a := range d.agents {
		if a.ParentID == id && !a.Deleted {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// reparentChildrenLocked runs when `removed` leaves the tree (deleted): each
// direct child is re-pointed at the removed agent's own parent while keeping
// its EFFECTIVE values byte-identical — any value it used to inherit from the
// removed layer that the grandparent does not provide becomes an override on
// the child. With no grandparent the child becomes a root. Caller holds d.mu
// for writing.
func (d *DB) reparentChildrenLocked(removed Agent) error {
	grandparentID := removed.ParentID
	for _, child := range d.childrenLocked(removed.ID) {
		effective := d.resolveAgentLocked(child)
		var next Agent
		if grandparentID == "" {
			next = materialize(effective)
		} else {
			gp, ok := d.agents[grandparentID]
			if !ok {
				next = materialize(effective)
			} else {
				gpEffective := d.resolveAgentLocked(gp)
				next = child
				next.ParentID = grandparentID
				copyInheritable(&next, effective) // raw cache = what it resolved to
				set := overrideSet(child.Overrides)
				for _, key := range diffInheritable(gpEffective, effective) {
					set[key] = true
				}
				keys := make([]string, 0, len(set))
				for k := range set {
					keys = append(keys, k)
				}
				next.Overrides = normalizeOverrides(keys)
			}
		}
		next.UpdatedAt = now()
		if err := d.persistAgentLocked(next); err != nil {
			return err
		}
	}
	return nil
}

// systemRoleTakenLocked reports whether an ENABLED, non-locked agent other than
// excludeID already serves the system role `key`. Caller holds d.mu.
func (d *DB) systemRoleTakenLocked(key, excludeID string) bool {
	if key == "" {
		return false
	}
	for _, a := range d.agents {
		if a.ID == excludeID || a.Deleted || a.Locked || a.Disabled {
			continue
		}
		if a.System && a.SystemKey == key {
			return true
		}
	}
	return false
}

// AgentChildren returns the live direct children of id (resolved), oldest first.
func (d *DB) AgentChildren(id string) []Agent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	kids := d.childrenLocked(id)
	out := make([]Agent, 0, len(kids))
	for _, k := range kids {
		out = append(out, d.resolveAgentLocked(k))
	}
	return out
}
