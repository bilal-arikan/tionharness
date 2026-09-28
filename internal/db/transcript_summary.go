package db

import (
	"encoding/json"
	"strings"
)

// TranscriptSummary keeps composer metadata independent of the visible history
// page. It holds no tool output and is recomputed only when cached data changes.
type TranscriptSummary struct {
	LastMessageAt      int64           `json:"lastMessageAt"`
	ColdTurns          int             `json:"coldTurns"`
	Todo               *TranscriptTodo `json:"todo"`
	seenAssistantUsage bool
}

type TranscriptTodo struct {
	Todos        []map[string]any `json:"todos"`
	OccurrenceID string           `json:"occurrenceId"`
}

func (s *TranscriptSummary) appendMessages(msgs []Message) {
	for _, m := range msgs {
		s.LastMessageAt = m.CreatedAt
		if m.Role != "user" && m.Usage != nil {
			if s.seenAssistantUsage && m.Usage.CacheWriteTokens > 0 && m.Usage.CacheReadTokens == 0 {
				s.ColdTurns++
			}
			s.seenAssistantUsage = true
		}
		// Most traces have no checklist: skip their large JSON strings outright.
		if !strings.Contains(m.Steps, `"todo"`) && !strings.Contains(m.Steps, `"todo_write"`) {
			continue
		}
		var steps []struct {
			Kind  string           `json:"kind"`
			Tool  string           `json:"tool"`
			ID    *string          `json:"id"`
			Todos []map[string]any `json:"todos"`
			Input json.RawMessage  `json:"input"`
		}
		if json.Unmarshal([]byte(m.Steps), &steps) != nil {
			continue
		}
		ordinal := 0
		for _, step := range steps {
			if step.Kind != "todo" && step.Tool != "todo_write" {
				continue
			}
			todos := step.Todos
			if len(todos) == 0 {
				var input struct {
					Todos []map[string]any `json:"todos"`
				}
				if json.Unmarshal(step.Input, &input) == nil {
					todos = input.Todos
				}
			}
			valid := make([]map[string]any, 0, len(todos))
			for _, todo := range todos {
				if _, ok := todo["content"].(string); ok {
					valid = append(valid, todo)
				}
			}
			if len(valid) == 0 {
				continue
			}
			var identity any = ordinal
			if step.ID != nil {
				identity = *step.ID
			}
			occurrence, _ := json.Marshal([]any{m.ID, identity})
			s.Todo = &TranscriptTodo{valid, string(occurrence)}
			ordinal++
		}
	}
}
