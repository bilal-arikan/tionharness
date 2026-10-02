package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

type decisionEvidenceMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type decisionEvidenceState struct {
	Unavailable        bool                      `json:"unavailable,omitempty"`
	Title              string                    `json:"title,omitempty"`
	InitialRequest     string                    `json:"initialRequest,omitempty"`
	Summary            string                    `json:"summary,omitempty"`
	RecentConversation []decisionEvidenceMessage `json:"recentConversation"`
	ProtectedContext   []map[string]string       `json:"protectedContext"`
}

// decisionEvidence adds bounded task evidence, not tool output or unrestricted
// system instructions. Newer messages are retained in chronological order so
// corrections can supersede older requests. Hub redaction still applies.
func (r *Runtime) decisionEvidence(ctx context.Context, summary string) decisionEvidenceState {
	e := decisionEvidenceState{RecentConversation: []decisionEvidenceMessage{}, ProtectedContext: []map[string]string{}}
	if r.db == nil || SessionIDFrom(ctx) == "" {
		e.Summary = policyText(summary, 4000)
		return e
	}
	sid := SessionIDFrom(ctx)
	if session, err := r.db.GetSession(ctx, sid); err == nil {
		e.Title = policyText(session.Title, 200)
		if summary == "" {
			summary = session.Summary
		}
	} else {
		e.Unavailable = true
	}
	e.Summary = policyText(summary, 4000)
	if err := r.db.StreamMessages(ctx, sid, func(m db.Message) bool {
		if m.Role != "user" || strings.TrimSpace(m.Text) == "" {
			return true
		}
		e.InitialRequest = policyText(m.Text, 2000)
		return false
	}); err != nil {
		e.Unavailable = true
	}
	if messages, _, err := r.db.ListMessagesTail(ctx, sid, 16); err == nil {
		remaining := 6000
		for i := len(messages) - 1; i >= 0 && remaining > 0; i-- {
			m := messages[i]
			if (m.Role != "user" && m.Role != "assistant") || strings.TrimSpace(m.Text) == "" {
				continue
			}
			text := policyText(m.Text, min(remaining, 2000))
			e.RecentConversation = append(e.RecentConversation, decisionEvidenceMessage{Role: m.Role, Text: text})
			remaining -= len(text)
		}
		for i, j := 0, len(e.RecentConversation)-1; i < j; i, j = i+1, j-1 {
			e.RecentConversation[i], e.RecentConversation[j] = e.RecentConversation[j], e.RecentConversation[i]
		}
	} else {
		e.Unavailable = true
	}
	if state, err := r.db.ReadSessionDecisions(ctx, sid); err == nil {
		pins := []db.DecisionMemory{}
		for _, m := range state.Memories {
			if m.Pinned || m.Mandatory {
				pins = append(pins, m)
			}
		}
		e.ProtectedContext = r.decisionMemoryEvidence(ctx, pins, 2000)
	}
	return e
}

// Canonical message text matters when labels omit the actual constraint.
// Missing canonical sources never fall back to a stale cached text copy.
func (r *Runtime) decisionMemoryEvidence(ctx context.Context, memories []db.DecisionMemory, budget int) []map[string]string {
	rows := []map[string]string{}
	for i, m := range memories {
		if budget <= 0 {
			break
		}
		text := m.Text
		if m.SourceID != "" {
			text = ""
			if r.db != nil {
				if source, err := r.db.FindMessage(ctx, SessionIDFrom(ctx), m.SourceID); err == nil {
					text = source.Text
				}
			}
		}
		text = policyText(text, min(4000, budget/(len(memories)-i)))
		rows = append(rows, map[string]string{"key": m.Key, "label": policyText(m.Label, 140), "text": text})
		budget -= len(text)
	}
	return rows
}
