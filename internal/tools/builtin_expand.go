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

// ExpandTool is the structural drill-down companion to get_view (_Docs/68): it
// lists a node's CHILDREN — what expanding it in the Workspace Explorer map
// reveals — without rendering a full projection for each one. It walks the exact
// same graph the map UI does, so an agent explores the workspace the same way a
// human does: get_view{workspace} to read the root, expand it to see the six
// buckets, expand the relevant bucket, then get_view only the branch that matters.
//
// It is deliberately thin: children are cheap edges (label + ref), so an agent can
// fan out over the tree at a fraction of the tokens a full projection per node
// would cost, and drill deep with get_view only where it needs the detail.
type ExpandTool struct {
	db      *db.DB
	wsName  string
	sources ViewSources
}

// NewExpandTool binds the tool to a workspace DB.
func NewExpandTool(database *db.DB) ExpandTool { return ExpandTool{db: database} }

// WithSources attaches the workspace identity and the optional projection
// sources, so the children an agent walks are the ones the Explorer map shows.
func (t ExpandTool) WithSources(wsName string, src ViewSources) ExpandTool {
	t.wsName = wsName
	t.sources = src
	return t
}

func (ExpandTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "expand",
		Description: "List the CHILDREN of a node in the Workspace Explorer map — the structural " +
			"drill-down, WITHOUT rendering a full summary for each child. Pair it with get_view: " +
			"get_view reads ONE node, expand tells you what that node drills into.\n\n" +
			"Typical walk to answer \"what is going on?\":\n" +
			"  1. get_view{kind:'workspace',id:'workspace'} — read the root.\n" +
			"  2. expand{kind:'workspace',id:'workspace'} — the eleven buckets (Oturumlar, Akışlar, " +
			"Pano, Ajanlar, Artifacts, Otomasyonlar, Skill'ler, İçgörüler, Günlükler, Bütçe, Araçlar).\n" +
			"  3. expand{kind:'category',id:'sessions'} (or 'flows'/'agents'/'artifacts'/'automations'/" +
			"'skills'/'insights') — the members.\n" +
			"  4. get_view the one session / run / card / artifact / automation / skill / finding that matters.\n\n" +
			"Also: expand{kind:'board',id:'board'} → columns; expand{kind:'category',id:'col:in_progress'} " +
			"→ that column's cards; expand{kind:'agent',id:AG} → the agent's sessions; " +
			"expand{kind:'session',id:SES} → a coordinator's worker sessions.\n\n" +
			"lens narrows the children: 'errors' returns only the troubled ones (stuck sessions, failed " +
			"runs, failed cards, failed automations, fresh/regressed findings). Leaves (budget, tools, logs, " +
			"a single card, a flow run, a schedule, one artifact/automation/skill/finding) have no " +
			"children and return an empty list.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "kind": { "type": "string", "enum": ["workspace","category","board","agent","session"], "description": "Node type to expand." },
    "id": { "type": "string", "description": "Node id ('workspace'/'board' for the singletons; a category id like 'sessions'/'flows'/'agents'/'col:<column>'; an agent or session id)." },
    "sub": { "type": "string", "description": "Optional drill-down selector on the node (unused for most; a board with a sub is a single card and has no children)." },
    "lens": { "type": "string", "enum": ["health","stale","recent","errors"], "description": "Which children matter (default health; 'errors' = troubled only). For expand, only 'errors' filters — 'stale'/'recent' behave like 'health' (pass-through) here." }
  },
  "required": ["kind","id"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"kind":"workspace","id":"workspace"}`),
			json.RawMessage(`{"kind":"category","id":"sessions"}`),
			json.RawMessage(`{"kind":"category","id":"sessions","lens":"errors"}`),
			json.RawMessage(`{"kind":"board","id":"board"}`),
			json.RawMessage(`{"kind":"agent","id":"AG1"}`),
		},
	}
}

func (t ExpandTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Sub  string `json:"sub"`
		Lens string `json:"lens"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if t.db == nil {
		return "", fmt.Errorf("expand: no workspace store in this context")
	}
	kind := view.Kind(strings.TrimSpace(in.Kind))
	id := strings.TrimSpace(in.ID)
	// The board and the workspace are singletons with no id of their own.
	switch {
	case id == "" && kind == view.KindBoard:
		id = view.BoardRefID
	case id == "" && kind == view.KindSpace:
		id = view.WorkspaceRefID
	}
	if id == "" {
		return "", fmt.Errorf("expand: id is required")
	}

	ref := view.Ref{Kind: kind, ID: id, Sub: strings.TrimSpace(in.Sub)}
	lens := view.ParseLens(in.Lens)
	children, err := ViewProjector(t.db, t.wsName, t.sources).Children(ctx, ref, lens)
	if err != nil {
		// A bad ref is an error, never an empty list — a silent empty result reads
		// like a genuine leaf node and would hide the mistake.
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CHILDREN of %s (lens=%s) · %d", ref.String(), lens, len(children))
	if len(children) == 0 {
		b.WriteString(" — leaf (no children)")
		return b.String(), nil
	}

	rows := make([]string, 0, len(children))
	for _, h := range children {
		// Point each child at the right next call: expand for a node that itself has
		// children, get_view for a leaf. Both are always valid; the hint just steers.
		next := "get_view"
		if view.IsExpandable(h.Ref) {
			next = "expand"
		}
		rows = append(rows, fmt.Sprintf("  %s → %s{kind:%q,id:%q%s}", h.Label, next, h.Ref.Kind, h.Ref.ID, subArg(h.Ref.Sub)))
	}
	// A category on a mature workspace holds every session/run/artifact ever
	// created, so an uncapped listing defeats the whole point of the view layer.
	// The total is already in the header, and the drop is stated outright — a
	// silently short list would read like a complete one.
	kept, dropped := view.CapLines(rows, expandMaxBytes)
	b.WriteString("\n")
	for _, ln := range kept {
		b.WriteString("\n")
		b.WriteString(ln)
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "\n\n  … +%d more (of %d) elided — narrow with lens, or get_view the branch you need",
			dropped, len(children))
	}
	return b.String(), nil
}

// expandMaxBytes budgets the child listing. Roughly 1k tokens: enough for a full
// board or agent roster, small enough that expanding a busy category cannot blow
// a turn's context.
const expandMaxBytes = 4000
