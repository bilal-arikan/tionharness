package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// Durable Ask (MVP): a native chat turn that calls ask_user at a "clean" tool-loop
// point is parked to disk (db.SessionAsk) instead of blocking a goroutine on the
// interactive asker, so the wait survives a restart/crash — the session analog of
// the flow await-input waiting model (_Docs/62). The whole behavior is gated by
// WithDurableAsk on ctx: only the interactive chat turn runner sets it, so every
// other caller (autonomous/flow/claude-cli) keeps today's blocking behavior.

// askUserToolName is the native interactive-ask tool the suspend seam intercepts.
const askUserToolName = "ask_user"

// durableAskKey gates the durable-ask suspend behavior on a request context.
type durableAskKey struct{}

// WithDurableAsk enables durable ask_user suspend/resume for this turn. Set only
// by the interactive chat turn runner (an asker is present + the client can be
// resumed via the answer endpoint). Absent everywhere else → the loop keeps the
// legacy blocking asker path untouched.
func WithDurableAsk(ctx context.Context) context.Context {
	return context.WithValue(ctx, durableAskKey{}, true)
}

// durableAskEnabled reports whether this turn opted into durable ask suspend.
func durableAskEnabled(ctx context.Context) bool {
	v, _ := ctx.Value(durableAskKey{}).(bool)
	return v
}

// wouldPromptPermission reports whether the permission gate would block on the
// interactive prompter for this call (mode "ask", a write/exec risk, no standing
// grant, a prompter present). At a clean point the durable seam suspends instead of
// prompting — the permission analog of intercepting ask_user. It mirrors permGate's
// "ask" branch so the two never diverge on what needs approval.
func wouldPromptPermission(ctx context.Context, mode string, call providers.ToolCall) bool {
	if mode != "ask" || tools.Classify(call.Name) == tools.RiskRead {
		return false
	}
	arg := tools.RepresentativeArg(call.Name, call.Input)
	if tools.GrantsFrom(ctx).Matches(call.Name, arg) {
		return false
	}
	return tools.PermissionPrompterFrom(ctx) != nil
}

// permissionCardPayload builds the durable permission card descriptor (matches the
// live openInteraction("permission", …) shape the frontend renders).
func permissionCardPayload(call providers.ToolCall) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"tool":    call.Name,
		"reason":  string(tools.Classify(call.Name)),
		"text":    tools.RepresentativeArg(call.Name, call.Input),
		"options": tools.PermissionOptions,
	})
	return b
}

// askSuspend is the sentinel the native tool-loop returns at a clean suspend point
// when durable-ask is enabled: either an ask_user call (Kind "ask") or a write/exec
// call awaiting a permission decision (Kind "permission"). It carries the pending
// tool_use id + the full call (permission resume must EXECUTE it on approval), the
// card payload, and the in-flight history (already ending with the assistant
// tool_use turn, so resume folds the outcome as this call's tool_result).
type askSuspend struct {
	Kind     string // "ask" | "permission"
	CallID   string
	Call     providers.ToolCall
	Payload  json.RawMessage
	Messages []providers.Message
}

func (e *askSuspend) Error() string { return "durable interaction suspend (" + e.Kind + ")" }

// askSuspendState is the serialized snapshot persisted in db.SessionAsk.State: the
// exact provider request the resume driver re-enters with (replayed verbatim so the
// static system prefix stays cache-stable), plus the pre-suspend step trace so the
// finished assistant message carries the whole turn. Tools are NOT persisted — the
// loop re-ships them from the live catalog (shipFor) on resume.
type askSuspendState struct {
	Kind          string              `json:"kind"`
	Model         string              `json:"model"`
	System        string              `json:"system"`
	SystemDynamic string              `json:"systemDynamic,omitempty"`
	OutputSchema  json.RawMessage     `json:"outputSchema,omitempty"`
	Messages      []providers.Message `json:"messages"`
	Steps         []TurnStep          `json:"steps,omitempty"`
	CallID        string              `json:"callId"`
	// Call is the pending tool call — needed for a permission resume, which
	// EXECUTES it on approval to produce the real tool_result (an ask resume just
	// folds the answer text, so Call is unused there).
	Call providers.ToolCall `json:"call,omitempty"`
}

// persistAskSuspend parks a suspended ask to disk and returns the created row. The
// caller supplies the original request (for the replay fields) and the sentinel
// (whose Messages supersede req.Messages, as they include the assistant tool_use
// turn). The card payload is stored so a window re-opened after a restart can
// re-render the prompt.
func (r *Runtime) persistAskSuspend(ctx context.Context, agent db.Agent, sessionID string, req providers.Request, steps []TurnStep, sus *askSuspend) (db.SessionAsk, error) {
	state := askSuspendState{
		Kind:          sus.Kind,
		Model:         req.Model,
		System:        req.System,
		SystemDynamic: req.SystemDynamic,
		OutputSchema:  req.OutputSchema,
		Messages:      sus.Messages,
		Steps:         steps,
		CallID:        sus.CallID,
		Call:          sus.Call,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return db.SessionAsk{}, fmt.Errorf("marshal ask suspend state: %w", err)
	}
	return r.db.CreateSessionAsk(ctx, db.SessionAsk{
		SessionID: sessionID,
		AgentID:   agent.ID,
		Kind:      sus.Kind,
		CallID:    sus.CallID,
		Payload:   string(sus.Payload),
		State:     string(data),
	})
}

// ResumeAsk delivers an answer to a durably-suspended ask and re-drives the turn.
// Exactly one answerer wins the waiting→resolved CAS (ClaimSessionAsk); the answer
// is folded back as the pending tool_use's tool_result and the native loop resumes
// in-place. Returns the turn's response and full step trace (pre-suspend steps +
// the resumed continuation). If the resumed turn hits ANOTHER clean ask, it parks
// again and returns an *askSuspend sentinel (the caller opens a fresh card).
func (r *Runtime) ResumeAsk(ctx context.Context, askID, answer string, onStep func(TurnStep)) (*providers.Response, []TurnStep, error) {
	ask, err := r.db.ClaimSessionAsk(ctx, askID, answer)
	if err != nil {
		return nil, nil, err // ErrNotFound or not-waiting (already answered) → caller 409s
	}
	agentRow, err := r.db.GetAgent(ctx, ask.AgentID)
	if err != nil {
		return nil, nil, fmt.Errorf("resume ask: agent gone: %w", err)
	}
	provider, err := r.providers.Get(agentRow.ProviderRef())
	if err != nil {
		return nil, nil, err
	}
	resp, steps, _, err := r.driveResumedAsk(ctx, ask, agentRow, provider, answer, onStep)
	return resp, steps, err
}

// ResumeAskAndRecord drives an already-claimed ask to completion (or re-suspend)
// and records the resulting assistant turn on the session. onStep streams live
// steps to the caller (which broadcasts them on the session hub). Returns the
// recorded message on completion, OR a freshly-parked ask when the resumed turn
// asked again (the caller opens a new card). Hub I/O stays in the API layer; turn
// logic + persistence stay here. The ask must already be CAS-claimed by the caller
// (single-winner), so this never double-drives.
func (r *Runtime) ResumeAskAndRecord(ctx context.Context, ask db.SessionAsk, answer string, onStep func(TurnStep)) (*db.Message, *db.SessionAsk, error) {
	agentRow, err := r.db.GetAgent(ctx, ask.AgentID)
	if err != nil {
		return nil, nil, fmt.Errorf("resume ask: agent gone: %w", err)
	}
	provider, err := r.providers.Get(agentRow.ProviderRef())
	if err != nil {
		return nil, nil, err
	}
	resp, steps, reSuspend, err := r.driveResumedAsk(ctx, ask, agentRow, provider, answer, onStep)
	if err != nil {
		return nil, nil, err
	}
	if reSuspend != nil {
		return nil, reSuspend, nil // the resumed turn asked again — caller opens a card
	}
	text := resp.Text
	if strings.TrimSpace(text) == "" {
		text = "ℹ️ Ajan bu yanıt için boş cevap döndürdü."
	}
	msg, aerr := r.db.AddMessage(ctx, db.Message{
		SessionID:  ask.SessionID,
		AgentID:    ask.AgentID,
		Role:       providers.RoleAssistant,
		Text:       text,
		Steps:      encodeSteps(steps),
		Model:      resp.Model,
		StopReason: resp.StopReason,
	})
	if aerr != nil {
		return nil, nil, aerr
	}
	return &msg, nil, nil
}

// resolveResumeResult turns the human decision into the pending call's tool_result.
// For an ask it is the answer text. For a permission decision it EXECUTES the
// approved call (producing the real tool_result + a tool card for the transcript)
// or synthesizes the same denial the live gate feeds the model. MVP: "always" acts
// as "allow" for THIS call — no standing session grant is persisted on the durable
// path (the resume is detached from the session's in-memory grants).
func (r *Runtime) resolveResumeResult(ctx context.Context, agentRow db.Agent, state askSuspendState, answer string) (providers.ToolResult, *TurnStep) {
	if state.Kind != "permission" {
		return providers.ToolResult{CallID: state.CallID, Content: answer}, nil
	}
	switch tools.NormalizePermission(answer) {
	case "allow", "always":
		reg := r.buildRegistry(ctx, agentRow)
		res := reg.Call(ctx, state.Call)
		step := TurnStep{Kind: StepTool, Tool: state.Call.Name, Input: state.Call.Input, Output: res.Content, IsError: res.IsError}
		return providers.ToolResult{CallID: state.CallID, Content: res.Content, IsError: res.IsError}, &step
	default:
		denied := fmt.Sprintf("permission denied by user: %q was not approved", state.Call.Name)
		step := TurnStep{Kind: StepError, Tool: state.Call.Name, Reason: "permission_denied", Text: denied, IsError: true}
		return providers.ToolResult{CallID: state.CallID, Content: denied, IsError: true}, &step
	}
}

// SuspendAskFromError parks a durable-ask suspend to disk when err is the suspend
// sentinel returned by the native loop, returning the created ask (with its card
// payload) and suspended=true. When err is anything else it reports suspended=false
// and the caller handles it as before. The exported seam the interactive chat turn
// runner uses without importing the internal sentinel type.
func (r *Runtime) SuspendAskFromError(ctx context.Context, agentRow db.Agent, sessionID string, req providers.Request, steps []TurnStep, err error) (db.SessionAsk, bool, error) {
	var sus *askSuspend
	if !errors.As(err, &sus) {
		return db.SessionAsk{}, false, nil
	}
	ask, perr := r.persistAskSuspend(ctx, agentRow, sessionID, req, steps, sus)
	return ask, true, perr
}

// driveResumedAsk is the provider-injected core of the resume paths (split out so
// the round-trip is testable without the provider registry): it folds the answer
// into the persisted history as the pending ask's tool_result and re-enters the
// native loop, prepending the pre-suspend trace to the continuation. When the
// resumed turn asks again at a clean point it re-parks (returns the new ask).
func (r *Runtime) driveResumedAsk(ctx context.Context, ask db.SessionAsk, agentRow db.Agent, provider providers.Provider, answer string, onStep func(TurnStep)) (*providers.Response, []TurnStep, *db.SessionAsk, error) {
	var state askSuspendState
	if err := json.Unmarshal([]byte(ask.State), &state); err != nil {
		return nil, nil, nil, fmt.Errorf("resume ask: corrupt state: %w", err)
	}
	// Compute the pending call's tool_result from the decision. An ask resume folds
	// the answer text verbatim; a permission resume EXECUTES the approved call (or
	// synthesizes a denial), producing the same tool_result the live gate would.
	toolResult, execStep := r.resolveResumeResult(ctx, agentRow, state, answer)
	// The executed permission call surfaces as a tool card in the transcript.
	pre := state.Steps
	if execStep != nil {
		pre = append(append([]TurnStep{}, state.Steps...), *execStep)
	}
	msgs := append(state.Messages, providers.Message{
		Role:        providers.RoleUser,
		ToolResults: []providers.ToolResult{toolResult},
	})
	req := providers.Request{
		Model:         state.Model,
		System:        state.System,
		SystemDynamic: state.SystemDynamic,
		OutputSchema:  state.OutputSchema,
		Messages:      msgs,
	}
	// Resume is interactive: keep durable-ask enabled so a follow-up ask suspends
	// again rather than blocking. WithSessionID lets step emission attribute live
	// activity to the session.
	rctx := WithDurableAsk(WithSessionID(ctx, ask.SessionID))
	resp, steps, err := r.CompleteWithToolsStream(rctx, agentRow, provider, req, false, onStep)
	// Prepend the pre-suspend trace (+ the executed permission tool card, if any) so
	// the finished assistant message reads as one continuous turn.
	all := append(append([]TurnStep{}, pre...), steps...)
	// A follow-up clean ask: re-park with the full accumulated trace so the next
	// resume continues to carry the whole turn.
	var sus *askSuspend
	if errors.As(err, &sus) {
		newAsk, perr := r.persistAskSuspend(ctx, agentRow, ask.SessionID, req, all, sus)
		if perr != nil {
			return nil, all, nil, perr
		}
		return nil, all, &newAsk, nil
	}
	if err != nil {
		return resp, all, nil, err
	}
	return resp, all, nil, nil
}

// StartWaitingAskSweeper launches the durable-ask timeout sweeper: it periodically
// fails any waiting ask/permission whose TimeoutSec has elapsed, so a card nobody
// ever answers can't sleep forever. Stops when ctx is cancelled. TimeoutSec is 0
// (unlimited) by default — the sweeper is a no-op until a timeout is stamped — so
// legitimate long waits survive; a workspace can opt into a bound later.
func (r *Runtime) StartWaitingAskSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(waitingSweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.sweepWaitingAsksAt(ctx, time.Now().Unix())
			}
		}
	}()
}

// sweepWaitingAsksAt closes every waiting ask whose TimeoutSec has elapsed by `now`.
// It CAS-closes each (waiting→timeout) so a concurrent live answer always wins over
// the sweeper. Split from StartWaitingAskSweeper with an explicit `now` so it is
// deterministically testable.
func (r *Runtime) sweepWaitingAsksAt(ctx context.Context, now int64) {
	asks, err := r.db.ListWaitingSessionAsks(ctx)
	if err != nil {
		return
	}
	for _, a := range asks {
		if !awaitTimeoutExceeded(a.TimeoutSec, a.CreatedAt, now) {
			continue
		}
		if closed, cerr := r.db.CloseSessionAsk(ctx, a.ID, db.SessionAskTimeout); cerr == nil && closed {
			r.logger.Info("durable ask timed out", "ask", a.ID, "session", a.SessionID, "timeoutSec", a.TimeoutSec)
		}
	}
}
