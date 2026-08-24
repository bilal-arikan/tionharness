package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// todoItem is a single entry in the agent's working checklist.
type todoItem struct {
	Content  string   `json:"content"`
	Status   string   `json:"status"` // pending | in_progress | completed
	Category string   `json:"category,omitempty"`
	Steps    []string `json:"steps,omitempty"`
}

// todoInput is the ask shape for the todo_write tool: either a full-list
// replace (todos) or a compact status-only update (set) — exactly one.
type todoInput struct {
	Todos []todoItem        `json:"todos"`
	Set   map[string]string `json:"set"` // 1-based index (as string) → new status
}

// TodoWriteTool lets the agent publish/replace its working checklist. The list
// itself is surfaced to the user by the chat UI (which special-cases this tool's
// input into a live checklist card); the tool call returns only a short text
// confirmation. Full state is not kept in the tool — each `todos` call carries
// the whole list; the compact `set` form merges into the list last persisted by
// the todo sink (progress file), so status flips don't resend unchanged content.
type TodoWriteTool struct{}

// NewTodoWriteTool constructs the todo_write tool.
func NewTodoWriteTool() TodoWriteTool { return TodoWriteTool{} }

func (TodoWriteTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "todo_write",
		// Eager tool: the description keeps only the two behavioral rules a schema
		// cannot state — prefer `set` once the list exists, and hold exactly one item
		// in_progress. The `set` example stays: its string-keyed 1-based index shape is
		// the one thing models get wrong.
		Description: "Publish or update your working checklist for the current task. Pass exactly one form: " +
			"`todos` REPLACES the whole list (creation, or when item texts change); `set` is a cheap " +
			"status-only update keyed by 1-based index, e.g. {\"set\":{\"1\":\"completed\",\"2\":\"in_progress\"}} " +
			"— prefer it once the list exists instead of resending unchanged items. Keep exactly one item " +
			"in_progress while you work on it, then completed when done.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "description": "Full checklist, replacing the previous one.",
      "items": {
        "type": "object",
        "properties": {
          "content": { "type": "string", "description": "Short imperative step." },
          "status": { "type": "string", "enum": ["pending", "in_progress", "completed"] },
          "category": { "type": "string", "description": "Optional grouping label (e.g. 'tests', 'docs')." },
          "steps": { "type": "array", "items": { "type": "string" }, "description": "Optional verification sub-steps." }
        },
        "required": ["content", "status"],
        "additionalProperties": false
      }
    },
    "set": {
      "type": "object",
      "description": "Status-only update: string key = 1-based item index.",
      "additionalProperties": { "type": "string", "enum": ["pending", "in_progress", "completed"] }
    }
  },
  "additionalProperties": false
}`),
	}
}

func (TodoWriteTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[todoInput]("todo_write", input)
	if err != nil {
		return "", err
	}
	switch {
	case len(in.Todos) > 0 && len(in.Set) > 0:
		return "", fmt.Errorf("pass either todos (full replace) or set (status update), not both")
	case len(in.Set) > 0:
		return applyTodoSet(ctx, in.Set)
	case len(in.Todos) == 0:
		return "", fmt.Errorf("todos must not be empty")
	}
	for i, t := range in.Todos {
		if strings.TrimSpace(t.Content) == "" {
			return "", fmt.Errorf("todo %d: content is required", i+1)
		}
		if !validTodoStatus(t.Status) {
			return "", fmt.Errorf("todo %d: invalid status %q (want pending|in_progress|completed)", i+1, t.Status)
		}
	}
	// Persist the list to the project's durable progress file when a sink is
	// attached, so it survives across sessions and backs later `set` updates.
	// Best-effort: a persistence failure never fails the tool — the live UI
	// checklist still updates.
	if sink := todoSinkFrom(ctx); sink != nil {
		_ = sink.SaveTodos(ctx, todoSinkItems(in.Todos))
	}
	return todoSummary(in.Todos), nil
}

// applyTodoSet merges a status-only update into the checklist last persisted by
// the todo sink. It needs the sink to know the previous list — without one (or
// before any full-list call) the update fails loudly with a hint to resend the
// full list. The merged list is persisted back and returned as JSON so the
// trace layer can still render the full checklist card from this call.
func applyTodoSet(ctx context.Context, set map[string]string) (string, error) {
	sink := todoSinkFrom(ctx)
	if sink == nil {
		return "", fmt.Errorf("set requires a persisted checklist, which is unavailable this turn — send the full todos array instead")
	}
	prev, ok, err := sink.LoadTodos(ctx)
	if err != nil {
		return "", fmt.Errorf("load previous checklist: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("no existing checklist to update — create one first with the full todos array")
	}
	for key, status := range set {
		idx, convErr := strconv.Atoi(key)
		if convErr != nil || idx < 1 || idx > len(prev) {
			return "", fmt.Errorf("set: invalid item index %q (list has %d items, indices are 1-based)", key, len(prev))
		}
		if !validTodoStatus(status) {
			return "", fmt.Errorf("set: item %d: invalid status %q (want pending|in_progress|completed)", idx, status)
		}
		prev[idx-1].Status = status
	}
	if err := sink.SaveTodos(ctx, prev); err != nil {
		return "", fmt.Errorf("persist updated checklist: %w", err)
	}
	merged := make([]todoItem, len(prev))
	for i, t := range prev {
		merged[i] = todoItem{Content: t.Content, Status: t.Status, Category: t.Category, Steps: t.Steps}
	}
	// JSON result (not plain text): carries the merged full list so the trace
	// layer can promote this call to a checklist card — the input alone no
	// longer holds the items.
	body, err := json.Marshal(struct {
		Summary string     `json:"summary"`
		Todos   []todoItem `json:"todos"`
	}{Summary: todoSummary(merged), Todos: merged})
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// todoSummary renders the short confirmation line for a checklist state.
func todoSummary(todos []todoItem) string {
	var done, active, pending int
	for _, t := range todos {
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			active++
		default:
			pending++
		}
	}
	return fmt.Sprintf("Checklist updated: %d total — %d completed, %d in progress, %d pending.",
		len(todos), done, active, pending)
}

func validTodoStatus(s string) bool {
	return s == "pending" || s == "in_progress" || s == "completed"
}

// todoSinkItems converts checklist entries to the sink's wire shape.
func todoSinkItems(todos []todoItem) []TodoSinkItem {
	items := make([]TodoSinkItem, len(todos))
	for i, t := range todos {
		items[i] = TodoSinkItem{Content: t.Content, Status: t.Status, Category: t.Category, Steps: t.Steps}
	}
	return items
}
