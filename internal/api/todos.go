package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/agent"
	"github.com/bilal-arikan/swarmgo/internal/db"
)

// todoContextBlock builds a system-prompt section showing the session's active
// todo checklist (the latest todo_write), so the agent keeps tracking it even
// after the original tool message has scrolled out of the live context window or
// been folded into the rolling summary by compaction. Returns "" when there is
// no active list (none yet, or the latest is fully completed). Kept in the
// dynamic (uncached) suffix since it changes whenever the list is updated.
func todoContextBlock(ctx context.Context, database *db.DB, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	msgs, err := database.ListMessages(ctx, sessionID)
	if err != nil {
		return ""
	}
	return renderTodoBlock(latestSessionTodos(msgs))
}

// renderTodoBlock formats a checklist as the system-prompt section. Returns ""
// for an empty list or one that is fully completed (nothing left to track —
// mirrors the UI, which hides a completed list).
func renderTodoBlock(todos []agent.TodoItem) string {
	if len(todos) == 0 {
		return ""
	}
	allDone := true
	for _, t := range todos {
		if t.Status != "completed" {
			allDone = false
			break
		}
	}
	if allDone {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Active todo list (this session)\n")
	b.WriteString("This is the checklist you are tracking with the todo_write tool. It persists here even if the original message has scrolled out of context. Keep it current by calling todo_write as you start and finish items.\n")
	for _, t := range todos {
		mark := " "
		switch t.Status {
		case "completed":
			mark = "x"
		case "in_progress":
			mark = "~"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", mark, t.Content)
	}
	return strings.TrimSpace(b.String())
}

// latestSessionTodos returns the most recent todo checklist across a session's
// messages (scanning newest-first), mirroring the frontend's latestTodos. Reads
// the persisted step trace on each message.
func latestSessionTodos(msgs []db.Message) []agent.TodoItem {
	for i := len(msgs) - 1; i >= 0; i-- {
		steps := parseMessageSteps(msgs[i].Steps)
		for j := len(steps) - 1; j >= 0; j-- {
			if todos := stepTodos(steps[j]); len(todos) > 0 {
				return todos
			}
		}
	}
	return nil
}

// parseMessageSteps decodes a message's persisted step trace (JSON array).
func parseMessageSteps(raw string) []agent.TurnStep {
	if raw == "" || raw == "[]" {
		return nil
	}
	var steps []agent.TurnStep
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		return nil
	}
	return steps
}

// stepTodos returns the checklist items carried by a todo step (kind 'todo', or
// a legacy todo_write tool step whose input still holds them).
func stepTodos(step agent.TurnStep) []agent.TodoItem {
	if step.Kind != agent.StepTodo && step.Tool != "todo_write" {
		return nil
	}
	if len(step.Todos) > 0 {
		return step.Todos
	}
	var in struct {
		Todos []agent.TodoItem `json:"todos"`
	}
	if len(step.Input) > 0 && json.Unmarshal(step.Input, &in) == nil {
		return in.Todos
	}
	return nil
}
