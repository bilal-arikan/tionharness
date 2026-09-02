// Package conversation keeps a chat within the model's context window. When the
// running history grows past a token budget, it folds the oldest turns into a
// rolling summary (compaction) and sends only the summary plus the most recent
// turns — so long chats stay cheap and never overflow the context.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
	"github.com/bilal-arikan/tionharness/internal/providers"
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

// The compaction prompt template lives in the central prompt registry
// (internal/prompts, key "compact"): a fixed set of sections plus an explicit
// anti-decay instruction — the model must carry every durable fact from the
// existing summary forward rather than re-compressing it, which is what made
// repeated folds erode early context. Two named placeholders ({{summary}},
// {{messages}}) mark the data slots; reactive.go reuses the same template.

// CompactionPromptText returns the conversation-compaction prompt as
// human-readable reference text — the {{summary}}/{{messages}} data slots are
// shown as labels rather than filled in. Exposed so the UI can display the
// ACTUAL summarization prompt read-only.
func CompactionPromptText() string {
	return prompts.Render(prompts.Default("compact"), map[string]string{
		"summary":  "‹the running summary so far›",
		"messages": "‹the new messages to fold in›",
	})
}

// CompactPromptDefault returns the compiled-in compaction prompt template RAW
// (with its {{summary}}/{{messages}} slots). It is the fallback when the
// per-workspace "compact" override is missing, blank, or malformed.
func CompactPromptDefault() string { return prompts.Default("compact") }

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
// compiled-in default. Validity = registry validation for the "compact" key
// (both {{summary}} and {{messages}} present), so a user's edit that drops a
// slot can never produce a prompt missing its data — it silently falls back
// instead. The upstream resolver (Runtime.readPrompt) already validates; this
// is defense in depth for direct WithCompactPrompt callers.
func compactPromptFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(compactPromptCtxKey{}).(string); ok {
		if prompts.Validate("compact", v) == nil {
			return v
		}
	}
	return prompts.Default("compact")
}

// preCompactCtxKey carries a callback fired just before Prepare folds history
// into the rolling summary — the seam the API layer uses to run PreCompact
// lifecycle hooks without conversation importing agent (which would cycle).
type preCompactCtxKey struct{}

// WithPreCompact returns a context carrying a callback invoked once, right before
// a compaction runs, with the trigger ("auto" for the routine fold, "manual" for
// an explicit /compact).
// nil is a no-op.
func WithPreCompact(ctx context.Context, fn func(trigger string)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, preCompactCtxKey{}, fn)
}

// firePreCompact invokes the ctx-carried PreCompact callback if present.
func firePreCompact(ctx context.Context, trigger string) {
	if fn, ok := ctx.Value(preCompactCtxKey{}).(func(string)); ok && fn != nil {
		fn(trigger)
	}
}

// contextOverheadCtxKey carries the estimated token cost of the NON-message
// context shipped on every turn — the static system prefix, skill/tool catalogs,
// eager tool schemas and the artifact block. Prepare folds only the message
// history, but the model actually receives messages PLUS this fixed overhead, so
// the fold decision must budget for both: a big static prefix (a claude-cli
// coordinator with many MCP tool schemas) can hold the message-only estimate
// under the budget forever while the real footprint sits far above it, so no fold
// ever fires. Threading the overhead in makes the fold trigger against the true
// footprint — the same basis the session_info context meter reports, so the meter
// and the engine finally agree on both sides of the ratio.
type contextOverheadCtxKey struct{}

// WithContextOverhead returns a context carrying the non-message token overhead
// for the upcoming turn. Non-positive input is a no-op (overhead 0 → unchanged
// message-only budgeting, so direct callers and tests behave exactly as before).
func WithContextOverhead(ctx context.Context, tokens int) context.Context {
	if tokens <= 0 {
		return ctx
	}
	return context.WithValue(ctx, contextOverheadCtxKey{}, tokens)
}

// contextOverheadFrom returns the non-message overhead stamped on the context, or
// 0 when none was set.
func contextOverheadFrom(ctx context.Context) int {
	if v, ok := ctx.Value(contextOverheadCtxKey{}).(int); ok && v > 0 {
		return v
	}
	return 0
}

// contextOverheadStepBaseCtxKey carries the transcript index from which the
// overhead above includes the persisted Steps trace of a warm CLI thread. It is
// what makes the overhead FOLD-AWARE: the step term is charged for
// history[base:], so when Prepare folds history[:newCount] into the summary the
// steps of the folded messages leave the footprint with them. Without it Prepare
// reported the post-fold footprint with the pre-fold overhead — on a real session
// that meant an "after" of 100201 tokens against a 70000 budget where the true
// figure was 24778, and the same stale number drove the pressure ratio.
type contextOverheadStepBaseCtxKey struct{}

// WithContextOverheadStepBase records that the overhead carried on ctx includes
// the persisted Steps of history[base:]. A negative base (no warm CLI thread, so
// no step trace was counted) is stored as-is and simply yields no deduction.
func WithContextOverheadStepBase(ctx context.Context, base int) context.Context {
	return context.WithValue(ctx, contextOverheadStepBaseCtxKey{}, base)
}

// contextOverheadStepBaseFrom returns the step baseline stamped on the context.
// ok is false when the caller supplied no baseline — the overhead is then treated
// as opaque and left untouched by a fold, exactly as before.
func contextOverheadStepBaseFrom(ctx context.Context) (base int, ok bool) {
	v, ok := ctx.Value(contextOverheadStepBaseCtxKey{}).(int)
	return v, ok
}

// foldedStepOverhead reports how much of the overhead's persisted-Steps term
// belongs to messages this fold just moved into the summary — the amount that
// must leave the post-fold footprint. base is the index the overhead started
// charging steps from and newCount the new summary boundary; the overlap is
// history[max(base,0):newCount]. Returns 0 when the caller stamped no baseline,
// when there is no warm thread (base < 0) or when the ranges do not overlap.
func foldedStepOverhead(ctx context.Context, history []db.Message, newCount int) (int, error) {
	base, ok := contextOverheadStepBaseFrom(ctx)
	if !ok || base < 0 {
		return 0, nil
	}
	if base > newCount {
		return 0, nil
	}
	newCount = clampStart(newCount, len(history))
	if base > newCount {
		return 0, nil
	}
	return EstimatePersistedStepTokens(history[base:newCount])
}

// Manager performs token-budgeted compaction. It is safe to share and its
// limits can be updated live from the Settings screen.
type Manager struct {
	mu             sync.RWMutex
	maxTokens      int
	keepRecent     int
	budgetFraction float64      // share of the model window spendable on transcript
	budgetCeil     int          // hard cap on the auto-derived budget (tokens)
	logger         *slog.Logger // optional: compaction events to the in-app Logs (nil-safe)
	// Auto-compaction strategy ("" = rolling). Accessors live in autocompact.go.
	autoCompactMode string
	// Per-session history length at which native compaction was last attempted —
	// the anti-loop guard for the native path (see claimNativeAttempt in
	// nativecompact.go). Keyed by session because one Manager serves every session.
	lastNativeCompactAt map[string]int
	// Per-session deadline until which the AUTOMATIC rolling fold stands down
	// after its summarizer call failed (see foldfailure.go). Same keying rationale
	// as lastNativeCompactAt.
	foldFailedUntil map[string]time.Time
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

// NewManager builds a manager, reading TIONHARNESS_MAX_CONTEXT_TOKENS,
// TIONHARNESS_KEEP_RECENT_MSGS, TIONHARNESS_CONTEXT_BUDGET_FRACTION and
// TIONHARNESS_CONTEXT_BUDGET_CEIL when set.
func NewManager() *Manager {
	return &Manager{
		maxTokens:      envInt("TIONHARNESS_MAX_CONTEXT_TOKENS", defaultMaxTokens),
		keepRecent:     envInt("TIONHARNESS_KEEP_RECENT_MSGS", defaultKeepRecent),
		budgetFraction: envFloat("TIONHARNESS_CONTEXT_BUDGET_FRACTION", 0), // 0 = auto (per-family adaptive)
		budgetCeil:     envInt("TIONHARNESS_CONTEXT_BUDGET_CEIL", defaultBudgetAutoCeil),
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

// Compaction trigger tags. They name WHY a fold ran and travel all the way to
// the on-screen compaction step and the debug journal, so the same vocabulary is
// used in both places.
const (
	TriggerAuto     = "auto"     // routine budgeted fold in Prepare
	TriggerManual   = "manual"   // explicit /compact (ForceCompact)
	TriggerReactive = "reactive" // mid-turn overflow recovery (CompactInFlightMessages)
)

// Compaction modes. They name WHO compacted: the built-in rolling-summary fold,
// or the CLI provider's own native compactor (which rebuilds the provider's
// window and leaves the TionHarness transcript untouched — hence FoldedMsgs 0 and
// an unchanged token estimate on that path).
const (
	ModeRolling = "rolling"
	ModeNative  = "native"
)

// Compaction describes one fold of history into the rolling summary: how many
// messages went in, the context footprint (messages + fixed overhead) before and
// after, and what triggered it. Zero value = no fold happened.
type Compaction struct {
	FoldedMsgs   int    // messages folded into the summary
	BeforeTokens int    // estimated context tokens before the fold
	AfterTokens  int    // estimated context tokens after the fold
	Trigger      string // TriggerAuto | TriggerManual | TriggerReactive
	Mode         string // ModeRolling | ModeNative ("" on records written before modes existed)
}

// Prepared is the result of budgeting a session for one turn.
type Prepared struct {
	Summary       string              // rolling summary to inject into the system prompt ("" if none)
	Messages      []providers.Message // the turns to actually send
	ContextTokens int                 // estimated tokens of summary + sent messages
	Compacted     bool                // whether this call folded new messages into the ROLLING summary
	// NativeCompacted reports that the CLI provider compacted its own window for
	// this turn instead of the rolling fold. Deliberately a separate flag from
	// Compacted: callers react to a rolling fold by dropping the warm CLI session
	// and starting the provider fresh, which is exactly what must NOT happen after
	// a native compaction (it would throw away the window the CLI just rebuilt).
	NativeCompacted bool
	Fold            Compaction // the compaction this call performed (zero value unless Compacted or NativeCompacted)
	Pressure        float64    // ContextTokens / maxTokens (0..1+); 0 when maxTokens <= 0
	// FoldFailed reports that this turn was over budget, a rolling fold was
	// attempted, and its summarizer call failed — so the turn runs UNCOMPACTED.
	// It is not an error: the turn is still valid, just larger than intended, and
	// a mid-turn overflow is still caught by the reactive fold
	// (CompactInFlightMessages). Callers MUST surface it; a silently oversized
	// context is exactly the failure this flag exists to prevent.
	FoldFailed bool
	// FoldError is the provider's message for the failed summarizer call, for the
	// on-screen warning. Empty unless FoldFailed.
	FoldError string
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
	// Non-message context shipped every turn (static prefix + tool/skill catalogs +
	// eager tool schemas + artifact block). The model receives messages PLUS this,
	// so the fold must budget for both — otherwise a large fixed prefix keeps the
	// message-only estimate under budget forever and no fold ever fires while the
	// real footprint runs over. 0 when the caller did not supply an estimate, which
	// preserves the previous message-only behaviour exactly.
	overhead := contextOverheadFrom(ctx)
	compacted := false
	nativeCompacted := false
	foldFailed := false
	foldError := ""
	var foldStat Compaction
	if before := EstimateTokens(summary, pending); before+overhead > maxTokens {
		// Native-first strategies: ask the CLI provider to compact its own window
		// before folding anything ourselves. Only nil means it happened; any error
		// (unsupported provider, no resumable thread, a failed call) falls through to
		// the rolling fold below, so the "rolling" mode — the default — runs exactly
		// the code path it ran before this branch existed.
		if mode := m.AutoCompactMode(); mode == AutoCompactNative || mode == AutoCompactAuto {
			// claimNativeAttempt is the anti-loop guard: native compaction does not
			// shrink OUR transcript, so without it the same over-budget footprint would
			// re-trigger it every turn. See nativecompact.go.
			if m.claimNativeAttempt(session.ID, start) {
				nativeResult, nativeErr := fireNativeCompact(ctx)
				if nativeErr == nil {
					nativeCompacted = true
					// The transcript is untouched, so both sides report the same footprint;
					// Mode is what tells this apart from a rolling fold downstream.
					foldStat = Compaction{
						BeforeTokens: before + overhead,
						AfterTokens:  before + overhead,
						Trigger:      TriggerAuto,
						Mode:         ModeNative,
					}
					m.log(slog.LevelInfo, "context compacted (CLI native)",
						"session", session.ID, "agent", agent.ID,
						"before_tokens", before, "overhead_tokens", overhead, "budget", maxTokens)
					if !nativeResult.SuccessDebugPersisted {
						m.recordNativeCompactionDebug(database, session.ID, agent.ID, before+overhead, maxTokens)
					}
				} else {
					// The claim is spent whether or not the attempt worked, so record what
					// consumed it — otherwise the NEXT over-budget turn's native_skipped
					// has no visible cause.
					m.recordNativeCompactDebug(database, session.ID, agent.ID,
						"claim_consumed", nativeCompactErrorKind(nativeErr))
				}
			} else {
				m.recordNativeCompactDebug(database, session.ID, agent.ID, "native_skipped", "")
			}
			// "auto" promises native-first with a rolling safety net; when native did
			// not happen the fold below IS that net, and nothing else says so.
			if mode == AutoCompactAuto && !nativeCompacted {
				m.recordNativeCompactDebug(database, session.ID, agent.ID, "native_fallback_rolling", "")
			}
		}
		if fold, keepTail, newCount, ok := foldBoundary(history, start, keepRecent); ok && !nativeCompacted {
			if until, cooling := m.foldCoolingDown(session.ID); cooling {
				// A fold failed for this session moments ago and nothing has changed
				// since, so retrying now would just lose another summarizer call. Skip
				// it; compacted stays false, so the pressure warning below still fires
				// and the over-budget state remains visible. See foldfailure.go.
				m.recordFoldCooldownDebug(database, session.ID, agent.ID, until)
			} else if res, err := m.applyRollingFold(ctx, rollingFoldInput{
				database: database, provider: provider, session: session, agent: agent,
				history: history, summary: summary, fold: fold, keepTail: keepTail,
				newCount: newCount, before: before, overhead: overhead, maxTokens: maxTokens,
			}); err != nil {
				// Only the summarizer call is recoverable. A failed summary means the
				// turn runs uncompacted — larger than intended, but alive; killing the
				// turn instead would let one transient 429 on the fold provider destroy
				// a user's turn, and the mid-turn overflow it risks is already caught by
				// the reactive fold (reactive.go). Everything else applyRollingFold can
				// fail at (writing the summary, re-measuring the step overhead) is local
				// state this turn depends on, so those stay fatal.
				if !errors.Is(err, errFoldSummary) {
					return Prepared{}, err
				}
				foldFailed = true
				foldError = err.Error()
				until := m.noteFoldFailure(session.ID)
				m.recordFoldFailureDebug(database, session.ID, agent.ID, until)
				m.log(slog.LevelWarn, "context fold failed; turn continues uncompacted",
					"session", session.ID, "agent", agent.ID,
					"before_tokens", before, "overhead_tokens", overhead, "budget", maxTokens,
					"error", err)
			} else {
				m.clearFoldCooldown(session.ID)
				summary = res.summary
				pending = res.pending
				overhead = res.overhead
				foldStat = res.stat
				compacted = true
			}
		}
	}

	contextTokens := EstimateTokens(summary, pending)
	// Pressure is how full the context budget is after this turn's compaction —
	// surfaced to the agent so it can persist anything important BEFORE the next
	// silent fold (see api/chat_turn.go). Measured against the SAME footprint the
	// fold gates on (messages + fixed overhead) so pressure reaches 100% exactly
	// when the engine would fold. 0 when the budget is disabled.
	pressure := 0.0
	if maxTokens > 0 {
		pressure = float64(contextTokens+overhead) / float64(maxTokens)
		// Early warning: the turn fits, but only just. Journalling it makes the
		// approach to the threshold readable BEFORE the fold, which is the only
		// way to tell "the gate is working and about to fire" apart from "the gate
		// never sees the real footprint" — the failure this figure went unread
		// through. A turn that actually folded already has its compaction event, so
		// it needs no second warning.
		if !compacted && !nativeCompacted && pressure >= pressureWarnRatio {
			m.recordPressureDebug(database, session.ID, agent.ID, contextTokens+overhead, maxTokens, pressure)
		}
	}

	return Prepared{
		Summary:         summary,
		Messages:        toProviderMessages(ctx, pending),
		ContextTokens:   contextTokens,
		Compacted:       compacted,
		NativeCompacted: nativeCompacted,
		Fold:            foldStat,
		Pressure:        pressure,
		FoldFailed:      foldFailed,
		FoldError:       foldError,
	}, nil
}

func (m *Manager) recordNativeCompactionDebug(database *db.DB, sessionID, agentID string, usedTokens, budget int) {
	if database == nil || sessionID == "" {
		return
	}
	detail := fmt.Sprintf("CLI native compaction · %d tokens over budget basis", usedTokens)
	if budget > 0 {
		detail += fmt.Sprintf(" · budget %d", budget)
	}
	detail += " · transcript unchanged (rolling fold skipped)"
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type: db.DebugCompaction, AgentID: agentID,
		Name: TriggerAuto + "-" + ModeNative, Detail: detail,
	}); err != nil {
		m.log(slog.LevelError, "native compaction journal append failed", "session", sessionID, "error", err)
	}
}

// ForceCompact folds all but the most recent keepRecent messages into the
// rolling summary regardless of the token budget — the manual "/compact" chat
// command. Returns the fold (tagged TriggerManual, same shape Prepare reports for
// the budgeted fold) and the updated summary; folds nothing (zero Compaction)
// when there are not enough pending messages.
func (m *Manager) ForceCompact(ctx context.Context, database *db.DB, provider providers.Provider, session db.Session, agent db.Agent, history []db.Message) (fold Compaction, summary string, err error) {
	summary = session.Summary
	start := clampStart(session.SummaryMsgCount, len(history))
	_, keepRecent := m.limits()
	foldMsgs, keepTail, newCount, ok := foldBoundary(history, start, keepRecent)
	if !ok {
		return Compaction{}, summary, nil // not enough to compact
	}
	firePreCompact(ctx, TriggerManual)
	beforeTokens := EstimateTokens(summary, history[start:])
	newSummary, err := m.summarizeTimed(ctx, database, provider, agent, session.ID, summary, foldMsgs)
	if err != nil {
		return Compaction{}, "", err
	}
	foldIndex, err := database.SetSessionSummary(ctx, session.ID, newSummary, newCount)
	if err != nil {
		return Compaction{}, "", err
	}
	fold = Compaction{
		FoldedMsgs:   len(foldMsgs),
		BeforeTokens: beforeTokens,
		AfterTokens:  EstimateTokens(newSummary, keepTail),
		Trigger:      TriggerManual,
		Mode:         ModeRolling,
	}
	m.log(slog.LevelInfo, "context compacted (manual /compact)",
		"session", session.ID, "agent", agent.ID, "folded_msgs", fold.FoldedMsgs)
	// Journal the manual fold too; budget 0 → omitted from Detail (manual is
	// budget-independent).
	m.recordCompactionDebug(database, session.ID, agent.ID, fold.Trigger,
		fold.FoldedMsgs, fold.BeforeTokens, fold.AfterTokens, 0, len(renderDBMessages(foldMsgs)),
		foldIndex, len(newSummary))
	return fold, newSummary, nil
}

// SimulateCompaction reports how THIS turn's budgeted compaction would reshape the
// message array, WITHOUT any side effect (no LLM summarize, no summary persist). It
// mirrors Prepare's front half — same budget math (limits + budgetShape +
// EffectiveBudget) and fold boundary — but instead of summarizing, it just returns
// how many pending messages WOULD be folded into the rolling summary and the tail
// that would remain on the wire. keptTail always excludes the already-summarized
// head (history[:SummaryMsgCount]); when wouldCompact is false (budget disabled or
// summary+pending already fit) it is the full pending slice. Read-only, for the
// context preview's "simulate compaction" toggle.
func (m *Manager) SimulateCompaction(session db.Session, agent db.Agent, history []db.Message) (foldCount int, keptTail []db.Message, wouldCompact bool) {
	summary := session.Summary
	start := clampStart(session.SummaryMsgCount, len(history))
	pending := history[start:]
	maxTokens, keepRecent := m.limits()
	if maxTokens > 0 {
		fraction, ceil := m.budgetShape()
		maxTokens = EffectiveBudget(agent.Provider, agent.Model, maxTokens, fraction, ceil)
	}
	if maxTokens > 0 && EstimateTokens(summary, pending) > maxTokens {
		if fold, tail, _, ok := foldBoundary(history, start, keepRecent); ok {
			return len(fold), tail, true
		}
	}
	return 0, pending, false
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
	// Self-pin the workspace claude-home before the direct Complete: this fold
	// runs outside guardedComplete/the tool loop, so without a pin the shared
	// claude-cli provider uses its global-default config dir and fails auth. The
	// home is carried on ctx via WithClaudeHome by every fold entry point.
	pinClaudeHome(ctx, provider)
	resp, err := provider.Complete(foldCtx(ctx), providers.Request{
		Model:     agent.Model,
		MaxTokens: compactMaxOutputTokens,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompts.Render(compactPromptFromCtx(ctx), map[string]string{
				"summary":  existing,
				"messages": rendered,
			})},
		},
	})
	if err != nil {
		recordFailedCompaction(ctx, database, agent, err)
		return "", err
	}
	recordCompaction(ctx, database, agent, resp.Usage)
	return strings.TrimSpace(resp.Text), nil
}

// recordCompactionDebug appends a structured "compaction" event to the session's
// debug journal (debug.jsonl) so every fold — routine budgeted (trigger "auto")
// or manual /compact ("manual") — is observable in the Debug modal, the
// read_session_debug tool and the debug summary, not only in the in-app Logs.
// This is the fix for the "compaction ran on a spawned/coordinator turn but I
// can't see where it ran" gap: the fold happens in this layer regardless of the
// turn kind, so journaling it here covers every path in one choke point.
//
// Side-effect-only: a journal write failure never breaks a turn
// (AppendDebugEventGated takes only its own lock), but it is logged rather than
// discarded. The emit obeys the user's debugJournalEnabled/debugJournalCap
// setting — the same gate the runtime's emitDebug applies.
// Token figures ride Detail (not In/Out) because the debug summary sums In/Out for
// llm_call events only — keeping them off the compaction event leaves the token
// series clean while SavedBytes feeds the summary's existing compaction rollup.
// foldIndex is the 1-based ordinal of this fold in the session (from
// SetSessionSummary) and summaryBytes the size of the summary it produced —
// together they make cumulative summary drift readable across folds. A
// foldIndex <= 0 means the ordinal could not be established; it is then left off
// the event and said so in Detail rather than silently journalled as fold #0.
// pressureWarnRatio is how full the context budget has to be for Prepare to
// journal an early-warning event instead of staying silent until the fold.
const pressureWarnRatio = 0.85

// recordPressureDebug journals a "context budget nearly full" warning for a turn
// that did NOT fold. Same gating and failure handling as recordCompactionDebug.
func (m *Manager) recordPressureDebug(database *db.DB, sessionID, agentID string, usedTokens, budget int, pressure float64) {
	if database == nil || sessionID == "" {
		return
	}
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:    db.DebugPressure,
		AgentID: agentID,
		Name:    "context_pressure",
		Detail: fmt.Sprintf("context %d/%d tokens · %.0f%% of budget · fold at 100%%",
			usedTokens, budget, pressure*100),
	}); err != nil {
		m.log(slog.LevelError, "context pressure journal append failed", "session", sessionID, "error", err)
	}
}

func (m *Manager) recordCompactionDebug(database *db.DB, sessionID, agentID, trigger string, foldedMsgs, beforeTokens, afterTokens, budget, savedBytes, foldIndex, summaryBytes int) {
	if database == nil || sessionID == "" {
		return
	}
	detail := fmt.Sprintf("folded %d msgs · %d→%d tokens", foldedMsgs, beforeTokens, afterTokens)
	if budget > 0 {
		detail += fmt.Sprintf(" · budget %d", budget)
	}
	if foldIndex > 0 {
		detail += fmt.Sprintf(" · fold #%d", foldIndex)
	} else {
		detail += " · fold # unknown"
	}
	detail += fmt.Sprintf(" · summary %dB", summaryBytes)
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:         db.DebugCompaction,
		AgentID:      agentID,
		Name:         trigger,
		SavedBytes:   savedBytes,
		FoldIndex:    foldIndex,
		SummaryBytes: summaryBytes,
		Detail:       detail,
	}); err != nil {
		m.log(slog.LevelError, "compaction journal append failed", "session", sessionID, "error", err)
	}
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

// recordFailedCompaction bills a fold whose provider call ENDED IN AN ERROR.
//
// A fold sends the whole pending transcript, so a call that dies after the
// prompt was accepted has already been paid for in full. These two call sites
// (summarizeRendered, BuildHandoff) issue provider.Complete directly — outside
// guardedComplete and outside the tool loop — so the agent layer's
// recordFailedUsage never sees them, and returning the error bare made the most
// expensive request of the session cost zero on the books.
//
// Only an error CARRYING usage is billed: providers attach a UsageError when
// they know what the attempt spent. An error without one spent nothing we can
// attribute, and inventing a number would be worse than the gap. Nil-safe and
// non-fatal for the same reason as recordCompaction — the caller is already
// returning a failure and must not have it replaced by a counting error.
func recordFailedCompaction(ctx context.Context, database *db.DB, agent db.Agent, err error) {
	ue, ok := providers.UsageFromError(err)
	if !ok || database == nil {
		return
	}
	calls := ue.ProviderCalls
	if calls <= 0 {
		calls = 1
	}
	model := ue.Model
	if model == "" {
		model = agent.Model
	}
	_ = database.AddUsageKind(ctx, agent.ID, db.UsageKindCompact, agent.Provider, model, db.DeltaFromUsage(calls, ue.Usage))
}

// ToProviderMessages maps a tail of stored turns to provider messages (same
// rules as the internal compaction path). Exposed for the claude-cli resume path,
// which sends only the messages the CLI has not yet seen (the delta) instead of
// the full transcript. ctx should carry WithAttachmentRoot (see attachments.go).
func ToProviderMessages(ctx context.Context, msgs []db.Message) []providers.Message {
	return toProviderMessages(ctx, msgs)
}

// toProviderMessages maps stored user/assistant turns to provider messages,
// folding any user-message attachments into the text the model sees.
func toProviderMessages(ctx context.Context, msgs []db.Message) []providers.Message {
	out := make([]providers.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role == providers.RoleUser || msg.Role == providers.RoleAssistant {
			out = append(out, providers.Message{Role: msg.Role, Text: withAttachments(ctx, msg)})
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

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return fallback
}
