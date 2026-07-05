package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// SpawnResult is the outcome of a spawn: the new independent session's id and the
// resolved target agent's name.
type SpawnResult struct {
	SessionID string
	AgentName string
}

// SpawnFunc opens a new independent session, records the prompt, and runs the
// target agent's turn in the background (fire-and-forget), returning immediately.
// Implemented in the agent package (which owns the runtime) and injected at
// construction so the tools package need not import it. target is a display name
// or id; modelOverride swaps only the model ("" keeps the agent's own).
type SpawnFunc func(ctx context.Context, target, prompt, modelOverride string) (SpawnResult, error)

// spawnSessionInput is the argument shape for the spawn_session tool.
type spawnSessionInput struct {
	Agent         string `json:"agent"`
	Prompt        string `json:"prompt"`
	ModelOverride string `json:"modelOverride"`
}

// SpawnSessionTool lets an agent launch a brand-new, independent session for
// another agent and walk away — a fire-and-forget parallel worker. Unlike
// run_subagent's synchronous mode (same turn, returns the reply), spawn opens a
// FRESH session and runs the turn in the background; the spawner does not wait.
// It backs run_subagent's wait:"async" mode and is the building block for
// autonomous "swarm" fan-out.
//
// A per-turn budget caps how many spawns one turn may launch, complementing the
// runtime's global concurrency cap, so a single turn can't trigger a spawn storm.
type SpawnSessionTool struct {
	selfID     string
	maxPerTurn int
	launched   *int32 // per-tool-instance counter (registry is rebuilt each turn)
	spawn      SpawnFunc
}

// NewSpawnSessionTool constructs the tool bound to the calling agent's id, the
// per-turn spawn budget, and the spawn function. maxPerTurn <= 0 disables the
// per-turn cap (the global concurrency cap still applies).
func NewSpawnSessionTool(selfID string, maxPerTurn int, spawn SpawnFunc) *SpawnSessionTool {
	var n int32
	return &SpawnSessionTool{selfID: selfID, maxPerTurn: maxPerTurn, launched: &n, spawn: spawn}
}

func (*SpawnSessionTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "spawn_session",
		Description: "Spawn a NEW, independent session for an agent in this workspace and let it " +
			"run on its own — fire-and-forget. The agent works the prompt in the background in a " +
			"SEPARATE session; you do NOT wait for it and its result does NOT come back to this " +
			"conversation — it lands in that new session's transcript (visible in the activity " +
			"feed). So do NOT promise to relay its answer here. When you instead need an answer " +
			"back NOW, in this turn, use run_subagent (if available). Use spawn_session to fan " +
			"work out to parallel autonomous workers. Returns the new session id.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "agent": { "type": "string", "description": "Name (or id) of the agent that will run the spawned session." },
    "prompt": { "type": "string", "description": "The task/instruction the spawned agent should work on." },
    "modelOverride": { "type": "string", "description": "Optional model id to use instead of the agent's default (provider is unchanged)." }
  },
  "required": ["agent", "prompt"],
  "additionalProperties": false
}`),
	}
}

func (t *SpawnSessionTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[spawnSessionInput]("spawn_session", input)
	if err != nil {
		return "", err
	}
	in.Agent = strings.TrimSpace(in.Agent)
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.ModelOverride = strings.TrimSpace(in.ModelOverride)
	if in.Agent == "" || in.Prompt == "" {
		return "", fmt.Errorf("both \"agent\" and \"prompt\" are required")
	}
	if t.spawn == nil {
		return "", fmt.Errorf("spawning is not available in this context")
	}
	// Per-turn budget: bound how many spawns this turn may launch.
	if t.maxPerTurn > 0 {
		if atomic.AddInt32(t.launched, 1) > int32(t.maxPerTurn) {
			atomic.AddInt32(t.launched, -1)
			return "", fmt.Errorf("spawn budget (%d per turn) exhausted; do not spawn more this turn", t.maxPerTurn)
		}
	}
	res, err := t.spawn(ctx, in.Agent, in.Prompt, in.ModelOverride)
	if err != nil {
		if t.maxPerTurn > 0 {
			atomic.AddInt32(t.launched, -1) // refund a failed spawn
		}
		return "", err
	}
	return fmt.Sprintf("Spawned a new session for agent %q (session %s). It is running in the background; you do not need to wait for it.", res.AgentName, res.SessionID), nil
}
