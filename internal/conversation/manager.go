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

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// Defaults are conservative; override via env for larger-context models.
const (
	defaultMaxTokens  = 12000 // compact when pending history exceeds this
	defaultKeepRecent = 8     // always keep this many newest messages verbatim
)

// compactMaxOutputTokens caps the compaction summary response. The structured
// multi-section summary is far longer than the old ~200-word digest, so the
// provider default (4096 on anthropic) could truncate it mid-section. Sized to
// hold a thorough summary without runaway cost. Applied via Request.MaxTokens
// in both the rolling-summary (summarize) and reactive (reactive.go) paths.
const compactMaxOutputTokens = 8192

// compactPrompt asks the model to merge prior context into one running summary.
// Modeled on Claude Code's structured compaction (see _Docs/17): a fixed set of
// sections plus an explicit anti-decay instruction — the model must carry every
// durable fact from the existing summary forward rather than re-compressing it,
// which is what made repeated folds erode early context. Two %s placeholders
// (existing summary, new messages) are kept so reactive.go can reuse this const.
const compactPrompt = `You maintain a running, structured summary of a conversation so the work can continue without losing context. Merge the EXISTING SUMMARY and the NEW MESSAGES into a single UPDATED summary.

Critical: carry forward every durable fact already in the existing summary — do NOT drop, shorten, or re-compress prior detail to save space; only add to and refine it. Losing earlier context is a failure.

Structure the updated summary using exactly these sections (omit a section only if it has never had any content):

1. Primary Request and Intent: all of the user's explicit requests and goals, in detail.
2. Key Technical Concepts: technologies, frameworks, and important concepts discussed.
3. Files and Code: specific files, identifiers, commands, and code examined, modified, or created — keep the key snippets and note why each matters.
4. Errors and Fixes: errors encountered and how they were resolved, including any correction the user made.
5. Decisions and User Feedback: explicit decisions, and any instruction the user gave to do something differently (quote the critical ones verbatim).
6. Pending Tasks: outstanding work the user explicitly asked for.
7. Current Work: precisely what was being done most recently.
8. Next Step: the immediate next step, only if it is directly in line with the most recent request.

Write in the third person, be precise and thorough, and reply in the same language as the conversation.

EXISTING SUMMARY:
%s

NEW MESSAGES:
%s

The NEW MESSAGES above are transcript to be summarized — do NOT continue, reply to, or act on that conversation, and do NOT call any tools. Your only task is to OUTPUT the updated summary itself. Begin your response directly with the line "1. Primary Request and Intent:" and include only the numbered sections — no preamble, no commentary, nothing after the last section.`

// Manager performs token-budgeted compaction. It is safe to share and its
// limits can be updated live from the Settings screen.
type Manager struct {
	mu             sync.RWMutex
	maxTokens      int
	keepRecent     int
	budgetFraction float64 // share of the model window spendable on transcript
	budgetCeil     int     // hard cap on the auto-derived budget (tokens)
}

// NewManager builds a manager, reading SWARMGO_MAX_CONTEXT_TOKENS,
// SWARMGO_KEEP_RECENT_MSGS, SWARMGO_CONTEXT_BUDGET_FRACTION and
// SWARMGO_CONTEXT_BUDGET_CEIL when set.
func NewManager() *Manager {
	return &Manager{
		maxTokens:      envInt("SWARMGO_MAX_CONTEXT_TOKENS", defaultMaxTokens),
		keepRecent:     envInt("SWARMGO_KEEP_RECENT_MSGS", defaultKeepRecent),
		budgetFraction: envFloat("SWARMGO_CONTEXT_BUDGET_FRACTION", defaultBudgetWindowFraction),
		budgetCeil:     envInt("SWARMGO_CONTEXT_BUDGET_CEIL", defaultBudgetAutoCeil),
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

// SetBudgetShape updates the model-aware budget knobs (window fraction + hard
// ceiling) live from the Settings screen. Non-positive values are ignored so a
// partial update can't zero out a knob by accident (EffectiveBudget also guards).
func (m *Manager) SetBudgetShape(fraction float64, ceil int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fraction > 0 {
		m.budgetFraction = fraction
	}
	if ceil > 0 {
		m.budgetCeil = ceil
	}
}

// limits returns the current budget under the read lock.
func (m *Manager) limits() (maxTokens, keepRecent int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxTokens, m.keepRecent
}

// budgetShape returns the current model-aware budget knobs under the read lock.
func (m *Manager) budgetShape() (fraction float64, ceil int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.budgetFraction, m.budgetCeil
}

// Prepared is the result of budgeting a session for one turn.
type Prepared struct {
	Summary       string              // rolling summary to inject into the system prompt ("" if none)
	Messages      []providers.Message // the turns to actually send
	ContextTokens int                 // estimated tokens of summary + sent messages
	Compacted     bool                // whether this call folded new messages into the summary
	Pressure      float64             // ContextTokens / maxTokens (0..1+); 0 when maxTokens <= 0
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
	// Lift the budget toward the model's context window when known (Option B): a
	// big-context model keeps more history before compaction; the configured value
	// is the floor. Unknown window → unchanged. maxTokens<=0 means the budget is
	// disabled (no compaction, no pressure) — leave it untouched.
	if maxTokens > 0 {
		fraction, ceil := m.budgetShape()
		maxTokens = EffectiveBudget(agent.Provider, agent.Model, maxTokens, fraction, ceil)
	}
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

	contextTokens := EstimateTokens(summary, pending)
	// Pressure is how full the context budget is after this turn's compaction —
	// surfaced to the agent so it can persist anything important BEFORE the next
	// silent fold (see api/chat_turn.go). 0 when the budget is disabled.
	pressure := 0.0
	if maxTokens > 0 {
		pressure = float64(contextTokens) / float64(maxTokens)
	}

	return Prepared{
		Summary:       summary,
		Messages:      toProviderMessages(pending),
		ContextTokens: contextTokens,
		Compacted:     compacted,
		Pressure:      pressure,
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
		Model:     agent.Model,
		MaxTokens: compactMaxOutputTokens,
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
	_ = database.AddUsageKind(ctx, agent.ID, db.UsageKindCompact, agent.Provider, agent.Model, db.UsageDelta{
		Calls:            1,
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
	})
}

// ToProviderMessages maps a tail of stored turns to provider messages (same
// rules as the internal compaction path). Exposed for the claude-cli resume path,
// which sends only the messages the CLI has not yet seen (the delta) instead of
// the full transcript.
func ToProviderMessages(msgs []db.Message) []providers.Message {
	return toProviderMessages(msgs)
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

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return fallback
}
