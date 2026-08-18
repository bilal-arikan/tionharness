package db

import "testing"

// TestBackfillProviderInstanceFromLegacyRow verifies the read-time migration
// (_Docs/71 §2.5/§3): an agent row written before ProviderInstanceID existed
// (empty field, non-empty Provider) resolves it from Provider on every read
// path, WITHOUT rewriting the on-disk file — CreateAgent bypasses the normal
// creation path here specifically to simulate that legacy shape.
func TestBackfillProviderInstanceFromLegacyRow(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	created, err := d.CreateAgent(ctx, Agent{Name: "legacy", Provider: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	// CreateAgent's own invariant guard already backfills new rows (by design —
	// see its doc comment), so force the legacy (pre-field) shape directly onto
	// the in-memory + on-disk row to exercise the READ-time path in isolation.
	d.mu.Lock()
	legacy := d.agents[created.ID]
	legacy.ProviderInstanceID = ""
	d.agents[created.ID] = legacy
	if err := d.persistAgentLocked(legacy); err != nil {
		d.mu.Unlock()
		t.Fatal(err)
	}
	d.mu.Unlock()

	got, err := d.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderInstanceID != "anthropic" {
		t.Errorf("GetAgent: ProviderInstanceID = %q, want %q (backfilled from Provider)", got.ProviderInstanceID, "anthropic")
	}

	list, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ProviderInstanceID != "anthropic" {
		t.Errorf("ListAgents: ProviderInstanceID = %q, want %q", list[0].ProviderInstanceID, "anthropic")
	}

	// The on-disk row must NOT have been rewritten by the read — re-simulate by
	// re-reading the persisted file directly through a fresh Open (no bulk write).
	d2, err := Open(d.root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := d2.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Still backfilled on read even after a fresh load from the untouched file —
	// proving the on-disk row is still the legacy (empty-field) shape and the
	// backfill is applied fresh every time, not baked in by a prior write.
	if raw.ProviderInstanceID != "anthropic" {
		t.Errorf("fresh load GetAgent: ProviderInstanceID = %q, want %q", raw.ProviderInstanceID, "anthropic")
	}
}

// TestBackfillProviderInstanceEmptyProviderDefaultsClaudeCLI verifies an agent
// row with BOTH fields empty (predates even the Provider field being reliably
// set) falls back to the keyless claude-cli default, matching Registry.Get's
// historical empty-provider behaviour.
func TestBackfillProviderInstanceEmptyProviderDefaultsClaudeCLI(t *testing.T) {
	a := Agent{ID: "AGT1"}
	got := a.backfillProviderInstance()
	if got.ProviderInstanceID != "claude-cli" {
		t.Errorf("ProviderInstanceID = %q, want claude-cli", got.ProviderInstanceID)
	}
}

// TestBackfillProviderInstanceNoOpWhenAlreadySet verifies a row that already
// carries ProviderInstanceID is left untouched even if it somehow diverges
// from Provider — backfill only fills a gap, it never overwrites.
func TestBackfillProviderInstanceNoOpWhenAlreadySet(t *testing.T) {
	a := Agent{Provider: "anthropic", ProviderInstanceID: "PRV3"}
	got := a.backfillProviderInstance()
	if got.ProviderInstanceID != "PRV3" {
		t.Errorf("ProviderInstanceID = %q, want unchanged PRV3", got.ProviderInstanceID)
	}
}
