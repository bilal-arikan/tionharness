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
