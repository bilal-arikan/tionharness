package api

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// wakeTurnRunner builds the history-aware self-wake turn runner for a runtime. A
// self-wake (schedule_wake) re-enters its originating chat session, but the
// scheduler's prompt-only invoke gave the woken agent ONLY the wake prompt — none
// of the conversation it was meant to continue. This runner composes the SAME
// rich request an interactive chat turn gets (full history with author labels,
// running summary, memory, workdir, etc.) and runs the agentic loop, so the
// woken agent truly continues the thread.
//
// The wake prompt was already persisted as the session's last user message by
// deliverWake before this runs, so the loaded history carries it — there is no
// separate prompt to append.
func (s *Server) wakeTurnRunner(rt *agent.Runtime) agent.WakeTurnFunc {
	return func(ctx context.Context, ag db.Agent, sessionID, prompt string) (string, []agent.TurnStep, error) {
		wsp, err := s.workspaces.Get(rt.WorkspaceID())
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: workspace lookup: %w", err)
		}
		session, err := wsp.DB.GetSession(ctx, sessionID)
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: session lookup: %w", err)
		}
		provider, err := s.providers.Get(ag.ProviderRef())
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: provider: %w", err)
		}
		history, err := wsp.DB.ListMessages(ctx, sessionID)
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: history: %w", err)
		}
		// Annotate the history with each assistant turn's author (no-op in a
		// single-agent session) so a woken agent in a shared thread can still tell
		// who said what.
		history, multiAgent := s.labelMultiAgentHistory(ctx, wsp.DB, ag.ID, history)
		toolRecap := recentToolActivityBlock(history)
		feedbackRecap := recentFeedbackBlock(history)
		ctx = conversation.WithCompactPrompt(ctx, wsp.Runtime.CompactPromptTemplate())
		ctx = conversation.WithAttachmentRoot(ctx, wsp.SandboxRoot())
		// Pin the workspace claude-home BEFORE Prepare's fold: the tool loop below
		// pins the provider itself, but this Prepare runs first and folds via a
		// direct provider.Complete — without the pin an autonomous wake on a large
		// session fails compaction against the global claude-home.
		ctx = conversation.WithClaudeHome(ctx, wsp.Runtime.ClaudeHomeDir())
		// Autonomous turns have no request SSE, so publish hook steps through the
		// session emitter while still isolating hook failures in RunLifecycleHooks.
		emit := wsp.Runtime.SessionStepEmitter(ctx)
		ctx = conversation.WithPreCompact(ctx, func(trigger string) {
			pc := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookPreCompact, agent.LifecycleExtras{Trigger: trigger})
			if emit != nil {
				for _, st := range pc.Steps {
					emit(st)
				}
			}
		})
		// Budget the fold against the true per-turn footprint (messages + the static
		// prefix / tool schemas / artifacts shipped every turn), not messages alone —
		// otherwise a large static prefix (e.g. a claude-cli coordinator draining
		// worker notifications) holds the message-only estimate under budget and no
		// fold ever fires while the real context runs over.
		overhead, stepBase, err := s.contextOverheadTokens(ctx, wsp, session, history, multiAgent)
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: context overhead: %w", err)
		}
		ctx = conversation.WithContextOverhead(ctx, overhead)
		ctx = conversation.WithContextOverheadStepBase(ctx, stepBase)
		// Native-compaction seam (autoCompactMode native/auto): let the CLI compact
		// its own window before the rolling fold. Any error means "not compacted" and
		// conversation falls back to the fold, so it is returned as-is.
		ctx = conversation.WithNativeCompact(ctx, func(ctx context.Context) error {
			_, nerr := s.runNativeCompact(ctx, wsp, session, history, nativeCompactAuto)
			return nerr
		})
		prep, err := s.convo.Prepare(ctx, wsp.DB, provider, session, ag, history)
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: prepare: %w", err)
		}
		if prep.Compacted {
			wsp.Runtime.DropWarmCLISession(session.ID)
		}
		if prep.NativeCompacted {
			session, err = wsp.DB.GetSession(ctx, session.ID)
			if err != nil {
				return "", nil, fmt.Errorf("wake turn: reload session after native compaction: %w", err)
			}
		}
		// freshSession=false: a wake always continues an existing conversation.
		req := s.composeTurnRequest(ctx, wsp, session, ag, []db.Agent{ag}, prompt, prep, false, multiAgent, toolRecap, feedbackRecap, "")
		// autonomous=true: a wake is a headless, budget-gated run (no live client);
		// completeTraced auto-wires the Interaction MCP bridge for CLI agents. The
		// session-step emitter streams this turn's activity to the bus so a window
		// viewing the session sees the woken/worker/coordinator turn unfold live,
		// just like an interactive chat turn (nil when the ctx carries no session id).
		// Auto-compaction visibility on autonomous turns (schedule_wake / worker /
		// coordinator): Prepare may have folded older history silently. Surface it as a
		// head-of-turn step on the SAME feed the turn's own steps use — emitted live for
		// a watching window and prepended to the persisted trace below so it survives a
		// refresh. This is the path that carried the invisible SES548 spawned-turn fold.
		var compactionStep *agent.TurnStep
		if prep.Compacted {
			st := compactionLeadStep(prep.Fold, provider)
			compactionStep = &st
			if emit != nil {
				emit(st)
			}
		}
		resp, steps, err := wsp.Runtime.CompleteWithToolsStream(ctx, ag, provider, req, true, emit)
		// Prepend on both success and error so a folded-then-failed turn still records
		// that the compaction happened.
		if compactionStep != nil {
			steps = append([]agent.TurnStep{*compactionStep}, steps...)
		}
		if err != nil {
			return "", steps, err
		}
		return resp.Text, steps, nil
	}
}
