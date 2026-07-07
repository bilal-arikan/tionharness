package conversation

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func call(id, name string) providers.ToolCall {
	return providers.ToolCall{ID: id, Name: name}
}

func result(id string) providers.ToolResult {
	return providers.ToolResult{CallID: id, Content: "ok"}
}

func countRule(notes []RepairNote, rule string) int {
	n := 0
	for _, note := range notes {
		if note.Rule == rule {
			n++
		}
	}
	return n
}

func TestRepairSequence_WellFormedUntouched(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleUser, Text: "do it"},
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "read_file")}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("a")}},
		{Role: providers.RoleAssistant, Text: "done"},
	}
	out, notes := RepairSequence(msgs)
	if len(notes) != 0 {
		t.Fatalf("well-formed slice produced notes: %+v", notes)
	}
	if &out[0] != &msgs[0] {
		t.Errorf("well-formed slice was copied; want the same backing array")
	}
}

func TestRepairSequence_DropsOrphanResult(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleUser, Text: "hi"},
		{Role: providers.RoleAssistant, Text: "plain answer"},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("ghost")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "orphan_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one orphan_tool_result", notes)
	}
	// The result message had nothing else — it must be dropped entirely.
	if len(out) != 2 {
		t.Errorf("len(out) = %d, want 2 (orphan message dropped)", len(out))
	}
}

func TestRepairSequence_KeepsTextWhenDroppingOrphan(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, Text: "x"},
		{Role: providers.RoleUser, Text: "note", ToolResults: []providers.ToolResult{result("ghost")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "orphan_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one orphan_tool_result", notes)
	}
	if len(out) != 2 || out[1].Text != "note" || len(out[1].ToolResults) != 0 {
		t.Errorf("text-bearing message must survive with results stripped: %+v", out)
	}
}

func TestRepairSequence_DedupesDuplicateResults(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "read_file")}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("a"), result("a")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "duplicate_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one duplicate_tool_result", notes)
	}
	if len(out[1].ToolResults) != 1 {
		t.Errorf("results = %d, want 1 after dedupe", len(out[1].ToolResults))
	}
}

func TestRepairSequence_SynthesizesMissingResults(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "terminal")}},
		{Role: providers.RoleAssistant, Text: "moving on"},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "missing_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one missing_tool_result", notes)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3 (synthetic result message inserted)", len(out))
	}
	syn := out[1]
	if syn.Role != providers.RoleUser || len(syn.ToolResults) != 1 || !syn.ToolResults[0].IsError || syn.ToolResults[0].CallID != "a" {
		t.Errorf("synthetic result message malformed: %+v", syn)
	}
}

func TestRepairSequence_TrailingDanglingToolUse(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleUser, Text: "go"},
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "read_file"), call("b", "grep")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "missing_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one missing_tool_result", notes)
	}
	last := out[len(out)-1]
	if last.Role != providers.RoleUser || len(last.ToolResults) != 2 {
		t.Errorf("trailing dangling tool_use not answered: %+v", last)
	}
}

func TestRepairSequence_FillsPartialBatch(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "read_file"), call("b", "grep")}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("a")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "missing_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one missing_tool_result", notes)
	}
	if len(out[1].ToolResults) != 2 {
		t.Fatalf("results = %d, want 2 (gap filled)", len(out[1].ToolResults))
	}
	if out[1].ToolResults[0].CallID != "a" || out[1].ToolResults[1].CallID != "b" || !out[1].ToolResults[1].IsError {
		t.Errorf("partial batch fill wrong: %+v", out[1].ToolResults)
	}
}

func TestRepairSequence_MergesSplitAssistantBatch(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "read_file")}},
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("b", "grep")}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("a"), result("b")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "merged_assistant_tool_calls") != 1 {
		t.Fatalf("notes = %+v, want one merged_assistant_tool_calls", notes)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2 (merged assistant + results)", len(out))
	}
	if len(out[0].ToolCalls) != 2 {
		t.Errorf("merged calls = %d, want 2", len(out[0].ToolCalls))
	}
	if len(out[1].ToolResults) != 2 {
		t.Errorf("results = %d, want 2 kept after merge", len(out[1].ToolResults))
	}
}

func TestRepairSequence_DoesNotMergeRawContent(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "read_file")}, RawContent: []byte(`[{"type":"text"}]`)},
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("b", "grep")}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("b")}},
	}
	out, notes := RepairSequence(msgs)
	if countRule(notes, "merged_assistant_tool_calls") != 0 {
		t.Fatalf("RawContent messages must not merge: %+v", notes)
	}
	// The first batch instead gets synthetic results so pairing holds.
	if countRule(notes, "missing_tool_result") != 1 {
		t.Fatalf("notes = %+v, want one missing_tool_result for the raw batch", notes)
	}
	if len(out) != 4 {
		t.Errorf("len(out) = %d, want 4 (raw assistant, synthetic results, assistant, results)", len(out))
	}
}

func TestRepairSequence_Idempotent(t *testing.T) {
	msgs := []providers.Message{
		{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{call("a", "terminal")}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{result("ghost")}},
	}
	once, notes1 := RepairSequence(msgs)
	if len(notes1) == 0 {
		t.Fatalf("expected repairs on the broken slice")
	}
	twice, notes2 := RepairSequence(once)
	if len(notes2) != 0 {
		t.Errorf("second pass repaired again: %+v (not idempotent)", notes2)
	}
	if len(twice) != len(once) {
		t.Errorf("second pass changed length %d → %d", len(once), len(twice))
	}
}
