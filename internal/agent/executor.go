package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// invokeTraced calls the agent's provider with a single user prompt and also
// returns the agent's activity trace (thinking/tool steps), so callers can
// persist a rich chat turn rather than a bare text reply. Used by the scheduler
// so scheduled runs render like normal chat turns in the agent's schedule session.
func (r *Runtime) invokeTraced(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, []TurnStep, error) {
	provider, err := r.providers.Get(agent.ProviderRef())
	if err != nil {
		return "", nil, err
	}
	// A session-scoped step emitter (nil when the turn has no session id) streams
	// this autonomous turn's activity to the bus, so a window viewing the session
	// sees thinking/tool steps live — the same feed a chat turn gets. The returned
	// slice is unchanged (still the full persistable trace); only live emission is
	// added.
	resp, steps, err := r.CompleteWithToolsStream(ctx, agent, provider, providers.Request{
		Model:  agent.Model,
		System: r.autonomousSystemPrompt(ctx, agent),
		// Volatile per-turn context (turn-start clock + the session's persistent
		// lessons, when scheduler/spawn/peer stamp a session id in ctx) rides the
		// dynamic suffix so the static prefix above stays byte-stable and cacheable.
		SystemDynamic: r.autonomousDynamicSuffix(ctx, agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompt},
		},
	}, autonomous, r.SessionStepEmitter(ctx))
	if err != nil {
		return "", nil, err
	}
	return resp.Text, steps, nil
}
