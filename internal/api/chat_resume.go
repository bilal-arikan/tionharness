package api

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// cliResumePlan captures one CLI resume decision so the caller can persist the
// returned session/thread id and updated transcript boundary afterwards.
type cliResumePlan struct {
	active    bool
	sentCount int // raw message count the CLI will know AFTER this turn's user msg
	// coldStart is true when resume was ENABLED for this turn but no warm thread
	// was carried into it, so the CLI conversation restarts here: first CLI turn,
	// a fold re-baseline, or a stored id the provider could not resume. It is the
	// signal the transcript draws its "new CLI session" divider from; a turn where
	// resume is off entirely leaves the whole plan zero and never sets it.
	coldStart bool
}

// planCLIResume keeps Claude's existing opt-in/persistent-session semantics and
// adds Codex's scoped durable-thread path. Non-CLI providers remain inert.
func (s *Server) planCLIResume(ctx context.Context, provider providers.Provider, agentCount int, session db.Session, agentRow db.Agent, rawHistory []db.Message, compacted bool, llmReq *providers.Request) cliResumePlan {
	switch provider.Name() {
	case "claude-cli":
		return s.planClaudeResume(ctx, provider, agentCount, session, rawHistory, compacted, llmReq)
	case "codex-cli":
		return s.planCodexResume(ctx, provider, agentCount, session, agentRow, rawHistory, compacted, llmReq)
	default:
		return cliResumePlan{}
	}
}

// planCodexResume enables warm `codex exec resume` only when three independent
// safety gates hold: one responding agent, one session participant, and the
// default session persona. Its opaque scope also includes provider/model and the
// frozen static system prompt; changing persona or CODEX_HOME makes the stored
// thread unverifiable and forces a cold, full-summary restart.
func (s *Server) planCodexResume(ctx context.Context, provider providers.Provider, agentCount int, session db.Session, agentRow db.Agent, rawHistory []db.Message, compacted bool, llmReq *providers.Request) cliResumePlan {
	resumer, ok := provider.(providers.ScopedCLIResumer)
	multiParticipant := len(db.SessionParticipants(session)) > 1
	personaMatch := strings.TrimSpace(session.AgentID) != "" && session.AgentID == agentRow.ID
	scope := strings.Join([]string{
		session.ID,
		agentRow.ID,
		agentRow.ProviderRef(),
		agentRow.Model,
		llmReq.System,
	}, "\x00")
	enabled := ok && agentCount == 1 && !multiParticipant && personaMatch && resumer.ResumeScopeReady(scope)
	plan, resumeID, deltaStart := claudeResumeDecision(enabled, session.CLISessionID, session.CLISentMsgCount, len(rawHistory), compacted)
	if !enabled {
		return plan
	}

	// The scope is required even on a cold turn: it selects the durable, isolated
	// CODEX_HOME where this turn's rollout is written for the next resume.
	llmReq.CLIResumeScope = scope
	if resumeID != "" && !resumer.CanResumeScoped(scope, resumeID) {
		if s.logger != nil {
			s.logger.Info("codex resume reset: thread is absent from the scoped home",
				"component", "conversation", "session", session.ID,
				"agent", agentRow.ID, "prev_cli_session", resumeID)
		}
		// The decision said warm, but the scoped home cannot serve that thread — this
		// turn really does start a new Codex conversation, so re-flag it cold.
		resumeID = ""
		plan.coldStart = true
	}
	if resumeID != "" {
		llmReq.ResumeSessionID = resumeID
		llmReq.Messages = conversation.ToProviderMessages(ctx, rawHistory[deltaStart:])
	}
	if compacted && session.CLISessionID != "" && s.logger != nil {
		s.logger.Info("cli resume reset: fold re-baselined the codex-cli session",
			"component", "conversation", "session", session.ID,
			"prev_cli_session", session.CLISessionID, "raw_msgs", len(rawHistory))
	}
	return plan
}

// planClaudeResume decides whether to resume the claude-cli session for this turn
// and, if so, mutates llmReq to carry the resume id and send only the delta (the
// messages the CLI has not seen yet) instead of the full transcript — so the CLI
// reuses its warm prompt cache. Opt-in (ClaudeResume setting), claude-cli only,
// and single-agent only (the resume id is tracked per session, so a multi-agent
// thread would collide). rawHistory is the un-annotated message list (it includes
// this turn's just-added user message). Returns a plan whose sentCount is stored
// after the turn together with the rotated Response.SessionID.
func (s *Server) planClaudeResume(ctx context.Context, provider providers.Provider, agentCount int, session db.Session, rawHistory []db.Message, compacted bool, llmReq *providers.Request) cliResumePlan {
	set := s.settings.Get()
	// A multi-participant thread (2+ agents have taken part) must NOT warm-resume:
	// the CLI session id is tracked per session, so resuming it for a DIFFERENT agent
	// would (a) continue the wrong persona/claude-home and (b) drop the author-labeled
	// history (labelMultiAgentHistory) in favour of the raw delta, defeating cross-
	// agent attribution. agentCount==1 alone is insufficient — each turn routes to one
	// agent, but the SESSION may still be shared by several agents across turns.
	multiParticipant := len(db.SessionParticipants(session)) > 1
	enabled := resumeGateEnabled(set.ClaudeResume, set.ClaudePersistentSession, agentCount, provider.Name(), multiParticipant)
	plan, resumeID, deltaStart := claudeResumeDecision(enabled, session.CLISessionID, session.CLISentMsgCount, len(rawHistory), compacted)
	// The stored id may name a conversation this CLI config home no longer has —
	// the app-global claude-home replacing the per-workspace ones left every
	// session pointing at a transcript in the OLD home. Resuming it fails the turn
	// ("No conversation found with session ID"), and the failure is retryable-
	// looking, so the same dead id burns the retries too. Verify first and fall
	// back to a cold start, which keeps the full prepared transcript (llmReq is
	// left untouched) instead of the unseen delta.
	if resumeID != "" {
		if v, ok := provider.(providers.ResumeVerifier); ok && !v.CanResume(resumeID) {
			if s.logger != nil {
				s.logger.Info("cli resume reset: stored session id is not resumable from this config home",
					"component", "conversation", "session", session.ID, "prev_cli_session", resumeID)
			}
			// Same as the Codex path: the warm decision does not survive verification,
			// so this turn opens a fresh CLI conversation and is flagged cold.
			resumeID = ""
			plan.coldStart = true
		}
	}
	if resumeID != "" {
		// Warm resume: send only the unseen delta and ask the CLI to --resume.
		llmReq.ResumeSessionID = resumeID
		llmReq.Messages = conversation.ToProviderMessages(ctx, rawHistory[deltaStart:])
	}
	// On a fold (compacted → cold), llmReq.Messages is left untouched: it stays the
	// compacted tail (summary + keepRecent) that Prepare produced, which a FRESH CLI
	// session now receives — the point at which TionHarness compaction actually reaches
	// the claude-cli window (a warm --resume would keep the stale full history and
	// only stack the summary on top). plan.sentCount stays rawLen so the next turn's
	// delta continues from the compacted baseline, exactly like a warm turn.
	//
	// Surface that reset in the Logs: a fold that dropped an EXISTING warm CLI session
	// is the single most useful diagnostic event here (it explains the one cache-cold
	// turn and confirms compaction reached the CLI). Only when a warm session actually
	// existed — a cold-anyway turn (first turn, edited history) is not noteworthy.
	if enabled && compacted && session.CLISessionID != "" && s.logger != nil {
		s.logger.Info("cli resume reset: fold re-baselined the claude-cli session",
			"component", "conversation", "session", session.ID,
			"prev_cli_session", session.CLISessionID, "raw_msgs", len(rawHistory))
	}
	return plan
}

// resumeGateEnabled is the pure (testable) gate for the --resume delta path. It is
// MUTUALLY EXCLUSIVE with ClaudePersistentSession: the persistent long-lived process
// holds the conversation itself and needs the FULL transcript on a cold (re)start, so
// when it is on we must NOT trim to the delta here — persistent supersedes --resume.
// Also single-agent only (the resume id is tracked per session, so a multi-agent
// thread would collide) and claude-cli only. multiParticipant blocks resume once a
// session is shared by 2+ agents across turns — the per-turn agentCount==1 check
// does not catch that, and resuming one agent's CLI session for another loses the
// author-labeled history + continues the wrong persona.
//
// Codex has a separate scoped-home gate in planCodexResume. Keeping this helper
// Claude-only preserves the ClaudePersistentSession precedence contract.
func resumeGateEnabled(claudeResume, persistentSession bool, agentCount int, providerName string, multiParticipant bool) bool {
	return claudeResume && !persistentSession && agentCount == 1 && providerName == "claude-cli" && !multiParticipant
}

// claudeResumeDecision is the pure (testable) core of planClaudeResume. Given the
// gate result and the session's recorded resume boundary, it returns the plan to
// persist after the turn plus, when a warm resume applies, the id to --resume and
// the index in rawHistory where the unseen delta begins. A zero resumeID means a
// cold start (send the full transcript, capture a fresh id). The warm path engages
// only when a prior id exists, the boundary is in (0, rawLen], and there is at
// least one unseen message — otherwise it falls back to cold (e.g. after edits
// shrank the history past the boundary).
func claudeResumeDecision(enabled bool, cliSessionID string, sentCount, rawLen int, compacted bool) (plan cliResumePlan, resumeID string, deltaStart int) {
	if !enabled {
		return cliResumePlan{}, "", 0
	}
	// coldStart starts true and is cleared only on the warm branch below, so every
	// new cold path added here is flagged by default rather than silently missing
	// the divider.
	plan = cliResumePlan{active: true, sentCount: rawLen, coldStart: true}
	// A fold just re-baselined the transcript into the rolling summary. Warm-resuming
	// here would keep the CLI's now-stale FULL history warm server-side and merely
	// stack the fresh summary on top — the fold would never actually shrink the CLI's
	// window (it would even grow it by the summary's size). Force a COLD start so this
	// turn ships the compacted tail (summary + keepRecent) to a FRESH CLI session: the
	// only point where TionHarness compaction reaches the claude-cli. plan.sentCount
	// stays rawLen, so subsequent turns resume from this compacted baseline normally.
	if compacted {
		return plan, "", 0
	}
	if cliSessionID != "" && sentCount > 0 && sentCount < rawLen {
		plan.coldStart = false
		return plan, cliSessionID, sentCount
	}
	return plan, "", 0
}
