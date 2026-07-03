package agent

import "testing"

func TestIsPermissionDenyError(t *testing.T) {
	deny := []TurnStep{
		{Kind: StepError, Tool: "Bash", Reason: "permission_denied", IsError: true},
		{Kind: StepTool, Tool: "Bash", IsError: true, Output: "Claude requested permissions to use Bash, but you haven't granted it yet."},
		{Kind: StepTool, Tool: "Write", IsError: true, Text: "This tool is not allowed in the current mode."},
		{Kind: StepTool, Tool: "Task", IsError: true, Output: "Tool 'Task' is disallowed."},
	}
	for i, st := range deny {
		if !isPermissionDenyError(st) {
			t.Errorf("deny[%d] should be a permission denial: %+v", i, st)
		}
	}

	real := []TurnStep{
		{Kind: StepTool, Tool: "Bash", IsError: true, Output: "exit status 1: command not found"},
		{Kind: StepTool, Tool: "Read", IsError: true, Output: "no such file or directory"},
		{Kind: StepTool, Tool: "Grep", IsError: true, Text: "invalid regex"},
	}
	for i, st := range real {
		if isPermissionDenyError(st) {
			t.Errorf("real[%d] should NOT be a denial (a genuine tool error): %+v", i, st)
		}
	}
}
