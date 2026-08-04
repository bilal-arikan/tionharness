package view

import "encoding/json"

// Step is the minimal shape of a persisted agent.TurnStep that summarising code
// needs: what kind of step it was, which tool it ran, whether it failed, and the
// text it produced.
//
// It is decoded STRUCTURALLY rather than by importing internal/agent. That
// package depends on internal/tools, which depends on this one (the get_view
// tool), so importing it would close a cycle — and internal/insight had grown its
// own byte-identical copy for the same reason. The fields here are part of the
// persisted session format (session.jsonl), not internal API, so decoding them
// directly is stable.
//
// This type is the single home for that decoding. Adding a third copy is the
// thing it exists to prevent.
type Step struct {
	Kind    string          `json:"kind"`
	Tool    string          `json:"tool,omitempty"`
	Text    string          `json:"text,omitempty"`
	Output  string          `json:"output,omitempty"`
	IsError bool            `json:"isError,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Todos   []TodoItem      `json:"todos,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
}

// TodoItem is one entry of a todo_write checklist.
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

// DecodeSteps parses a message's persisted step trace.
//
// A trace that will not parse yields no steps rather than an error: callers use
// steps for optional enrichment (a checklist line, a signal count, an evidence
// slice), and one corrupt message must not take down the whole summary.
func DecodeSteps(raw string) []Step {
	if raw == "" || raw == "[]" {
		return nil
	}
	var steps []Step
	if json.Unmarshal([]byte(raw), &steps) != nil {
		return nil
	}
	return steps
}

// TodoItems reads a step's checklist, tolerating the legacy form where the items
// lived in the todo_write tool input instead of the step's own field.
func (s Step) TodoItems() []TodoItem {
	if s.Kind != "todo" && s.Tool != "todo_write" {
		return nil
	}
	if len(s.Todos) > 0 {
		return s.Todos
	}
	var in struct {
		Todos []TodoItem `json:"todos"`
	}
	if len(s.Input) > 0 && json.Unmarshal(s.Input, &in) == nil {
		return in.Todos
	}
	return nil
}
