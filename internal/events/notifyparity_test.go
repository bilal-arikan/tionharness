package events

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The backend NotifyKinds list and the frontend NOTIFY_TYPES registry are a
// hand-maintained contract: the settings screen derives its per-type mute
// toggles from the frontend list, so a kind that exists on only one side is
// either un-mutable (backend-only) or a dead toggle that never fires
// (frontend-only). Both files carry a "keep the two in sync" comment; these
// tests are what actually enforces it.
//
// These established UI notifications live outside the core NotifyKinds list:
// prompt comes from session interactions, rota from workspace-stream projections,
// and agent-model-changed is published directly by api/change_notifier.go. The
// old object-literal regex accidentally skipped commented and hyphenated entries.
var additionalFrontendKinds = []string{"prompt", "rota", "agent-model-changed"}

// notifyTypesPath locates the frontend registry relative to this package.
func notifyTypesPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "frontend", "src", "shared", "lib", "notifyTypes.ts")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("frontend notify registry not found at %s: %v", p, err)
	}
	return p
}

// notifyTypesEntry reads stable event IDs passed to the frontend factory. Human
// labels are now locale-aware getters, so object-literal matching is no longer
// appropriate. Hyphenated IDs are part of the event contract too.
var notifyTypesEntry = regexp.MustCompile(`notifyType\(\s*'([a-z_-]+)'`)

// parseFrontendNotifyTypes returns the `type` keys declared in NOTIFY_TYPES, in
// source order.
func parseFrontendNotifyTypes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(notifyTypesPath(t))
	if err != nil {
		t.Fatalf("read notifyTypes.ts: %v", err)
	}
	_, registry, ok := strings.Cut(string(src), "export const NOTIFY_TYPES: NotifyType[] = [")
	if !ok {
		t.Fatal("NOTIFY_TYPES array declaration not found; update the registry parser")
	}
	registry, _, ok = strings.Cut(registry, "\n]")
	if !ok {
		t.Fatal("NOTIFY_TYPES array terminator not found; update the registry parser")
	}
	matches := notifyTypesEntry.FindAllStringSubmatch(registry, -1)
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

	want := map[string]bool{}
	for _, k := range additionalFrontendKinds {
		want[k] = true
	}
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
	slices.Sort(missingInFrontend)
	slices.Sort(extraInFrontend)

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
