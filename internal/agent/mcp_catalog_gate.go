package agent

import (
	"context"
	"sync"
)

// Catalog failure bookkeeping must finish before a queued build checks the
// breaker. The pool's connection mutex alone cannot protect that sequence.
type mcpCatalogGate struct {
	once  sync.Once
	token chan struct{}
}

func (g *mcpCatalogGate) acquire(ctx context.Context) (func(), error) {
	g.once.Do(func() { g.token = make(chan struct{}, 1) })
	select {
	case g.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-g.token
			return nil, err
		}
		return func() { <-g.token }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
