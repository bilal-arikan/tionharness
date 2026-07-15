package api

import "testing"

// toMCPRow (shared by create + edit) must accept the known scopes and reject an
// unknown one loudly rather than silently coercing it to "shared".
func TestToMCPRowScopeValidation(t *testing.T) {
	base := func(scope string) createMCPReq {
		return createMCPReq{Name: "s", Transport: "stdio", Command: "x", Scope: scope}
	}

	for _, ok := range []string{"", "shared", "scoped"} {
		row, err := base(ok).toMCPRow()
		if err != nil {
			t.Errorf("scope %q should be valid, got %v", ok, err)
			continue
		}
		if row.Scope != ok {
			t.Errorf("scope %q not carried to row, got %q", ok, row.Scope)
		}
	}

	if _, err := base("session").toMCPRow(); err == nil {
		t.Error("unknown scope should be rejected, not silently accepted")
	}
}

// The scope check must not mask the existing transport validation.
func TestToMCPRowStillValidatesTransport(t *testing.T) {
	if _, err := (createMCPReq{Name: "s", Transport: "stdio", Scope: "scoped"}).toMCPRow(); err == nil {
		t.Error("stdio without a command should fail regardless of scope")
	}
}
