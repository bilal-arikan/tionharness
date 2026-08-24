package providers

import (
	"strings"
	"testing"
)

// TestPermissionModeArgs locks the mapping from TionHarness permission modes to the
// claude CLI's permission flags. Headless mode must always carry an explicit
// mode, otherwise the CLI refuses Edit/Write/Bash.
func TestPermissionModeArgs(t *testing.T) {
	cases := map[string]string{
		"read-only": "--permission-mode plan",
		"ask":       "--permission-mode acceptEdits",
		"auto":      "--dangerously-skip-permissions",
		"":          "--dangerously-skip-permissions", // empty defaults to auto
		"bogus":     "--dangerously-skip-permissions", // unknown is treated as auto
	}
	for mode, want := range cases {
		got := strings.Join(permissionModeArgs(mode), " ")
		if got != want {
			t.Errorf("permissionModeArgs(%q) = %q, want %q", mode, got, want)
		}
	}
}
