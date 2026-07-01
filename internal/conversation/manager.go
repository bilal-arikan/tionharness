// Package conversation keeps a chat within the model's context window. When the
// running history grows past a token budget, it folds the oldest turns into a
// rolling summary (compaction) and sends only the summary plus the most recent
// turns — so long chats stay cheap and never overflow the context.
package conversation

import (
	"context"
	"fmt"
	"log/slog"
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

// CompactionPromptText returns the conversation-compaction prompt as
// human-readable reference text — the runtime %s data slots (existing summary /
// new messages) are shown as labels rather than filled in. Exposed so the UI can
// display the ACTUAL summarization prompt read-only (it is not user-editable: the
// structure is deliberately fixed, see the comment on compactPrompt).
func CompactionPromptText() string {
	return fmt.Sprintf(compactPrompt, "‹the running summary so far›", "‹the new messages to fold in›")
}

// CompactPromptDefault returns the compiled-in compaction prompt template RAW
// (with its two %s slots: existing summary, new messages). It seeds the editable
// per-workspace "compact" runtime prompt and is the fallback when that file is
// missing, blank, or malformed. Kept distinct from CompactionPromptText (which
// fills the slots with labels for read-only display).
func CompactPromptDefault() string { return compactPrompt }

// compactPromptCtxKey carries a per-workspace compaction template on the turn
// context so the shared (global) Manager and the package-level compaction core
// can honor a workspace's edited "compact" prompt without a per-workspace Manager.
type compactPromptCtxKey struct{}

// WithCompactPrompt returns a context carrying a per-workspace compaction prompt
// template. Empty input is a no-op (the default stays in force). See
// compactPromptFromCtx for the validation applied when it is read back.
func WithCompactPrompt(ctx context.Context, tmpl string) context.Context {
	if strings.TrimSpace(tmpl) == "" {
		return ctx
	}
	return context.WithValue(ctx, compactPromptCtxKey{}, tmpl)
}

// compactPromptFromCtx returns a VALID compaction template from ctx or the
// compiled-in default. Validity = exactly the two %s slots and NO other % verb,
// so a user's edit (e.g. a stray "%" or a dropped slot) can never make
// fmt.Sprintf emit a "%!"-marked, broken prompt — it silently falls back instead.
func compactPromptFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(compactPromptCtxKey{}).(string); ok {
		if strings.Count(v, "%s") == 2 && strings.Count(v, "%") == 2 {
			return v
		}
	}
	return compactPrompt
}

// Manager performs token-budgeted compaction. It is safe to share and its
// limits can be updated live from the Settings screen.
type Manager struct {
	mu             sync.RWMutex
	maxTokens      int
	keepRecent     int
	budgetFraction float64 // share of the model window spendable on transcript
	budgetCeil     int     // hard cap on the auto-derived budget (tokens)
	logger         *slog.Logger // optional: compaction events to the in-app Logs (nil-safe)
}

// SetLogger attaches a logger so the routine budgeted fold (and manual /compact)
// surface in the in-app Logs screen — the single most useful context event was
// previously visible only in the chat response, never logged. Optional/nil-safe.
func (m *Manager) SetLogger(l *slog.Logger) {
	m.mu.Lock()
	m.logger = l
	m.mu.Unlock()
}

// log emits at the given level via the attached logger, if any. Nil-safe.
func (m *Manager) log(level slog.Level, msg string, args ...any) {
	m.mu.RLock()
	l := m.logger
	m.mu.RUnlock()
	if l != nil {
		l.Log(context.Background(), level, msg, args...)
	}
}

// NewManager builds a manager, reading SWARMGO_MAX_CONTEXT_TOKENS,
// SWARMGO_KEEP_RECENT_MSGS, SWARMGO_CONTEXT_BUDGET_FRACTION and
// SWARMGO_CONTEXT_BUDGET_CEIL when set.
func NewManager() *Manager {
	return &Manager{
		maxTokens:      envInt("SWARMGO_MAX_CONTEXT_TOKENS", defaultMaxTokens),
		keepRecent:     envInt("SWARMGO_KEEP_RECENT_MSGS", defaultKeepRecent),
		budgetFraction: envFloat("SWARMGO_CONTEXT_BUDGET_FRACTION", 0), // 0 = auto (per-family adaptive)
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
// ceiling) live from the Settings screen. A fraction of exactly 0 is a meaningful
// value ("auto" → per-family adaptive, see EffectiveBudget) and is stored as-is;
// only a negative (invalid) fraction is ignored. ceil<=0 is ignored so a partial
// update can't zero the ceiling by accident (validate clamps it to >=8000 anyway).
func (m *Manager) SetBudgetShape(fraction float64, ceil int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fraction >= 0 {
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
	start := clampStart(session.SummaryMsgCount, len(history))
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
	if before := EstimateTokens(summary, pending); before > maxTokens {
		if fold, keepTail, newCount, ok := foldBoundary(history, start, keepRecent); ok {
			newSummary, err := m.summarize(ctx, database, provider, agent, summary, fold)
			if err != nil {
				return Prepared{}, err
			}
			summary = newSummary
			if err := database.SetSessionSummary(ctx, session.ID, summary, newCount); err != nil {
				return Prepared{}, err
			}
			pending = keepTail
			compacted = true
			m.log(slog.LevelInfo, "context compacted (rolling summary fold)",
				"session", session.ID, "agent", agent.ID,
				"folded_msgs", len(fold), "before_tokens", before,
				"after_tokens", EstimateTokens(summary, pending), "budget", maxTokens)
		}
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
	start := clampStart(session.SummaryMsgCount, len(history))
	_, keepRecent := m.limits()
	fold, _, newCount, ok := foldBoundary(history, start, keepRecent)
	if !ok {
		return 0, summary, nil // not enough to compact
	}
	newSummary, err := m.summarize(ctx, database, provider, agent, summary, fold)
	if err != nil {
		return 0, "", err
	}
	if err := database.SetSessionSummary(ctx, session.ID, newSummary, newCount); err != nil {
		return 0, "", err
	}
	m.log(slog.LevelInfo, "context compacted (manual /compact)",
		"session", session.ID, "agent", agent.ID, "folded_msgs", len(fold))
	return len(fold), newSummary, nil
}

// clampStart caps a recorded SummaryMsgCount at the current history length —
// defensive against a history shorter than recorded (e.g. after message edits).
func clampStart(start, n int) int {
	if start > n {
		return n
	}
	return start
}

// foldBoundary computes the compaction split for a session's pending history.
// Given the start index (clamped SummaryMsgCount) and keepRecent, it returns the
// older slice to fold into the summary (fold), the newest turns to keep verbatim
// (keepTail), and the resulting SummaryMsgCount after the fold (newCount). ok is
// false when there are not enough pending messages to fold (<= keepRecent), in
// which case nothing should be compacted.
func foldBoundary(history []db.Message, start, keepRecent int) (fold, keepTail []db.Message, newCount int, ok bool) {
	start = clampStart(start, len(history))
	pending := history[start:]
	if len(pending) <= keepRecent {
		return nil, pending, start, false
	}
	cut := len(pending) - keepRecent
	return pending[:cut], pending[cut:], start + cut, true
}

// summarize folds stored messages into the existing summary via the provider.
func (m *Manager) summarize(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, existing string, msgs []db.Message) (string, error) {
	return summarizeRendered(ctx, database, provider, agent, existing, renderDBMessages(msgs))
}

// summarizeRendered is the single compaction core shared by the rolling-summary
// path (Manager.summarize) and the reactive mid-loop path (reactive.go): it folds
// an already-rendered transcript into the existing summary with the structured
// compactPrompt and records the call's token usage under UsageKindCompact —
// otherwise compaction spend would be invisible to the daily meter. An empty
// existing summary is rendered as "(none)".
func summarizeRendered(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, existing, rendered string) (string, error) {
	if existing == "" {
		existing = "(none)"
	}
	resp, err := provider.Complete(ctx, providers.Request{
		Model:     agent.Model,
		MaxTokens: compactMaxOutputTokens,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: fmt.Sprintf(compactPromptFromCtx(ctx), existing, rendered)},
		},
	})
	if err != nil {
		return "", err
	}
	recordCompaction(ctx, database, agent, resp.Usage)
	return strings.TrimSpace(resp.Text), nil
}

// renderDBMessages flattens stored turns to the "role: text" transcript the
// compaction prompt expects.
func renderDBMessages(msgs []db.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// recordCompaction attributes a compaction provider call's token usage to the
// agent's daily counters under UsageKindCompact, tagged with the agent's
// provider+model for cost. Nil-safe and non-fatal: a counting failure must
// never break the turn it was compacting for.
func recordCompaction(ctx context.Context, database *db.DB, agent db.Agent, u providers.Usage) {
	if database == nil {
		return
	}
	_ = database.AddUsageKind(ctx, agent.ID, db.UsageKindCompact, agent.Provider, agent.Model, db.DeltaFromUsage(1, u))
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
