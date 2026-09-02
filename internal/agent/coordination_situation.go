package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// coordinatorSituationBlock is the per-turn PUSH snapshot a coordinator would
// otherwise assemble by hand, every turn, with three tool calls.
//
// SES2570 made 62 list_workers, 63 list_agents and 69 get_view calls across 81
// turns — ~2.4 status calls per turn, almost all of them re-reading state that
// had not changed since the previous turn, at roughly 3K tokens a turn. The
// fleet snapshot was already pushed (coordinatorWorkerStatusBlock); the roster
// and the board were not, so the model kept fetching them.
//
// This block adds both and, critically, states that it is AUTHORITATIVE and
// refreshed every turn — without that sentence a model re-queries anyway,
// because it cannot tell a stale paste from a live projection.
func (r *Runtime) coordinatorSituationBlock(ctx context.Context, coordSessionID string) string {
	var sections []string
	if fleet := r.coordinatorWorkerStatusBlock(ctx, coordSessionID); fleet != "" {
		sections = append(sections, fleet)
	}
	if rota := r.coordinatorTrajectoryBlock(ctx, coordSessionID); rota != "" {
		sections = append(sections, rota)
	}
	if roster := r.coordinatorAgentRosterBlock(ctx); roster != "" {
		sections = append(sections, roster)
	}
	if board := r.coordinatorBoardBlock(ctx); board != "" {
		sections = append(sections, board)
	}
	if gate := r.reviewGateBlock(ctx); gate != "" {
		sections = append(sections, gate)
	}
	if len(sections) == 0 {
		return ""
	}
	sections = append(sections, coordinatorSituationFreshnessNote)
	return strings.Join(sections, "\n\n")
}

// CoordinatorSituationBlock is coordinatorSituationBlock for the api layer. The
// CHAT path composes its own dynamic suffix (composeTurnRequest) and therefore
// never went through autonomousDynamicSuffix — so an interactive coordinator got
// no fleet block at all and re-derived the whole picture by hand every turn.
// Returns "" for a session that is not a coordinator.
func (r *Runtime) CoordinatorSituationBlock(ctx context.Context, session db.Session) string {
	if !session.IsCoordinator() || strings.TrimSpace(session.ID) == "" {
		return ""
	}
	return r.coordinatorSituationBlock(ctx, session.ID)
}

// coordinatorSituationFreshnessNote is the half that actually saves the calls.
// It names the tools whose answers are already above, and says what each one is
// still good for — a coordinator that needs a worker's transcript or a single
// card's detail must still be able to ask.
const coordinatorSituationFreshnessNote = "<situation-freshness>\n" +
	"The blocks above are AUTHORITATIVE and regenerated from live state at the start of EVERY turn. " +
	"Do not spend a tool call re-reading them: list_workers, list_agents and get_view(board) return " +
	"the same information you have just been given.\n" +
	"Call them only for something the blocks omit — a specific card's full detail (get_view with a " +
	"sub id), one agent's configuration, or a worker's own transcript.\n" +
	"</situation-freshness>"

// coordinatorAgentRosterBlock lists the agents a coordinator may target with
// spawn_worker, each tagged with whether it can WRITE. The capability tag is the
// preventive half of the spawn capability gate (see checkWorkerCapability): a
// coordinator that can see "read-only" next to a name does not hand it a brief
// that says "create the file" — SES2570 did that six times in 70 minutes.
func (r *Runtime) coordinatorAgentRosterBlock(ctx context.Context) string {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		r.logger.Warn("coordination: agent roster block failed", "error", err)
		return ""
	}
	type row struct{ name, cap string }
	var rows []row
	for _, a := range agents {
		mutators, constrained, merr := agentMutationTools(a)
		capability := "read+write"
		switch {
		case merr != nil:
			// An unreadable allowlist denies every tool at the filter, so the honest
			// label is "unusable", not "unknown".
			capability = "UNUSABLE (allowed_tools is malformed)"
		case !constrained || len(mutators) > 0:
			if constrained {
				capability = "read+write (" + strings.Join(mutators, ", ") + ")"
			}
		default:
			capability = "READ-ONLY — cannot create, edit or run anything"
		}
		rows = append(rows, row{name: a.Name, cap: capability})
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	var b strings.Builder
	b.WriteString("<available-agents>\n")
	b.WriteString("Targets for spawn_worker, with what each one is ALLOWED to do:\n")
	for _, rw := range rows {
		fmt.Fprintf(&b, "- %s — %s\n", rw.name, rw.cap)
	}
	b.WriteString("A brief that must produce or change a file has to go to a read+write agent; " +
		"spawning a READ-ONLY agent for it is rejected.\n")
	b.WriteString("</available-agents>")
	return b.String()
}

// reviewGateBlock names the cards that have exhausted the review round budget
// and states, unambiguously, that another review round is not the next action.
// Empty when no card is over budget — the common case, so the block costs
// nothing on a healthy board.
func (r *Runtime) reviewGateBlock(ctx context.Context) string {
	// Active cards only, same as the board block above — an archived card cannot
	// be on a treadmill, whatever its round count says.
	tasks, err := r.db.ListActiveTasks(ctx)
	if err != nil {
		r.logger.Warn("coordination: review gate block failed", "error", err)
		return ""
	}
	var over []db.Task
	for _, t := range tasks {
		if t.ReviewBounces < db.ReviewRoundBudget {
			continue
		}
		if t.BoardState == db.BoardDone || t.BoardState == db.BoardCancelled {
			continue
		}
		over = append(over, t)
	}
	if len(over) == 0 {
		return ""
	}
	sort.Slice(over, func(i, j int) bool { return over[i].ReviewBounces > over[j].ReviewBounces })
	var b strings.Builder
	b.WriteString("<review-gate-exhausted>\n")
	for _, t := range over {
		fmt.Fprintf(&b, "- %s %q has FAILED %d verification rounds (budget %d).\n",
			t.ID, t.Title, t.ReviewBounces, db.ReviewRoundBudget)
	}
	b.WriteString("Another review round is NOT the next action for these cards. Spawning a fresh " +
		"reviewer here samples opinions instead of converging: each new reviewer re-litigates " +
		"findings earlier rounds already decided.\n")
	b.WriteString("Do exactly one of the following, and say which you chose:\n")
	b.WriteString("1. Narrow the card to what has already passed and move the rejected part to a NEW card.\n")
	b.WriteString("2. Ask the user to decide — state the disagreement between rounds and what it blocks.\n")
	b.WriteString("</review-gate-exhausted>")
	return b.String()
}

// coordinatorBoardBlock renders the kanban board as the same signal report
// get_view(board) would return, so the coordinator starts each turn already
// knowing which cards are open and which have stopped moving.
func (r *Runtime) coordinatorBoardBlock(ctx context.Context) string {
	// ListActiveTasks, not ListTasks: archived cards are finished work the user
	// has already cleared off the board. Including them made this block report a
	// 270-card board where the UI (and get_view, which loads the same active list)
	// shows 117 — a "done 197" column that no longer exists.
	tasks, err := r.db.ListActiveTasks(ctx)
	if err != nil {
		r.logger.Warn("coordination: board block failed", "error", err)
		return ""
	}
	if len(tasks) == 0 {
		return ""
	}
	v, err := view.ProjectBoard(view.BoardInput{Tasks: tasks}, view.LevelCard)
	if err != nil {
		r.logger.Warn("coordination: board block render failed", "error", err)
		return ""
	}
	return v.Text()
}
