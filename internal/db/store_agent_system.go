package db

import (
	"context"
	"errors"
	"strings"
)

// ErrSystemAgentDelete explains the supported alternative to deleting a
// built-in agent. Callers may use errors.Is to map it to an API conflict.
var ErrSystemAgentDelete = errors.New("system agent cannot be deleted; disable it instead")

// SystemAgentDefinition is the persistence-layer representation of a built-in
// agent default. Callers own the registry and pass definitions into the store,
// keeping db independent of the agent package.
type SystemAgentDefinition struct {
	SystemKey      string
	Name           string
	Description    string
	SystemPrompt   string
	SuggestedModel string
	AllowedTools   string
	Disabled       bool

	// Avatar (single emoji/glyph) and Color (hex) are the canonical visual
	// identity of a built-in agent, so the same system agent looks the same in
	// every workspace. Unlike Name/Soul/Model they are only SEEDED, never
	// re-imposed on an existing row that already carries a value — see
	// EnsureSystemAgents.
	Avatar string
	Color  string
}

// FindAgentBySystemKey returns the live system agent for key, if present.
func (d *DB) FindAgentBySystemKey(key string) (*Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, a := range d.agents {
		if a.System && a.SystemKey == key && !a.Deleted {
			found := a.backfillProviderInstance()
			return &found, true
		}
	}
	return nil, false
}

// EnsureSystemAgents creates missing built-in agents and leaves existing rows
// untouched so user customisations survive repeated seeding.
func (d *DB) EnsureSystemAgents(ctx context.Context, defs ...SystemAgentDefinition) error {
	if legacy, ok := d.FindAgentBySystemKey("compactor"); ok {
		if _, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) {
			a.SystemKey = "overview-summarizer"
		}); err != nil {
			return err
		}
	}
	for _, def := range defs {
		if strings.HasPrefix(def.SystemKey, "subagent-") {
			legacyName := "worker:" + strings.TrimPrefix(def.SystemKey, "subagent-")
			legacy, legacyFound := d.findLiveAgentByName(legacyName)
			_, systemFound := d.FindAgentBySystemKey(def.SystemKey)
			if legacyFound {
				if systemFound {
					if _, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) { a.Disabled = true }); err != nil {
						return err
					}
				} else {
					if _, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) {
						a.Name = def.Name
						a.Identity = def.Description
						a.AllowedTools = def.AllowedTools
						a.System = true
						a.SystemKey = def.SystemKey
					}); err != nil {
						return err
					}
				}
			}
		}
		if existing, ok := d.FindAgentBySystemKey(def.SystemKey); ok {
			// Seed-only backfill: a row written before the definitions carried a
			// visual identity has an empty Avatar/Color and gets the canonical one,
			// which is what makes existing workspaces converge. A value the user
			// picked is left alone — this runs on every boot, so overwriting here
			// would make the field permanently uneditable.
			if (existing.Avatar == "" && def.Avatar != "") || (existing.Color == "" && def.Color != "") {
				if _, err := d.mutateAgentLocked(existing.ID, func(a *Agent) {
					if a.Avatar == "" {
						a.Avatar = def.Avatar
					}
					if a.Color == "" {
						a.Color = def.Color
					}
				}); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := d.CreateAgent(ctx, Agent{
			Name:         def.Name,
			Soul:         def.SystemPrompt,
			Identity:     def.Description,
			Model:        def.SuggestedModel,
			AllowedTools: def.AllowedTools,
			Avatar:       def.Avatar,
			Color:        def.Color,
			System:       true,
			SystemKey:    def.SystemKey,
			Disabled:     def.Disabled,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) findLiveAgentByName(name string) (Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, a := range d.agents {
		if !a.Deleted && !a.System && a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}
