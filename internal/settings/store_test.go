package settings

import (
	"testing"
)

// noopCipher is a pass-through cipher for tests (no real encryption needed).
type noopCipher struct{}

func (noopCipher) Encrypt(s string) (string, error) { return s, nil }
func (noopCipher) Decrypt(s string) (string, error) { return s, nil }

func ptrBool(b bool) *bool { return &b }
func ptrInt(i int) *int    { return &i }

// TestGatedToolFlagsRoundTrip verifies the new gated-capability settings persist
// through Apply and survive a reload, and that the delegation guards are clamped.
func TestGatedToolFlagsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, noopCipher{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Fresh store starts from defaults: capabilities off, guards at 3/8.
	cur := store.Get()
	if cur.EnableDelegation || cur.EnableShell {
		t.Fatalf("gated capabilities must default off, got %+v", cur)
	}
	if cur.DelegationMaxDepth != 3 || cur.DelegationMaxCalls != 8 {
		t.Fatalf("default guards want 3/8, got %d/%d", cur.DelegationMaxDepth, cur.DelegationMaxCalls)
	}

	// Enable delegation with out-of-range guards → clamped to [1,10] / [1,100].
	next, err := store.Apply(Patch{
		EnableDelegation:   ptrBool(true),
		EnableShell:        ptrBool(true),
		DelegationMaxDepth: ptrInt(99),
		DelegationMaxCalls: ptrInt(0),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !next.EnableDelegation || !next.EnableShell {
		t.Fatalf("enable flags did not stick: %+v", next)
	}
	if next.DelegationMaxDepth != 10 {
		t.Fatalf("depth want clamp to 10, got %d", next.DelegationMaxDepth)
	}
	if next.DelegationMaxCalls != 1 {
		t.Fatalf("calls want clamp to 1, got %d", next.DelegationMaxCalls)
	}

	// Reload from disk: persisted values come back intact.
	reopened, err := Open(dir, noopCipher{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.Get()
	if !got.EnableDelegation || got.DelegationMaxDepth != 10 || got.DelegationMaxCalls != 1 {
		t.Fatalf("reloaded settings lost values: %+v", got)
	}

	// DTO exposes the flags to the client.
	dto := got.ToDTO()
	if !dto.EnableDelegation || dto.DelegationMaxDepth != 10 {
		t.Fatalf("DTO missing delegation fields: %+v", dto)
	}
}

