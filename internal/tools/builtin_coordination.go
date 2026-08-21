package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// builtin_coordination.go exposes the M2 coordinator/worker tools (see _Docs/47).
// They are registered ONLY for a session with coordinator mode on and read their
// runner from the context (like run_subagent), so an ordinary session never sees
// them.
//
// Since the unlimited-depth rework a WORKER may have coordinator mode too (its
// spawner asked for it, or it turned the mode on itself), which is exactly how a
// tree nests past one level. The recursion brake is therefore no longer "workers
// can't see these tools" but the explicit depth + subtree budgets enforced in
// agent.SpawnWorker.

// CoordinationToolNames is every tool in this file — the coordination SURFACE, as
// opposed to the work tools an agent uses to actually do things.
//
// The distinction matters because these tools are gated by the SESSION
// (CoordinationFuncs, built from db.Session), not by the agent's tool config: a
// session either may drive workers / owes a report upward, or it does not. Callers
// that filter an agent's work tools consult IsCoordinationTool to leave this
// surface alone — see agent.toolFilter for the case that made it necessary.
var CoordinationToolNames = []string{
	"spawn_worker",
	"send_to_worker",
	"stop_worker",
	"list_workers",
	"report_to_coordinator",
	"set_coordinator_mode",
}

// IsCoordinationTool reports whether name is part of the coordination surface.
func IsCoordinationTool(name string) bool {
	for _, n := range CoordinationToolNames {
		if n == name {
			return true
		}
	}
	return false
}

// WorkerSpawnSpec carries the nesting options of one spawn_worker call from the
// tool layer down to the runtime.
type WorkerSpawnSpec struct {
	ModelOverride string
	// Coordinator makes the spawned worker a sub-coordinator (it may spawn its own
	// workers). Rejected past the depth limit rather than downgraded.
	Coordinator bool
	// Workflow pins a coordinator recipe on the sub-coordinator; only meaningful
	// together with Coordinator.
	Workflow string
	// WorkingDir pins the worker session's cwd. Empty means "inherit the
	// coordinator's own working directory" — a worker that silently landed in the
	// workspace default while its task described another repository was the whole
	// reason this field exists.
	WorkingDir string
}

// WorkerRow is the structured view of one worker session, the row shape behind
// list_workers (CoordinationFuncs.ListRows). It mirrors the runtime's
// agent.WorkerInfo plus the timestamps and stuck flag needed for filtering and
// sorting without making the tools package depend on the agent package.
type WorkerRow struct {
	SessionID  string `json:"sessionId"`
	AgentName  string `json:"agentName"`
	Title      string `json:"title"`
	Running    bool   `json:"running"`
	Delegating bool   `json:"delegating,omitempty"`
	Queued     bool   `json:"queued,omitempty"`
	Stuck      bool   `json:"stuck,omitempty"`
	Summary    string `json:"summary,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

// CoordinationFuncs is the agent-side implementation the coordination tools call.
// Injected per turn via WithCoordination, capturing the coordinator session id so
// the tools need only carry the worker-facing arguments.
type CoordinationFuncs struct {
	// Spawn launches a background worker (existing agent target) under the
	// coordinator and returns its session id immediately.
	Spawn func(ctx context.Context, agentRef, task string, spec WorkerSpawnSpec) (SpawnResult, error)
	// Send delivers a follow-up to an existing worker and re-runs its turn. When the
	// worker is still mid-turn the message is parked in a single-slot per-worker
	// queue and delivered the moment that turn ends; SendResult reports which of the
	// two happened (and, when queued, how long the running turn has been going) so
	// the coordinator need not reach for stop_worker.
	Send func(ctx context.Context, workerSessionID, message string) (SendResult, error)
	// Stop cancels an in-flight worker turn (and, for a sub-coordinator, its whole
	// subtree).
	Stop func(ctx context.Context, workerSessionID string) error
	// List returns a human-readable snapshot of this coordinator's workers. subtree
	// widens it from the direct children to every descendant.
	List func(ctx context.Context, subtree bool) (string, error)
	// ListRows returns the same workers as structured rows so list_workers can
	// filter (state), sort and paginate. subtree widens it to every descendant.
	ListRows func(ctx context.Context, subtree bool) ([]WorkerRow, error)
	// Report closes this session's task upstream, for a session that is itself a
	// worker of another coordinator. Nil on a root coordinator (nothing above it),
	// which is what gates registration of report_to_coordinator.
	Report func(ctx context.Context, status, summary string) error
	// SetMode turns THIS session's coordinator capability on or off at the agent's
	// own initiative. Returns a human-readable note (e.g. when the change only
	// takes effect on the next turn).
	SetMode func(ctx context.Context, enabled bool) (string, error)
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
	Coordinator   bool   `json:"coordinator"`
	Workflow      string `json:"workflow"`
	Cwd           string `json:"cwd"`
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
			"research), \"planner\" (read-only; turns a task into an ordered implementation plan " +
			"with file:line change sites and a verify command), \"coder\" (write/edit code), " +
			"\"reviewer\" (read-only review), \"validator\" " +
			"(runs tests/typecheck/build/e2e to prove a change works and returns a compact PASS/FAIL " +
			"verdict, does NOT edit code) — which is " +
			"materialized into a reusable worker agent. The worker runs detached; you do NOT wait for " +
			"it. When it finishes, its result is injected back into THIS session as a <task-notification> " +
			"and a new coordinator turn starts automatically. Call several times in one turn to fan out " +
			"parallel workers, then end your turn.\n\n" +
			"Set `coordinator: true` to make the worker a SUB-COORDINATOR that can split its task further " +
			"and drive its own workers. Use it only when the subtask genuinely decomposes into independent " +
			"parts — every extra level multiplies turns and tokens, and a sub-coordinator reports back only " +
			"once its whole branch is done. It is refused (not silently downgraded) past the configured " +
			"depth limit. For a sub-coordinator prefer an EXISTING agent over a profile: the profiles are " +
			"single-purpose leaf personas (an \"explore\" sub-coordinator is told both to research and to " +
			"delegate), so pick the agent whose role actually owns that branch.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "agent": { "type": "string", "description": "An existing agent (name or id) OR a profile: \"explore\" | \"planner\" | \"coder\" | \"reviewer\" | \"validator\"." },
    "task": { "type": "string", "description": "A self-contained instruction. The worker starts fresh and sees only this — include file paths, line numbers, and what 'done' means." },
    "modelOverride": { "type": "string", "description": "Optional model id override (provider unchanged)." },
    "coordinator": { "type": "boolean", "description": "Make this worker a sub-coordinator that may spawn its own workers. Default false (a plain leaf worker). Only for tasks that genuinely decompose further." },
    "workflow": { "type": "string", "description": "Optional coordinator recipe slug for the sub-coordinator (only with coordinator: true). Not inherited from you — set it deliberately or leave empty for free coordination." },
    "cwd": { "type": "string", "description": "Absolute path the worker runs in. Defaults to YOUR working directory. Set it whenever the task targets a different repository — a path written in the task text is only prose, it does not move the worker." }
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
	if !in.Coordinator && strings.TrimSpace(in.Workflow) != "" {
		// A recipe only means something to a coordinator. Accepting it silently on a
		// leaf worker would look like it took effect.
		return "", fmt.Errorf("\"workflow\" only applies to a sub-coordinator; pass \"coordinator\": true or drop it")
	}
	res, err := f.Spawn(ctx, in.Agent, in.Task, WorkerSpawnSpec{
		ModelOverride: strings.TrimSpace(in.ModelOverride),
		Coordinator:   in.Coordinator,
		Workflow:      strings.TrimSpace(in.Workflow),
		WorkingDir:    strings.TrimSpace(in.Cwd),
	})
	if err != nil {
		return "", err
	}
	kind := "worker"
	if in.Coordinator {
		kind = "SUB-COORDINATOR (it may spawn its own workers, and reports back only when its whole branch is done)"
	}
	msg := fmt.Sprintf("Spawned %s %q (session %s). It runs in the background; you will get a <task-notification> when it finishes. Do not wait for it — end your turn.", kind, res.AgentName, res.SessionID)
	msg += treeBudgetLine(res.TreeBudgetUsed, res.TreeBudgetTotal)
	return msg, nil
}

// treeBudgetLine renders the coordinator tree's remaining live-worker quota after
// a spawn, with an escalating warning as it fills, so a coordinator sees the wall
// coming instead of only hitting it. Empty when no ceiling is configured
// (total <= 0) — there is nothing to report then.
func treeBudgetLine(used, total int) string {
	if total <= 0 {
		return ""
	}
	remaining := total - used
	if remaining < 0 {
		remaining = 0
	}
	line := fmt.Sprintf("\nTree budget: %d/%d live worker sessions in use (%d remaining; finished workers are reclaimed automatically).", used, total, remaining)
	switch frac := float64(used) / float64(total); {
	case frac >= 0.90:
		line += " ⚠️ Over 90% used — conclude or stop finished workers before fanning out further, or further spawns will be refused."
	case frac >= 0.75:
		line += " ⚠️ Over 75% used — watch the remaining capacity."
	}
	return line
}

// ---- send_to_worker ----

// SendResult reports how a send_to_worker landed. Exactly one of Delivered /
// Queued is true. When Queued, the worker was still running a prior turn and the
// message was parked in its single-slot queue; RunningForSeconds is the elapsed
// time of that in-flight turn (0 when unknown), surfaced so the coordinator can
// see the worker is genuinely busy rather than wedged — and therefore keep
// waiting instead of issuing a destructive stop_worker.
type SendResult struct {
	Delivered         bool
	Queued            bool
	RunningForSeconds int64
}

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
	res, err := f.Send(ctx, in.Worker, in.Message)
	if err != nil {
		return "", err
	}
	if res.Queued {
		busy := ""
		if res.RunningForSeconds > 0 {
			busy = fmt.Sprintf(" (running for %ds)", res.RunningForSeconds)
		}
		return fmt.Sprintf("Worker %s is still running its current turn%s, so your message was QUEUED. "+
			"It will be delivered automatically the instant that turn ends, and you will get a <task-notification> "+
			"when the follow-up finishes. Do NOT stop_worker — the worker is busy, not stuck, and stopping would "+
			"discard its in-flight work. Only one message can be queued per worker; sending another before this one "+
			"is delivered will be refused.", in.Worker, busy), nil
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

type listWorkersInput struct {
	Scope  string `json:"scope"`
	State  string `json:"state"`
	Sort   string `json:"sort"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// ListWorkersTool reports this coordinator's workers and their status (Claude
// Code's TaskList), optionally widened to the whole subtree below it.
type ListWorkersTool struct{}

func NewListWorkersTool() ListWorkersTool { return ListWorkersTool{} }

func (ListWorkersTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_workers",
		Description: "List the workers spawned under this coordinator and their status (running / finished, " +
			"with a one-line summary of each finished worker's last reply). Use it to see what is still in " +
			"flight before deciding your next step. Pass scope \"subtree\" to also see the workers your " +
			"sub-coordinators spawned (reported flat, without indentation). Results are PAGINATED: pass limit " +
			"(default 20, max 100) and offset to page; the reply reports total and hasMore, and you reach the " +
			"next page with offset += limit. Filters: state (running | idle | stuck). Sort: updated_desc " +
			"(default), updated_asc, created_desc, created_asc, name_asc, name_desc (name = worker agent name).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "scope": { "type": "string", "enum": ["children", "subtree"], "description": "\"children\" (default) = your direct workers only. \"subtree\" = every descendant, including sub-coordinators' workers." },
    "state": { "type": "string", "enum": ["running", "idle", "stuck"], "description": "Only workers in this state: running = a turn is in flight; idle = finished/not running; stuck = tagged stuck by the watchdog." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc)." },
    "limit": { "type": "integer", "description": "Max workers per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching workers to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (ListWorkersTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	f := CoordinationFrom(ctx)
	if f == nil || f.ListRows == nil {
		return "", fmt.Errorf("list_workers is only available in a coordinator session")
	}
	// The schema allows an empty body, so a missing/blank input means defaults
	// rather than a parse error. Parse ONCE into a single struct — the previous
	// code unmarshaled the same input twice (Scope-only struct, then a separate
	// State/Sort/Limit/Offset struct).
	var in listWorkersInput
	if len(strings.TrimSpace(string(input))) > 0 && string(input) != "{}" {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	scope := strings.TrimSpace(in.Scope)
	if scope == "" {
		scope = "children"
	}
	switch scope {
	case "children", "subtree":
	default:
		return "", fmt.Errorf("scope must be \"children\" or \"subtree\", got %q", scope)
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	rows, err := f.ListRows(ctx, scope == "subtree")
	if err != nil {
		return "", err
	}

	state := strings.TrimSpace(in.State)
	switch state {
	case "", "running", "idle", "stuck":
	default:
		return "", fmt.Errorf("state must be one of running, idle, stuck; got %q", state)
	}
	matches := make([]WorkerRow, 0, len(rows))
	for _, w := range rows {
		switch state {
		case "running":
			if !w.Running {
				continue
			}
		case "idle":
			if w.Running {
				continue
			}
		case "stuck":
			if !w.Stuck {
				continue
			}
		}
		matches = append(matches, w)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	// Sort the FILTERED rows (matches), not the raw rows: less compares by index,
	// and matches is a strict subset — indexing rows with matches' positions
	// compares wrong elements whenever a state filter is active.
	less, err := SortByField(matches, field, asc,
		func(w WorkerRow) int64 { return w.UpdatedAt },
		func(w WorkerRow) int64 { return w.CreatedAt },
		func(w WorkerRow) string { return w.AgentName },
		func(w WorkerRow) string { return w.SessionID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	return pageResult(page, total, offset, limit)
}

// ---- report_to_coordinator ----

type reportToCoordinatorInput struct {
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

// ReportToCoordinatorTool closes a SUB-COORDINATOR's task upstream. It exists
// because a mid-level node's turn ending means nothing: it typically ends right
// after fanning out its own workers, while its actual result only exists once
// those workers report and it synthesizes them. The runtime therefore withholds
// the completion notification while its workers are live, and this tool is how
// the node says "now I am done".
//
// Registered only when the session HAS a coordinator above it.
type ReportToCoordinatorTool struct{}

func NewReportToCoordinatorTool() ReportToCoordinatorTool { return ReportToCoordinatorTool{} }

func (ReportToCoordinatorTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "report_to_coordinator",
		Description: "Report YOUR finished result to the coordinator that spawned you, closing your task " +
			"upstream. You are a sub-coordinator: simply ending a turn does NOT report you as done (your " +
			"coordinator is told you are still delegating while your own workers run). Call this once your " +
			"part is genuinely complete — with the synthesis written out in full, because your coordinator " +
			"cannot read your workers' sessions. If you are blocked or a worker failed, still call it with " +
			"status \"failed\" or \"incomplete\" and say what is missing; staying silent stalls everything " +
			"above you.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": { "type": "string", "description": "The complete result for your coordinator: your own synthesis of your workers' findings, concrete and self-contained (file paths, line numbers, what changed, what is left)." },
    "status": { "type": "string", "enum": ["completed", "incomplete", "failed"], "description": "\"completed\" (default) only when the task is genuinely done. \"incomplete\" when partially done, \"failed\" when it could not be done." }
  },
  "required": ["summary"],
  "additionalProperties": false
}`),
	}
}

func (ReportToCoordinatorTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[reportToCoordinatorInput]("report_to_coordinator", input)
	if err != nil {
		return "", err
	}
	in.Summary = strings.TrimSpace(in.Summary)
	if in.Summary == "" {
		return "", fmt.Errorf("\"summary\" is required — your coordinator cannot read your workers' sessions, so an empty report tells it nothing")
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "completed"
	}
	switch status {
	case "completed", "incomplete", "failed":
	default:
		return "", fmt.Errorf("status must be \"completed\", \"incomplete\" or \"failed\", got %q", status)
	}
	f := CoordinationFrom(ctx)
	if f == nil || f.Report == nil {
		return "", fmt.Errorf("report_to_coordinator is only available in a session that was spawned by a coordinator")
	}
	if err := f.Report(ctx, status, in.Summary); err != nil {
		return "", err
	}
	return fmt.Sprintf("Reported to your coordinator with status %q. Your task is now closed upstream — do not report again unless it sends you new work.", status), nil
}

// ---- set_coordinator_mode ----

type setCoordinatorModeInput struct {
	Enabled *bool `json:"enabled"`
}

// SetCoordinatorModeTool lets an agent turn its OWN session's coordinator mode on
// or off, without a user toggling it in the UI. Registered on every session that
// may hold the capability, including ones that do not have it yet — otherwise an
// ordinary session could never turn it on.
type SetCoordinatorModeTool struct{}

func NewSetCoordinatorModeTool() SetCoordinatorModeTool { return SetCoordinatorModeTool{} }

func (SetCoordinatorModeTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "set_coordinator_mode",
		Description: "Turn coordinator mode on or off for YOUR OWN session. With it on you get the " +
			"coordinator manual and the spawn_worker/send_to_worker/stop_worker/list_workers tools, so you " +
			"can split work across background workers; with it off you work alone. Turn it on when a task " +
			"is big enough to genuinely parallelize, and off when you are back to single-threaded work. " +
			"Turning it OFF is refused while you still have running workers — stop or await them first.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "enabled": { "type": "boolean", "description": "true to become a coordinator, false to go back to working alone." }
  },
  "required": ["enabled"],
  "additionalProperties": false
}`),
	}
}

func (SetCoordinatorModeTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[setCoordinatorModeInput]("set_coordinator_mode", input)
	if err != nil {
		return "", err
	}
	if in.Enabled == nil {
		return "", fmt.Errorf("\"enabled\" is required (true or false)")
	}
	f := CoordinationFrom(ctx)
	if f == nil || f.SetMode == nil {
		return "", fmt.Errorf("set_coordinator_mode is not available in this session")
	}
	return f.SetMode(ctx, *in.Enabled)
}
