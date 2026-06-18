package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Flow self-management tools let an agent create, edit, delete and list
// multi-agent orchestration flows in its workspace. Provenance is enforced: an
// agent may only edit/delete agent-created flows, never user-made ones.
//
// A flow's graph is the orchestration JSON (see internal/orchestration.Graph):
// a set of nodes (agent / branch / parallel) wired by edges, with template
// placeholders like {{input}} / {{last}} / {{node.id}}.

// runFlowFn runs a flow by id with the given input and returns the finished run.
type runFlowFn func(ctx context.Context, flowID, input string) (db.FlowRun, error)

type flowDeps struct {
	db      *db.DB
	actorID string
	run     runFlowFn
}

func (d flowDeps) requireFlowCreatedByAgent(ctx context.Context, id string) (db.Flow, error) {
	f, err := d.db.GetFlow(ctx, id)
	if err != nil {
		return db.Flow{}, fmt.Errorf("no flow with id %q (use list_flows)", id)
	}
	if f.CreatedBy == "" {
		return db.Flow{}, fmt.Errorf("flow %q was created by the user and cannot be edited or deleted by an agent", f.Name)
	}
	return f, nil
}

// validGraphJSON ensures the supplied graph string is valid JSON (the engine
// parses it into an orchestration.Graph). An empty string is allowed (defaults
// to an empty graph).
func validGraphJSON(graph string) error {
	graph = strings.TrimSpace(graph)
	if graph == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(graph), &v); err != nil {
		return fmt.Errorf("graph must be valid JSON: %w", err)
	}
	return nil
}

// CreateFlowTool creates a multi-agent orchestration flow.
type CreateFlowTool struct{ d flowDeps }

// NewCreateFlowTool constructs create_flow.
func NewCreateFlowTool(database *db.DB, actorID string) CreateFlowTool {
	return CreateFlowTool{d: flowDeps{db: database, actorID: actorID}}
}

func (CreateFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "create_flow",
		Description: "Create a multi-agent orchestration flow. The graph is the orchestration JSON (nodes of type agent/branch/parallel wired by edges, with {{input}}/{{last}}/{{node.id}} templates). The flow is tagged as created by you. Returns the new flow id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Flow name"},
				"description":{"type":"string","description":"What the flow does"},
				"graph":{"type":"string","description":"The orchestration graph as a JSON string"}
			},
			"required":["name"],
			"additionalProperties":false
		}`),
	}
}

func (t CreateFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Graph       string `json:"graph"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := validGraphJSON(in.Graph); err != nil {
		return "", err
	}
	created, err := t.d.db.CreateFlow(ctx, db.Flow{
		Name:        in.Name,
		Description: in.Description,
		Graph:       strings.TrimSpace(in.Graph),
		CreatedBy:   t.d.actorID,
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
		Name: "update_flow",
		Description: "Edit an agent-created flow (not one made by the user). Pass the flow id and the fields to change (name, description, graph).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The flow id (see list_flows)"},
				"name":{"type":"string"},
				"description":{"type":"string"},
				"graph":{"type":"string","description":"The orchestration graph as a JSON string"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID          string  `json:"id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Graph       *string `json:"graph"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.requireFlowCreatedByAgent(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if in.Name != nil {
		cur.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		cur.Description = *in.Description
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
		Name: "delete_flow",
		Description: "Delete an agent-created flow (not one made by the user). This also removes the flow's runs. Pass the flow id.",
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
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.requireFlowCreatedByAgent(ctx, in.ID); err != nil {
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
		Name:        "list_flows",
		Description: "List the orchestration flows in this workspace (id, name, description, and whether each was created by an agent and is therefore editable/deletable by you).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListFlowsTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	flows, err := t.d.db.ListFlows(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(flows))
	for _, f := range flows {
		out = append(out, row{
			ID:             f.ID,
			Name:           f.Name,
			Description:    f.Description,
			CreatedByAgent: f.CreatedBy != "",
		})
	}
	b, _ := json.Marshal(out)
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
		Name: "run_flow",
		Description: "Run an orchestration flow now with the given input (the value bound to {{input}} in the graph). Drives the flow to completion, records a run in the executions feed, and returns the status and (truncated) final output. Allowed on any flow.",
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
		return "", fmt.Errorf("invalid arguments: %w", err)
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
