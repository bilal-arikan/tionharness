package events

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// The backend NotifyKinds list and the frontend NOTIFY_TYPES registry are a
// hand-maintained contract: the settings screen derives its per-type mute
// toggles from the frontend list, so a kind that exists on only one side is
// either un-mutable (backend-only) or a dead toggle that never fires
// (frontend-only). Both files carry a "keep the two in sync" comment; these
// tests are what actually enforces it.
//
// `prompt` is deliberately frontend-only: ask/permission/plan cues are a
// client-side session-stream signal with no backend event behind them.
const frontendOnlyKind = "prompt"

// notifyTypesPath locates the frontend registry relative to this package.
func notifyTypesPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "frontend", "src", "shared", "lib", "notifyTypes.ts")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("frontend notify registry not found at %s: %v", p, err)
	}
	return p
}

// notifyTypesEntry matches one `{ type: 'chat', ... }` record inside the
// NOTIFY_TYPES array literal.
var notifyTypesEntry = regexp.MustCompile(`\{\s*type:\s*'([a-z_]+)'`)

// parseFrontendNotifyTypes returns the `type` keys declared in NOTIFY_TYPES, in
// source order.
func parseFrontendNotifyTypes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(notifyTypesPath(t))
	if err != nil {
		t.Fatalf("read notifyTypes.ts: %v", err)
	}
	matches := notifyTypesEntry.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("parsed 0 entries from NOTIFY_TYPES — the array literal shape changed, so this parity test is no longer checking anything; update the regex")
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func TestNotifyKindsMatchFrontendRegistry(t *testing.T) {
	frontend := parseFrontendNotifyTypes(t)

	want := map[string]bool{frontendOnlyKind: true}
	for _, k := range NotifyKinds {
		want[k] = true
	}
	got := map[string]bool{}
	for _, k := range frontend {
		got[k] = true
	}

	var missingInFrontend, extraInFrontend []string
	for k := range want {
		if !got[k] {
			missingInFrontend = append(missingInFrontend, k)
		}
	}
	for k := range got {
		if !want[k] {
			extraInFrontend = append(extraInFrontend, k)
		}
	}
	sort.Strings(missingInFrontend)
	sort.Strings(extraInFrontend)

	if len(missingInFrontend) > 0 {
		t.Errorf("backend emits these notify kinds but frontend/src/shared/lib/notifyTypes.ts does not list them (they would be un-mutable in Settings): %v", missingInFrontend)
	}
	if len(extraInFrontend) > 0 {
		t.Errorf("notifyTypes.ts lists these kinds but no backend NotifyKind emits them (dead toggles in Settings): %v", extraInFrontend)
	}
}

func TestFrontendNotifyTypesHaveNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range parseFrontendNotifyTypes(t) {
		if seen[k] {
			t.Errorf("notifyTypes.ts declares %q twice — the byType Map silently keeps the last one", k)
		}
		seen[k] = true
	}
}

func TestNotifyKindsHaveNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range NotifyKinds {
		if seen[k] {
			t.Errorf("NotifyKinds contains %q twice", k)
		}
		seen[k] = true
	}
}

// Control/stream types drive refreshes and live frames; if one leaked into
// NotifyKinds it would start raising desktop toasts for every log line or
// turn-step frame.
func TestControlTypesAreNotNotifyKinds(t *testing.T) {
	control := []string{
		TypeSession, TypeSettings, TypeWorkspaces, TypeNavigate,
		TypeProgress, TypeSkills, TypeSessionStep, TypeFlowNode, TypeLog,
	}
	for _, c := range control {
		if IsNotifyKind(c) {
			t.Errorf("control/stream type %q is in NotifyKinds — it would surface as a desktop toast", c)
		}
	}
}

func TestIsNotifyKindCoversEveryDeclaredKind(t *testing.T) {
	for _, k := range NotifyKinds {
		if !IsNotifyKind(k) {
			t.Errorf("IsNotifyKind(%q) = false but it is in NotifyKinds", k)
		}
	}
	if IsNotifyKind("definitely-not-a-kind") {
		t.Error("IsNotifyKind returned true for an unregistered type")
	}
}
