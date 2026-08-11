package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCodebaseWorkspaceSearchDef(t *testing.T) {
	tool := NewCodebaseWorkspaceSearchTool(`C:\x\codebase-memory-mcp.exe`)
	if tool.Def().Name != "codebase_workspace_search" {
		t.Errorf("unexpected tool name %q", tool.Def().Name)
	}
}

func TestCodebaseWorkspaceSearchRequiresPattern(t *testing.T) {
	tool := NewCodebaseWorkspaceSearchTool(`does-not-exist`)
	// Empty pattern must fail before any CLI invocation.
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"pattern":"   "}`)); err == nil {
		t.Error("expected error for blank pattern")
	}
	if _, err := tool.Call(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Error("expected error for missing pattern")
	}
}
