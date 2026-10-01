package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// ReadDeciderDebugTool gives agents the same metadata-only decision evidence
// shown in Settings. A default call is scoped to their current session.
type ReadDeciderDebugTool struct {
	hub         func() *decider.Hub
	workspaceID string
}

func NewReadDeciderDebugTool(hub func() *decider.Hub, workspace ...string) ReadDeciderDebugTool {
	id := ""
	if len(workspace) > 0 {
		id = workspace[0]
	}
	return ReadDeciderDebugTool{hub: hub, workspaceID: id}
}

func (ReadDeciderDebugTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "read_decider_debug",
		Description: "Read decision-authority diagnostics without making a model call. Defaults to the current session. Returns a retained-window summary, operational warnings and optionally the event timeline: primary/fallback/challenger attempts, HTTP retries, thresholds, probabilities, applied outcomes, costs and session/turn correlation. No conversation, commands, question text or credentials are stored. Agreement is not accuracy; confidence is not a correctness guarantee. Pass all_sessions=true for app-wide diagnostics or ref for an exact session, flow run or turn. Old ledger entries cannot be reconstructed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"boolean","description":"Summary only (default true). Set false for the bounded event timeline."},"all_sessions":{"type":"boolean","description":"Inspect the retained app-wide history (default false)."},"ref":{"type":"string","maxLength":128},"authority":{"type":"string","maxLength":128},"instance":{"type":"string","maxLength":128},"trace_id":{"type":"string","maxLength":128},"days":{"type":"integer","minimum":1,"maximum":90},"limit":{"type":"integer","minimum":1,"maximum":1000}},"additionalProperties":false}`),
		Examples:    []json.RawMessage{json.RawMessage(`{"summary":true}`), json.RawMessage(`{"summary":false,"authority":"stall-judge","limit":100}`)},
	}
}

func (t ReadDeciderDebugTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Summary   *bool  `json:"summary"`
		All       bool   `json:"all_sessions"`
		Ref       string `json:"ref"`
		Authority string `json:"authority"`
		Instance  string `json:"instance"`
		TraceID   string `json:"trace_id"`
		Days      int    `json:"days"`
		Limit     int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	if t.hub == nil || t.hub() == nil {
		return "Decision diagnostics are not available in this context.", nil
	}
	if in.Days == 0 {
		in.Days = 7
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Days < 1 || in.Days > 90 || in.Limit < 1 || in.Limit > 1000 {
		return "", fmt.Errorf("days must be 1..90 and limit 1..1000")
	}
	for _, v := range []string{in.Ref, in.Authority, in.Instance, in.TraceID} {
		if len(v) > 128 {
			return "", fmt.Errorf("debug filter is too long")
		}
	}
	if in.Ref == "" && !in.All {
		in.Ref = CurrentSessionID(ctx)
		if in.Ref == "" {
			return "No current session: pass ref or all_sessions=true.", nil
		}
	}
	workspaceID := t.workspaceID
	if in.All {
		workspaceID = ""
	}
	report := t.hub().Debug(decider.DebugFilter{Since: time.Now().Add(-time.Duration(in.Days) * 24 * time.Hour), Ref: in.Ref, Authority: in.Authority, Instance: in.Instance, TraceID: in.TraceID, Limit: in.Limit, WorkspaceID: workspaceID})
	if in.Summary == nil || *in.Summary {
		report.Events = []decider.DebugEvent{}
	}
	b, err := json.MarshalIndent(report, "", "  ")
	return string(b), err
}
