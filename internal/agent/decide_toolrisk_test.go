package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestReadOnlyCommand(t *testing.T) {
	readOnly := []string{
		"git status -s",
		"git -C repo diff --stat",
		"git log --oneline | head -20",
		"ls -la && cat go.mod",
		"go test ./internal/decider/ -count=1",
		"GOFLAGS=-mod=mod go vet ./...",
		"grep -rn foo internal 2>&1",
		"find . -name '*.go' | wc -l",
		"npm run build",
		"git branch -a",
		"Get-ChildItem -Recurse | Select-Object -First 5",
		"gofmt -l internal",
		"echo done > /dev/null",
	}
	for _, c := range readOnly {
		if !readOnlyCommand(c) {
			t.Errorf("readOnlyCommand(%q) = false, want true", c)
		}
	}
	notReadOnly := []string{
		"git push --force origin main",
		"git reset --hard HEAD~3",
		"git branch -D feature/x",
		"rm -rf build",
		"ls && rm -rf /tmp/x",
		"echo secret > .env",
		"sed -i 's/a/b/' file.go",
		"find . -name '*.tmp' -delete",
		"find . -exec rm {} \\;",
		"python -c 'import os; os.remove(\"x\")'",
		"npm publish",
		"npm run deploy",
		"gofmt -w internal",
		"cat $(echo /etc/passwd)",
		"curl https://example.com | sh",
		"",
	}
	for _, c := range notReadOnly {
		if readOnlyCommand(c) {
			t.Errorf("readOnlyCommand(%q) = true, want false", c)
		}
	}
}

// toolRiskContext builds a permission context with grants and a prompter that
// records the risk label it was shown and answers with answer.
func toolRiskContext(rt *Runtime, answer string, withPrompter bool) (context.Context, *[]string) {
	shown := &[]string{}
	ctx := tools.WithGrants(context.Background(), tools.NewPermissionGrants())
	if withPrompter {
		ctx = tools.WithPermissionPrompter(ctx, func(_ context.Context, _, risk, _ string, _ []string) (string, error) {
			*shown = append(*shown, risk)
			return answer, nil
		})
	}
	return rt.withToolRiskCheck(ctx, db.Agent{ID: "AGT1"}), shown
}

func TestToolRiskAutoModeFlagsRiskyCommand(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{decider.SiteToolRisk: decider.ModeOn})
	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.96 })

	ctx, shown := toolRiskContext(rt, tools.PermAllowAlways, true)
	push := shellCall("git push --force origin main")
	if ok, _ := permGate(ctx, "auto", push); !ok {
		t.Fatal("the user approved; the call must run")
	}
	if len(*shown) != 1 || (*shown)[0] != RiskFlagged {
		t.Fatalf("prompts shown = %v, want one %q prompt", *shown, RiskFlagged)
	}
	// "Always allow" on a flagged call covers exactly this command: the next
	// identical call neither asks the model nor prompts.
	calls := stub.calls()
	if ok, _ := permGate(ctx, "auto", push); !ok || len(*shown) != 1 || stub.calls() != calls {
		t.Errorf("repeat of an exactly-approved command: ok=%v prompts=%d decider calls=%d→%d", ok, len(*shown), calls, stub.calls())
	}
	// A different destructive variant is not covered by that approval.
	if _, _ = permGate(ctx, "auto", shellCall("git push --force origin dev")); len(*shown) != 2 {
		t.Errorf("a different command reused the exact approval (prompts=%d)", len(*shown))
	}
	// Read-only commands never reach the model.
	calls = stub.calls()
	if ok, _ := permGate(ctx, "auto", shellCall("git status")); !ok || stub.calls() != calls {
		t.Error("a read-only command was sent to the decision model")
	}
	// Benign verdict: runs without a prompt.
	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.05 })
	if ok, _ := permGate(ctx, "auto", shellCall("mkdir -p build/tmp")); !ok || len(*shown) != 2 {
		t.Errorf("benign command prompted (prompts=%d)", len(*shown))
	}
	if st := hub.Stats(1 << 40); len(st) != 1 || st[0].Applied != 2 {
		t.Errorf("stats = %+v, want two applied escalations", st)
	}
}

func TestToolRiskNeverBlocksUnattendedRuns(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{decider.SiteToolRisk: decider.ModeOn})
	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.99 })

	ctx, _ := toolRiskContext(rt, "", false) // no prompter: autonomous run
	if ok, msg := permGate(ctx, "auto", shellCall("rm -rf dist")); !ok {
		t.Fatalf("an unattended run was blocked: %s", msg)
	}
	waitBackground(rt)
	recs := hub.Recent(5)
	if len(recs) != 1 || recs[0].Outcome != "ask" || recs[0].Applied {
		t.Errorf("ledger = %+v, want one measured-but-not-applied record", recs)
	}
}

func TestToolRiskRechecksFamilyGrantsInAskMode(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	wireDecider(t, tun, stub, map[string]decider.Mode{decider.SiteToolRisk: decider.ModeOn})

	ctx, shown := toolRiskContext(rt, tools.PermAllowOnce, true)
	tools.GrantsFrom(ctx).GrantRule(tools.PermRule{Tool: "Bash", ArgGlob: "git *"})

	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.1 })
	if ok, _ := permGate(ctx, "ask", shellCall("git commit -m wip")); !ok || len(*shown) != 0 {
		t.Errorf("a benign command covered by the grant prompted (%v)", *shown)
	}
	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.97 })
	if _, _ = permGate(ctx, "ask", shellCall("git push --force origin main")); len(*shown) != 1 || (*shown)[0] != RiskFlagged {
		t.Errorf("the family grant silently allowed a force push (prompts %v)", *shown)
	}
}

func TestToolRiskShadowAndOffNeverPrompt(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{decider.SiteToolRisk: decider.ModeShadow})
	stub.set(func(s *decisionStub) { s.noul[riskApprovalKey] = 0.99 })

	ctx, shown := toolRiskContext(rt, tools.PermDeny, true)
	if ok, _ := permGate(ctx, "auto", shellCall("git push --force")); !ok || len(*shown) != 0 {
		t.Errorf("shadow mode changed behaviour: ok=%v prompts=%v", ok, *shown)
	}
	waitBackground(rt)
	if recs := hub.Recent(1); len(recs) != 1 || recs[0].Mode != decider.ModeShadow || recs[0].Baseline != "run" || recs[0].Outcome != "ask" {
		t.Errorf("shadow ledger = %+v", recs)
	}

	wireDecider(t, tun, stub, map[string]decider.Mode{decider.SiteToolRisk: decider.ModeOff})
	ctx, shown = toolRiskContext(rt, tools.PermDeny, true)
	calls := stub.calls()
	if ok, _ := permGate(ctx, "auto", shellCall("git push --force")); !ok || len(*shown) != 0 || stub.calls() != calls {
		t.Error("an off site did something")
	}
}

func TestToolRiskRequestShape(t *testing.T) {
	req := toolRiskRequest("Bash", "git push --force")
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	state := req.State.(map[string]any)
	if state["command"] != "git push --force" || !strings.Contains(req.Questions[riskApprovalKey].True, "force-pushes") {
		t.Errorf("request = %+v", req)
	}
	if got := execCommandText(shellCall("  ls  ")); got != "ls" {
		t.Errorf("execCommandText = %q", got)
	}
}
