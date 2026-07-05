package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// TestPermission_ReadOnlyBlocksWrite proves the permission gate inside the tool
// loop: in read-only mode a read tool runs but a write tool is denied — fed back
// to the model as an error result (a StepError, reason permission_denied) and the
// file is never written.
func TestPermission_ReadOnlyBlocksWrite(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Reading first.", tc("c1", "Read", map[string]any{"path": "data.txt"})),
		callTools("Now writing.", tc("c2", "Write", map[string]any{
			"path": "out.txt", "content": "should not persist",
		})),
		sayText("Done what I could."),
	)
	h := newHarness(t, prov)

	// Seed a file the read-only agent is allowed to read.
	if err := os.WriteFile(filepath.Join(h.workDir, "data.txt"), []byte("readable payload"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	ag := h.newAgent("Ranger", func(a *db.Agent) { a.PermissionMode = "read-only" })
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Read data.txt then write out.txt.")

	// The read was allowed and returned the content.
	read := findToolStep(res.steps, "Read")
	if read == nil || read.IsError || !strings.Contains(read.Output, "readable payload") {
		t.Fatalf("read should be allowed in read-only mode, got %+v", read)
	}

	// The write was blocked: a permission_denied error step, and no file on disk.
	if _, err := os.Stat(filepath.Join(h.workDir, "out.txt")); !os.IsNotExist(err) {
		t.Errorf("write should have been blocked, but out.txt exists (err=%v)", err)
	}
	if !hasErrorStep(res.steps, "permission_denied") {
		t.Errorf("expected a permission_denied error step, got: %+v", res.steps)
	}
}

// TestPermission_AskModeApprovesViaPrompter wires an interactive permission
// prompter (as the chat layer does) and confirms an "ask"-mode write proceeds
// when the prompter approves it — the file lands and the prompter saw the call.
func TestPermission_AskModeApprovesViaPrompter(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Saving with approval.", tc("c1", "Write", map[string]any{
			"path": "approved.txt", "content": "approved write",
		})),
		sayText("Saved."),
	)
	h := newHarness(t, prov)

	var prompted []string
	h.decorate = func(ctx context.Context) context.Context {
		return tools.WithPermissionPrompter(ctx, func(_ context.Context, tool, _risk, _arg string, _opts []string) (string, error) {
			prompted = append(prompted, tool)
			return "allow", nil
		})
	}

	ag := h.newAgent("Asker", func(a *db.Agent) { a.PermissionMode = "ask" })
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Write approved.txt please.")

	if len(prompted) == 0 || prompted[0] != "Write" {
		t.Fatalf("permission prompter was not consulted for Write: %+v", prompted)
	}
	data, err := os.ReadFile(filepath.Join(h.workDir, "approved.txt"))
	if err != nil {
		t.Fatalf("approved write did not land: %v", err)
	}
	if string(data) != "approved write" {
		t.Errorf("file content = %q", string(data))
	}
	if w := findToolStep(res.steps, "Write"); w == nil || w.IsError {
		t.Errorf("Write step missing or errored: %+v", w)
	}
}

// TestPermission_AskModeDenied confirms the symmetric path: when the prompter
// rejects the call, the write is blocked and reported back as an error.
func TestPermission_AskModeDenied(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Trying to write.", tc("c1", "Write", map[string]any{
			"path": "denied.txt", "content": "nope",
		})),
		sayText("Understood, not writing."),
	)
	h := newHarness(t, prov)
	h.decorate = func(ctx context.Context) context.Context {
		return tools.WithPermissionPrompter(ctx, func(_ context.Context, _tool, _risk, _arg string, _opts []string) (string, error) {
			return "deny", nil
		})
	}

	ag := h.newAgent("Denier", func(a *db.Agent) { a.PermissionMode = "ask" })
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Write denied.txt.")

	if _, err := os.Stat(filepath.Join(h.workDir, "denied.txt")); !os.IsNotExist(err) {
		t.Errorf("denied write should not exist (err=%v)", err)
	}
	if !hasErrorStep(res.steps, "permission_denied") {
		t.Errorf("expected a permission_denied error step, got: %+v", res.steps)
	}
}

// hasErrorStep reports whether the trace holds an error step with the given reason.
func hasErrorStep(steps []agent.TurnStep, reason string) bool {
	for _, s := range steps {
		if s.Kind == agent.StepError && s.Reason == reason {
			return true
		}
	}
	return false
}
