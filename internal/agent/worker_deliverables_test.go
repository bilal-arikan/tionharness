package agent

import (
	"context"
	"errors"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerFileContractGuardsCompletion(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "result.md")
	empty := filepath.Join(dir, "empty.md")
	missing := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(good, []byte("Evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		paths []string
		want  string
	}{
		{"present", []string{good}, turnStatusCompleted}, {"missing", []string{good, missing}, turnStatusIncomplete},
		{"empty", []string{empty}, turnStatusIncomplete}, {"directory", []string{dir}, turnStatusIncomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, text, step := checkWorkerDeliverables(turnStatusCompleted, "Worker claimed done", tc.paths)
			if status != tc.want || step == nil || step.Operation != "delivery_check" {
				t.Fatalf("outcome %s %s %+v", status, text, step)
			}
			digest := digestWorkerSteps([]TurnStep{*step, {Kind: StepTool, Tool: "Bash", Output: strings.Repeat("noise", 1000)}})
			if len(digest) != 1 || digest[0].Output != step.Output {
				t.Fatal("parent delivery evidence lost or tool noise retained")
			}
		})
	}
	status, _, step := checkWorkerDeliverables(turnStatusTimeout, "Interrupted", []string{good})
	if status != turnStatusTimeout || step != nil {
		t.Fatal("presence promoted an interrupted turn")
	}
	paths, err := normalizeWorkerDeliverables([]string{"result.md", good}, dir)
	if err != nil || len(paths) != 1 || paths[0] != good {
		t.Fatalf("normalized: %v %v", paths, err)
	}
	if _, err := normalizeWorkerDeliverables([]string{"*.md"}, dir); err == nil {
		t.Fatal("wildcard contract accepted")
	}
}

func TestRuntimePreservesPartialUsageAndUnfinishedOutcome(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	row, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Interrupted", Provider: "claude-cli", Model: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	partial := &providers.Response{Model: "opus", Text: "Still working", Usage: providers.Usage{InputTokens: 4321, OutputTokens: 9}, ProviderCalls: 2}
	failure := providers.WithUsage(&providers.CLIInterruption{Partial: partial, Reason: "cli_idle_timeout", Window: time.Second, Cause: errors.New("idle watchdog")}, partial.Model, partial.Usage, 2)
	turn := toolLoopTurn{r: rt, ctx: ctx, agent: row, provider: failingProvider{err: failure}, req: providers.Request{Model: "opus"}}
	resp, steps, err := turn.completeAndTraceCLI()
	if err != nil || resp != partial || resp.StopReason != providers.StopInterrupted {
		t.Fatalf("partial completion: %+v %v", resp, err)
	}
	if got := classifyTurnOutcome(ctx, steps, 0, 0); got.Status != turnStatusTimeout {
		t.Fatalf("partial claimed completed: %+v", got)
	}
	if len(digestWorkerSteps(steps)) != 1 {
		t.Fatal("interruption missing from parent evidence")
	}
	usages, err := rt.db.UsageForDay(ctx, time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	var totalIn, totalCalls int
	for _, u := range usages {
		if u.AgentID == row.ID {
			totalIn += u.InputTokens
			totalCalls += u.ProviderCalls
		}
	}
	if totalIn != 4321 || totalCalls != 2 {
		t.Fatalf("usage lost or double-counted: input=%d calls=%d", totalIn, totalCalls)
	}
}
