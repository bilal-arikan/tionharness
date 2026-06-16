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
		if ok, _ := permGate(context.Background(), mode, call("shell"), map[string]bool{}); !ok {
			t.Errorf("mode %q: shell should be allowed", mode)
		}
		if ok, _ := permGate(context.Background(), mode, call("write_file"), map[string]bool{}); !ok {
			t.Errorf("mode %q: write_file should be allowed", mode)
		}
	}
}

func TestPermGate_ReadOnlyBlocksWritesAllowsReads(t *testing.T) {
	if ok, _ := permGate(context.Background(), "read-only", call("read_file"), map[string]bool{}); !ok {
		t.Error("read-only: read_file should be allowed")
	}
	if ok, msg := permGate(context.Background(), "read-only", call("write_file"), map[string]bool{}); ok || msg == "" {
		t.Errorf("read-only: write_file should be blocked with a message, got ok=%v msg=%q", ok, msg)
	}
	// Unknown / MCP tools default to write and are blocked.
	if ok, _ := permGate(context.Background(), "read-only", call("github__create_issue"), map[string]bool{}); ok {
		t.Error("read-only: unknown tool should be blocked")
	}
}

func TestPermGate_AskWithoutAskerDenies(t *testing.T) {
	// No asker on ctx (autonomous run) → write/exec denied, reads allowed.
	if ok, _ := permGate(context.Background(), "ask", call("grep"), map[string]bool{}); !ok {
		t.Error("ask: read tool should be allowed without an asker")
	}
	if ok, _ := permGate(context.Background(), "ask", call("edit_file"), map[string]bool{}); ok {
		t.Error("ask: write tool should be denied without an asker")
	}
}

func TestPermGate_AskApproveAlwaysRemembered(t *testing.T) {
	asks := 0
	ctx := tools.WithAsker(context.Background(), func(_ context.Context, _ string, _ []string) (string, error) {
		asks++
		return permApproveAlways, nil
	})
	granted := map[string]bool{}
	if ok, _ := permGate(ctx, "ask", call("write_file"), granted); !ok {
		t.Fatal("ask: first write_file should be approved")
	}
	if ok, _ := permGate(ctx, "ask", call("write_file"), granted); !ok {
		t.Fatal("ask: second write_file should be approved")
	}
	if asks != 1 {
		t.Errorf("expected the asker to be called once (Always allow remembered), got %d", asks)
	}
}

func TestPermGate_AskDeny(t *testing.T) {
	ctx := tools.WithAsker(context.Background(), func(_ context.Context, _ string, _ []string) (string, error) {
		return permDeny, nil
	})
	if ok, msg := permGate(ctx, "ask", call("shell"), map[string]bool{}); ok || msg == "" {
		t.Errorf("ask: denied shell should not run, got ok=%v msg=%q", ok, msg)
	}
}
