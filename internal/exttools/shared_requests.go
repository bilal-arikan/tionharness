package exttools

import (
	"context"
	"sync"
	"time"
)

type sharedResult[T any] struct {
	done  chan struct{}
	value T
	err   error
}

// sharedRequests shares bounded work without making other callers inherit the
// first caller's cancellation. Each waiter can still leave immediately.
type sharedRequests[T any] struct {
	mu     sync.Mutex
	active map[string]*sharedResult[T]
}

func (g *sharedRequests[T]) do(ctx context.Context, key string, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	g.mu.Lock()
	if g.active == nil {
		g.active = make(map[string]*sharedResult[T])
	}
	r := g.active[key]
	if r == nil {
		r = &sharedResult[T]{done: make(chan struct{})}
		g.active[key] = r
		go func() {
			work, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			defer cancel()
			r.value, r.err = fn(work)
			g.mu.Lock()
			delete(g.active, key)
			close(r.done)
			g.mu.Unlock()
		}()
	}
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case <-r.done:
		return r.value, r.err
	}
}
