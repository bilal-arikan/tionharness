package settings

import "testing"

func strptr(s string) *string { return &s }

func TestValidateRejectsBadEnums(t *testing.T) {
	cases := []struct {
		name  string
		patch Patch
		ok    bool
	}{
		{"good theme", Patch{Theme: strptr("light")}, true},
		{"bad theme", Patch{Theme: strptr("neon")}, false},
		{"bad language", Patch{Language: strptr("de")}, false},
		{"bad provider", Patch{DefaultProvider: strptr("openai")}, false},
		{"bad permission", Patch{DefaultPermissionMode: strptr("yolo")}, false},
		{"empty permission ok", Patch{DefaultPermissionMode: strptr("")}, true},
		{"bad accent", Patch{Accent: strptr("purple")}, false},
		{"good accent", Patch{Accent: strptr("#8b5cf6")}, true},
		{"empty patch", Patch{}, true},
	}
	for _, c := range cases {
		err := Validate(c.patch)
		if (err == nil) != c.ok {
			t.Errorf("%s: Validate err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// TestApplyRejectsInvalidPatch verifies a bad value never reaches the store.
func TestApplyRejectsInvalidPatch(t *testing.T) {
	st, err := Open(t.TempDir(), noopCipher{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	before := st.Get().Theme
	if _, err := st.Apply(Patch{Theme: strptr("neon")}); err == nil {
		t.Fatal("expected Apply to reject invalid theme")
	}
	if st.Get().Theme != before {
		t.Fatalf("invalid patch mutated state: theme=%q", st.Get().Theme)
	}
}

// TestNormalizeCoercesBadFile verifies a hand-edited bad value is coerced (not a
// crash) on load — accent and permission mode fall back to safe defaults.
func TestNormalizeCoercesBadValues(t *testing.T) {
	v := normalize(Settings{Accent: "not-a-color", DefaultPermissionMode: "bogus", Language: "xx"})
	if v.Accent != "#8b5cf6" {
		t.Errorf("accent not coerced: %q", v.Accent)
	}
	if v.DefaultPermissionMode != "auto" {
		t.Errorf("permission mode not coerced: %q", v.DefaultPermissionMode)
	}
	if v.Language != "tr" {
		t.Errorf("language not coerced: %q", v.Language)
	}
}

// TestApplyClampsNumerics verifies out-of-range numbers are clamped, not rejected.
func TestApplyClampsNumerics(t *testing.T) {
	st, err := Open(t.TempDir(), noopCipher{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	huge := 9_999_999
	out, err := st.Apply(Patch{MaxContextTokens: &huge, DelegationMaxCalls: &huge})
	if err != nil {
		t.Fatalf("Apply clamped numerics should not error: %v", err)
	}
	if out.DelegationMaxCalls != 100 {
		t.Errorf("delegationMaxCalls not clamped: %d", out.DelegationMaxCalls)
	}
}
