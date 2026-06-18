package agent

import (
	"context"
	"testing"

	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

func call(name string) providers.ToolCall { return providers.ToolCall{ID: "1", Name: name} }

func TestPermGate_AutoAllowsEverything(t *testing.T) {
	for _, mode := range []string{"auto", ""} {
		if ok, _ := permGate(context.Background(), mode, call("shell")); !ok {
			t.Errorf("mode %q: shell should be allowed", mode)
		}
		if ok, _ := permGate(context.Background(), mode, call("write_file")); !ok {
			t.Errorf("mode %q: write_file should be allowed", mode)
		}
	}
}

func TestPermGate_ReadOnlyBlocksWritesAllowsReads(t *testing.T) {
	if ok, _ := permGate(context.Background(), "read-only", call("read_file")); !ok {
		t.Error("read-only: read_file should be allowed")
	}
	if ok, msg := permGate(context.Background(), "read-only", call("write_file")); ok || msg == "" {
		t.Errorf("read-only: write_file should be blocked with a message, got ok=%v msg=%q", ok, msg)
	}
	// Unknown / MCP tools default to write and are blocked.
	if ok, _ := permGate(context.Background(), "read-only", call("github__create_issue")); ok {
		t.Error("read-only: unknown tool should be blocked")
	}
}

func TestPermGate_AskWithoutPrompterDenies(t *testing.T) {
	// No prompter on ctx (autonomous run) → write/exec denied, reads allowed.
	if ok, _ := permGate(context.Background(), "ask", call("grep")); !ok {
		t.Error("ask: read tool should be allowed without a prompter")
	}
	if ok, _ := permGate(context.Background(), "ask", call("edit_file")); ok {
		t.Error("ask: write tool should be denied without a prompter")
	}
}

func TestPermGate_AskAlwaysAllowRememberedAcrossCalls(t *testing.T) {
	prompts := 0
	ctx := tools.WithGrants(context.Background(), tools.NewPermissionGrants())
	ctx = tools.WithPermissionPrompter(ctx, func(_ context.Context, _, _ string, _ []string) (string, error) {
		prompts++
		return tools.PermAllowAlways, nil
	})
	if ok, _ := permGate(ctx, "ask", call("write_file")); !ok {
		t.Fatal("ask: first write_file should be approved")
	}
	if ok, _ := permGate(ctx, "ask", call("write_file")); !ok {
		t.Fatal("ask: second write_file should be approved (granted)")
	}
	if prompts != 1 {
		t.Errorf("expected the prompter to be called once (Always allow remembered), got %d", prompts)
	}
}

func TestPermGate_AskDeny(t *testing.T) {
	ctx := tools.WithGrants(context.Background(), tools.NewPermissionGrants())
	ctx = tools.WithPermissionPrompter(ctx, func(_ context.Context, _, _ string, _ []string) (string, error) {
		return tools.PermDeny, nil
	})
	if ok, msg := permGate(ctx, "ask", call("shell")); ok || msg == "" {
		t.Errorf("ask: denied shell should not run, got ok=%v msg=%q", ok, msg)
	}
}
