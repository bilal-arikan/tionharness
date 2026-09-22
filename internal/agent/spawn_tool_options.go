package agent

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// SessionKindChat is the kind of an ordinary, human-continuable conversation —
// the sidebar's "Sohbet" group.
const SessionKindChat = "chat"

// SpawnToolOptions builds the SpawnOptions for a session an agent opens with the
// spawn_session tool (TSK1005).
//
// Such a session is an independent top-level conversation, not a delegated run:
// the spawner does not wait for it and nothing reports back. So it is created as
// a plain "chat" rather than the generic "spawned" kind, and lands under the
// sidebar's "Sohbet" label next to every other conversation. Real delegations
// keep their own classification — run_subagent children are created by
// RunSubagent with the subagent category, spawn_worker children by SpawnWorker
// with the worker kind — and never pass through here.
//
// Because Kind alone would now read as a user-started chat, the lineage is
// stamped explicitly: origin "spawn", triggered by the caller's session (when
// known), so the session inspector, the Rota graph and the trajectory binder
// still attribute it to the session that spawned it.
func SpawnToolOptions(callerAgentID, callerSessionID, modelOverride, workingDir string) SpawnOptions {
	return SpawnOptions{
		ModelOverride: modelOverride,
		CreatedBy:     callerAgentID,
		WorkingDir:    workingDir,
		Kind:          SessionKindChat,
		Origin: &db.SessionOrigin{
			Kind:             db.OriginSpawn,
			TriggerSessionID: strings.TrimSpace(callerSessionID),
		},
	}
}
