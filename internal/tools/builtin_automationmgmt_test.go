package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestCreateAutomationPromptRequirementDependsOnBoardAction(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	tool := NewCreateAutomationTool(database, "actor-1")
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.Def().InputSchema, &schema); err != nil {
		t.Fatalf("decode create_automation schema: %v", err)
	}
	for _, field := range schema.Required {
		if field == "promptTemplate" {
			t.Fatal("create_automation schema must allow promptless board archive/move actions")
		}
	}

	if _, err := tool.Call(ctx, json.RawMessage(`{"name":"Archive done cards","triggerKind":"board","boardAction":"archive","boardToState":"done"}`)); err != nil {
		t.Fatalf("create promptless archive automation: %v", err)
	}
	agent, err := database.CreateAgent(ctx, db.Agent{Name: "Worker"})
	if err != nil {
		t.Fatalf("create target agent: %v", err)
	}
	_, err = tool.Call(ctx, json.RawMessage(`{"name":"Spawn worker","triggerKind":"board","boardAction":"spawn","boardToState":"todo","targetAgentId":"`+agent.ID+`"}`))
	if !errors.Is(err, db.ErrAutomationShape) || !strings.Contains(err.Error(), "promptTemplate is required") {
		t.Fatalf("promptless spawn error = %v, want ErrAutomationShape requiring promptTemplate", err)
	}
}

func TestCreateTargetlessBoardMoveAutomation(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	tool := NewCreateAutomationTool(database, "actor-1")

	if _, err := tool.Call(ctx, json.RawMessage(`{"name":"Move reviewed cards","triggerKind":"board","boardAction":"move","boardToState":"review","boardMoveToState":"done"}`)); err != nil {
		t.Fatalf("create targetless promptless move automation: %v", err)
	}
	automations, err := database.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("list automations: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("automation count = %d, want 1", len(automations))
	}
	move := automations[0]
	if move.BoardAction != db.BoardActionMove || move.BoardMoveToState != db.BoardDone {
		t.Fatalf("persisted move contract = action %q destination %q", move.BoardAction, move.BoardMoveToState)
	}
	if move.TargetAgentID != "" || move.FlowID != "" || move.PromptTemplate != "" {
		t.Fatalf("targetless move retained execution fields: agent=%q flow=%q prompt=%q", move.TargetAgentID, move.FlowID, move.PromptTemplate)
	}
}
