package agent

import "testing"

func TestIsPermissionDenyError(t *testing.T) {
	deny := []TurnStep{
		{Kind: StepError, Tool: "Bash", Reason: "permission_denied", IsError: true},
		{Kind: StepTool, Tool: "Bash", IsError: true, Output: "Claude requested permissions to use Bash, but you haven't granted it yet."},
		{Kind: StepTool, Tool: "Write", IsError: true, Text: "This tool is not allowed in the current mode."},
		{Kind: StepTool, Tool: "Task", IsError: true, Output: "Tool 'Task' is disallowed."},
		// Bare-name mis-address: the model called the shell as `PowerShell` instead of
		// the namespaced mcp__tionswarm_interaction__PowerShell; the CLI rejects it and
		// the model retries. Self-recovered, not a repairable failure → not tool-error.
		{Kind: StepTool, Tool: "PowerShell", IsError: true, Output: "<tool_use_error>Error: No such tool available: PowerShell. PowerShell exists but is not enabled in this context. Use one of the available tools instead.</tool_use_error>"},
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

func TestIsAuthErrorText(t *testing.T) {
	auth := []string{
		`provider error: claude CLI failed: exit status 1 ... {"error":"authentication_failed"}`,
		"Not logged in · Please run /login",
		"invalid x-api-key",
		"OAuth token has expired",
	}
	for i, s := range auth {
		if !isAuthErrorText(s) {
			t.Errorf("auth[%d] should be an auth error: %q", i, s)
		}
	}

	notAuth := []string{
		"provider error: context canceled",
		"claude CLI usage/rate limit reached",
		"exit status 1: command not found",
		"",
	}
	for i, s := range notAuth {
		if isAuthErrorText(s) {
			t.Errorf("notAuth[%d] should NOT be an auth error: %q", i, s)
		}
	}
}
