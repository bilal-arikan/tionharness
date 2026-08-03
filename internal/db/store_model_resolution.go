package db

import (
	"context"
	"strings"
	"time"
)

// modelResolutionsFile is the singleton workspace document mapping a REQUESTED
// model id to the one a provider actually served.
const modelResolutionsFile = "model-resolutions.json"

// ModelResolution is one observed "asked for X, got Y" fact.
//
// It exists because claude-cli's model ids are aliases: an agent configured with
// "opus" — or with the empty session default — cannot say WHICH Opus answered.
// The concrete id is only knowable from a completed turn, so it is observed and
// remembered rather than hardcoded. A static alias→model table would rot every
// time Anthropic ships a model, which is exactly the failure mode
// ContextWindowFor avoids by staying family-based.
type ModelResolution struct {
	Provider  string `json:"provider"`
	Requested string `json:"requested"` // "" = the provider's own session default
	Resolved  string `json:"resolved"`  // concrete id, e.g. "claude-opus-5"
	// SeenAt is when this resolution FIRST appeared, not when it was last
	// confirmed: re-confirming writes nothing (see NoteModelResolution), so the
	// timestamp answers "since when has this alias meant this model".
	SeenAt time.Time `json:"seenAt"`
}

func modelResolutionKey(provider, requested string) string {
	return provider + "|" + requested
}

// NoteModelResolution records that `provider` served `resolved` for `requested`.
//
// A no-op when nothing new is learned, so the turn hot path does not write a file
// per completion — after warm-up this touches disk only when the provider starts
// serving a different model behind the same alias.
func (d *DB) NoteModelResolution(ctx context.Context, provider, requested, resolved string) error {
	provider = strings.TrimSpace(provider)
	requested = strings.TrimSpace(requested)
	resolved = strings.TrimSpace(resolved)
	// Nothing to learn: no provider, no answer, or the provider simply echoed the
	// concrete id it was given (every native provider does — only aliases matter).
	if provider == "" || resolved == "" || resolved == requested {
		return nil
	}

	key := modelResolutionKey(provider, requested)
	d.mu.Lock()
	defer d.mu.Unlock()
	if cur, ok := d.modelResolutions[key]; ok && cur.Resolved == resolved {
		return nil
	}
	d.modelResolutions[key] = ModelResolution{
		Provider:  provider,
		Requested: requested,
		Resolved:  resolved,
		SeenAt:    time.Now(),
	}
	return atomicWriteJSON(d.dir(modelResolutionsFile), d.modelResolutions)
}

// ResolvedModelFor returns the concrete model last observed behind a requested
// id, or "" when this workspace has never completed a turn with it.
func (d *DB) ResolvedModelFor(provider, requested string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.modelResolutions[modelResolutionKey(provider, requested)].Resolved
}

// ModelResolutions returns a copy of every observed resolution, keyed
// "<provider>|<requested>".
func (d *DB) ModelResolutions(ctx context.Context) map[string]ModelResolution {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]ModelResolution, len(d.modelResolutions))
	for k, v := range d.modelResolutions {
		out[k] = v
	}
	return out
}

// loadModelResolutions reads the singleton document. An absent or unreadable file
// leaves the map empty: these are re-learned from the next turn, so a lost file
// costs one turn of "no version shown", never correctness.
func (d *DB) loadModelResolutions() error {
	var m map[string]ModelResolution
	if err := readJSONFile(d.dir(modelResolutionsFile), &m); err != nil {
		return nil
	}
	for k, v := range m {
		d.modelResolutions[k] = v
	}
	return nil
}
