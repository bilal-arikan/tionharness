package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

const (
	// subagentSessionKind is the session kind every run_subagent child carries.
	subagentSessionKind = "subagent"
	// runStateRunning is the RunState a child holds while its turn is in flight.
	runStateRunning = "running"
)

// resolveRetryLineage validates a run_subagent retry_of reference and returns the
// lineage to stamp on the new child session.
//
// A retry does NOT reuse the previous session: the failed transcript is what the
// caller (and a human reading the tree later) needs in order to judge whether the
// second attempt was worth making, so it stays untouched and the new run is a
// fresh child linked back to it.
//
// The reference is refused unless it names a subagent child of THIS caller's
// session: session ids are guessable, and without the ownership check one agent
// could chain its runs onto another session's tree — corrupting a lineage it does
// not own and leaking the existence of sessions it cannot otherwise see.
//
// A still-running attempt is refused too. Retrying it would leave two live runs
// answering the same contract, double-spending budget and racing to report; the
// caller must stop it first (stop_subagent) or wait for it to finish.
func (r *Runtime) resolveRetryLineage(ctx context.Context, parentSessionID, retryOf string) (prevID string, attempt int, err error) {
	retryOf = strings.TrimSpace(retryOf)
	if retryOf == "" {
		return "", 0, nil
	}
	prev, err := r.db.GetSession(ctx, retryOf)
	if err != nil {
		return "", 0, fmt.Errorf("retry_of session %s not found: %w", retryOf, err)
	}
	if prev.Kind != subagentSessionKind {
		return "", 0, fmt.Errorf("retry_of %s is not a subagent run", retryOf)
	}
	if prev.ParentSessionID != parentSessionID {
		return "", 0, fmt.Errorf("retry_of %s is not one of your subagent runs", retryOf)
	}
	if prev.RunState == runStateRunning {
		return "", 0, fmt.Errorf("retry_of %s is still running; stop it or wait for it to finish before retrying", retryOf)
	}
	attempt = prev.Attempt
	if attempt < 1 {
		// Runs that predate the lineage fields carry no attempt number; they are the
		// first try by definition.
		attempt = 1
	}
	return prev.ID, attempt + 1, nil
}

// stampRetryLineage writes the resolved lineage onto a child session record. A
// non-retry run is left untouched: Attempt stays 0 rather than 1 so "this row is
// part of a retry chain" remains a single readable condition, and so no existing
// session.json gains a field it did not have.
func stampRetryLineage(meta *db.Session, prevID string, attempt int) {
	if prevID == "" {
		return
	}
	meta.RetryOfSessionID = prevID
	meta.Attempt = attempt
}
