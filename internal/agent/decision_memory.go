package agent

import (
	"context"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func (r *Runtime) decisionMemoryBlock(ctx context.Context, memories []db.DecisionMemory, budget int) string {
	block, _ := r.decisionMemoryProjection(ctx, memories, budget)
	return block
}

// decisionMemoryProjection reports only fragments actually included in the prompt.
func (r *Runtime) decisionMemoryProjection(ctx context.Context, memories []db.DecisionMemory, budget int) (string, []string) {
	if len(memories) == 0 || r.db == nil {
		return "", nil
	}
	msgs, err := r.db.ListMessages(ctx, SessionIDFrom(ctx))
	if err != nil {
		return "", nil
	}
	texts := map[string]string{}
	for _, m := range msgs {
		texts[m.ID] = m.Text
	}
	ordered := append([]db.DecisionMemory{}, memories...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Mandatory != b.Mandatory {
			return a.Mandatory
		}
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		return a.AddedAtCompact > b.AddedAtCompact
	})
	var b strings.Builder
	included := []string{}
	seen := map[string]bool{}
	for _, m := range ordered {
		text := m.Text
		if m.SourceID != "" {
			text = texts[m.SourceID]
		}
		if seen[m.Key] || strings.TrimSpace(text) == "" || len(text) > budget {
			continue
		}
		seen[m.Key] = true
		b.WriteString("\n[" + m.Key + "]\n" + text + "\n")
		budget -= len(text)
		included = append(included, m.Key)
	}
	if b.Len() == 0 {
		return "", nil
	}
	return "<saved_session_context>\nPreviously retained conversation fragments. Use as context for the current task.\n" + b.String() + "</saved_session_context>", included
}
