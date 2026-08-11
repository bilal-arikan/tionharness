package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// Flow self-management tools let an agent create, edit, delete and list
// multi-agent orchestration flows in its workspace. No provenance gate: an
// agent may edit/delete any flow, user- or agent-created.
//
// A flow's graph is the orchestration JSON (see internal/orchestration.Graph):
// a set of nodes (agent / branch / parallel) wired by edges, with template
// placeholders like {{input}} / {{last}} / {{node.id}}.

// runFlowFn runs a flow by id with the given input and returns the finished run.
type runFlowFn func(ctx context.Context, flowID, input string) (db.FlowRun, error)

// resumeFlowFn delivers input to a run suspended at an await-input node and
// resumes it (background), returning the run (now running). Errors if the run is
// not currently waiting (already resumed/finished/unknown).
type resumeFlowFn func(ctx context.Context, runID, input string) (db.FlowRun, error)

type flowDeps struct {
	db      *db.DB
	actorID string
	run     runFlowFn
	resume  resumeFlowFn
}

// requireFlow loads a flow by id, returning a friendly error if it does not
// exist. No provenance gate: user- and agent-created flows are both editable.
func (d flowDeps) requireFlow(ctx context.Context, id string) (db.Flow, error) {
	f, err := d.db.GetFlow(ctx, id)
	if err != nil {
		return db.Flow{}, fmt.Errorf("no flow with id %q (use list_flows)", id)
	}
	return f, nil
}

// validGraphJSON ensures the supplied graph string is a structurally valid
// orchestration graph: it parses into an orchestration.Graph AND passes the
// engine's Validate (start node present, unique ids, resolvable references,
// agent nodes assigned). This rejects broken graphs at create/update time
// instead of letting them fail only when run. An empty string is allowed and
// defaults to an empty graph.
func validGraphJSON(graph string) error {
	graph = strings.TrimSpace(graph)
	if graph == "" {
		return nil
	}
	g, err := orchestration.ParseGraph(graph)
	if err != nil {
		return fmt.Errorf("graph must be valid JSON: %w. %s", err, graphSchemaHint(err.Error()))
	}
	if err := g.Validate(); err != nil {
		return fmt.Errorf("invalid graph: %w. %s", err, graphSchemaHint(err.Error()))
	}
	return nil
}

// nodeSchemaCheat is a compact one-line reminder of the node field names that
// agents most often get wrong. Appended to graph errors so the fix is visible
// in the tool result instead of requiring a separate doc lookup.
const nodeSchemaCheat = `Node fields: agent={type:"agent",agentId,prompt,next}; ` +
	`parallel={type:"parallel",parallel:["id1","id2"],joinNext}; ` +
	`branch={type:"branch",branches:[{contains,next}]}.`

// graphSchemaHint inspects a graph error string and returns a short, actionable
// "do this" hint (a few words) tailored to the most common mistakes, falling
// back to the field cheat-sheet. Keeps tool errors self-correcting.
func graphSchemaHint(msg string) string {
	switch {
	case strings.Contains(msg, "Node.nodes.branches"):
		// Tried to use branches:["id"] (strings) — usually a parallel fan-out.
		return `Fix: parallel fan-out uses "parallel":["id1","id2"],"joinNext":"id" — not "branches"/"next". ` + nodeSchemaCheat
	case strings.Contains(msg, "has no children"):
		return `Fix: list child agent node ids in "parallel":["id1","id2"]. ` + nodeSchemaCheat
	case strings.Contains(msg, "has no branches"):
		return `Fix: branch node needs "branches":[{"contains":"x","next":"id"}]. ` + nodeSchemaCheat
	case strings.Contains(msg, "must be an agent node"):
		return `Fix: parallel children must be type "agent" nodes. ` + nodeSchemaCheat
	default:
		return nodeSchemaCheat
	}
}

// CreateFlowTool creates a multi-agent orchestration flow.
type CreateFlowTool struct{ d flowDeps }

// NewCreateFlowTool constructs create_flow.
func NewCreateFlowTool(database *db.DB, actorID string) CreateFlowTool {
	return CreateFlowTool{d: flowDeps{db: database, actorID: actorID}}
}

func (CreateFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_flow",
		Description: "Create a multi-agent orchestration flow. The graph is the orchestration JSON (nodes of type agent/branch/parallel wired by edges, with {{input}}/{{last}}/{{node.id}} templates). The flow is tagged as created by you. Returns the new flow id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Flow name"},
				"graph":{"type":"string","description":"The orchestration graph as a JSON string"},
				"emoji":{"type":"string","description":"Optional cosmetic emoji shown wherever the flow is listed/picked"}
			},
			"required":["name"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// graph is a JSON STRING (escaped) describing Graph{start,nodes}.
			json.RawMessage(`{"name":"Draft then review","graph":"{\"start\":\"draft\",\"nodes\":[{\"id\":\"draft\",\"type\":\"agent\",\"agentId\":\"agt_writer\",\"prompt\":\"Write a short post about: {{input}}\",\"next\":\"review\"},{\"id\":\"review\",\"type\":\"agent\",\"agentId\":\"agt_reviewer\",\"prompt\":\"Critique this draft: {{node.draft}}\",\"next\":\"\"}]}"}`),
			// Minimal single-node flow.
			json.RawMessage(`{"name":"Quick classify","graph":"{\"start\":\"c\",\"nodes\":[{\"id\":\"c\",\"type\":\"agent\",\"agentId\":\"agt_triage\",\"prompt\":\"Classify: {{input}}\",\"next\":\"\"}]}"}`),
			// Parallel fan-out + join: a parallel node lists child agent node ids in
			// "parallel" (NOT "branches") and continues via "joinNext" (NOT "next").
			json.RawMessage(`{"name":"Research then synthesize","graph":"{\"start\":\"fan\",\"nodes\":[{\"id\":\"fan\",\"type\":\"parallel\",\"parallel\":[\"a\",\"b\"],\"joinNext\":\"merge\"},{\"id\":\"a\",\"type\":\"agent\",\"agentId\":\"agt_x\",\"prompt\":\"Angle A: {{input}}\"},{\"id\":\"b\",\"type\":\"agent\",\"agentId\":\"agt_y\",\"prompt\":\"Angle B: {{input}}\"},{\"id\":\"merge\",\"type\":\"agent\",\"agentId\":\"agt_z\",\"prompt\":\"Merge {{node.a}} and {{node.b}}\",\"next\":\"\"}]}"}`),
		},
	}
}

func (t CreateFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name  string `json:"name"`
		Graph string `json:"graph"`
		Emoji string `json:"emoji"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := validGraphJSON(in.Graph); err != nil {
		return "", err
	}
	created, err := t.d.db.CreateFlow(ctx, db.Flow{
		Name:      in.Name,
		Graph:     strings.TrimSpace(in.Graph),
		Emoji:     strings.TrimSpace(in.Emoji),
		CreatedBy: t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create flow: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": created.ID, "name": created.Name, "action": "created"})
	return string(b), nil
}

// UpdateFlowTool edits an agent-created flow.
type UpdateFlowTool struct{ d flowDeps }

// NewUpdateFlowTool constructs update_flow.
func NewUpdateFlowTool(database *db.DB, actorID string) UpdateFlowTool {
	return UpdateFlowTool{d: flowDeps{db: database, actorID: actorID}}
}

func (UpdateFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_flow",
		Description: "Edit a flow (user- or agent-created). Pass the flow id and the fields to change (name, graph, tags, emoji).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The flow id (see list_flows)"},
				"name":{"type":"string"},
				"graph":{"type":"string","description":"The orchestration graph as a JSON string"},
				"tags":{"type":"array","items":{"type":"string"},"description":"Replace the flow's organizational tags with this exact set"},
				"emoji":{"type":"string","description":"Replace the flow's cosmetic emoji (empty string clears it)"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// Partial update: pass id + only the fields to change. Rename only.
			json.RawMessage(`{"id":"flw_9a2","name":"Draft and review v2"}`),
			// Replace just the graph (escaped JSON string); other fields untouched.
			json.RawMessage(`{"id":"flw_9a2","graph":"{\"start\":\"c\",\"nodes\":[{\"id\":\"c\",\"type\":\"agent\",\"agentId\":\"agt_triage\",\"prompt\":\"Classify: {{input}}\",\"next\":\"\"}]}"}`),
		},
	}
}

func (t UpdateFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID    string    `json:"id"`
		Name  *string   `json:"name"`
		Graph *string   `json:"graph"`
		Tags  *[]string `json:"tags"`
		Emoji *string   `json:"emoji"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.requireFlow(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if in.Name != nil {
		cur.Name = strings.TrimSpace(*in.Name)
	}
	if in.Graph != nil {
		if err := validGraphJSON(*in.Graph); err != nil {
			return "", err
		}
		cur.Graph = strings.TrimSpace(*in.Graph)
	}
	if err := t.d.db.UpdateFlow(ctx, cur); err != nil {
		return "", fmt.Errorf("update flow: %w", err)
	}
	// Tags are persisted separately (UpdateFlow does not touch them), mirroring how
	// update_schedule applies enabled via its own store call.
	if in.Tags != nil {
		if err := t.d.db.SetFlowTags(ctx, in.ID, *in.Tags); err != nil {
			return "", fmt.Errorf("set flow tags: %w", err)
		}
	}
	// Emoji is persisted separately too (UpdateFlow does not touch it), so it
	// survives independent name/graph saves.
	if in.Emoji != nil {
		if err := t.d.db.SetFlowEmoji(ctx, in.ID, strings.TrimSpace(*in.Emoji)); err != nil {
			return "", fmt.Errorf("set flow emoji: %w", err)
		}
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// DeleteFlowTool removes an agent-created flow.
type DeleteFlowTool struct{ d flowDeps }

// NewDeleteFlowTool constructs delete_flow.
func NewDeleteFlowTool(database *db.DB, actorID string) DeleteFlowTool {
	return DeleteFlowTool{d: flowDeps{db: database, actorID: actorID}}
}

func (DeleteFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_flow",
		Description: "Delete a flow (user- or agent-created). This also removes the flow's runs. Pass the flow id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The flow id (see list_flows)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.requireFlow(ctx, in.ID); err != nil {
		return "", err
	}
	if err := t.d.db.DeleteFlow(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete flow: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}

// ListFlowsTool lists the workspace flows with provenance.
type ListFlowsTool struct{ d flowDeps }

// NewListFlowsTool constructs list_flows.
func NewListFlowsTool(database *db.DB, actorID string) ListFlowsTool {
	return ListFlowsTool{d: flowDeps{db: database, actorID: actorID}}
}

func (ListFlowsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_flows",
		Description: "List the orchestration flows in this workspace (id, name, emoji, tags, and whether each was " +
			"created by an agent — provenance only; you can edit/delete any of them). Results are PAGINATED: pass " +
			"limit (default 20, max 100) and offset to page; the reply reports total and hasMore, and you reach " +
			"the next page with offset += limit. Filters: tags (comma-separated; a flow must carry ALL of them). " +
			"Sort: updated_desc (default), updated_asc, created_desc, created_asc, name_asc, name_desc.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "tags": { "type": "string", "description": "Comma-separated tags; a flow must carry ALL of them." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc)." },
    "limit": { "type": "integer", "description": "Max flows per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching flows to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListFlowsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Tags   string `json:"tags"`
		Sort   string `json:"sort"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	flows, err := t.d.db.ListFlows(ctx)
	if err != nil {
		return "", err
	}

	wantTags := SplitTags(in.Tags)
	matches := make([]db.Flow, 0, len(flows))
	for _, f := range flows {
		if len(wantTags) > 0 && !HasAllTags(f.Tags, wantTags) {
			continue
		}
		matches = append(matches, f)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	less, err := SortByField(matches, field, asc,
		func(f db.Flow) int64 { return f.UpdatedAt },
		func(f db.Flow) int64 { return f.CreatedAt },
		func(f db.Flow) string { return f.Name },
		func(f db.Flow) string { return f.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		Emoji          string   `json:"emoji,omitempty"`
		Tags           []string `json:"tags,omitempty"`
		CreatedByAgent bool     `json:"createdByAgent"`
	}
	out := make([]row, 0, len(page))
	for _, f := range page {
		out = append(out, row{
			ID:             f.ID,
			Name:           f.Name,
			Emoji:          f.Emoji,
			Tags:           f.Tags,
			CreatedByAgent: f.CreatedBy != "",
		})
	}
	return pageResult(out, total, offset, limit)
}

// ---- get_flow ----

// GetFlowTool returns one flow in full, including its graph JSON, so an agent
// can read the current graph, modify it, and write it back via update_flow.
// list_flows intentionally omits the (potentially large) graph; get_flow is the
// way to fetch it. Read-only, allowed on any flow.
type GetFlowTool struct{ d flowDeps }

// NewGetFlowTool constructs get_flow.
func NewGetFlowTool(database *db.DB, actorID string) GetFlowTool {
	return GetFlowTool{d: flowDeps{db: database, actorID: actorID}}
}

func (GetFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "get_flow",
		Description: "Get one orchestration flow in full, including its graph JSON (the node graph). Use this to read a flow's current graph before editing it with update_flow. Returns id, name, graph, emoji and whether it was created by an agent. Allowed on any flow.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The flow id (see list_flows)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t GetFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	f, err := t.d.db.GetFlow(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no flow with id %q (use list_flows)", in.ID)
	}
	b, _ := json.Marshal(map[string]any{
		"id":             f.ID,
		"name":           f.Name,
		"emoji":          f.Emoji,
		"graph":          f.Graph,
		"createdByAgent": f.CreatedBy != "",
	})
	return string(b), nil
}

// ---- run_flow ----

// RunFlowTool executes a flow now with a given input (allowed on any flow).
// Running is non-destructive, so — unlike edit/delete — it is not provenance
// gated: an agent may run user-made flows too. The run is recorded in the
// executions feed like a task run.
type RunFlowTool struct{ d flowDeps }

// NewRunFlowTool constructs run_flow.
func NewRunFlowTool(database *db.DB, actorID string, run runFlowFn) RunFlowTool {
	return RunFlowTool{d: flowDeps{db: database, actorID: actorID, run: run}}
}

func (RunFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "run_flow",
		Description: "Run an orchestration flow now with the given input (the value bound to {{input}} in the graph). Runs the flow — which may SUSPEND at an await-input node (returned status 'waiting', resume it with deliver_flow_input) rather than finishing — records a run in the executions feed, and returns the current status and (truncated) output. Allowed on any flow.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The flow id (see list_flows)"},
				"input":{"type":"string","description":"Input bound to {{input}} in the flow graph"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t RunFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID    string `json:"id"`
		Input string `json:"input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if t.d.run == nil {
		return "", fmt.Errorf("run_flow is not wired in this context")
	}
	if _, err := t.d.db.GetFlow(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no flow with id %q (use list_flows)", in.ID)
	}
	run, err := t.d.run(ctx, in.ID, in.Input)
	if err != nil {
		return "", fmt.Errorf("run flow: %w", err)
	}
	b, _ := json.Marshal(map[string]string{
		"id":     in.ID,
		"runId":  run.ID,
		"status": run.Status,
		"output": truncateForTool(run.Output, 2000),
		"error":  truncateForTool(run.Error, 500),
	})
	return string(b), nil
}

// ---- list_flow_runs ----

// ListFlowRunsTool lists flow runs (optionally one flow / one status) so an agent
// can discover runs — in particular ones WAITING at an await-input node that it
// (or a peer) can feed via deliver_flow_input. Read-only.
type ListFlowRunsTool struct{ d flowDeps }

// NewListFlowRunsTool constructs list_flow_runs.
func NewListFlowRunsTool(database *db.DB, actorID string) ListFlowRunsTool {
	return ListFlowRunsTool{d: flowDeps{db: database, actorID: actorID}}
}

func (ListFlowRunsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_flow_runs",
		Description: "List orchestration flow runs (newest first). Optionally filter by flowId and/or status (running|waiting|success|failure). Use status='waiting' to find runs suspended at an await-input node, then feed one with deliver_flow_input. Returns id, flowId, status, waitingAt (the await node id when waiting), input and a truncated output.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"flowId":{"type":"string","description":"Only runs of this flow (optional)"},
				"status":{"type":"string","enum":["running","waiting","success","failure"],"description":"Only runs with this status (optional)"},
				"limit":{"type":"integer","description":"Max runs to return (default 20)"}
			},
			"additionalProperties":false
		}`),
	}
}

func (t ListFlowRunsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		FlowID string `json:"flowId"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	runs, err := t.d.db.ListFlowRuns(ctx, strings.TrimSpace(in.FlowID))
	if err != nil {
		return "", err
	}
	want := strings.TrimSpace(in.Status)
	type row struct {
		ID        string `json:"id"`
		FlowID    string `json:"flowId"`
		Status    string `json:"status"`
		WaitingAt string `json:"waitingAt,omitempty"`
		Input     string `json:"input,omitempty"`
		Output    string `json:"output,omitempty"`
	}
	out := make([]row, 0, in.Limit)
	for _, r := range runs {
		if want != "" && r.Status != want {
			continue
		}
		waitingAt := ""
		if r.Status == db.FlowWaiting {
			var st struct {
				WaitingAt string `json:"waitingAt"`
			}
			_ = json.Unmarshal([]byte(r.State), &st)
			waitingAt = st.WaitingAt
		}
		out = append(out, row{
			ID:        r.ID,
			FlowID:    r.FlowID,
			Status:    r.Status,
			WaitingAt: waitingAt,
			Input:     truncateForTool(r.Input, 200),
			Output:    truncateForTool(r.Output, 200),
		})
		if len(out) >= in.Limit {
			break
		}
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ---- deliver_flow_input ----

// DeliverFlowInputTool delivers input to a flow run suspended at an await-input
// node and resumes it — the peer/agent half of the flow↔session bridge, so a
// coordinator or any agent can feed a waiting flow (not just a human via the UI).
type DeliverFlowInputTool struct{ d flowDeps }

// NewDeliverFlowInputTool constructs deliver_flow_input.
func NewDeliverFlowInputTool(database *db.DB, actorID string, resume resumeFlowFn) DeliverFlowInputTool {
	return DeliverFlowInputTool{d: flowDeps{db: database, actorID: actorID, resume: resume}}
}

func (DeliverFlowInputTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "deliver_flow_input",
		Description: "Deliver input to a flow run that is WAITING at an await-input node, resuming it. The input becomes {{last}} for the node after the await. Find waiting runs with list_flow_runs (status='waiting'). Errors if the run is not currently waiting (already resumed/finished/unknown).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"runId":{"type":"string","description":"The waiting flow run id (see list_flow_runs status='waiting')"},
				"input":{"type":"string","description":"Input delivered to the await-input node (becomes {{last}})"}
			},
			"required":["runId"],
			"additionalProperties":false
		}`),
	}
}

func (t DeliverFlowInputTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		RunID string `json:"runId"`
		Input string `json:"input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.RunID = strings.TrimSpace(in.RunID)
	if in.RunID == "" {
		return "", fmt.Errorf("runId is required")
	}
	if t.d.resume == nil {
		return "", fmt.Errorf("deliver_flow_input is not wired in this context")
	}
	run, err := t.d.resume(ctx, in.RunID, in.Input)
	if err != nil {
		return "", fmt.Errorf("deliver input: %w", err)
	}
	b, _ := json.Marshal(map[string]string{
		"runId":  run.ID,
		"status": run.Status,
		"action": "resumed",
	})
	return string(b), nil
}
