package db

import (
	"fmt"
	"strings"
)

// Session origin kinds — WHO started a session. This is the single vocabulary
// for session lineage: every creation path stamps exactly one of these (see
// createSessionLocked), and every consumer that wants to draw an edge between
// sessions (the network graph, the trajectory/"Rota" projection, the executions
// feed) reads Session.Lineage() instead of guessing from Kind/SourceID/
// ParentSessionID/CoordinatorSessionID.
//
// Kind stays what it is — the broad transcript category shown in the sidebar —
// and is NOT redefined here; origin is an additional axis, not a replacement.
const (
	// OriginUser: created by a human through the UI/API (plain chat).
	OriginUser = "user"
	// OriginSpawn: a detached spawn ordered by an agent or the API (spawn_session,
	// POST /spawn) that is neither a worker nor a subagent.
	OriginSpawn = "spawn"
	// OriginCoordinator: a worker spawned by a coordinator (spawn_worker).
	// TriggerSessionID is the coordinator, RootSessionID the tree root.
	OriginCoordinator = "coordinator"
	// OriginSubagent: a synchronous run_subagent child. TriggerSessionID is the
	// parent turn's session.
	OriginSubagent = "subagent"
	// OriginFlow: the per-run transcript of a flow run, or a session a flow node
	// opened (coordinator node). EntityID is the flow, RunID the run, NodeID the
	// node when one applies.
	OriginFlow = "flow"
	// OriginSchedule: a scheduled prompt delivery (reuse thread or spawn-mode run).
	// EntityID is the schedule when known.
	OriginSchedule = "schedule"
	// OriginAutomation: an automation fire (continue thread or one-shot run).
	// EntityID is the automation; TriggerSessionID the session whose turn/tag
	// caused the fire when the trigger was session-scoped.
	OriginAutomation = "automation"
	// OriginHandoff: a context-reset continuation. TriggerSessionID is the session
	// this one continues (also mirrored in ParentSessionID).
	OriginHandoff = "handoff"
	// OriginInsight: the read-only transcript of a retrospective insight scan.
	// RunID is the scan run id.
	OriginInsight = "insight"
)

var originKinds = []string{
	OriginUser, OriginSpawn, OriginCoordinator, OriginSubagent, OriginFlow,
	OriginSchedule, OriginAutomation, OriginHandoff, OriginInsight,
}

// SessionOrigin records who started a session and from where. Stamped ONCE at
// creation (createSessionLocked) and never rewritten afterwards, except that a
// flow run's RunID may be filled in right after the run row exists
// (SetSessionOriginRun) because the transcript session is created before the run.
//
// Field meaning depends on Kind (see the Origin* constants). Every field but Kind
// and At is optional; consumers must tolerate an empty EntityID/RunID/NodeID.
type SessionOrigin struct {
	Kind string `json:"kind"`
	// EntityID names the owning entity: automation (AUT…), schedule (SCH…), flow
	// (FLW…). Empty when the origin has no entity (user, spawn, handoff).
	EntityID string `json:"entityId,omitempty"`
	// RunID is the flow run (RUN…) or insight scan run this session belongs to.
	RunID string `json:"runId,omitempty"`
	// NodeID is the flow node that opened this session (a coordinator node), or
	// later a trajectory phase id.
	NodeID string `json:"nodeId,omitempty"`
	// TriggerSessionID is the session whose activity caused this one to exist:
	// the coordinator for a worker, the parent turn for a subagent, the handed-off
	// session for a continuation, the tagged session for a tag-fired automation.
	TriggerSessionID string `json:"triggerSessionId,omitempty"`
	// RootSessionID is the top of this session's tree. Empty means SELF (this
	// session is a root) — never a self-reference, mirroring FlowRun.RootRunID and
	// Session.RootCoordinatorSessionID. Read it through Session.RootSession().
	RootSessionID string `json:"rootSessionId,omitempty"`
	// At is when the origin was stamped (unix seconds) — the session's CreatedAt.
	At int64 `json:"at"`
}

// IsValidOriginKind reports whether k is one of the Origin* constants.
func IsValidOriginKind(k string) bool {
	for _, o := range originKinds {
		if o == k {
			return true
		}
	}
	return false
}

// validateOrigin rejects an origin with an unknown kind. An empty kind is a bug
// in the caller (createSessionLocked always fills it), so it fails too.
func validateOrigin(o *SessionOrigin) error {
	if o == nil {
		return nil
	}
	if !IsValidOriginKind(o.Kind) {
		return fmt.Errorf("invalid session origin kind %q", o.Kind)
	}
	return nil
}

// Lineage returns the session's origin, deriving one from the legacy fields when
// the header predates Session.Origin. The derivation is the SAME one the boot
// loader applies in memory (reconcileHeader) and createSessionLocked uses as the
// default when a caller passes no explicit origin, so a session created by an
// old build and one created today answer identically.
//
// Derivation order matters: the coordinator back-link beats everything (a worker
// is spawned-by, whatever its Kind says), then the execution classification,
// then Kind + SourceID, then ParentSessionID (handoff), then user.
func (s Session) Lineage() SessionOrigin {
	if s.Origin != nil {
		return *s.Origin
	}
	o := deriveOrigin(s)
	o.At = s.CreatedAt
	return o
}

// RootSession returns the id of the top of this session's tree, resolving the
// "empty means self" encoding of SessionOrigin.RootSessionID. A root session
// reports its own id.
func (s Session) RootSession() string {
	if root := s.Lineage().RootSessionID; root != "" {
		return root
	}
	return s.ID
}

// deriveOrigin builds an origin from the legacy lineage fields. It never
// consults Origin itself (callers do that) so it can serve as both the backfill
// and the default-at-creation path.
func deriveOrigin(s Session) SessionOrigin {
	coord := strings.TrimSpace(s.CoordinatorSessionID)
	if coord != "" || s.Role == SessionRoleWorker {
		root := s.RootCoordinatorSessionID
		if root == "" {
			// Pre-root-stamp worker: every such tree was one level deep, so the
			// coordinator IS the root (same rule as RootCoordinator()).
			root = coord
		}
		return SessionOrigin{Kind: OriginCoordinator, TriggerSessionID: coord, RootSessionID: root}
	}
	if s.ExecutionType == ExecutionSubagent {
		return SessionOrigin{Kind: OriginSubagent, TriggerSessionID: s.ParentSessionID, RootSessionID: s.ParentSessionID}
	}
	switch s.Kind {
	case "flow":
		return SessionOrigin{Kind: OriginFlow, EntityID: s.SourceID}
	case "flow-coordinator":
		return SessionOrigin{Kind: OriginFlow, RunID: s.SourceID}
	case "insight":
		return SessionOrigin{Kind: OriginInsight, RunID: s.SourceID}
	case "schedule", "schedule-run":
		return SessionOrigin{Kind: OriginSchedule}
	case "automation":
		return SessionOrigin{Kind: OriginAutomation, EntityID: s.SourceID}
	case "automation-run":
		return SessionOrigin{Kind: OriginAutomation, TriggerSessionID: s.ParentSessionID}
	}
	if parent := strings.TrimSpace(s.ParentSessionID); parent != "" {
		// A parent without a coordinator link is a context-reset continuation: the
		// continuation stays in its parent's tree (same lane, "next" edge).
		return SessionOrigin{Kind: OriginHandoff, TriggerSessionID: parent, RootSessionID: parent}
	}
	if s.Kind == "spawned" {
		return SessionOrigin{Kind: OriginSpawn}
	}
	return SessionOrigin{Kind: OriginUser}
}
