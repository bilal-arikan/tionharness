package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// builtin_coordination.go exposes the M2 coordinator/worker tools (see _Docs/47).
// They are registered ONLY for a coordinator session and read their runner from
// the context (like run_subagent), so an ordinary or worker session never sees
// them — that also blocks a worker from spawning its own workers (recursion).

// CoordinationFuncs is the agent-side implementation the coordination tools call.
// Injected per turn via WithCoordination, capturing the coordinator session id so
// the tools need only carry the worker-facing arguments.
type CoordinationFuncs struct {
	// Spawn launches a background worker (existing agent target) under the
	// coordinator and returns its session id immediately.
	Spawn func(ctx context.Context, agentRef, task, modelOverride string) (SpawnResult, error)
	// Send delivers a follow-up to an existing worker and re-runs its turn.
	Send func(ctx context.Context, workerSessionID, message string) error
	// Stop cancels an in-flight worker turn.
	Stop func(ctx context.Context, workerSessionID string) error
	// List returns a human-readable snapshot of this coordinator's workers.
	List func(ctx context.Context) (string, error)
}

type coordinationKey struct{}

// WithCoordination attaches the coordination runner to ctx for one turn. Only a
// coordinator session's turn installs it; its presence also gates registration of
// the coordination tools in buildRegistry.
func WithCoordination(ctx context.Context, f *CoordinationFuncs) context.Context {
	return context.WithValue(ctx, coordinationKey{}, f)
}

// CoordinationFrom returns the coordination runner on ctx, or nil when the turn is
// not a coordinator turn.
func CoordinationFrom(ctx context.Context) *CoordinationFuncs {
	f, _ := ctx.Value(coordinationKey{}).(*CoordinationFuncs)
	return f
}

// ---- spawn_worker ----

type spawnWorkerInput struct {
	Agent         string `json:"agent"`
	Task          string `json:"task"`
	ModelOverride string `json:"modelOverride"`
}

// SpawnWorkerTool launches an async background worker under the current
// coordinator session. Unlike run_subagent (sync, returns the reply this turn),
// a worker runs detached; when it finishes, its result arrives as a
// <task-notification> and a fresh coordinator turn is triggered automatically —
// so you fan out here, end your turn, and react to notifications as they land.
type SpawnWorkerTool struct{}

func NewSpawnWorkerTool() SpawnWorkerTool { return SpawnWorkerTool{} }

func (SpawnWorkerTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "spawn_worker",
		Description: "Launch an asynchronous background WORKER under this coordinator session. `agent` is " +
			"either an EXISTING agent (name or id) OR a built-in profile — \"explore\" (read-only " +
			"research), \"coder\" (write/edit code), \"reviewer\" (read-only review) — which is " +
			"materialized into a reusable worker agent. The worker runs detached; you do NOT wait for " +
			"it. When it finishes, its result is injected back into THIS session as a <task-notification> " +
			"and a new coordinator turn starts automatically. Call several times in one turn to fan out " +
			"parallel workers, then end your turn.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "agent": { "type": "string", "description": "An existing agent (name or id) OR a profile: \"explore\" | \"coder\" | \"reviewer\"." },
    "task": { "type": "string", "description": "A self-contained instruction. The worker starts fresh and sees only this — include file paths, line numbers, and what 'done' means." },
    "modelOverride": { "type": "string", "description": "Optional model id override (provider unchanged)." }
  },
  "required": ["agent", "task"],
  "additionalProperties": false
}`),
	}
}

func (SpawnWorkerTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[spawnWorkerInput]("spawn_worker", input)
	if err != nil {
		return "", err
	}
	in.Agent = strings.TrimSpace(in.Agent)
	in.Task = strings.TrimSpace(in.Task)
	if in.Agent == "" || in.Task == "" {
		return "", fmt.Errorf("both \"agent\" and \"task\" are required")
	}
	f := CoordinationFrom(ctx)
	if f == nil || f.Spawn == nil {
		return "", fmt.Errorf("spawn_worker is only available in a coordinator session")
	}
	res, err := f.Spawn(ctx, in.Agent, in.Task, strings.TrimSpace(in.ModelOverride))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Spawned worker %q (session %s). It runs in the background; you will get a <task-notification> when it finishes. Do not wait for it — end your turn.", res.AgentName, res.SessionID), nil
}

// ---- send_to_worker ----

type sendToWorkerInput struct {
	Worker  string `json:"worker"`
	Message string `json:"message"`
}

// SendToWorkerTool continues an existing worker with a follow-up, reusing its
// loaded context (Claude Code's SendMessage-to-worker). The worker re-runs its
// turn and notifies you again on completion.
type SendToWorkerTool struct{}

func NewSendToWorkerTool() SendToWorkerTool { return SendToWorkerTool{} }

func (SendToWorkerTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "send_to_worker",
		Description: "Send a follow-up to an existing worker (by its session id from spawn_worker or a " +
			"<task-notification>'s task-id). The worker continues with its FULL prior context and re-runs " +
			"its turn; its new result arrives as another <task-notification>. Use this to continue a " +
			"worker whose context overlaps the next task (e.g. it just researched the files you now want " +
			"changed), or to correct a worker after a failure.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "worker": { "type": "string", "description": "The worker's session id (task-id from its <task-notification>)." },
    "message": { "type": "string", "description": "The follow-up instruction. Self-contained; include specifics (file:line, what to change, what 'done' looks like)." }
  },
  "required": ["worker", "message"],
  "additionalProperties": false
}`),
	}
}

func (SendToWorkerTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[sendToWorkerInput]("send_to_worker", input)
	if err != nil {
		return "", err
	}
	in.Worker = strings.TrimSpace(in.Worker)
	in.Message = strings.TrimSpace(in.Message)
	if in.Worker == "" || in.Message == "" {
		return "", fmt.Errorf("both \"worker\" and \"message\" are required")
	}
	f := CoordinationFrom(ctx)
	if f == nil || f.Send == nil {
		return "", fmt.Errorf("send_to_worker is only available in a coordinator session")
	}
	if err := f.Send(ctx, in.Worker, in.Message); err != nil {
		return "", err
	}
	return fmt.Sprintf("Sent follow-up to worker %s. It is re-running in the background; you will get a <task-notification> when it finishes.", in.Worker), nil
}

// ---- stop_worker ----

type stopWorkerInput struct {
	Worker string `json:"worker"`
}

// StopWorkerTool cancels an in-flight worker turn (Claude Code's TaskStop). The
// worker session survives and can be continued later with send_to_worker.
type StopWorkerTool struct{}

func NewStopWorkerTool() StopWorkerTool { return StopWorkerTool{} }

func (StopWorkerTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "stop_worker",
		Description: "Stop a worker you sent in the wrong direction (Claude Code's TaskStop). Its current " +
			"turn is cancelled and reported as killed; the worker session survives, so you can redirect it " +
			"afterwards with send_to_worker. Pass the worker's session id.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "worker": { "type": "string", "description": "The worker's session id to stop." }
  },
  "required": ["worker"],
  "additionalProperties": false
}`),
	}
}

func (StopWorkerTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[stopWorkerInput]("stop_worker", input)
	if err != nil {
		return "", err
	}
	in.Worker = strings.TrimSpace(in.Worker)
	if in.Worker == "" {
		return "", fmt.Errorf("\"worker\" is required")
	}
	f := CoordinationFrom(ctx)
	if f == nil || f.Stop == nil {
		return "", fmt.Errorf("stop_worker is only available in a coordinator session")
	}
	if err := f.Stop(ctx, in.Worker); err != nil {
		return "", err
	}
	return fmt.Sprintf("Stopped worker %s. Continue it later with send_to_worker if needed.", in.Worker), nil
}

// ---- list_workers ----

// ListWorkersTool reports this coordinator's workers and their status (Claude
// Code's TaskList).
type ListWorkersTool struct{}

func NewListWorkersTool() ListWorkersTool { return ListWorkersTool{} }

func (ListWorkersTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_workers",
		Description: "List the workers spawned under this coordinator and their status (running / " +
			"finished, with a one-line summary of each finished worker's last reply). Use it to see what " +
			"is still in flight before deciding your next step.",
		InputSchema: json.RawMessage(`{ "type": "object", "properties": {}, "additionalProperties": false }`),
	}
}

func (ListWorkersTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	f := CoordinationFrom(ctx)
	if f == nil || f.List == nil {
		return "", fmt.Errorf("list_workers is only available in a coordinator session")
	}
	return f.List(ctx)
}
