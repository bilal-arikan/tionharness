package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Flow self-management (_Docs/93): an agent may read its own main flow, edit
// it with the shared op vocabulary, roll back to an earlier version, read its
// recent runs and rewrite its own prompts. Every change goes through the
// runtime bridge, which validates the graph, enforces the growth budget and
// records a version with the agent as author.

// FlowInfo is the agent-facing view of a flow.
type FlowInfo struct {
	ID      string          `json:"id"`
	AgentID string          `json:"agentId"`
	Name    string          `json:"name"`
	Version int             `json:"version"`
	Shape   string          `json:"shape"`
	Note    string          `json:"note,omitempty"`
	Policy  string          `json:"policy"`
	Graph   json.RawMessage `json:"graph"`
	Stats   struct {
		Runs      int    `json:"runs"`
		Success   int    `json:"success"`
		Failure   int    `json:"failure"`
		AvgMs     int64  `json:"avgMs"`
		AvgTokens int64  `json:"avgTokens"`
		LastRunAt int64  `json:"lastRunAt,omitempty"`
		LastRunID string `json:"lastRunId,omitempty"`
	} `json:"stats"`
	Versions []FlowVersionInfo `json:"versions,omitempty"`
}

// FlowVersionInfo is one line of a flow's history.
type FlowVersionInfo struct {
	Version   int    `json:"version"`
	Author    string `json:"author"`
	Reason    string `json:"reason,omitempty"`
	Diff      string `json:"diff,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// FlowRunInfo is one run as the agent sees it.
type FlowRunInfo struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Version    int             `json:"version"`
	SessionID  string          `json:"sessionId,omitempty"`
	DurationMs int64           `json:"durationMs"`
	Tokens     int64           `json:"tokens"`
	Feedback   int             `json:"feedback,omitempty"`
	Input      string          `json:"input"`
	Output     string          `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	Steps      json.RawMessage `json:"steps,omitempty"`
	CreatedAt  int64           `json:"createdAt"`
}

// FlowBridge is what the runtime provides to the flow tools. agentID "" means
// the calling agent.
type FlowBridge interface {
	GetFlow(ctx context.Context, agentID string, withVersions bool) (FlowInfo, error)
	EditFlow(ctx context.Context, agentID string, ops json.RawMessage, reason string) (FlowInfo, error)
	RevertFlow(ctx context.Context, agentID string, version int, reason string) (FlowInfo, error)
	ListFlowRuns(ctx context.Context, agentID string, limit int, withSteps bool) ([]FlowRunInfo, error)
	UpdatePrompts(ctx context.Context, soul, identity *string, reason string) (string, error)
}

const flowOpsCheat = `Ops (applied in order): insert_between{from,to,node} · add_node{node} · remove_node{id} (linear node is bridged) · ` +
	`update_node{id,fields} (id/type immutable) · add_edge{edge:{from,to,when}} · update_edge{id,edge:{when|from|to}} · remove_edge{id} · set_max_steps{maxSteps}. ` +
	`Node types: llm{prompt,context:thread|fresh,tools:inherit|none,outputSchema?,model?,agentId?}, route{mode:contains|equals|regex|json|judge|criteria,jsonField?,question?,criteria?:[yes/no statements],maxVisits?}, transform{template}, trigger{automationId,template?} (fires an automation with the rendered payload and passes {{last}} through). ` +
	`Route modes: judge = a decision model picks the arm whose label fits the last output; criteria = the decision model checks every statement in criteria, all true → the arm labelled "pass", else the arm labelled "fail" (keep a default arm). ` +
	`Templates: {{input}} {{last}} {{node.<id>}} {{visit}}. Exactly one input and one output node; every loop needs a route with a default (unlabelled) arm. The output node renders {{last}} by default — after a critic/route stage point it at the answer ({"op":"update_node","id":"output","fields":{"template":"{{node.respond}}"}}) or the user sees the verdict.`

// ---- get_flow ----

type GetFlowTool struct{ b FlowBridge }

func NewGetFlowTool(b FlowBridge) GetFlowTool { return GetFlowTool{b: b} }

func (GetFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "get_flow",
		Description: "Read your main flow (the stage graph every one of your turns runs through): shape, graph JSON, policy, run stats and " +
			"optionally the version history. Pass agentId to read another agent's flow. Use it before edit_flow so your ops target real node/edge ids.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agentId":{"type":"string","description":"Another agent's id (default: you)"},
				"versions":{"type":"boolean","description":"Include the version history (default false)"}
			},
			"additionalProperties":false
		}`),
	}
}

func (t GetFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		AgentID  string `json:"agentId"`
		Versions bool   `json:"versions"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	info, err := t.b.GetFlow(ctx, strings.TrimSpace(in.AgentID), in.Versions)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(info)
	return string(b), nil
}

// ---- edit_flow ----

type EditFlowTool struct{ b FlowBridge }

func NewEditFlowTool(b FlowBridge) EditFlowTool { return EditFlowTool{b: b} }

func (EditFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "edit_flow",
		Description: "Evolve your main flow by applying ops to its graph (a new version is recorded with you as the author). " +
			"Typical use: insert a critic stage with a feedback loop, add a planning stage, sharpen a stage prompt, remove a stage that never helps. " +
			"The result is validated in code (one input, one output, every node reachable, bounded loops, growth budget); an invalid op set changes nothing. " + flowOpsCheat,
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ops":{"type":"array","items":{"type":"object"},"description":"Ordered edit ops (see the description)"},
				"reason":{"type":"string","description":"Why this change, with the evidence (recorded on the version)"},
				"agentId":{"type":"string","description":"Edit another agent's flow (default: yours)"}
			},
			"required":["ops","reason"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"reason":"Son 4 cevap doğrulanmadan gitti; eleştiri döngüsü ekle","ops":[{"op":"insert_between","from":"respond","to":"output","node":{"id":"critic","type":"llm","title":"Eleştiri","context":"fresh","tools":"none","prompt":"Aşağıdaki cevabı kullanıcı isteğine göre değerlendir. Sorun varsa 'REVISE: <neden>', yoksa 'APPROVE' yaz.\n\nİstek: {{input}}\n\nCevap: {{node.respond}}"}},{"op":"insert_between","from":"critic","to":"output","node":{"id":"check","type":"route","title":"Onay?","mode":"contains","maxVisits":2}},{"op":"update_edge","id":"e_check_output","edge":{"when":"APPROVE"}},{"op":"add_edge","edge":{"from":"check","to":"respond"}},{"op":"update_node","id":"respond","fields":{"prompt":"{{input}}\n\n{{node.critic}}"}},{"op":"update_node","id":"output","fields":{"template":"{{node.respond}}"}}]}`),
		},
	}
}

func (t EditFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Ops     json.RawMessage `json:"ops"`
		Reason  string          `json:"reason"`
		AgentID string          `json:"agentId"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(in.Reason) == "" {
		return "", fmt.Errorf("reason is required (say what the runs showed)")
	}
	info, err := t.b.EditFlow(ctx, strings.TrimSpace(in.AgentID), in.Ops, in.Reason)
	if err != nil {
		return "", fmt.Errorf("edit_flow: %w. %s", err, flowOpsCheat)
	}
	b, _ := json.Marshal(map[string]any{"version": info.Version, "shape": info.Shape, "flowId": info.ID})
	return string(b), nil
}

// ---- revert_flow ----

type RevertFlowTool struct{ b FlowBridge }

func NewRevertFlowTool(b FlowBridge) RevertFlowTool { return RevertFlowTool{b: b} }

func (RevertFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "revert_flow",
		Description: "Make an earlier version of your main flow the head again (recorded as a new version). Read the flow with versions:true first to see the history.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"version":{"type":"integer","description":"The version number to restore"},
				"reason":{"type":"string"},
				"agentId":{"type":"string","description":"Another agent's flow (default: yours)"}
			},
			"required":["version"],
			"additionalProperties":false
		}`),
	}
}

func (t RevertFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Version int    `json:"version"`
		Reason  string `json:"reason"`
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if in.Version <= 0 {
		return "", fmt.Errorf("version must be a positive version number")
	}
	info, err := t.b.RevertFlow(ctx, strings.TrimSpace(in.AgentID), in.Version, in.Reason)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{"version": info.Version, "shape": info.Shape})
	return string(b), nil
}

// ---- list_flow_runs ----

type ListFlowRunsTool struct{ b FlowBridge }

func NewListFlowRunsTool(b FlowBridge) ListFlowRunsTool { return ListFlowRunsTool{b: b} }

func (ListFlowRunsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_flow_runs",
		Description: "List your recent flow runs (one per turn): status, version, duration, tokens, the user's feedback and the input/output excerpts; " +
			"steps:true adds the per-node trace. Read these before deciding whether a stage helps.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"limit":{"type":"integer","description":"How many (default 10, max 50)"},
				"steps":{"type":"boolean","description":"Include each run's node trace"},
				"agentId":{"type":"string","description":"Another agent's runs (default: yours)"}
			},
			"additionalProperties":false
		}`),
	}
}

func (t ListFlowRunsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Limit   int    `json:"limit"`
		Steps   bool   `json:"steps"`
		AgentID string `json:"agentId"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	if in.Limit <= 0 {
		in.Limit = 10
	}
	if in.Limit > 50 {
		in.Limit = 50
	}
	runs, err := t.b.ListFlowRuns(ctx, strings.TrimSpace(in.AgentID), in.Limit, in.Steps)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{"runs": runs, "count": len(runs)})
	return string(b), nil
}

// ---- update_my_prompt ----

type UpdateMyPromptTool struct{ b FlowBridge }

func NewUpdateMyPromptTool(b FlowBridge) UpdateMyPromptTool { return UpdateMyPromptTool{b: b} }

func (UpdateMyPromptTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "update_my_prompt",
		Description: "Rewrite your own soul (system prompt) and/or identity. Pass the FULL new text, not a diff; the previous text is kept as a prompt version " +
			"so the user can restore it. Takes effect on your next turn. Use it only for a lasting lesson about your role, never for one-off instructions.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"soul":{"type":"string","description":"Full new soul text (omit to keep)"},
				"identity":{"type":"string","description":"Full new identity text (omit to keep)"},
				"reason":{"type":"string","description":"What the runs showed and what the change fixes"}
			},
			"required":["reason"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateMyPromptTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Soul     *string `json:"soul"`
		Identity *string `json:"identity"`
		Reason   string  `json:"reason"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if in.Soul == nil && in.Identity == nil {
		return "", fmt.Errorf("pass soul and/or identity")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return "", fmt.Errorf("reason is required")
	}
	msg, err := t.b.UpdatePrompts(ctx, in.Soul, in.Identity, in.Reason)
	if err != nil {
		return "", err
	}
	return msg, nil
}
