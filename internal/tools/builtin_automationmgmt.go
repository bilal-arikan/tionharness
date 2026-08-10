package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// Automation self-management tools let an agent create, edit, delete and list
// tag-triggered automations — event-driven rules that spawn a new session
// whenever a session carrying a trigger tag finishes a turn, forming bounded
// self-continuing loops. No provenance gate: an agent may edit/delete any
// automation, user- or agent-created.

// defaultAutomationMax mirrors the API default so an agent that omits the cap
// still gets a runaway brake.
const defaultAutomationMax = 50

type automationDeps struct {
	db      *db.DB
	actorID string
}

// requireAutomation loads an automation by id, returning a friendly error if it
// does not exist. No provenance gate: user- and agent-created automations are both editable.
func (d automationDeps) requireAutomation(ctx context.Context, id string) (db.Automation, error) {
	a, err := d.db.GetAutomation(ctx, id)
	if err != nil {
		return db.Automation{}, fmt.Errorf("no automation with id %q (use list_automations)", id)
	}
	return a, nil
}

// CreateAutomationTool creates a tag-triggered automation.
type CreateAutomationTool struct{ d automationDeps }

// NewCreateAutomationTool constructs create_automation.
func NewCreateAutomationTool(database *db.DB, actorID string) CreateAutomationTool {
	return CreateAutomationTool{d: automationDeps{db: database, actorID: actorID}}
}

func (CreateAutomationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "create_automation",
		Description: "Create an event-driven automation. Three trigger kinds: (a) triggerKind='tag' (default) — when a session " +
			"carrying triggerTag finishes a turn, its final reply is rendered into promptTemplate ({{result}}, {{title}}, " +
			"{{tag}}, {{sessionId}}) and the target runs; (b) triggerKind='board' — when a kanban card changes (created/moved/" +
			"updated/deleted), the target runs with the card context ({{taskId}}, {{title}}, {{op}}, {{from}}, {{to}}, " +
			"{{toLabel}}, {{tags}}, {{owner}}, {{priority}}); (c) triggerKind='token' — when cumulative token spend crosses each " +
			"tokenThreshold multiple (tokenScope='session' watches one session's lifetime spend, 'workspace' the whole day's), " +
			"the target runs with {{tokens}}, {{threshold}}, {{scope}}, {{sessionId}} — good for self-maintenance/cleanup; " +
			"(d) triggerKind='counter' — when an activity counter crosses each counterInterval multiple " +
			"(counterMetric='message'|'tool'; counterScope='session' watches the crossing session, 'workspace' the whole " +
			"workspace's cumulative counter), the target runs with {{count}}, {{interval}}, {{metric}}, {{scope}}, {{sessionId}}. " +
			"Prefer 'counter' over 'token' for a stable work-cadence — token counts are cache-inflated and fire unpredictably. " +
			"The target is EITHER an agent (targetAgentId → a NEW session is spawned) OR an orchestration flow " +
			"(flowId → the rendered prompt is run as the flow input). For a tag automation the spawned session carries " +
			"triggerTag by default (a self-continuing loop bounded by maxIterations); board and token automations do not self-loop. " +
			"A board automation's boardAction='spawn' (default) makes the board drive EXECUTION (a card entering a column starts " +
			"an agent/flow); boardAction='archive' instead archives the card with NO LLM call (the cheap 'done → archive' cleanup, " +
			"no target needed).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Optional display name"},
				"triggerKind":{"type":"string","enum":["tag","board","token","counter"],"description":"What fires the automation: 'tag' (default; session tag), 'board' (kanban card change), 'token' (token-spend threshold crossing), or 'counter' (message/tool count crossing)"},
				"triggerTag":{"type":"string","description":"[tag kind] The session tag that fires this automation when a tagged session's turn ends"},
				"boardOp":{"type":"string","enum":["any","move","create","update","delete"],"description":"[board kind] Which card change fires it (default 'move')"},
				"boardFromState":{"type":"string","description":"[board kind] Only fire when a card LEAVES this column (empty = any source)"},
				"boardToState":{"type":"string","description":"[board kind] Only fire when a card ENTERS this column (empty = any target)"},
				"boardPriority":{"type":"integer","description":"[board kind] Fire order among automations matching the SAME card change; lower runs first (default 0). Use it to sequence two rules on one column instead of letting them race."},
				"boardExclusive":{"type":"boolean","description":"[board kind] Claim sole ownership of a matching card change: only this automation fires and every other match is suppressed (default false). Among several exclusive matches the lowest boardPriority wins."},
				"boardAction":{"type":"string","enum":["spawn","archive"],"description":"[board kind] What firing does: 'spawn' (default) runs the target agent/flow — the board drives execution; 'archive' archives the card with no LLM call (needs no target). Use 'archive' for a 'done → archive' cleanup rule."},
				"tokenScope":{"type":"string","enum":["session","workspace"],"description":"[token kind] What to watch: 'session' (default; one session's lifetime tokens) or 'workspace' (whole workspace's tokens today)"},
				"tokenThreshold":{"type":"integer","description":"[token kind] Token INTERVAL; fires each time cumulative spend crosses another multiple (e.g. 100000 → at 100k, 200k…). Min 1000. Tokens = input+output+cache."},
				"counterMetric":{"type":"string","enum":["message","tool"],"description":"[counter kind] Which counter to watch: 'message' (default; every user/assistant message) or 'tool' (executed tool calls)"},
				"counterScope":{"type":"string","enum":["session","workspace"],"description":"[counter kind] What to watch: 'session' (default; the crossing session's own counter) or 'workspace' (the whole workspace's cumulative counter — sum of every session; good for a multi-agent work-cadence trigger)"},
				"counterInterval":{"type":"integer","description":"[counter kind] Count INTERVAL; fires each time the watched counter crosses another multiple (e.g. 10 → at 10, 20…). Min 2."},
				"sessionMode":{"type":"string","enum":["spawn","continue"],"description":"[agent-backed] Session strategy per fire: 'spawn' (fresh session each time — tag/board default) or 'continue' (one persistent per-automation thread that carries prior turns forward, history-aware — token/counter default). Omit to use the per-kind default. Ignored for flow-backed rules."},
				"targetAgentId":{"type":"string","description":"The agent that runs the spawned session (see list_agents). Omit when flowId is set."},
				"flowId":{"type":"string","description":"Run this orchestration flow with the rendered prompt as its input instead of spawning an agent session (see list_flows)."},
				"promptTemplate":{"type":"string","description":"Prompt for the spawned session (or flow input). Tag placeholders: {{result}}, {{title}}, {{tag}}, {{sessionId}}, {{prevPrompt}}, {{agent}}. Board placeholders: {{taskId}}, {{title}}, {{op}}, {{from}}, {{to}}, {{fromLabel}}, {{toLabel}}, {{board}}, {{tags}}, {{owner}}, {{priority}}. Token placeholders: {{tokens}}, {{threshold}}, {{scope}}, {{sessionId}}. Counter placeholders: {{count}}, {{interval}}, {{metric}}, {{sessionId}}. Common: {{iteration}}, {{maxIterations}}, {{automation}}, {{date}}, {{time}}, {{datetime}}"},
				"spawnTags":{"type":"array","items":{"type":"string"},"description":"Tags applied to the spawned session (tag kind default: [triggerTag] → loop; pass [] to break the loop). Ignored for flow-backed and board automations."},
				"maxIterations":{"type":"integer","description":"Max total fires before auto-disabling. Range 1-500; 0/unlimited is REJECTED (infinite-loop risk). Omit for the default 50."},
				"cooldownSec":{"type":"integer","description":"Minimum seconds between fires (default 0)"},
				"expiresAt":{"type":"integer","description":"Optional end date (unix seconds); after it the automation auto-disables. 0 = no end date"},
				"enabled":{"type":"boolean","description":"Active immediately (default true)"}
			},
			"required":["promptTemplate"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"name":"Research loop","triggerTag":"research-loop","targetAgentId":"AGT3","promptTemplate":"Continue the research. Previous findings:\n{{result}}","maxIterations":20}`),
		},
	}
}

func (t CreateAutomationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name            string   `json:"name"`
		TriggerKind     string   `json:"triggerKind"`
		TriggerTag      string   `json:"triggerTag"`
		BoardOp         string   `json:"boardOp"`
		BoardFromState  string   `json:"boardFromState"`
		BoardToState    string   `json:"boardToState"`
		BoardPriority   *int     `json:"boardPriority"`
		BoardExclusive  *bool    `json:"boardExclusive"`
		BoardAction     string   `json:"boardAction"`
		TokenScope      string   `json:"tokenScope"`
		TokenThreshold  *int     `json:"tokenThreshold"`
		CounterMetric   string   `json:"counterMetric"`
		CounterScope    string   `json:"counterScope"`
		CounterInterval *int     `json:"counterInterval"`
		SessionMode     string   `json:"sessionMode"`
		TargetAgentID   string   `json:"targetAgentId"`
		FlowID          string   `json:"flowId"`
		PromptTemplate  string   `json:"promptTemplate"`
		SpawnTags       []string `json:"spawnTags"`
		MaxIterations   *int     `json:"maxIterations"`
		CooldownSec     *int     `json:"cooldownSec"`
		ExpiresAt       *int64   `json:"expiresAt"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.TriggerKind = strings.TrimSpace(in.TriggerKind)
	in.CounterMetric = strings.TrimSpace(in.CounterMetric)
	in.CounterScope = strings.TrimSpace(in.CounterScope)
	in.SessionMode = strings.TrimSpace(in.SessionMode)
	in.TriggerTag = strings.TrimSpace(in.TriggerTag)
	in.BoardOp = strings.TrimSpace(in.BoardOp)
	in.BoardFromState = strings.TrimSpace(in.BoardFromState)
	in.BoardToState = strings.TrimSpace(in.BoardToState)
	in.BoardAction = strings.TrimSpace(in.BoardAction)
	in.TokenScope = strings.TrimSpace(in.TokenScope)
	in.TargetAgentID = strings.TrimSpace(in.TargetAgentID)
	in.FlowID = strings.TrimSpace(in.FlowID)
	if strings.TrimSpace(in.PromptTemplate) == "" {
		return "", fmt.Errorf("promptTemplate is required")
	}
	// Pointer→value extraction for the interval fields + the one check the shape
	// validator cannot express ("required interval omitted"). Every other per-kind
	// rule is enforced once by db.ValidateAutomationShape below — the SAME validator
	// the REST path runs, so the two entry points cannot drift.
	tokenThreshold := 0
	if in.TriggerKind == db.TriggerToken {
		if in.TokenThreshold == nil {
			return "", fmt.Errorf("tokenThreshold is required for token automations")
		}
		tokenThreshold = *in.TokenThreshold
	}
	counterInterval := 0
	if in.TriggerKind == db.TriggerCounter {
		if in.CounterInterval == nil {
			return "", fmt.Errorf("counterInterval is required for counter automations")
		}
		counterInterval = *in.CounterInterval
	}
	// A board 'archive' automation needs no target (no LLM call). Otherwise either
	// a flow (flowId) or an agent (targetAgentId) is the target.
	archiveAction := in.TriggerKind == db.TriggerBoard && in.BoardAction == db.BoardActionArchive
	if archiveAction {
		// no target required
	} else if in.FlowID != "" {
		if _, err := t.d.db.GetFlow(ctx, in.FlowID); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", in.FlowID)
		}
	} else {
		if in.TargetAgentID == "" {
			return "", fmt.Errorf("targetAgentId or flowId is required")
		}
		if _, err := t.d.db.GetAgent(ctx, in.TargetAgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q (use list_agents)", in.TargetAgentID)
		}
	}
	maxIter := defaultAutomationMax
	if in.MaxIterations != nil {
		maxIter = *in.MaxIterations
		// Same validator the REST path uses. An agent creating an automation must
		// not be able to write a value a human would be refused — this tool is the
		// easier hole to slip an unbounded loop through, since nothing here is
		// reviewed by a person before it runs.
		if err := db.ValidateMaxIterations(maxIter); err != nil {
			return "", err
		}
	}
	cooldown := 0
	if in.CooldownSec != nil {
		cooldown = *in.CooldownSec
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	var expiresAt int64
	if in.ExpiresAt != nil {
		expiresAt = *in.ExpiresAt
	}
	boardPriority := 0
	if in.BoardPriority != nil {
		boardPriority = *in.BoardPriority
	}
	boardExclusive := false
	if in.BoardExclusive != nil {
		boardExclusive = *in.BoardExclusive
	}
	auto := db.Automation{
		Name:            strings.TrimSpace(in.Name),
		TriggerKind:     in.TriggerKind,
		TriggerTag:      in.TriggerTag,
		BoardOp:         in.BoardOp,
		BoardFromState:  in.BoardFromState,
		BoardToState:    in.BoardToState,
		BoardPriority:   boardPriority,
		BoardExclusive:  boardExclusive,
		BoardAction:     in.BoardAction,
		TokenScope:      in.TokenScope,
		TokenThreshold:  tokenThreshold,
		CounterMetric:   in.CounterMetric,
		CounterScope:    in.CounterScope,
		CounterInterval: counterInterval,
		SessionMode:     in.SessionMode,
		TargetAgentID:   in.TargetAgentID,
		FlowID:          in.FlowID,
		PromptTemplate:  in.PromptTemplate,
		SpawnTags:       in.SpawnTags,
		MaxIterations:   maxIter,
		CooldownSec:     cooldown,
		ExpiresAt:       expiresAt,
		Enabled:         enabled,
		CreatedBy:       t.d.actorID,
	}
	// Shared shape backstop (see db.ValidateAutomationShape) so create and update
	// enforce the same trigger/target contract.
	if err := db.ValidateAutomationShape(auto); err != nil {
		return "", err
	}
	created, err := t.d.db.CreateAutomation(ctx, auto)
	if err != nil {
		return "", fmt.Errorf("create automation: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"id": created.ID, "enabled": enabled, "action": "created"})
	return string(b), nil
}

// UpdateAutomationTool edits an automation (user- or agent-created).
type UpdateAutomationTool struct{ d automationDeps }

// NewUpdateAutomationTool constructs update_automation.
func NewUpdateAutomationTool(database *db.DB, actorID string) UpdateAutomationTool {
	return UpdateAutomationTool{d: automationDeps{db: database, actorID: actorID}}
}

func (UpdateAutomationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_automation",
		Description: "Edit an automation (user- or agent-created). Pass the id and the fields to change (name, triggerTag, targetAgentId, flowId, promptTemplate, spawnTags, maxIterations, cooldownSec, boardPriority, boardExclusive, enabled). Setting flowId makes it flow-backed (and clears the agent); setting targetAgentId switches it back to agent-backed.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The automation id (see list_automations)"},
				"name":{"type":"string"},
				"triggerKind":{"type":"string","enum":["tag","board","token","counter"]},
				"triggerTag":{"type":"string"},
				"boardOp":{"type":"string","enum":["any","move","create","update","delete"]},
				"boardFromState":{"type":"string","description":"[board kind] source-column filter (empty = any)"},
				"boardToState":{"type":"string","description":"[board kind] target-column filter (empty = any)"},
				"boardPriority":{"type":"integer","description":"[board kind] fire order among automations matching the same card change; lower runs first"},
				"boardExclusive":{"type":"boolean","description":"[board kind] only this automation fires for a matching change; all other matches are suppressed"},
				"boardAction":{"type":"string","enum":["spawn","archive"],"description":"[board kind] 'spawn' runs the target (board drives execution); 'archive' archives the card with no LLM call"},
				"tokenScope":{"type":"string","enum":["session","workspace"],"description":"[token kind] watch one session ('session') or the whole workspace/day ('workspace')"},
				"tokenThreshold":{"type":"integer","description":"[token kind] token interval; fires each time cumulative spend crosses another multiple (min 1000)"},
				"counterMetric":{"type":"string","enum":["message","tool"],"description":"[counter kind] watch 'message' count or 'tool' calls"},
				"counterScope":{"type":"string","enum":["session","workspace"],"description":"[counter kind] watch one session ('session') or the whole workspace's cumulative counter ('workspace')"},
				"counterInterval":{"type":"integer","description":"[counter kind] count interval; fires each time the watched counter crosses another multiple (min 2)"},
				"sessionMode":{"type":"string","enum":["spawn","continue"],"description":"[agent-backed] 'spawn' (fresh session per fire) or 'continue' (persistent per-automation thread, history-aware). Omit for the per-kind default; ignored for flow-backed rules."},
				"targetAgentId":{"type":"string"},
				"flowId":{"type":"string","description":"Run this flow with the rendered prompt as input instead of spawning an agent session (see list_flows). Setting it clears the agent."},
				"promptTemplate":{"type":"string"},
				"spawnTags":{"type":"array","items":{"type":"string"}},
				"maxIterations":{"type":"integer","description":"Max total fires before auto-disabling. Range 1-500; 0/unlimited is rejected."},
				"cooldownSec":{"type":"integer"},
				"enabled":{"type":"boolean"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateAutomationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID              string    `json:"id"`
		Name            *string   `json:"name"`
		TriggerKind     *string   `json:"triggerKind"`
		TriggerTag      *string   `json:"triggerTag"`
		BoardOp         *string   `json:"boardOp"`
		BoardFromState  *string   `json:"boardFromState"`
		BoardToState    *string   `json:"boardToState"`
		BoardPriority   *int      `json:"boardPriority"`
		BoardExclusive  *bool     `json:"boardExclusive"`
		BoardAction     *string   `json:"boardAction"`
		TokenScope      *string   `json:"tokenScope"`
		TokenThreshold  *int      `json:"tokenThreshold"`
		CounterMetric   *string   `json:"counterMetric"`
		CounterScope    *string   `json:"counterScope"`
		CounterInterval *int      `json:"counterInterval"`
		SessionMode     *string   `json:"sessionMode"`
		TargetAgentID   *string   `json:"targetAgentId"`
		FlowID          *string   `json:"flowId"`
		PromptTemplate  *string   `json:"promptTemplate"`
		SpawnTags       *[]string `json:"spawnTags"`
		MaxIterations   *int      `json:"maxIterations"`
		CooldownSec     *int      `json:"cooldownSec"`
		ExpiresAt       *int64    `json:"expiresAt"`
		Enabled         *bool     `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.requireAutomation(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if in.Name != nil {
		cur.Name = strings.TrimSpace(*in.Name)
	}
	if in.TriggerKind != nil {
		cur.TriggerKind = strings.TrimSpace(*in.TriggerKind)
	}
	if in.TriggerTag != nil {
		cur.TriggerTag = strings.TrimSpace(*in.TriggerTag)
	}
	// Field assignments only — format/range/coherence for the merged result is
	// enforced once by db.ValidateAutomationShape below (the same validator the REST
	// update + both create paths run), so the entry points cannot drift.
	if in.BoardOp != nil {
		cur.BoardOp = strings.TrimSpace(*in.BoardOp)
	}
	if in.BoardFromState != nil {
		cur.BoardFromState = strings.TrimSpace(*in.BoardFromState)
	}
	if in.BoardToState != nil {
		cur.BoardToState = strings.TrimSpace(*in.BoardToState)
	}
	if in.BoardPriority != nil {
		cur.BoardPriority = *in.BoardPriority
	}
	if in.BoardExclusive != nil {
		cur.BoardExclusive = *in.BoardExclusive
	}
	if in.BoardAction != nil {
		cur.BoardAction = strings.TrimSpace(*in.BoardAction)
	}
	if in.TokenScope != nil {
		cur.TokenScope = strings.TrimSpace(*in.TokenScope)
	}
	if in.TokenThreshold != nil {
		cur.TokenThreshold = *in.TokenThreshold
	}
	if in.CounterMetric != nil {
		cur.CounterMetric = strings.TrimSpace(*in.CounterMetric)
	}
	if in.CounterScope != nil {
		cur.CounterScope = strings.TrimSpace(*in.CounterScope)
	}
	if in.CounterInterval != nil {
		cur.CounterInterval = *in.CounterInterval
	}
	if in.SessionMode != nil {
		cur.SessionMode = strings.TrimSpace(*in.SessionMode)
	}
	// A non-empty flowId switches to flow-backed (and clears the agent); an
	// explicit targetAgentId switches back to agent-backed (and clears the flow).
	if in.FlowID != nil && strings.TrimSpace(*in.FlowID) != "" {
		fid := strings.TrimSpace(*in.FlowID)
		if _, err := t.d.db.GetFlow(ctx, fid); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", fid)
		}
		cur.FlowID = fid
		cur.TargetAgentID = ""
	} else if in.TargetAgentID != nil && strings.TrimSpace(*in.TargetAgentID) != "" {
		if _, err := t.d.db.GetAgent(ctx, *in.TargetAgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q", *in.TargetAgentID)
		}
		cur.TargetAgentID = *in.TargetAgentID
		cur.FlowID = ""
	}
	if in.PromptTemplate != nil {
		cur.PromptTemplate = *in.PromptTemplate
	}
	if in.SpawnTags != nil {
		cur.SpawnTags = *in.SpawnTags
	}
	if in.MaxIterations != nil {
		if err := db.ValidateMaxIterations(*in.MaxIterations); err != nil {
			return "", err
		}
		cur.MaxIterations = *in.MaxIterations
	}
	if in.CooldownSec != nil {
		cur.CooldownSec = *in.CooldownSec
	}
	if in.ExpiresAt != nil {
		cur.ExpiresAt = *in.ExpiresAt
	}
	// Final backstop on the merged result: update must not persist a shape create
	// would reject (e.g. a kind switch that leaves triggerTag or the target empty).
	// Shared with the REST update + both create paths via db.ValidateAutomationShape.
	if err := db.ValidateAutomationShape(cur); err != nil {
		return "", err
	}
	if err := t.d.db.UpdateAutomation(ctx, cur); err != nil {
		return "", fmt.Errorf("update automation: %w", err)
	}
	if in.Enabled != nil {
		if err := t.d.db.SetAutomationEnabled(ctx, in.ID, *in.Enabled); err != nil {
			return "", fmt.Errorf("set enabled: %w", err)
		}
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// DeleteAutomationTool removes an automation (user- or agent-created).
type DeleteAutomationTool struct{ d automationDeps }

// NewDeleteAutomationTool constructs delete_automation.
func NewDeleteAutomationTool(database *db.DB, actorID string) DeleteAutomationTool {
	return DeleteAutomationTool{d: automationDeps{db: database, actorID: actorID}}
}

func (DeleteAutomationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_automation",
		Description: "Delete an automation (user- or agent-created). Pass the automation id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The automation id (see list_automations)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteAutomationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
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
	if _, err := t.d.requireAutomation(ctx, in.ID); err != nil {
		return "", err
	}
	if err := t.d.db.DeleteAutomation(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete automation: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}

// ListAutomationsTool lists the workspace automations with provenance + counters.
type ListAutomationsTool struct{ d automationDeps }

// NewListAutomationsTool constructs list_automations.
func NewListAutomationsTool(database *db.DB, actorID string) ListAutomationsTool {
	return ListAutomationsTool{d: automationDeps{db: database, actorID: actorID}}
}

func (ListAutomationsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_automations",
		Description: "List the automations in this workspace (id, name, triggerKind, triggerTag, targetAgentId, " +
			"enabled, iterationCount/maxIterations, and whether each was created by an agent — provenance only; " +
			"you can edit/delete any of them). Results are PAGINATED: pass limit (default 20, max 100) and offset " +
			"to page; the reply reports total and hasMore, and you reach the next page with offset += limit. " +
			"Filters: enabled (true/false), triggerKind (tag|board|token|counter), targetAgentId (exact). Sort: " +
			"updated_desc (default), updated_asc, created_desc, created_asc, name_asc, name_desc.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "enabled": { "type": "boolean", "description": "Only enabled (true) or disabled (false) automations." },
    "triggerKind": { "type": "string", "enum": ["tag", "board", "token", "counter"], "description": "Only automations with this trigger kind." },
    "targetAgentId": { "type": "string", "description": "Only automations targeting this agent." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc)." },
    "limit": { "type": "integer", "description": "Max automations per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching automations to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListAutomationsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Enabled       *bool  `json:"enabled"`
		TriggerKind   string `json:"triggerKind"`
		TargetAgentID string `json:"targetAgentId"`
		Sort          string `json:"sort"`
		Limit         int    `json:"limit"`
		Offset        int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	autos, err := t.d.db.ListAutomations(ctx)
	if err != nil {
		return "", err
	}

	triggerKind := strings.TrimSpace(in.TriggerKind)
	switch triggerKind {
	case "", "tag", "board", "token", "counter":
	default:
		return "", fmt.Errorf("triggerKind must be one of tag, board, token, counter; got %q", triggerKind)
	}
	target := strings.TrimSpace(in.TargetAgentID)
	matches := make([]db.Automation, 0, len(autos))
	for _, a := range autos {
		if in.Enabled != nil && a.Enabled != *in.Enabled {
			continue
		}
		if triggerKind != "" {
			kind := a.TriggerKind
			if kind == "" {
				kind = db.TriggerTag
			}
			if kind != triggerKind {
				continue
			}
		}
		if target != "" && a.TargetAgentID != target {
			continue
		}
		matches = append(matches, a)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	less, err := SortByField(matches, field, asc,
		func(a db.Automation) int64 { return a.UpdatedAt },
		func(a db.Automation) int64 { return a.CreatedAt },
		func(a db.Automation) string { return a.Name },
		func(a db.Automation) string { return a.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		TriggerKind     string `json:"triggerKind"`
		TriggerTag      string `json:"triggerTag,omitempty"`
		BoardOp         string `json:"boardOp,omitempty"`
		BoardToState    string `json:"boardToState,omitempty"`
		BoardPriority   int    `json:"boardPriority,omitempty"`
		BoardExclusive  bool   `json:"boardExclusive,omitempty"`
		BoardAction     string `json:"boardAction,omitempty"`
		TokenScope      string `json:"tokenScope,omitempty"`
		TokenThreshold  int    `json:"tokenThreshold,omitempty"`
		CounterMetric   string `json:"counterMetric,omitempty"`
		CounterScope    string `json:"counterScope,omitempty"`
		CounterInterval int    `json:"counterInterval,omitempty"`
		TargetAgentID   string `json:"targetAgentId"`
		FlowID          string `json:"flowId,omitempty"`
		Enabled         bool   `json:"enabled"`
		IterationCount  int    `json:"iterationCount"`
		MaxIterations   int    `json:"maxIterations"`
		CreatedByAgent  bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(page))
	for _, a := range page {
		kind := a.TriggerKind
		if kind == "" {
			kind = db.TriggerTag
		}
		out = append(out, row{
			ID:              a.ID,
			Name:            a.Name,
			TriggerKind:     kind,
			TriggerTag:      a.TriggerTag,
			BoardOp:         a.BoardOp,
			BoardToState:    a.BoardToState,
			BoardPriority:   a.BoardPriority,
			BoardExclusive:  a.BoardExclusive,
			BoardAction:     a.BoardAction,
			TokenScope:      a.TokenScope,
			TokenThreshold:  a.TokenThreshold,
			CounterMetric:   a.CounterMetric,
			CounterScope:    a.CounterScope,
			CounterInterval: a.CounterInterval,
			TargetAgentID:   a.TargetAgentID,
			FlowID:          a.FlowID,
			Enabled:         a.Enabled,
			IterationCount:  a.IterationCount,
			MaxIterations:   a.MaxIterations,
			CreatedByAgent:  a.CreatedBy != "",
		})
	}
	return pageResult(out, total, offset, limit)
}
