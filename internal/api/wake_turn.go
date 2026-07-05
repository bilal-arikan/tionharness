package api

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
)

// wakeTurnRunner builds the history-aware self-wake turn runner for a runtime. A
// self-wake (schedule_wake) re-enters its originating chat session, but the
// scheduler's prompt-only invoke gave the woken agent ONLY the wake prompt — none
// of the conversation it was meant to continue. This runner composes the SAME
// rich request an interactive chat turn gets (full history with author labels,
// running summary, memory, goal, workdir, etc.) and runs the agentic loop, so the
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
		provider, err := s.providers.Get(ag.Provider)
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
		history = appendRecentToolSummaries(history)
		ctx = conversation.WithCompactPrompt(ctx, wsp.Runtime.CompactPromptTemplate())
		prep, err := s.convo.Prepare(ctx, wsp.DB, provider, session, ag, history)
		if err != nil {
			return "", nil, fmt.Errorf("wake turn: prepare: %w", err)
		}
		// freshSession=false: a wake always continues an existing conversation.
		req := s.composeTurnRequest(ctx, wsp, session, ag, []db.Agent{ag}, prompt, prep, false, multiAgent)
		// autonomous=true: a wake is a headless, budget-gated run (no live client);
		// completeTraced auto-wires the Interaction MCP bridge for CLI agents. The
		// session-step emitter streams this turn's activity to the bus so a window
		// viewing the session sees the woken/worker/coordinator turn unfold live,
		// just like an interactive chat turn (nil when the ctx carries no session id).
		resp, steps, err := wsp.Runtime.CompleteWithToolsStream(ctx, ag, provider, req, true, wsp.Runtime.SessionStepEmitter(ctx))
		if err != nil {
			return "", steps, err
		}
		return resp.Text, steps, nil
	}
}
