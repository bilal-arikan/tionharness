package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// GetViewTool is the PULL channel of the projection layer (_Docs/66): the agent
// asks for a compact summary of a large entity when it needs one, instead of the
// runtime pushing that summary into every turn's prompt.
//
// Pull is the default on purpose. Pushing a live summary into the static prompt
// prefix would invalidate the prompt cache on every change (see _Docs/57), which
// costs far more than the summary saves. Only tiny, stable projections belong in
// a volatile suffix; everything else is fetched through this tool.
//
// The bytes returned here are byte-identical to what the Bağlam panel shows the
// user, so a wrong projection is something both of them can see.
type GetViewTool struct {
	db      *db.DB
	wsName  string
	sources ViewSources
}

// NewGetViewTool binds the tool to a workspace DB.
func NewGetViewTool(database *db.DB) GetViewTool { return GetViewTool{db: database} }

// WithSources attaches the workspace identity and the optional projection
// sources, so an agent's get_view renders the same nodes the Explorer map does
// (skills, insight findings, logs) instead of reporting them unavailable.
func (t GetViewTool) WithSources(wsName string, src ViewSources) GetViewTool {
	t.wsName = wsName
	t.sources = src
	return t
}

func (GetViewTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "get_view",
		// Description is deliberately terse: get_view is EAGER (its schema rides every
		// turn's cached prefix), so it carries only what the schema cannot express —
		// singleton ids, what sub means, and when to reach for this over the raw
		// listing tools. The per-kind catalogue that used to live here was pure
		// duplication of the kind enum: the projection is self-describing, so the
		// agent learns the detail by calling it, not by carrying a manual every turn.
		Description: "Get a COMPACT, deterministic summary of an entity instead of reading it whole: the " +
			"state worth acting on (failures, stuck turns, pending questions, what it is parked on), " +
			"never the raw transcript. Numbers are computed, never model-written, and whatever the view " +
			"hid it counts and hands back as a drill-down ref.\n\n" +
			"ids: 'workspace', 'board', 'budget', 'tools', 'logs' and category ids ('sessions', 'agents', …) " +
			"are singletons — pass the kind as the id. Everything else takes the entity's own id, as `expand` " +
			"returns it. sub= drills one level in (a card id on board, a node id on flowrun).\n" +
			"Prefer this over list_tasks / get_flow_run + parsing raw state: a fraction of the tokens.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "kind": { "type": "string", "enum": ["flowrun","session","board","workspace","schedule","agent","budget","tools","logs","artifact","automation","skill","insight","category"], "description": "Entity type to project." },
    "id": { "type": "string", "description": "Entity id (a flow run / session / schedule id, or 'board'/'workspace' for the singletons)." },
    "sub": { "type": "string", "description": "Optional drill-down target inside the entity (a node id for a flow run, a card id for the board)." },
    "level": { "type": "string", "enum": ["tiny","card","full"], "description": "Budget tier (default card)." }
  },
  "required": ["kind","id"],
  "additionalProperties": false
}`),
		// Four examples, not seven: they fold into the SHIPPED schema (foldExamples), so
		// each one is per-turn cost. These four cover every convention the schema alone
		// cannot express — singleton ids, sub drill-down, and level placement.
		Examples: []json.RawMessage{
			json.RawMessage(`{"kind":"workspace","id":"workspace"}`),
			json.RawMessage(`{"kind":"board","id":"board","sub":"T3"}`),
			json.RawMessage(`{"kind":"flowrun","id":"RUN7f2","sub":"fetch-b"}`),
			json.RawMessage(`{"kind":"session","id":"SES9a1","level":"full"}`),
		},
	}
}

func (t GetViewTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Sub   string `json:"sub"`
		Level string `json:"level"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if t.db == nil {
		return "", fmt.Errorf("get_view: no workspace store in this context")
	}
	kind := view.Kind(strings.TrimSpace(in.Kind))
	id := strings.TrimSpace(in.ID)
	// The board and the workspace are singletons with no id of their own, so an
	// omitted id for those is not a mistake to reject.
	switch {
	case id == "" && kind == view.KindBoard:
		id = view.BoardRefID
	case id == "" && kind == view.KindSpace:
		id = view.WorkspaceRefID
	}
	if id == "" {
		return "", fmt.Errorf("get_view: id is required")
	}

	ref := view.Ref{Kind: kind, ID: id, Sub: strings.TrimSpace(in.Sub)}
	v, err := ViewProjector(t.db, t.wsName, t.sources).
		Project(ctx, ref, view.ParseLevel(in.Level))
	if err != nil {
		// A bad ref is returned as an error rather than an empty summary: a blank
		// view reads like a healthy empty entity and would send the agent down the
		// wrong path.
		return "", err
	}

	out := v.Text()
	if len(v.Handles) > 0 {
		var b strings.Builder
		b.WriteString(out)
		b.WriteString("\n\nDrill-down:")
		for _, h := range v.Handles {
			lvl := h.Level
			if lvl == "" {
				lvl = view.LevelCard
			}
			b.WriteString(fmt.Sprintf("\n  %s → get_view{kind:%q,id:%q%s,level:%q}",
				h.Label, h.Ref.Kind, h.Ref.ID, subArg(h.Ref.Sub), lvl))
		}
		out = b.String()
	}
	return out, nil
}

// subArg renders the optional sub argument of a drill-down hint.
func subArg(sub string) string {
	if sub == "" {
		return ""
	}
	return fmt.Sprintf(",sub:%q", sub)
}
