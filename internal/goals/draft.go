package goals

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Draft is the JSON shape the goal-writer system agent answers with. It is
// deliberately flatter than db.Goal: the writer names things, the code decides
// status, provenance, ids and history.
type Draft struct {
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	Priority    int      `json:"priority"`
	Scope       Scope    `json:"scope"`
	Primary     Primary  `json:"primary"`
	Guardrails  []Guard  `json:"guardrails"`
	Rubric      string   `json:"rubric"`
	Policy      Policy   `json:"policy"`
	Questions   []string `json:"questions"`
	Notes       string   `json:"notes"`
}

// Scope, Primary, Guard and Policy mirror the db types. Numeric bounds are
// nullable in the schema so a real 0 stays distinguishable from "unset".
type Scope struct {
	Recipes     []string `json:"recipes"`
	Agents      []string `json:"agents"`
	Automations []string `json:"automations"`
	Tags        []string `json:"tags"`
}

type Primary struct {
	Metric    string   `json:"metric"`
	Direction string   `json:"direction"`
	Target    *float64 `json:"target"`
}

type Guard struct {
	Metric string   `json:"metric"`
	Min    *float64 `json:"min"`
	Max    *float64 `json:"max"`
}

type Policy struct {
	Mode              string   `json:"mode"`
	AutoApplySurfaces []string `json:"autoApplySurfaces"`
	CooldownHours     int      `json:"cooldownHours"`
	MinRuns           int      `json:"minRuns"`
}

// DraftSchema is the structured-output schema handed to the provider.
var DraftSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["name", "summary", "description", "kind", "priority", "scope", "primary", "guardrails", "rubric", "policy", "questions", "notes"],
  "properties": {
    "name": {"type": "string"},
    "summary": {"type": "string"},
    "description": {"type": "string"},
    "kind": {"type": "string", "enum": ["metric", "rubric", "mixed"]},
    "priority": {"type": "integer", "minimum": 1, "maximum": 5},
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
    "rubric": {"type": "string"},
    "policy": {
      "type": "object",
      "additionalProperties": false,
      "required": ["mode", "autoApplySurfaces", "cooldownHours", "minRuns"],
      "properties": {
        "mode": {"type": "string", "enum": ["propose", "auto", "off"]},
        "autoApplySurfaces": {"type": "array", "items": {"type": "string"}},
        "cooldownHours": {"type": "integer", "minimum": 0},
        "minRuns": {"type": "integer", "minimum": 0}
      }
    },
    "questions": {"type": "array", "items": {"type": "string"}},
    "notes": {"type": "string"}
  }
}`)

// FromDraft turns a writer draft into a db.Goal ready for Validate. rawText is
// the user's original statement; base, when non-nil, is the goal being
// re-written (its id, status and provenance are kept).
func FromDraft(d Draft, rawText string, base *db.Goal) db.Goal {
	g := db.Goal{
		Name:        d.Name,
		Summary:     d.Summary,
		Description: d.Description,
		RawText:     strings.TrimSpace(rawText),
		Status:      db.GoalStatusDraft,
		Kind:        d.Kind,
		Priority:    d.Priority,
		Scope: db.GoalScope{
			Recipes: d.Scope.Recipes, Agents: d.Scope.Agents, Automations: d.Scope.Automations, Tags: d.Scope.Tags,
		},
		Primary:   db.GoalMetric{Metric: d.Primary.Metric, Direction: d.Primary.Direction, Target: d.Primary.Target},
		Rubric:    d.Rubric,
		Policy:    db.GoalPolicy{Mode: d.Policy.Mode, AutoApplySurfaces: d.Policy.AutoApplySurfaces, CooldownHours: d.Policy.CooldownHours, MinRuns: d.Policy.MinRuns},
		Questions: d.Questions,
		Notes:     d.Notes,
		CreatedBy: db.GoalByWriter,
	}
	// Guardrails the writer named but could not bound, or that point at a
	// metric outside the catalog, are not a reason to refuse the whole draft:
	// they become open questions the user settles in the editor (an unbounded
	// guardrail would be meaningless to enforce; an unknown one is refused by
	// Validate anyway).
	g.Guardrails = make([]db.GoalGuardrail, 0, len(d.Guardrails))
	for _, gr := range d.Guardrails {
		key := strings.TrimSpace(gr.Metric)
		if key == "" {
			continue
		}
		if _, ok := Lookup(key); !ok {
			g.Questions = append(g.Questions, "Guardrail metriği katalogda yok: "+key+" — hangi katalog metriği kastedildi?")
			continue
		}
		if gr.Min == nil && gr.Max == nil {
			g.Questions = append(g.Questions, "Guardrail sınırı belirt: "+key+" en az / en çok ne olmalı?")
			continue
		}
		g.Guardrails = append(g.Guardrails, db.GoalGuardrail{Metric: key, Min: gr.Min, Max: gr.Max})
	}
	// The writer never decides policy escalation on its own: a draft always
	// lands as propose-only unless the user's own edit says otherwise.
	if g.Policy.Mode == db.GoalModeAuto {
		g.Policy.Mode = db.GoalModePropose
		g.Policy.AutoApplySurfaces = nil
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
