package db

import (
	"context"
	"errors"
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
		if _, ok := d.FindAgentBySystemKey(def.SystemKey); ok {
			continue
		}
		if _, err := d.CreateAgent(ctx, Agent{
			Name:         def.Name,
			Soul:         def.SystemPrompt,
			Identity:     def.Description,
			Model:        def.SuggestedModel,
			AllowedTools: def.AllowedTools,
			System:       true,
			SystemKey:    def.SystemKey,
			Disabled:     def.Disabled,
		}); err != nil {
			return err
		}
	}
	return nil
}
