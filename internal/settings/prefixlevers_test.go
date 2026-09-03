package settings

import "testing"

// TestPrefixLeversDefaultOnAndPatchable pins the two prompt-size levers added on
// 2026-09-03: both default ON (a fresh install gets the cheap path), both round-
// trip through Patch/Public, and an explicit false is honoured.
func TestPrefixLeversDefaultOnAndPatchable(t *testing.T) {
	def := Default()
	if !def.ClaudeCLIToolAllowlist || !def.AuxNativeRouting {
		t.Fatalf("defaults: allowlist=%v auxNative=%v, want both true", def.ClaudeCLIToolAllowlist, def.AuxNativeRouting)
	}
	store, err := Open(t.TempDir(), noopCipher{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if cur := store.Get(); !cur.ClaudeCLIToolAllowlist || !cur.AuxNativeRouting {
		t.Fatalf("fresh store must carry both levers on, got %+v", cur)
	}
	if _, err := store.Apply(Patch{ClaudeCLIToolAllowlist: ptrBool(false), AuxNativeRouting: ptrBool(false)}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	cur := store.Get()
	if cur.ClaudeCLIToolAllowlist || cur.AuxNativeRouting {
		t.Fatalf("patch false not applied: allowlist=%v auxNative=%v", cur.ClaudeCLIToolAllowlist, cur.AuxNativeRouting)
	}
}
