package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// StopSubagent cancels an async subagent run started by parentSessionID.
//
// Ownership is enforced the same way the retry path enforces it: the target must
// be a subagent child of the CALLING session. Session ids are guessable, so
// without that check an agent could cancel work belonging to another branch of
// the tree — or to a human's own session — and the failure would look like the
// run had simply died.
//
// Reports whether a turn was actually cancelled. A child that already reached a
// terminal state is NOT an error: by the time the caller decides to stop a
// detached run, it finishing on its own is a race it cannot win, and turning that
// into a tool error would push the model into pointless retries.
func (r *Runtime) StopSubagent(ctx context.Context, parentSessionID, childSessionID string) (bool, error) {
	childSessionID = strings.TrimSpace(childSessionID)
	if childSessionID == "" {
		return false, fmt.Errorf("stop_subagent requires a subagent session id")
	}
	child, err := r.db.GetSession(ctx, childSessionID)
	if err != nil {
		return false, fmt.Errorf("subagent session %s not found: %w", childSessionID, err)
	}
	if child.Kind != subagentSessionKind {
		return false, fmt.Errorf("session %s is not a subagent run", childSessionID)
	}
	if child.ParentSessionID != parentSessionID {
		return false, fmt.Errorf("session %s is not one of your subagent runs", childSessionID)
	}
	if !r.CancelSession(childSessionID) {
		return false, nil
	}
	// The cancelled invoke writes its own terminal transcript as it unwinds, but
	// that write races with this return and a caller that immediately re-reads the
	// session must not see it still "running". Stamping the terminal state here is
	// idempotent with what the unwinding turn writes.
	if err := r.db.SetSessionRunState(ctx, childSessionID, "killed", time.Now().Unix()); err != nil {
		return true, fmt.Errorf("subagent %s was cancelled but its state could not be persisted: %w", childSessionID, err)
	}
	return true, nil
}

// withStopSubagent wires the stop_subagent tool for one completion, binding it to
// the session that is calling. The binding is what makes the ownership check
// possible: the tool itself never names a parent, so a caller cannot claim one.
func (r *Runtime) withStopSubagent(ctx context.Context) context.Context {
	parentSessionID := SessionIDFrom(ctx)
	fn := func(sctx context.Context, childSessionID string) (bool, error) {
		if parentSessionID == "" {
			return false, fmt.Errorf("subagent control requires a parent session")
		}
		return r.StopSubagent(sctx, parentSessionID, childSessionID)
	}
	return tools.WithStopAgent(ctx, fn)
}
