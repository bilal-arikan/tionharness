package agent

import "context"

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
