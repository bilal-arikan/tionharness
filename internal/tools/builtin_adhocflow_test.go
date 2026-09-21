package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestParseAdhocFlowCanonicalPlan: the documented scan → triage → fix shape
// parses into the spec the runtime compiles.
func TestParseAdhocFlowCanonicalPlan(t *testing.T) {
	spec, err := parseAdhocFlowInput(json.RawMessage(`{
  "steps": [
    {"id": "scan", "type": "parallel", "tasks": [{"target": "reviewer", "task": "review"}], "next": "triage"},
    {"id": "triage", "type": "branch", "on": "scan", "json_field": "verdict",
     "branches": [{"equals": "FAIL", "next": "fix"}, {"next": ""}]},
    {"id": "fix", "type": "parallel", "tasks": [{"target": "coder", "task": "fix {{node.scan}}"}], "next": ""}
  ],
  "max_rounds": 3
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec.MaxRounds != 3 || len(spec.Steps) != 3 {
		t.Fatalf("unexpected spec %+v", spec)
	}
	triage := spec.Steps[1]
	if triage.On != "scan" || triage.JSONField != "verdict" || triage.MatchMode != "equals" || len(triage.Branches) != 2 {
		t.Fatalf("unexpected branch step %+v", triage)
	}
	if triage.Branches[0] != (AdhocFlowBranch{Value: "FAIL", Next: "fix"}) || triage.Branches[1] != (AdhocFlowBranch{}) {
		t.Fatalf("unexpected arms %+v", triage.Branches)
	}
	if fix := spec.Steps[2]; fix.Tasks[0].Target != "coder" || fix.Next != "" {
		t.Fatalf("unexpected fix step %+v", fix)
	}
}

// TestParseAdhocFlowRefusals: every malformed plan is refused with a reason, never
// half-accepted.
func TestParseAdhocFlowRefusals(t *testing.T) {
	cases := map[string]struct{ input, want string }{
		"nested fan-out in a leg": {
			`{"steps":[{"id":"a","type":"parallel","tasks":[{"target":"explore","task":"x","tasks":[{"task":"y"}]}]}]}`,
			`unknown field "tasks"`,
		},
		"no steps":           {`{"steps":[]}`, "at least one step"},
		"duplicate id":       {`{"steps":[{"id":"a","type":"end"},{"id":"a","type":"end"}]}`, "duplicate id"},
		"reserved id":        {`{"steps":[{"id":"__start","type":"end"}]}`, "reserved"},
		"unknown type":       {`{"steps":[{"id":"a","type":"loop"}]}`, "invalid type"},
		"unknown next":       {`{"steps":[{"id":"a","type":"parallel","tasks":[{"target":"explore","task":"x"}],"next":"zz"}]}`, "unknown step"},
		"parallel no tasks":  {`{"steps":[{"id":"a","type":"parallel"}]}`, "at least one task"},
		"leg without target": {`{"steps":[{"id":"a","type":"parallel","tasks":[{"task":"x"}]}]}`, "no \"target\""},
		"branch on non-parallel": {
			`{"steps":[{"id":"e","type":"end"},{"id":"b","type":"branch","on":"e","branches":[{"contains":"x","next":""}]}]}`,
			"must name a parallel step",
		},
		"branch without on": {
			`{"steps":[{"id":"b","type":"branch","branches":[{"contains":"x","next":""}]}]}`,
			"\"on\" is required",
		},
		"equals on multi-leg step": {
			`{"steps":[{"id":"a","type":"parallel","tasks":[{"target":"explore","task":"x"},{"target":"explore","task":"y"}],"next":"b"},
			  {"id":"b","type":"branch","on":"a","branches":[{"equals":"OK","next":""}]}]}`,
			"single-leg",
		},
		"mixed match modes": {
			`{"steps":[{"id":"a","type":"parallel","tasks":[{"target":"explore","task":"x"}],"next":"b"},
			  {"id":"b","type":"branch","on":"a","branches":[{"equals":"OK","next":""},{"contains":"NO","next":""}]}]}`,
			"same match",
		},
		"two defaults": {
			`{"steps":[{"id":"a","type":"parallel","tasks":[{"target":"explore","task":"x"}],"next":"b"},
			  {"id":"b","type":"branch","on":"a","branches":[{"contains":"x","next":""},{"next":""},{"next":""}]}]}`,
			"only one default",
		},
		"next on a branch": {
			`{"steps":[{"id":"a","type":"parallel","tasks":[{"target":"explore","task":"x"}],"next":"b"},
			  {"id":"b","type":"branch","on":"a","next":"a","branches":[{"contains":"x","next":""}]}]}`,
			"do not apply to a branch",
		},
		"negative rounds": {`{"steps":[{"id":"a","type":"end"}],"max_rounds":-1}`, "must be positive"},
	}
	for name, c := range cases {
		_, err := parseAdhocFlowInput(json.RawMessage(c.input))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}

// TestRunAdhocFlowToolNeedsRunner: without the runtime's runner on the context
// the tool refuses instead of pretending to run.
func TestRunAdhocFlowToolNeedsRunner(t *testing.T) {
	_, err := NewRunAdhocFlowTool().Call(context.Background(), json.RawMessage(`{"steps":[{"id":"a","type":"end"}]}`))
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("want a not-available error, got %v", err)
	}
}
