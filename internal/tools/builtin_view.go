package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/view"
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
type GetViewTool struct{ db *db.DB }

// NewGetViewTool binds the tool to a workspace DB.
func NewGetViewTool(database *db.DB) GetViewTool { return GetViewTool{db: database} }

func (GetViewTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "get_view",
		Description: "Get a COMPACT, deterministic summary of a large entity instead of reading it whole. " +
			"A 40-node flow run collapses to a couple of lines that keep the shape of the graph: which " +
			"nodes ran, how long each took, where the run is parked now, and what is wrong with it.\n\n" +
			"Numbers in a view are computed, never written by a model, so they can be trusted. The view " +
			"always reports how many items it hid and offers drill-down handles for them — nothing is " +
			"silently dropped.\n\n" +
			"kind: 'flowrun' (a flow execution). id: the entity id. sub: optional drill-down (a node id).\n" +
			"level: 'tiny' (one line) | 'card' (default) | 'full' (per-node detail).\n" +
			"lens: 'health' (default) | 'stale' | 'recent' | 'errors' (failures only).\n\n" +
			"Prefer this over get_flow_run + parsing state JSON: it is a fraction of the tokens and it " +
			"surfaces the warning signals (stuck node, retry, suspended await-input) directly.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "kind": { "type": "string", "enum": ["flowrun"], "description": "Entity type to project." },
    "id": { "type": "string", "description": "Entity id (e.g. a flow run id)." },
    "sub": { "type": "string", "description": "Optional drill-down target inside the entity (a node id for a flow run)." },
    "level": { "type": "string", "enum": ["tiny","card","full"], "description": "Budget tier (default card)." },
    "lens": { "type": "string", "enum": ["health","stale","recent","errors"], "description": "Which facts matter (default health)." }
  },
  "required": ["kind","id"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"kind":"flowrun","id":"RUN7f2"}`),
			json.RawMessage(`{"kind":"flowrun","id":"RUN7f2","level":"full","lens":"errors"}`),
			json.RawMessage(`{"kind":"flowrun","id":"RUN7f2","sub":"fetch-b"}`),
		},
	}
}

func (t GetViewTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Sub   string `json:"sub"`
		Level string `json:"level"`
		Lens  string `json:"lens"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if t.db == nil {
		return "", fmt.Errorf("get_view: no workspace store in this context")
	}
	if strings.TrimSpace(in.ID) == "" {
		return "", fmt.Errorf("get_view: id is required")
	}

	ref := view.Ref{
		Kind: view.Kind(strings.TrimSpace(in.Kind)),
		ID:   strings.TrimSpace(in.ID),
		Sub:  strings.TrimSpace(in.Sub),
	}
	v, err := view.NewProjector(t.db).Project(ctx, ref, view.ParseLevel(in.Level), view.ParseLens(in.Lens))
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
