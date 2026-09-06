package api

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/prompts"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// The Promptlar screen lists only prompts with no owning system agent. A
// system-owned prompt is its agent's Soul, edited on Settings ▸ "Sistem
// ajanları"; listing it here too offered a second editor whose value was
// ignored unless system agent resolution failed.
func TestBuildWSConfigDTOOmitsSystemOwnedPrompts(t *testing.T) {
	wsp := &workspace.Workspace{
		Meta:    workspace.Meta{ID: "WS1", Name: "Atölye"},
		DataDir: t.TempDir(),
	}
	dto := buildWSConfigDTO(wsp)

	listed := map[string]bool{}
	for _, k := range dto.PromptKeys {
		listed[k] = true
	}

	var wantAny bool
	for _, spec := range prompts.Specs() {
		if spec.OwnedBySystemKey != "" {
			if listed[spec.Key] {
				t.Errorf("system-owned prompt %q (owned by %q) must not be listed", spec.Key, spec.OwnedBySystemKey)
			}
			if _, ok := dto.PromptMeta[spec.Key]; ok {
				t.Errorf("system-owned prompt %q must carry no meta", spec.Key)
			}
			if _, ok := dto.Prompts[spec.Key]; ok {
				t.Errorf("system-owned prompt %q must carry no content", spec.Key)
			}
			continue
		}
		wantAny = true
		if !listed[spec.Key] {
			t.Errorf("free-standing prompt %q must still be listed", spec.Key)
		}
		if _, ok := dto.Defaults[spec.Key]; !ok {
			t.Errorf("free-standing prompt %q must carry its embedded default", spec.Key)
		}
	}
	if !wantAny {
		t.Fatal("registry has no free-standing prompts; the screen would be empty")
	}
	// No meta may claim system ownership any more — the field is not served.
	for key, meta := range dto.PromptMeta {
		if meta.OwnedBySystemKey != "" {
			t.Errorf("meta for %q still reports ownedBySystemKey=%q", key, meta.OwnedBySystemKey)
		}
	}
}
