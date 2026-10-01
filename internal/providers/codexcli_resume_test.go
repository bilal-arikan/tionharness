package providers

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCodexBuildArgsScopedResumeIsDurable(t *testing.T) {
	c := NewCodexCLI("codex", "", t.TempDir())
	args := c.buildArgs(Request{CLIResumeScope: "scope", ResumeSessionID: "thread-1"}, "")
	if slices.Contains(args, "--ephemeral") {
		t.Fatalf("scoped resume args are ephemeral: %v", args)
	}
	resume := slices.Index(args, "resume")
	if resume < 0 || resume+1 >= len(args) || args[resume+1] != "thread-1" {
		t.Fatalf("resume subcommand missing or malformed: %v", args)
	}
}

func TestCodexAuxiliaryCallsKeepRolloutForAccounting(t *testing.T) {
	c := NewCodexCLI("codex", "", t.TempDir())
	if args := c.buildArgs(Request{}, ""); slices.Contains(args, "--ephemeral") || slices.Contains(args, "resume") {
		t.Fatalf("auxiliary call cannot measure independent calls: %v", args)
	}
	home, cleanup, err := prepareCodexTurnHome(c.configDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "sessions", "rollout-thread-1.jsonl"), []byte(usageRecord("a", 100, 80, 10)+usageRecord("b", 200, 150, 20)), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := &Response{SessionID: "thread-1"}
	applyCodexRolloutUsage(resp, home, codexUsageCheckpoint{})
	cleanup()
	if resp.ProviderCalls != 2 || resp.FirstCallPromptTokens != 100 || resp.Usage.InputTokens != 70 {
		t.Fatalf("auxiliary accounting: %+v", resp)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("auxiliary rollout retained after accounting")
	}
}

func TestCodexScopedResumeHomeOwnsThread(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "auth.json"), []byte(`{"token":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := prepareCodexTurnHome(base, "session/persona/system")
	if err != nil {
		t.Fatalf("prepare scoped home: %v", err)
	}
	cleanup()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("scoped home was disposable: %v", err)
	}
	threadID := "019c-test-thread"
	rollouts := filepath.Join(dir, "sessions", "2026", "08", "30")
	if err := os.MkdirAll(rollouts, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rollouts, "rollout-2026-08-30T00-00-00-"+threadID+".jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCodexCLI("codex", "", base)
	if !c.ResumeScopeReady("session/persona/system") {
		t.Fatal("stable configured home rejected")
	}
	if !c.CanResumeScoped("session/persona/system", threadID) {
		t.Fatal("thread in matching scoped home was not resumable")
	}
	if c.CanResumeScoped("different persona", threadID) {
		t.Fatal("thread leaked across persona scopes")
	}
	if c.CanResumeScoped("session/persona/system", "../thread") {
		t.Fatal("unsafe thread id accepted")
	}
}

func TestCLICompactionLifecycleCapabilityFailsClosedWithoutBinary(t *testing.T) {
	if HasNativeCLICompactionEvents(NewCodexCLI("missing-codex-binary", "", "")) {
		t.Fatal("missing codex binary enabled native compaction")
	}
	if HasNativeCLICompactionEvents(NewClaudeCLI("missing-claude-binary", "", "", "", "")) {
		t.Fatal("missing claude binary enabled native compaction")
	}
}
