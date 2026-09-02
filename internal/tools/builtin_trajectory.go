package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// builtin_trajectory.go exposes the Rota ("trajectory") tool to a coordinator
// session (_Docs/77 F1). The trajectory is the declared-plus-observed graph of
// the coordinator tree: the runtime records what happens (spawns, reports,
// runs, gates) on its own; this tool is how the agent ANNOUNCES the plan it is
// following and moves between its phases, so the graph — and the Rota screen —
// show intent next to the observations.
//
// Gated like the other coordination tools: the runner is injected per turn
// (CoordinationFuncs.Trajectory) and only a coordinator session has one; the
// mutating actions are wired only on a ROOT coordinator (a sub-coordinator's
// phases belong to the root's plan).

// TrajectoryPhaseInput is one phase of an agent-declared plan.
type TrajectoryPhaseInput struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	// Gate is the exit condition: {kind: artifact|verdict|human|schema, value}.
	Gate *TrajectoryGateInput `json:"gate,omitempty"`
}

// TrajectoryGateInput is a phase's exit condition as the agent writes it.
type TrajectoryGateInput struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

// TrajectoryFuncs is the agent-side implementation behind the trajectory tool.
// Get is always set for a coordinator; Plan / Phase / Finish only on a root.
type TrajectoryFuncs struct {
	// Get renders the current graph as text.
	Get func(ctx context.Context) (string, error)
	// Plan declares (or re-declares) the phase list.
	Plan func(ctx context.Context, phases []TrajectoryPhaseInput) (string, error)
	// Phase moves one phase to a state (active | done | skipped | failed).
	Phase func(ctx context.Context, id, state, reason string) (string, error)
	// Finish closes the trajectory (done | failed) with an optional reason.
	Finish func(ctx context.Context, status, reason string) (string, error)
}

// TrajectoryToolName is the tool's registered name.
const TrajectoryToolName = "trajectory"

type trajectoryInput struct {
	Action string                 `json:"action"`
	Phases []TrajectoryPhaseInput `json:"phases"`
	ID     string                 `json:"id"`
	State  string                 `json:"state"`
	Status string                 `json:"status"`
	Reason string                 `json:"reason"`
}

// TrajectoryTool is the trajectory (Rota) tool.
type TrajectoryTool struct{}

func NewTrajectoryTool() TrajectoryTool { return TrajectoryTool{} }

func (TrajectoryTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "trajectory",
		Description: "Read or announce this coordinator tree's TRAJECTORY (Rota): the phases the work " +
			"goes through and what has happened under each. The runtime records spawns, worker reports, " +
			"flow runs and human gates by itself; use this tool to declare the plan and to move between " +
			"phases so the graph shows your intent next to the facts.\n\n" +
			"Actions: `get` renders the graph. `plan` declares the phase list (ids like plan/code/review; " +
			"re-planning may add phases but never drops one that is active or done). `phase` moves a " +
			"phase to `active` (closing the previously active one as done), `done`, `skipped` or `failed`. " +
			"`finish` closes the whole trajectory as `done` or `failed`. A recipe-driven coordinator " +
			"already has its phases: call `phase` when you move on, and `finish` at the end.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": { "type": "string", "enum": ["get", "plan", "phase", "finish"] },
    "phases": {
      "type": "array",
      "description": "plan: the ordered phase list.",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "description": "Lowercase slug, e.g. plan, code, review." },
          "label": { "type": "string" },
          "profile": { "type": "string", "description": "Expected worker profile (planner, coder, validator, …)." },
          "optional": { "type": "boolean" },
          "gate": {
            "type": "object",
            "properties": {
              "kind": { "type": "string", "enum": ["artifact", "verdict", "human", "schema"] },
              "value": { "type": "string" }
            },
            "required": ["kind"],
            "additionalProperties": false
          }
        },
        "required": ["id"],
        "additionalProperties": false
      }
    },
    "id": { "type": "string", "description": "phase: the phase id." },
    "state": { "type": "string", "enum": ["active", "done", "skipped", "failed"], "description": "phase: the new state." },
    "status": { "type": "string", "enum": ["done", "failed"], "description": "finish: the final status." },
    "reason": { "type": "string", "description": "phase/finish: one line explaining a skip, failure or early finish." }
  },
  "required": ["action"],
  "additionalProperties": false
}`),
	}
}

func (TrajectoryTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[trajectoryInput](TrajectoryToolName, input)
	if err != nil {
		return "", err
	}
	cf := CoordinationFrom(ctx)
	if cf == nil || cf.Trajectory == nil || cf.Trajectory.Get == nil {
		return "", fmt.Errorf("trajectory is only available in a coordinator session")
	}
	f := cf.Trajectory
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "get", "":
		return f.Get(ctx)
	case "plan":
		if f.Plan == nil {
			return "", fmt.Errorf("only the ROOT coordinator may declare the plan; this session is a sub-coordinator")
		}
		if len(in.Phases) == 0 {
			return "", fmt.Errorf("plan needs a non-empty \"phases\" list")
		}
		return f.Plan(ctx, in.Phases)
	case "phase":
		if f.Phase == nil {
			return "", fmt.Errorf("only the ROOT coordinator may move phases; this session is a sub-coordinator")
		}
		if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.State) == "" {
			return "", fmt.Errorf("phase needs \"id\" and \"state\"")
		}
		return f.Phase(ctx, in.ID, strings.ToLower(strings.TrimSpace(in.State)), in.Reason)
	case "finish":
		if f.Finish == nil {
			return "", fmt.Errorf("only the ROOT coordinator may finish the trajectory; this session is a sub-coordinator")
		}
		status := strings.ToLower(strings.TrimSpace(in.Status))
		if status == "" {
			status = "done"
		}
		return f.Finish(ctx, status, in.Reason)
	default:
		return "", fmt.Errorf("unknown action %q (want get | plan | phase | finish)", in.Action)
	}
}
