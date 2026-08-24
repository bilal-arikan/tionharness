package insight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderAppFixReport(t *testing.T) {
	out := RenderAppFixReport([]Finding{
		{Title: "Hung subprocess", Severity: "high", Occurrences: 3,
			EvidenceSessionIDs: []string{"SES1", "SES2"}, FilePointer: "internal/providers/claudecli.go",
			RootCause: "MCP initialize deadlock", ProposedFix: "startup watchdog", Signature: "cli:hung"},
	}, "2026-07-13")
	for _, want := range []string{"Hung subprocess", "high", "SES1, SES2", "claudecli.go", "insight-sig:cli:hung"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
	if empty := RenderAppFixReport(nil, "x"); !strings.Contains(empty, "No app-fix findings") {
		t.Fatalf("empty report should say so: %s", empty)
	}
}

func TestAppendBacklogIsIdempotent(t *testing.T) {
	repo := t.TempDir()
	f := Finding{Title: "Bug A", Signature: "sigA", Occurrences: 1}

	n, err := AppendBacklog(repo, []Finding{f})
	if err != nil || n != 1 {
		t.Fatalf("first append: n=%d err=%v", n, err)
	}
	path := filepath.Join(repo, backlogRelPath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("backlog not created: %v", err)
	}

	// Re-append the same signature -> skipped (idempotent).
	n, err = AppendBacklog(repo, []Finding{f})
	if err != nil || n != 0 {
		t.Fatalf("re-append should skip: n=%d err=%v", n, err)
	}

	// A new signature -> appended.
	n, err = AppendBacklog(repo, []Finding{f, {Title: "Bug B", Signature: "sigB"}})
	if err != nil || n != 1 {
		t.Fatalf("new signature should append: n=%d err=%v", n, err)
	}

	body, _ := os.ReadFile(path)
	if strings.Count(string(body), "insight-sig:sigA") != 1 {
		t.Fatalf("sigA should appear exactly once:\n%s", body)
	}
}

func TestAppendWorkspaceActionsIsIdempotent(t *testing.T) {
	root := t.TempDir()
	f := Finding{Title: "Disable unused tool", Signature: "wsA", Occurrences: 2,
		Channel: ChannelWorkspaceOpt, ProposedFix: "block search_web for this agent"}

	n, err := AppendWorkspaceActions(root, []Finding{f})
	if err != nil || n != 1 {
		t.Fatalf("first append: n=%d err=%v", n, err)
	}
	path := filepath.Join(root, workspaceActionsRelPath)
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "Workspace Optimization Actions") {
		t.Fatalf("actions doc missing header:\n%s", body)
	}
	if !strings.Contains(string(body), "block search_web") {
		t.Fatalf("actions doc missing proposed fix:\n%s", body)
	}

	// Re-append the same signature -> skipped (idempotent).
	n, err = AppendWorkspaceActions(root, []Finding{f})
	if err != nil || n != 0 {
		t.Fatalf("re-append should skip: n=%d err=%v", n, err)
	}

	// Blank root is a no-op.
	if n, err := AppendWorkspaceActions("", []Finding{f}); err != nil || n != 0 {
		t.Fatalf("blank root should be a no-op: n=%d err=%v", n, err)
	}
}

func TestAppendBacklogBlankRepoIsNoop(t *testing.T) {
	n, err := AppendBacklog("", []Finding{{Signature: "x"}})
	if err != nil || n != 0 {
		t.Fatalf("blank repo should be a no-op: n=%d err=%v", n, err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	root := t.TempDir()
	if s, err := LoadSettings(root); err != nil || s.MaxSessions != DefaultMaxSessions {
		t.Fatalf("missing settings should use default scan limit: %+v err=%v", s, err)
	}
	want := Settings{AppFixRepoPath: `C:\repo\TionHarness`, MaxSessions: 50}
	if err := SaveSettings(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(root)
	if err != nil || got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v err=%v", got, want, err)
	}

	want.MaxSessions = 0
	if err := SaveSettings(root, want); err != nil {
		t.Fatal(err)
	}
	got, err = LoadSettings(root)
	if err != nil || got != want {
		t.Fatalf("explicit unlimited setting lost: got %+v want %+v err=%v", got, want, err)
	}
}
