package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// renderFlowTranscript turns a finished flow run into a plain-text transcript: a
// header plus one section per executed node. Shared by the chat flow-trigger
// path (flow.go) to record a run as a readable assistant turn.
func renderFlowTranscript(flowName string, fr db.FlowRun, setupErr error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔀 %s akışı çalıştı\n", flowName)
	if setupErr != nil {
		fmt.Fprintf(&b, "\n⚠️ Akış başlatılamadı: %s", setupErr.Error())
		return strings.TrimSpace(b.String())
	}
	var st orchestration.State
	_ = json.Unmarshal([]byte(fr.State), &st)
	for i, t := range st.Trace {
		title := t.Title
		if title == "" {
			title = t.NodeID
		}
		fmt.Fprintf(&b, "\n%d. %s\n%s\n", i+1, title, t.Output)
	}
	if fr.Status == db.FlowFailure {
		fmt.Fprintf(&b, "\n⚠️ Durum: hata — %s", fr.Error)
	}
	return strings.TrimSpace(b.String())
}

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

// complete is the shared single-prompt provider call. system is the static
// prefix, systemDynamic the volatile suffix (see providers.Request);
// outputSchema (may be "") constrains the reply via structured outputs on
// supporting providers/models. It routes through CompleteWithTools, which
// enforces the daily budget (when autonomous), records usage, and runs the
// agentic tool loop when the agent has tools enabled. Used by the orchestration
// flow runner (flow.go).
func (r *Runtime) complete(ctx context.Context, agent db.Agent, system, systemDynamic, prompt, outputSchema string, autonomous bool) (string, error) {
	provider, err := r.providers.Get(agent.ProviderRef())
	if err != nil {
		return "", err
	}
	req := providers.Request{
		Model:         agent.Model,
		System:        system,
		SystemDynamic: systemDynamic,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompt},
		},
	}
	if s := strings.TrimSpace(outputSchema); s != "" {
		req.OutputSchema = json.RawMessage(s)
	}
	resp, steps, err := r.CompleteWithToolsStream(ctx, agent, provider, req, autonomous, func(st TurnStep) {
		r.emitFlowNodeStepCtx(ctx, st) // live to the run viewer (no-op off the flow path)
	})
	if err != nil {
		return "", err
	}
	// Persist the node's tool/thinking trace to a sidecar (no-op off the flow
	// path) so the run inspector can render it as a chat-like exchange.
	r.captureFlowNodeSteps(ctx, steps)
	return resp.Text, nil
}

// completeThread is the accumulate-mode variant of complete: prior turns
// (thread) precede the new user prompt in the message list, so the agent's
// stable system prefix + growing message prefix are reused by the provider's
// prompt cache across sequential nodes. The provider already takes a message
// slice, so this only widens the slice — no provider-side change.
func (r *Runtime) completeThread(ctx context.Context, agent db.Agent, system, systemDynamic string, thread []orchestration.Msg, prompt, outputSchema string, autonomous bool) (string, error) {
	provider, err := r.providers.Get(agent.ProviderRef())
	if err != nil {
		return "", err
	}
	msgs := make([]providers.Message, 0, len(thread)+1)
	for _, m := range thread {
		role := providers.RoleUser
		if m.Role == "assistant" {
			role = providers.RoleAssistant
		}
		msgs = append(msgs, providers.Message{Role: role, Text: m.Text})
	}
	msgs = append(msgs, providers.Message{Role: providers.RoleUser, Text: prompt})
	req := providers.Request{
		Model:         agent.Model,
		System:        system,
		SystemDynamic: systemDynamic,
		Messages:      msgs,
	}
	if s := strings.TrimSpace(outputSchema); s != "" {
		req.OutputSchema = json.RawMessage(s)
	}
	resp, steps, err := r.CompleteWithToolsStream(ctx, agent, provider, req, autonomous, func(st TurnStep) {
		r.emitFlowNodeStepCtx(ctx, st)
	})
	if err != nil {
		return "", err
	}
	r.captureFlowNodeSteps(ctx, steps)
	return resp.Text, nil
}
