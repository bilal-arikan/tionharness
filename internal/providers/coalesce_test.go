package providers

import "testing"

// TestCoalescePlainSameRole merges back-to-back same-role text turns (the
// multi-agent case) while leaving alternating turns and tool-bearing turns alone.
func TestCoalescePlainSameRole(t *testing.T) {
	// Two consecutive assistant text turns (two agents in one thread) → merged.
	in := []Message{
		{Role: RoleUser, Text: "selam"},
		{Role: RoleAssistant, Text: "[Ada]: ben Ada"},
		{Role: RoleAssistant, Text: "[Kai]: ben Kai"},
		{Role: RoleUser, Text: "tesekkurler"},
	}
	out := coalescePlainSameRole(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 messages after merge, got %d: %#v", len(out), out)
	}
	if out[1].Role != RoleAssistant || out[1].Text != "[Ada]: ben Ada\n\n[Kai]: ben Kai" {
		t.Errorf("merged assistant turn wrong: %q", out[1].Text)
	}
	if out[2].Role != RoleUser || out[2].Text != "tesekkurler" {
		t.Errorf("trailing user turn changed: %q", out[2].Text)
	}

	// A tool-bearing assistant turn must NOT be merged into an adjacent assistant
	// turn (would break tool_use/tool_result pairing).
	toolIn := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t1", Name: "x"}}},
		{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t1", Content: "ok"}}},
		{Role: RoleAssistant, Text: "done"},
	}
	toolOut := coalescePlainSameRole(toolIn)
	if len(toolOut) != 3 {
		t.Fatalf("tool-bearing turns must not merge: got %d", len(toolOut))
	}
}
