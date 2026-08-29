package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

type setRoleReq struct {
	Role string `json:"role"`
}

// handleSetSessionRole turns a session's COORDINATOR MODE on or off (M2,
// _Docs/47). The request field is still called "role" for wire compatibility and
// accepts "coordinator" (on) or "" (off).
//
// It no longer writes Session.Role: since the unlimited-depth rework Role is
// lineage ("worker" = spawned by a coordinator) and the coordinator capability is
// its own flag, so a WORKER can be given coordinator mode here and become a
// mid-level node instead of being severed from its parent. "worker" itself is
// still rejected — that lineage is established by spawn_worker, never by hand.
func (s *Server) handleSetSessionRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setRoleReq](w, r)
	if !ok {
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "" && role != "coordinator" {
		writeError(w, http.StatusBadRequest, "role must be \"coordinator\" or empty")
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	if _, err := wsp.DB.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	// Same path the agent-facing set_coordinator_mode tool takes, so both enforce
	// the "not while workers are running" rule and both refresh the prompt epoch —
	// otherwise a UI toggle would leave the frozen tool set behind.
	if _, err := wsp.Runtime.SetSessionCoordinatorMode(ctx, id, role == "coordinator"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Cross-window sync: a sibling window's sidebar swaps the role chip and the
	// coordination tools panel becomes visible/hidden as appropriate.
	emitSessionChange(wsp, id, "role")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "role": role})
}

// handleResumeCoordinator is the "Devam ettir" CTA behind the phantom-spawn halt
// badge: it clears the hard-halt state (and the nudge streak) and kicks one fresh
// coordinator turn. Idempotent-ish — pressing it on a non-halted coordinator just
// enqueues a turn. Rejected for a non-coordinator session so the UI never offers the
// action where it cannot apply.
func (s *Server) handleResumeCoordinator(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	wsp := ws(r)
	if err := wsp.Runtime.ResumeCoordinatorFromStall(ctx, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Cross-window sync: sibling windows drop the halt badge as the state clears.
	emitSessionChange(wsp, id, "coordinator-resume")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "resumed": true})
}

type setWorkflowReq struct {
	Workflow string `json:"workflow"`
}

// handleSetSessionWorkflow selects (or clears) the coordinator recipe (M5) for a
// session. The slug must resolve to a coordinator-workflow skill with a known
// pattern; an invalid selection is rejected rather than silently ignored. An
// empty slug clears the selection. Also persists the recipe's max_turns as the
// session's notify-loop cap override.
func (s *Server) handleSetSessionWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setWorkflowReq](w, r)
	if !ok {
		return
	}
	slug := strings.TrimSpace(req.Workflow)
	wsp := ws(r)
	ctx := r.Context()
	sess, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	if slug != "" && !sess.IsCoordinator() {
		writeError(w, http.StatusBadRequest, "session must be a coordinator to select a workflow")
		return
	}
	maxTurns, rerr := ResolveCoordinatorRecipe(wsp.Runtime.Skills(), slug)
	if rerr != nil {
		writeError(w, http.StatusBadRequest, rerr.Error())
		return
	}
	if err := wsp.DB.SetSessionCoordinatorWorkflow(ctx, id, slug, maxTurns); writeDBError(w, err, "") {
		return
	}
	emitSessionChange(wsp, id, "workflow")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "workflow": slug, "maxTurns": maxTurns})
}

// handleListWorkers returns the workers spawned under a coordinator session, for
// the coordination UI panel.
func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()
	if _, err := wsp.DB.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	workers, err := wsp.Runtime.ListWorkers(ctx, id)
	if writeDBError(w, err, "") {
		return
	}
	out := make([]map[string]any, 0, len(workers))
	for _, wk := range workers {
		out = append(out, map[string]any{
			"sessionId": wk.SessionID,
			"agentName": wk.AgentName,
			// Agent identity, so the panel can render a worker with the shared
			// agent-identity component (avatar + name + id + model) instead of a
			// bare name. Empty when the worker's agent row no longer exists.
			"agentId":       wk.AgentID,
			"agentAvatar":   wk.AgentAvatar,
			"agentColor":    wk.AgentColor,
			"agentProvider": wk.AgentProvider,
			"agentModel":    wk.AgentModel,
			"agentDeleted":  wk.AgentDeleted,
			"title":         wk.Title,
			"running":       wk.Running,
			"delegating":    wk.Delegating,
			"queued":        wk.Queued,
			"summary":       wk.Summary,
			"startedAt":     wk.StartedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"workers": out})
}

// handleSessionCoordinatorTree returns the whole coordinator tree a session
// belongs to, in breadth-first order from the ROOT — callable with ANY member's
// id (root, mid-level node, or leaf), which is normalized to the root first. Same
// contract as GET /api/flow-runs/{id}/tree (_Docs/62), so the UI can hand it
// whatever session the user is currently looking at.
func (s *Server) handleSessionCoordinatorTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()
	tree, err := wsp.DB.ListCoordinatorTree(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	// Cost is rolled up as the tree is walked. Billing is per-session, so a deep
	// tree is otherwise an invisible spend: the root's own card shows only the
	// coordinator's turns, while the actual work — and the actual money — sits in
	// descendants the user never opens. Merging the per-model stats (rather than
	// summing each session's dollar figure) keeps the arithmetic identical to the
	// Budget screen, including the prompt-cache savings.
	treeByModel := map[string]db.KindStat{}
	nodes := make([]map[string]any, 0, len(tree))
	for _, sess := range tree {
		node := map[string]any{
			"sessionId":       sess.ID,
			"agentName":       wsp.Runtime.AgentName(sess.AgentID),
			"title":           sess.Title,
			"depth":           sess.CoordinatorDepth,
			"parentSessionId": sess.CoordinatorSessionID,
			"isCoordinator":   sess.IsCoordinator(),
			"state":           sess.State,
			"running":         wsp.Runtime.IsSessionActive(sess.ID),
			// A parked send_to_worker follow-up, delivered when this node's turn ends —
			// the same signal the flat roster badges, surfaced here so a queued message
			// on a deep node is visible from the root too.
			"queued": wsp.Runtime.HasQueuedMessage(sess.ID),
			// Health, so a broken branch is visible from the root instead of only
			// inside the session that broke. A tree is exactly where this matters:
			// the deeper a failure sits, the less likely anyone opens that session.
			"health": coordinatorNodeHealth(sess),
			// Still owes its coordinator a report (see Session.CoordinatorReportPending)
			// — surfaced because a node stuck in this state is the one shape of
			// "silently blocking the whole branch above it".
			"reportPending": sess.CoordinatorReportPending,
			// Phantom-spawn hard-halt: auto-turns stopped for this coordinator until a
			// human resumes it — a distinct, actionable state from generic "stuck".
			"stallHalted": wsp.Runtime.CoordinatorStallHalted(sess.ID),
		}
		if u, err := wsp.DB.GetSessionUsage(ctx, sess.ID); err == nil {
			mergeModelStats(treeByModel, u.ByModel)
			_, cost, _, _, _, _, _ := modelRowsFor(u.ByModel)
			node["calls"] = u.Calls
			node["tokens"] = u.InputTokens + u.OutputTokens
			node["costUSD"] = cost
		}
		nodes = append(nodes, node)
	}
	_, treeCost, treeSavings, priced, estimated, _, _ := modelRowsFor(treeByModel)
	root := ""
	if len(tree) > 0 {
		root = tree[0].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rootSessionId": root,
		"nodes":         nodes,
		// Whole-tree spend, so the cost of a fan-out is visible at the root.
		"totalCostUSD":    treeCost,
		"totalSavingsUSD": treeSavings,
		"priced":          priced,
		"estimated":       estimated,
	})
}

// coordinatorNodeHealth classifies a tree node for the UI: "stuck" (autonomous
// turns are being refused — the branch cannot recover on its own), "error" (its
// last turn failed) or "" (fine).
//
// Derived from the auto-tags the turn pipeline already writes rather than from a
// new signal, so the tree agrees with the session list and the repair automations
// instead of inventing a second notion of "broken". Ordered worst-first: a stuck
// session is usually also tagged error, and stuck is the one that needs a human.
func coordinatorNodeHealth(sess db.Session) string {
	has := func(tag string) bool {
		for _, t := range sess.Tags {
			if t == tag {
				return true
			}
		}
		return false
	}
	switch {
	case has(agent.TagStuck):
		return "stuck"
	case has(agent.TagAuthError), has(agent.TagError), has(agent.TagToolError):
		return "error"
	default:
		return ""
	}
}

// mergeModelStats accumulates one session's per-model usage into a running total.
// Kept as raw token counts (not dollars) so the final rollup prices everything in
// one pass — per-session rounding would drift, and prompt-cache savings are only
// computable from the token split.
func mergeModelStats(dst map[string]db.KindStat, src map[string]db.KindStat) {
	for model, s := range src {
		cur := dst[model]
		cur.Calls += s.Calls
		cur.InputTokens += s.InputTokens
		cur.OutputTokens += s.OutputTokens
		cur.CacheReadTokens += s.CacheReadTokens
		cur.CacheWriteTokens += s.CacheWriteTokens
		dst[model] = cur
	}
}

// handleSessionCoordinatorAncestors returns the chain from a session's tree ROOT
// down to its direct coordinator (root first) — the breadcrumb a worker walks
// upward. Empty for a root or an ordinary session.
func (s *Server) handleSessionCoordinatorAncestors(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()
	chain, err := wsp.DB.ListCoordinatorAncestors(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	out := make([]map[string]any, 0, len(chain))
	for _, sess := range chain {
		out = append(out, map[string]any{
			"sessionId": sess.ID,
			"agentName": wsp.Runtime.AgentName(sess.AgentID),
			"title":     sess.Title,
			"depth":     sess.CoordinatorDepth,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ancestors": out})
}
