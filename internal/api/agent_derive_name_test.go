package api

import "testing"

// TestCustomizationName: the default name of a role-bound customisation carries
// the workspace it belongs to, and degrades to the generic marker when the
// workspace record has no name.
func TestCustomizationName(t *testing.T) {
	if got := customizationName("Titler", "TionHarnessRepo"); got != "Titler (TionHarnessRepo)" {
		t.Fatalf("named workspace: got %q", got)
	}
	if got := customizationName("Titler", "  "); got != "Titler (özel)" {
		t.Fatalf("blank workspace name: got %q", got)
	}
}
