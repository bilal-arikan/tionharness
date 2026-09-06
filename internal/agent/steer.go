package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// steerCtxKey keys the live steering channel on a request context.
type steerCtxKey struct{}

// steerPrefix labels an injected steering message so the model treats it as
// mid-turn guidance from the user.
const steerPrefix = "[Canlı kullanıcı yönlendirmesi] "

// WithSteer attaches a steering channel to ctx. The chat tool loop drains it at
// the start of the turn and between tool iterations, injecting each message as
// live user guidance so the agent can adjust course without restarting.
func WithSteer(ctx context.Context, ch <-chan string) context.Context {
	return context.WithValue(ctx, steerCtxKey{}, ch)
}

func steerFrom(ctx context.Context) <-chan string {
	ch, _ := ctx.Value(steerCtxKey{}).(<-chan string)
	return ch
}

// steerRoleFor picks the message role an injected steer rides on. Steer messages
// ride the operator channel ({"role":"system"} in messages) on models that
// support it — cache-safe, non-spoofable, and valid between a tool_result user
// turn and the next assistant turn. Elsewhere they stay user-role (the provider
// folds them to keep alternation intact).
func steerRoleFor(provider providers.Provider, model string) string {
	if provider.Name() == "anthropic" && providers.SupportsSystemInMessages(model) {
		return providers.RoleSystem
	}
	return providers.RoleUser
}

// foldSteer injects every steering message pending on the turn context into the
// request as live user guidance and records one step per message. Shared by both
// turn paths so neither can silently swallow guidance: the native loop calls it
// before every provider call, the plain (no-tools) path before its single one.
func (t *toolLoopTurn) foldSteer() {
	for _, m := range drainSteer(t.ctx) {
		t.req.Messages = append(t.req.Messages, providers.Message{Role: t.steerRole, Text: steerPrefix + m})
		st := TurnStep{Kind: StepSteer, Text: m}
		t.steps = append(t.steps, st)
		t.emitStep(st)
	}
}

// emitStep publishes a step live through whichever sink this path has: the
// native loop installs the serialising emitter, the plain path has only the
// caller's onStep (and possibly neither, on an autonomous turn).
func (t *toolLoopTurn) emitStep(st TurnStep) {
	switch {
	case t.emit != nil:
		t.emit(st)
	case t.onStep != nil:
		t.onStep(st)
	}
}

// drainSteer returns all currently-pending steering messages without blocking.
func drainSteer(ctx context.Context) []string {
	ch := steerFrom(ctx)
	if ch == nil {
		return nil
	}
	var out []string
	for {
		select {
		case m := <-ch:
			if m != "" {
				out = append(out, m)
			}
		default:
			return out
		}
	}
}
