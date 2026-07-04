package agent

import "testing"

func todoStep(statuses ...string) TurnStep {
	items := make([]TodoItem, len(statuses))
	for i, s := range statuses {
		items[i] = TodoItem{Content: "t", Status: s}
	}
	return TurnStep{Kind: StepTodo, Todos: items}
}

func TestNeedsAutoContinue(t *testing.T) {
	cases := []struct {
		name  string
		steps []TurnStep
		want  bool
	}{
		{"empty", nil, false},
		{"plain text answer", []TurnStep{{Kind: StepText, Text: "done"}}, false},
		{
			"open todo (in_progress)",
			[]TurnStep{{Kind: StepText}, todoStep("completed", "in_progress", "pending")},
			true,
		},
		{
			"all todos completed",
			[]TurnStep{{Kind: StepText}, todoStep("completed", "completed")},
			false,
		},
		{
			"ends on ToolSearch activation",
			[]TurnStep{{Kind: StepText, Text: "let me load tools"}, {Kind: StepTool, Tool: "ToolSearch"}},
			true,
		},
		{
			"ends on activate_tools then narration",
			[]TurnStep{{Kind: StepTool, Tool: "activate_tools"}, {Kind: StepText, Text: "loaded"}},
			true, // trailing narration is skipped; the last real action is the activation
		},
		{
			"ends on a normal tool call",
			[]TurnStep{{Kind: StepTool, Tool: "WebSearch"}, {Kind: StepText, Text: "here are results"}},
			false,
		},
		{
			"latest todo all done overrides an earlier open one",
			[]TurnStep{todoStep("pending"), {Kind: StepTool, Tool: "WebSearch"}, todoStep("completed")},
			false,
		},
	}
	for _, c := range cases {
		if got := needsAutoContinue(c.steps); got != c.want {
			t.Errorf("%s: needsAutoContinue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHasToolStep(t *testing.T) {
	if hasToolStep([]TurnStep{{Kind: StepText}, {Kind: StepThinking}}) {
		t.Error("text/thinking only should have no tool progress")
	}
	if !hasToolStep([]TurnStep{{Kind: StepText}, {Kind: StepTool, Tool: "Bash"}}) {
		t.Error("a tool call is tool progress")
	}
	if !hasToolStep([]TurnStep{todoStep("pending")}) {
		t.Error("a todo write is tool progress")
	}
}
