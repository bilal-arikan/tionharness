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
	"sync"

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

// Manager performs token-budgeted compaction. It is safe to share and its
// limits can be updated live from the Settings screen.
type Manager struct {
	mu         sync.RWMutex
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

// SetLimits updates the compaction budget at runtime. Non-positive values are
// ignored so a partial update can't disable compaction by accident.
func (m *Manager) SetLimits(maxTokens, keepRecent int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if maxTokens > 0 {
		m.maxTokens = maxTokens
	}
	if keepRecent > 0 {
		m.keepRecent = keepRecent
	}
}

// limits returns the current budget under the read lock.
func (m *Manager) limits() (maxTokens, keepRecent int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxTokens, m.keepRecent
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

	maxTokens, keepRecent := m.limits()
	compacted := false
	if EstimateTokens(summary, pending) > maxTokens && len(pending) > keepRecent {
		fold := pending[:len(pending)-keepRecent]
		newSummary, err := m.summarize(ctx, database, provider, agent, summary, fold)
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

// ForceCompact folds all but the most recent keepRecent messages into the
// rolling summary regardless of the token budget — the manual "/compact" chat
// command. Returns how many messages were folded and the updated summary; folds
// nothing (folded == 0) when there are not enough pending messages.
func (m *Manager) ForceCompact(ctx context.Context, database *db.DB, provider providers.Provider, session db.Session, agent db.Agent, history []db.Message) (folded int, summary string, err error) {
	summary = session.Summary
	start := session.SummaryMsgCount
	if start > len(history) {
		start = len(history)
	}
	pending := history[start:]

	_, keepRecent := m.limits()
	if len(pending) <= keepRecent {
		return 0, summary, nil // not enough to compact
	}
	fold := pending[:len(pending)-keepRecent]
	newSummary, err := m.summarize(ctx, database, provider, agent, summary, fold)
	if err != nil {
		return 0, "", err
	}
	newCount := start + len(fold)
	if err := database.SetSessionSummary(ctx, session.ID, newSummary, newCount); err != nil {
		return 0, "", err
	}
	return len(fold), newSummary, nil
}

// summarize folds messages into the existing summary via the provider. The
// compaction call spends real tokens, so its usage is recorded against the
// agent under UsageKindCompact — otherwise rolling-summary spend would be
// invisible to the daily meter and budget planning.
func (m *Manager) summarize(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, existing string, msgs []db.Message) (string, error) {
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
	recordCompaction(ctx, database, agent, resp.Usage)
	return strings.TrimSpace(resp.Text), nil
}

// recordCompaction attributes a compaction provider call's token usage to the
// agent's daily counters under UsageKindCompact, tagged with the agent's
// provider+model for cost. Nil-safe and non-fatal: a counting failure must
// never break the turn it was compacting for.
func recordCompaction(ctx context.Context, database *db.DB, agent db.Agent, u providers.Usage) {
	if database == nil {
		return
	}
	_ = database.AddUsageKind(ctx, agent.ID, db.UsageKindCompact, agent.Provider, agent.Model, 1, u.InputTokens, u.OutputTokens)
}

// toProviderMessages maps stored user/assistant turns to provider messages,
// folding any user-message attachments into the text the model sees.
func toProviderMessages(msgs []db.Message) []providers.Message {
	out := make([]providers.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role == providers.RoleUser || msg.Role == providers.RoleAssistant {
			out = append(out, providers.Message{Role: msg.Role, Text: withAttachments(msg)})
		}
	}
	return out
}

// InlineAttachments folds an attachment list into a piece of text using the same
// block format chat turns use (text/code inlined verbatim; binary/image listed by
// read_file path). Exposed so non-chat callers (e.g. flow runs) can give their
// agents the same attachment context. Returns text unchanged when atts is empty.
func InlineAttachments(text string, atts []db.Attachment) string {
	return withAttachments(db.Message{Text: text, Attachments: atts})
}

// withAttachments appends an "Attachments" block to a user message's text. Text
// and code attachments are inlined verbatim (the model reads them directly);
// binary/image attachments are listed by relative path so an agent with the
// read_file tool can open them from the workspace sandbox.
func withAttachments(msg db.Message) string {
	if len(msg.Attachments) == 0 {
		return msg.Text
	}
	var b strings.Builder
	b.WriteString(msg.Text)
	b.WriteString("\n\n## Attachments\n")
	for _, a := range msg.Attachments {
		if a.TextContent != "" {
			fmt.Fprintf(&b, "\n### %s (%s)\n```\n%s\n```\n", a.Name, a.Kind, a.TextContent)
			continue
		}
		if a.RelPath != "" {
			fmt.Fprintf(&b, "- %s (%s, %d bytes) — read_file path: %s\n", a.Name, a.Kind, a.Size, a.RelPath)
		} else {
			fmt.Fprintf(&b, "- %s (%s)\n", a.Name, a.Kind)
		}
	}
	return strings.TrimSpace(b.String())
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
