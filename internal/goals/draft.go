package goals

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// DraftSchema is the structured-output schema handed to the provider when the
// goal-writer system agent drafts a goal. It is the editable subset of db.Goal
// (same JSON keys, so the reply unmarshals straight into a db.Goal); status,
// provenance, ids and history are the code's to decide. Numeric bounds are
// nullable so a real 0 stays distinguishable from "unset".
var DraftSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["name", "description", "scope", "primary", "guardrails", "policy"],
  "properties": {
    "name": {"type": "string"},
    "description": {"type": "string"},
    "scope": {
      "type": "object",
      "additionalProperties": false,
      "required": ["recipes", "agents", "automations", "tags"],
      "properties": {
        "recipes": {"type": "array", "items": {"type": "string"}},
        "agents": {"type": "array", "items": {"type": "string"}},
        "automations": {"type": "array", "items": {"type": "string"}},
        "tags": {"type": "array", "items": {"type": "string"}}
      }
    },
    "primary": {
      "type": "object",
      "additionalProperties": false,
      "required": ["metric", "direction", "target"],
      "properties": {
        "metric": {"type": "string"},
        "direction": {"type": "string", "enum": ["min", "max"]},
        "target": {"type": ["number", "null"]}
      }
    },
    "guardrails": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["metric", "min", "max"],
        "properties": {
          "metric": {"type": "string"},
          "min": {"type": ["number", "null"]},
          "max": {"type": ["number", "null"]}
        }
      }
    },
    "policy": {
      "type": "object",
      "additionalProperties": false,
      "required": ["mode", "cooldownHours", "minRuns"],
      "properties": {
        "mode": {"type": "string", "enum": ["propose", "off"]},
        "cooldownHours": {"type": "integer", "minimum": 0},
        "minRuns": {"type": "integer", "minimum": 0}
      }
    }
  }
}`)

// FromDraft turns a writer draft (the editable fields of a db.Goal, as the
// model answered them) into a db.Goal ready for Validate. rawText is the
// user's original statement; base, when non-nil, is the goal being re-written
// (its id, status and provenance are kept).
func FromDraft(d db.Goal, rawText string, base *db.Goal) db.Goal {
	g := db.Goal{
		Name:        d.Name,
		Description: d.Description,
		RawText:     strings.TrimSpace(rawText),
		Status:      db.GoalStatusDraft,
		Scope:       d.Scope,
		Primary:     d.Primary,
		// A draft always lands as propose-only; the user decides escalation or
		// measure-only in the editor.
		Policy:    db.GoalPolicy{Mode: db.GoalModePropose, CooldownHours: d.Policy.CooldownHours, MinRuns: d.Policy.MinRuns},
		CreatedBy: db.GoalByWriter,
	}
	// Guardrails the writer named but could not bound, or that point at a
	// metric outside the catalog, are dropped rather than failing the whole
	// draft: an unbounded guardrail is meaningless to enforce and an unknown
	// one would be refused by Validate anyway. The user adds them in the editor.
	g.Guardrails = make([]db.GoalGuardrail, 0, len(d.Guardrails))
	for _, gr := range d.Guardrails {
		key := strings.TrimSpace(gr.Metric)
		if key == "" {
			continue
		}
		if _, ok := Lookup(key); !ok {
			continue
		}
		if gr.Min == nil && gr.Max == nil {
			continue
		}
		g.Guardrails = append(g.Guardrails, db.GoalGuardrail{Metric: key, Min: gr.Min, Max: gr.Max})
	}
	if base != nil {
		g.ID = base.ID
		g.Status = base.Status
		g.CreatedBy = base.CreatedBy
		g.CreatedAt = base.CreatedAt
		g.History = base.History
		// A re-written active goal drops back to draft so the user re-confirms.
		if base.Status == db.GoalStatusActive {
			g.Status = db.GoalStatusDraft
		}
	}
	Normalize(&g)
	return g
}
