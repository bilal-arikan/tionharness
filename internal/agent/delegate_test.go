package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestInheritedMessages keeps readable turns and drops tool plumbing so an
// inherited-context subagent never receives a dangling tool_use.
func TestInheritedMessages(t *testing.T) {
	req := &providers.Request{Messages: []providers.Message{
		{Role: providers.RoleUser, Text: "hi"},
		{Role: providers.RoleAssistant, Text: "thinking", ToolCalls: []providers.ToolCall{{ID: "x", Name: "run_subagent"}}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{{CallID: "x", Content: "result"}}},
		{Role: providers.RoleAssistant, Text: "final"},
	}}
	got := inheritedMessages(req)
	if len(got) != 3 {
		t.Fatalf("expected 3 readable turns, got %d: %+v", len(got), got)
	}
	for _, m := range got {
		if len(m.ToolCalls) != 0 || len(m.ToolResults) != 0 {
			t.Fatalf("tool plumbing leaked into inherited messages: %+v", m)
		}
	}
}
