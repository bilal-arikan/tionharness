package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/skills"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// coordinatorRecipeBlock returns the selected coordinator recipe (M5) as a system
// block for a coordinator session: the recipe's markdown body plus its suggested
// worker targets and stop condition. Returns "" when no recipe is selected, the
// slug does not resolve, or the resolved skill is not a coordinator-workflow.
// The recipe is ADVISORY guidance layered on top of the coordinator manual — it
// never removes tools or hard-limits behaviour (max_turns is enforced separately
// via Session.CoordinatorMaxTurns).
func coordinatorRecipeBlock(wsp *workspace.Workspace, session db.Session) string {
	if session.Role != "coordinator" || strings.TrimSpace(session.CoordinatorWorkflow) == "" {
		return ""
	}
	store := wsp.Runtime.Skills()
	if store == nil {
		return ""
	}
	sk, ok := store.Get(session.CoordinatorWorkflow)
	if !ok || !sk.IsCoordinatorWorkflow() {
		return ""
	}
	body, err := store.Body(session.CoordinatorWorkflow)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Active workflow: %s", sk.Name)
	if sk.Pattern != "" {
		fmt.Fprintf(&b, " (pattern: %s)", sk.Pattern)
	}
	b.WriteString("\nThe user selected this saved orchestration recipe for this session. Follow it as your operating plan for the task, on top of the general coordinator rules above.\n\n")
	b.WriteString(strings.TrimSpace(body))
	if len(sk.WorkerTargets) > 0 {
		fmt.Fprintf(&b, "\n\n**Suggested worker targets:** %s (advisory — override when the task needs different agents/profiles).", strings.Join(sk.WorkerTargets, ", "))
	}
	if sc := strings.TrimSpace(sk.StopCondition); sc != "" {
		fmt.Fprintf(&b, "\n\n**Stop condition:** %s — keep spawning follow-up workers until this holds, then synthesize and report.", sc)
	}
	return b.String()
}

// ResolveCoordinatorRecipe validates a recipe slug for a coordinator session and
// returns the per-session max-turns override to persist (0 = keep the workspace
// default). Thin alias over skills.ResolveCoordinatorWorkflow, which lives in the
// leaf package so a flow's coordinator node can share the same gate.
func ResolveCoordinatorRecipe(store *skills.Store, slug string) (maxTurns int, err error) {
	return skills.ResolveCoordinatorWorkflow(store, slug)
}

// coordinationScratchpadBlock returns a system-context line pointing a coordinator
// session and its workers at a SHARED scratchpad directory (M2/M3). The directory
// lives under the COORDINATOR session's on-disk folder so every worker resolves the
// same absolute path; it is created lazily. Returns "" for ordinary sessions.
func coordinationScratchpadBlock(wsp *workspace.Workspace, session db.Session) string {
	var coordID string
	switch {
	case session.Role == "coordinator":
		coordID = session.ID
	case session.CoordinatorSessionID != "":
		coordID = session.CoordinatorSessionID
	default:
		return ""
	}
	dir, err := wsp.DB.SessionDir(coordID)
	if err != nil {
		return ""
	}
	pad := filepath.Join(dir, "scratchpad")
	if err := os.MkdirAll(pad, 0o755); err != nil {
		return ""
	}
	return fmt.Sprintf(
		"## Shared scratchpad\nThis coordinator and all its workers share this directory for durable cross-worker notes:\n%s\nRead and write files here (with the normal file tools) to share findings, plans, and interim results across workers instead of repeating them in every task or notification. Prefer small, well-named files (e.g. findings.md, plan.md).",
		pad)
}

// The coordinator operating manual (M2, _Docs/47) lives in the central prompt
// registry (internal/prompts, key "coordinator"); buildStaticPrefix resolves it
// per workspace via agent.WorkspacePrompt. Adapted from Claude Code's
// coordinator mode, rewritten for TionSwarm's real tool surface: workers are
// async background sessions launched with spawn_worker, continued with
// send_to_worker, stopped with stop_worker, and inspected with list_workers;
// their results arrive as <task-notification> user messages that auto-trigger
// the next coordinator turn.
