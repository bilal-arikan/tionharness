package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// launchOriginKey carries a launcher-supplied session origin (RunSpec.Origin)
// from LaunchRun down to runFlowRecorded, which creates the run's transcript
// session. A context value rather than a parameter because the flow entry
// points (RunFlowRecorded / RunFlowRecordedStream / runFlowRecorded) already
// thread a dispatch key the same way and their signatures are shared with the
// HTTP handlers, which never carry an origin.
type launchOriginKey struct{}

// withLaunchOrigin tags ctx with the origin the produced session must carry.
func withLaunchOrigin(ctx context.Context, o *db.SessionOrigin) context.Context {
	if o == nil {
		return ctx
	}
	return context.WithValue(ctx, launchOriginKey{}, o)
}

// launchOriginFromContext returns the launcher-supplied origin, or nil.
func launchOriginFromContext(ctx context.Context) *db.SessionOrigin {
	o, _ := ctx.Value(launchOriginKey{}).(*db.SessionOrigin)
	return o
}

// flowTranscriptSessionKey carries the id of the per-run transcript session that
// runFlowRecorded created BEFORE the FlowRun row exists, so RunFlow can link the
// two the moment the row is created (FlowRun.SessionID + Origin.RunID) instead
// of after the run finishes. Distinct from WithSessionID on purpose: that key
// also names the CALLER's session when an agent node's run_flow tool starts a
// child run, and linking the child to the caller's transcript would be wrong.
type flowTranscriptSessionKey struct{}

// withFlowTranscriptSession tags ctx with the run's own transcript session id.
func withFlowTranscriptSession(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, flowTranscriptSessionKey{}, sessionID)
}

// flowTranscriptSessionFromContext returns the run's transcript session id set
// by runFlowRecorded ("" for a run started without one).
func flowTranscriptSessionFromContext(ctx context.Context) string {
	s, _ := ctx.Value(flowTranscriptSessionKey{}).(string)
	return s
}

// flowSessionOrigin builds the origin for a flow run's transcript session: the
// launcher's own origin when one was supplied (an automation or schedule that
// runs a flow), else a plain flow origin naming the flow. The run id is filled
// in by RunFlow (SetSessionOriginRun) once the run row exists; a parent run's
// node — a subflow/spawn node or an agent node's run_flow — is recorded so the
// child's session hangs under the right node of the parent's graph.
func flowSessionOrigin(ctx context.Context, flowID string) *db.SessionOrigin {
	var o db.SessionOrigin
	if launched := launchOriginFromContext(ctx); launched != nil {
		o = *launched
	} else {
		o = db.SessionOrigin{Kind: db.OriginFlow, EntityID: flowID}
	}
	if parentRunID, parentNodeID, _ := runLineage(ctx); parentRunID != "" {
		if o.NodeID == "" {
			o.NodeID = parentNodeID
		}
		if o.TriggerSessionID == "" {
			// The parent run's transcript session, when this run was started from
			// inside one (the value survives into subflow/spawn/run_flow contexts).
			o.TriggerSessionID = flowTranscriptSessionFromContext(ctx)
		}
	}
	if o.TriggerSessionID == "" {
		// A run_flow call from an ordinary session (a coordinator's turn, say):
		// the caller's session tripped this run. Only the trigger is recorded;
		// the run's transcript stays its own per-run session.
		o.TriggerSessionID = SessionIDFrom(ctx)
	}
	return &o
}
