package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// ReadSessionDebugTool lets an agent read its OWN per-session debug journal —
// the parallel observability stream (turn timings, per-call token spend, tool
// latency/size, hook decisions, errors, compaction, recovery) that the runtime
// appends to debug.jsonl. It is the self-improvement loop: an agent can inspect
// where its tokens and time went, which tools are slow or failing, and adjust.
// Backed by db.GetDebugSummary / db.ReadDebugEvents (a pure file read, no LLM
// call). Defaults to the current session; pass session_id to inspect another.
type ReadSessionDebugTool struct{ db *db.DB }

// NewReadSessionDebugTool binds the tool to a workspace DB.
func NewReadSessionDebugTool(database *db.DB) ReadSessionDebugTool {
	return ReadSessionDebugTool{db: database}
}

func (ReadSessionDebugTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "read_session_debug",
		Description: "Read this session's structured DEBUG journal — a parallel observability stream " +
			"separate from the conversation. It captures, per session: turn timings + stop reason, " +
			"per-LLM-call token spend (input/output/cache) by model, per-tool latency + output size + " +
			"errors, hook decisions, compaction, recovery, and other lifecycle events (cache-break attribution, " +
			"sequence repair, guardrail decisions, distilled lessons, prompt-epoch changes). Use it to self-diagnose and optimise: " +
			"see where tokens and time go, which tools are slow or failing, how often context is compacted.\n\n" +
			"Default returns a SUMMARY (totals + per-tool + per-model rollups + the slowest tools + " +
			"heuristic ANOMALIES — slow/failing tools, error bursts, frequent compaction — plus per-turn " +
			"duration and per-call token time series). " +
			"Set summary=false to get the raw event list (newest last), optionally filtered by type and " +
			"capped by limit. Defaults to the current session; pass session_id to inspect another.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "session_id": { "type": "string", "description": "Session to inspect (default: the current session)." },
    "summary": { "type": "boolean", "description": "Return an aggregate summary instead of raw events (default true)." },
    "type": { "type": "string", "enum": ["turn","llm_call","tool","hook","error","compaction","recovery","cache_break","repair","guardrail","lesson","epoch"], "description": "When summary=false, filter raw events to a single type." },
    "limit": { "type": "integer", "description": "When summary=false, max newest events to return (default 100, max 1000)." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"summary":true}`),
			json.RawMessage(`{"summary":false,"type":"tool","limit":50}`),
		},
	}
}

func (t ReadSessionDebugTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		SessionID string `json:"session_id"`
		Summary   *bool  `json:"summary"`
		Type      string `json:"type"`
		Limit     int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	if t.db == nil {
		return "Debug journal is not available in this context.", nil
	}
	sid := in.SessionID
	if sid == "" {
		sid = CurrentSessionID(ctx)
	}
	if sid == "" {
		return "No session id: pass session_id (this turn has no current session).", nil
	}

	// Summary is the default — the aggregate view is what an agent reads first.
	summary := in.Summary == nil || *in.Summary
	if summary {
		sum, err := t.db.GetDebugSummary(ctx, sid)
		if err != nil {
			return "", err
		}
		if sum.Events == 0 {
			return fmt.Sprintf("(no debug events recorded for session %s yet)", sid), nil
		}
		b, err := json.MarshalIndent(sum, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	evs, err := t.db.ReadDebugEvents(ctx, sid, in.Type, limit)
	if err != nil {
		return "", err
	}
	if len(evs) == 0 {
		return fmt.Sprintf("(no matching debug events for session %s)", sid), nil
	}
	b, err := json.Marshal(evs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
