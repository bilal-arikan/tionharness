package workspace

import "testing"

// TestDefaultWSSettings pins the always-on defaults (cross-session awareness is
// no longer configurable; codebase-memory defaults on).
func TestDefaultWSSettings(t *testing.T) {
	d := defaultWSSettings()
	if !d.CodebaseMemoryEnabled {
		t.Error("codebase-memory capability should default on")
	}
	if d.AutoCaptureArtifacts {
		t.Error("auto-capture artifacts should default OFF (deliberate create_artifact only)")
	}
}
