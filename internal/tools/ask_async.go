package tools

import "context"

// AsyncInput posts questions without blocking and collects replies at a safe
// model boundary. The chat layer owns persistence and continuation delivery.
type AsyncInput struct {
	Ask   func(context.Context, []AskQuestion) (string, error)
	Drain func() []string
}

type asyncInputKey struct{}

func WithAsyncInput(ctx context.Context, input *AsyncInput) context.Context {
	return context.WithValue(ctx, asyncInputKey{}, input)
}

func AsyncInputFrom(ctx context.Context) *AsyncInput {
	input, _ := ctx.Value(asyncInputKey{}).(*AsyncInput)
	return input
}

func DrainAsyncInput(ctx context.Context) []string {
	if input := AsyncInputFrom(ctx); input != nil && input.Drain != nil {
		return input.Drain()
	}
	return nil
}
