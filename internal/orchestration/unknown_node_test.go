package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// resolveErrRunner fails the way a real agent node fails when the node exists but
// its agentId cannot be resolved (internal/agent/flow.go).
type resolveErrRunner struct{}

func (resolveErrRunner) RunAgentNode(_ context.Context, agentID, _ string) (string, error) {
	return "", fmt.Errorf("cannot resolve agentId %q: %w", agentID, errors.New("agent not found"))
}

// TestRun_UnknownNodeErrorIsDistinct verifies that entering a node the graph does
// not contain says "unknown node", so it cannot be confused with a node whose own
// agentId failed to resolve (TSK444).
func TestRun_UnknownNodeErrorIsDistinct(t *testing.T) {
	g := Graph{
		Start: "a",
		Nodes: []Node{{ID: "a", Type: NodeAgent, AgentID: "a1", Prompt: "x", Next: "ghost"}},
	}
	eng := NewEngine(okRunner{})
	_, err := eng.Run(context.Background(), g, "in", State{Current: g.Start}, nil)
	if err == nil {
		t.Fatal("expected an error for the missing next node")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown node "ghost"`) {
		t.Fatalf("missing-node error should name it as an unknown node, got: %s", msg)
	}
	if strings.Contains(msg, "agentId") {
		t.Fatalf("missing-node error must not mention agentId, got: %s", msg)
	}
}

// TestRun_AgentResolutionErrorNamesAgentId verifies the opposite case: the node
// exists, its agentId does not — the message must point at the agentId and must
// not read as a missing node.
func TestRun_AgentResolutionErrorNamesAgentId(t *testing.T) {
	g := Graph{
		Start: "thanks",
		Nodes: []Node{{ID: "thanks", Type: NodeAgent, AgentID: "AGT37", Prompt: "x"}},
	}
	eng := NewEngine(resolveErrRunner{})
	_, err := eng.Run(context.Background(), g, "in", State{Current: g.Start}, nil)
	if err == nil {
		t.Fatal("expected an error from the failing agent node")
	}
	msg := err.Error()
	if !strings.Contains(msg, `cannot resolve agentId "AGT37"`) {
		t.Fatalf("agent-resolution error should name the agentId, got: %s", msg)
	}
	if strings.Contains(msg, "unknown node") {
		t.Fatalf("agent-resolution error must not read as a missing node, got: %s", msg)
	}
}
