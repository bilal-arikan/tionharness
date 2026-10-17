package settings

import "testing"

// TestPrefixLeversDefaultOnAndPatchable pins the two prompt-size levers added on
// 2026-09-03: both default ON (a fresh install gets the cheap path), both round-
// trip through Patch/Public, and an explicit false is honoured.
func TestPrefixLeversDefaultOnAndPatchable(t *testing.T) {
	def := Default()
	if !def.ClaudeCLIToolAllowlist {
		t.Fatalf("defaults: allowlist=%v, want true", def.ClaudeCLIToolAllowlist)
	}
	store, err := Open(t.TempDir(), noopCipher{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if cur := store.Get(); !cur.ClaudeCLIToolAllowlist {
		t.Fatalf("fresh store must carry both levers on, got %+v", cur)
	}
	if _, err := store.Apply(Patch{ClaudeCLIToolAllowlist: new(false)}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	cur := store.Get()
	if cur.ClaudeCLIToolAllowlist {
		t.Fatalf("patch false not applied: allowlist=%v", cur.ClaudeCLIToolAllowlist)
	}
}
