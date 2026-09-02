package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// coordinatorLeadBlock composes the system-prompt lead that ONLY a coordinator
// session receives, in injection order:
//
//	manual        the shared coordinator operating manual (registry prompt)
//	agentPrompt   this agent's own Agent.CoordinatorPrompt — per-agent delegation
//	              direction, riding directly behind the manual
//	recipe        the selected coordinator recipe (M5), if any
//	subordinate   a mid-level node's place in the tree + upward-reporting contract
//
// Every part is optional and blank parts are dropped whole: an empty agentPrompt
// contributes NOTHING — no header, no blank block — so the field is free for an
// agent that never coordinates. The caller gates the whole block on
// Session.IsCoordinator(), which is what keeps it out of a non-coordinator turn.
func coordinatorLeadBlock(manual, agentPrompt, recipe, subordinate string) string {
	parts := make([]string, 0, 4)
	for _, p := range []string{manual, agentPrompt, recipe, subordinate} {
		if t := strings.TrimSpace(p); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// coordinatorRecipeBlock returns the selected coordinator recipe (M5) as a system
// block for a coordinator session: the recipe's markdown body plus its suggested
// worker targets and stop condition. Returns "" when no recipe is selected, the
// slug does not resolve, or the resolved skill is not a coordinator-workflow.
// The recipe is ADVISORY guidance layered on top of the coordinator manual — it
// never removes tools or hard-limits behaviour (max_turns is enforced separately
// via Session.CoordinatorMaxTurns).
func coordinatorRecipeBlock(wsp *workspace.Workspace, session db.Session) string {
	if !session.IsCoordinator() || strings.TrimSpace(session.CoordinatorWorkflow) == "" {
		return ""
	}
	store := wsp.Runtime.Skills()
	if store == nil {
		return ""
	}
	// The session may carry "slug@version" (R6); the current file resolves.
	slug, _ := skills.ParseRecipeRef(session.CoordinatorWorkflow)
	sk, ok := store.Get(slug)
	if !ok || !sk.IsCoordinatorWorkflow() {
		return ""
	}
	body, err := store.Body(slug)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Active workflow: %s", sk.Name)
	if sk.Pattern != "" {
		fmt.Fprintf(&b, " (pattern: %s)", sk.Pattern)
	}
	if sk.Recipe != nil && len(sk.Recipe.Phases) > 0 {
		// The declared phases, so the coordinator and the trajectory seeded from
		// this recipe name the same steps (_Docs/77 R6).
		b.WriteString("\n**Declared phases (in order):** ")
		for i, p := range sk.Recipe.Phases {
			if i > 0 {
				b.WriteString(" → ")
			}
			b.WriteString(p.ID)
			if p.Profile != "" {
				fmt.Fprintf(&b, " (%s)", p.Profile)
			}
			if p.Gate != nil {
				fmt.Fprintf(&b, " [gate: %s", p.Gate.Kind)
				if p.Gate.Value != "" {
					fmt.Fprintf(&b, " %q", p.Gate.Value)
				}
				b.WriteString("]")
			}
			if p.Optional {
				b.WriteString(" (optional)")
			}
		}
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

// resolvableRecipe returns slug when it names a coordinator recipe that exists in
// this workspace, and "" otherwise. It is the install/seed-time filter for the
// coordinator recipe an agent (pack or template) was published with: a pinned slug
// that does not resolve here would leave the agent claiming a recipe while actually
// running free coordination, so it is dropped at the door instead. An empty slug
// passes through unchanged — "no recipe" is the normal case, not a failure.
func resolvableRecipe(store *skills.Store, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	if _, err := ResolveCoordinatorRecipe(store, slug); err != nil {
		return ""
	}
	return slug
}

// coordinationScratchpadBlock returns a system-context line pointing a coordinator
// session and its workers at a SHARED scratchpad directory (M2/M3). The directory
// lives under the COORDINATOR session's on-disk folder so every worker resolves the
// same absolute path; it is created lazily. Returns "" for ordinary sessions.
func coordinationScratchpadBlock(wsp *workspace.Workspace, session db.Session) string {
	// Keyed on the tree ROOT, not the direct parent: in a nested tree every node —
	// root, mid-level, leaf — must resolve the SAME absolute path, otherwise each
	// sub-coordinator would open its own private scratchpad and the cross-worker
	// notes would fragment by level. For a one-level tree the root IS the direct
	// coordinator, so this is unchanged behaviour there.
	coordID := session.RootCoordinator()
	if coordID == "" {
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

// coordinatorSubordinateBlock tells a MID-LEVEL coordinator (a worker session that
// also drives its own workers) where it sits in the tree and — the part that
// actually matters — that finishing its first turn is NOT finishing its task.
//
// Without this the nesting silently misreports: a mid-level node's first turn ends
// right after it spawns its sub-workers, and if it let that turn be reported as
// "completed" its parent would move on while the subtree is still working. The
// runtime already withholds the completion notification while sub-workers are
// live (see runWorker), but the model must know it owns the moment of reporting —
// hence the explicit report_to_coordinator contract.
//
// Returns "" for a root coordinator (no parent to report to).
func coordinatorSubordinateBlock(session db.Session) string {
	if session.CoordinatorSessionID == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## You are a sub-coordinator\n")
	fmt.Fprintf(&b, "You were spawned as a worker by coordinator session %s and you are at depth %d of a coordinator tree (root: %s). You are BOTH a worker (you owe that coordinator a result) and a coordinator (you may spawn your own workers with spawn_worker).\n\n",
		session.CoordinatorSessionID, session.CoordinatorDepth, session.RootCoordinator())
	b.WriteString("Reporting contract — this is the part you must not get wrong:\n")
	b.WriteString("- Ending a turn does NOT report your task as done. As long as your own workers are running, your coordinator is told you are still delegating.\n")
	b.WriteString("- When your part of the work is genuinely finished, call `report_to_coordinator` with the synthesized result. That — and only that — closes your task upstream.\n")
	b.WriteString("- Synthesize your workers' findings YOURSELF before reporting. Do not forward raw worker output or write \"see my workers' results\": your coordinator cannot read your workers' sessions.\n")
	b.WriteString("- If you cannot finish (blocked, out of budget, a worker failed), still call `report_to_coordinator` with status `failed` or `incomplete` and say what is missing. Silence stalls the whole tree above you.\n")
	b.WriteString("- Only delegate further if the work genuinely splits into independent parts. Depth costs turns and tokens at every level; do the work yourself when it fits in one session.")
	return b.String()
}

// The coordinator operating manual (M2, _Docs/47) lives in the central prompt
// registry (internal/prompts, key "coordinator"); buildStaticPrefix resolves it
// per workspace via agent.WorkspacePrompt. Adapted from Claude Code's
// coordinator mode, rewritten for TionHarness's real tool surface: workers are
// async background sessions launched with spawn_worker, continued with
// send_to_worker, stopped with stop_worker, and inspected with list_workers;
// their results arrive as <task-notification> user messages that auto-trigger
// the next coordinator turn.
