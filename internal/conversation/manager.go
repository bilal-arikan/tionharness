// Package conversation keeps a chat within the model's context window. When the
// running history grows past a token budget, it folds the oldest turns into a
// rolling summary (compaction) and sends only the summary plus the most recent
// turns — so long chats stay cheap and never overflow the context.
package conversation

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Defaults are conservative; override via env for larger-context models.
const (
	defaultMaxTokens  = 12000 // compact when pending history exceeds this
	defaultKeepRecent = 8     // always keep this many newest messages verbatim
)

// compactPrompt asks the model to merge prior context into one running summary.
const compactPrompt = `You maintain a running summary of a conversation. Update the summary below so it captures all durable facts, decisions, and context from the new messages. Keep it concise (under 200 words), third-person, no preamble.

Existing summary:
%s

New messages to fold in:
%s

Updated summary:`

// Manager performs token-budgeted compaction. It is safe to share.
type Manager struct {
	maxTokens  int
	keepRecent int
}

// NewManager builds a manager, reading SWARMGO_MAX_CONTEXT_TOKENS and
// SWARMGO_KEEP_RECENT_MSGS when set.
func NewManager() *Manager {
	return &Manager{
		maxTokens:  envInt("SWARMGO_MAX_CONTEXT_TOKENS", defaultMaxTokens),
		keepRecent: envInt("SWARMGO_KEEP_RECENT_MSGS", defaultKeepRecent),
	}
}

// Prepared is the result of budgeting a session for one turn.
type Prepared struct {
	Summary       string              // rolling summary to inject into the system prompt ("" if none)
	Messages      []providers.Message // the turns to actually send
	ContextTokens int                 // estimated tokens of summary + sent messages
	Compacted     bool                // whether this call folded new messages into the summary
}

// Prepare returns the messages to send for a turn, compacting older history
// into the session summary when the pending history exceeds the token budget.
// provider is used only when a compaction is needed.
func (m *Manager) Prepare(ctx context.Context, database *db.DB, provider providers.Provider, session db.Session, agent db.Agent, history []db.Message) (Prepared, error) {
	summary := session.Summary
	start := session.SummaryMsgCount
	if start > len(history) {
		start = len(history) // defensive: history shorter than recorded (e.g. after edits)
	}
	pending := history[start:]

	compacted := false
	if EstimateTokens(summary, pending) > m.maxTokens && len(pending) > m.keepRecent {
		fold := pending[:len(pending)-m.keepRecent]
		newSummary, err := m.summarize(ctx, provider, agent, summary, fold)
		if err != nil {
			return Prepared{}, err
		}
		summary = newSummary
		newCount := start + len(fold)
		if err := database.SetSessionSummary(ctx, session.ID, summary, newCount); err != nil {
			return Prepared{}, err
		}
		pending = pending[len(fold):]
		compacted = true
	}

	return Prepared{
		Summary:       summary,
		Messages:      toProviderMessages(pending),
		ContextTokens: EstimateTokens(summary, pending),
		Compacted:     compacted,
	}, nil
}

// summarize folds messages into the existing summary via the provider.
func (m *Manager) summarize(ctx context.Context, provider providers.Provider, agent db.Agent, existing string, msgs []db.Message) (string, error) {
	var b strings.Builder
	for _, msg := range msgs {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Text)
		b.WriteString("\n")
	}
	if existing == "" {
		existing = "(none)"
	}
	resp, err := provider.Complete(ctx, providers.Request{
		Model: agent.Model,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: fmt.Sprintf(compactPrompt, existing, b.String())},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

// toProviderMessages maps stored user/assistant turns to provider messages.
func toProviderMessages(msgs []db.Message) []providers.Message {
	out := make([]providers.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role == providers.RoleUser || msg.Role == providers.RoleAssistant {
			out = append(out, providers.Message{Role: msg.Role, Text: msg.Text})
		}
	}
	return out
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
