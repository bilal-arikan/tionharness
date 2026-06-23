package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// todoItem is a single entry in the agent's working checklist.
type todoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

// todoInput is the ask shape for the todo_write tool.
type todoInput struct {
	Todos []todoItem `json:"todos"`
}

// TodoWriteTool lets the agent publish/replace its working checklist. The list
// itself is surfaced to the user by the chat UI (which special-cases this tool's
// input into a live checklist card); the tool call returns only a short text
// confirmation. State is intentionally not persisted server-side — each call
// carries the full list, mirroring how the model reasons about its plan.
type TodoWriteTool struct{}

// NewTodoWriteTool constructs the todo_write tool.
func NewTodoWriteTool() TodoWriteTool { return TodoWriteTool{} }

func (TodoWriteTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "todo_write",
		Description: "Publish or update your working checklist for the current task. " +
			"Pass the FULL list every time (it replaces the previous one). Use it to " +
			"plan multi-step work and show progress: mark exactly one item as " +
			"in_progress while you work on it, then completed when done.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "content": { "type": "string", "description": "Short imperative description of the step." },
          "status": { "type": "string", "enum": ["pending", "in_progress", "completed"] }
        },
        "required": ["content", "status"],
        "additionalProperties": false
      }
    }
  },
  "required": ["todos"],
  "additionalProperties": false
}`),
	}
}

func (TodoWriteTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var in todoInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("todo_write", err)
	}
	if len(in.Todos) == 0 {
		return "", fmt.Errorf("todos must not be empty")
	}
	var done, active, pending int
	for i, t := range in.Todos {
		if strings.TrimSpace(t.Content) == "" {
			return "", fmt.Errorf("todo %d: content is required", i+1)
		}
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			active++
		case "pending":
			pending++
		default:
			return "", fmt.Errorf("todo %d: invalid status %q (want pending|in_progress|completed)", i+1, t.Status)
		}
	}
	return fmt.Sprintf("Checklist updated: %d total — %d completed, %d in progress, %d pending.",
		len(in.Todos), done, active, pending), nil
}
