package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// adhocReservedPrefix is kept for node ids the runtime generates around the
// caller's steps (the start marker, a branch's routing node), so a step id can
// never collide with one.
const adhocReservedPrefix = "__"

// adhocFlowInput is the argument shape for the run_adhoc_flow tool.
type adhocFlowInput struct {
	Steps     []adhocStepInput `json:"steps"`
	MaxRounds int              `json:"max_rounds"`
}

// adhocStepInput is one element of "steps". The legs reuse fanOutTaskInput so a
// parallel step speaks exactly run_subagent's "tasks" vocabulary.
type adhocStepInput struct {
	ID        string             `json:"id"`
	Type      string             `json:"type"`
	Tasks     []fanOutTaskInput  `json:"tasks"`
	Next      string             `json:"next"`
	On        string             `json:"on"`
	JSONField string             `json:"json_field"`
	Branches  []adhocBranchInput `json:"branches"`
}

// adhocBranchInput is one arm of a branch step: match by "equals" or
// "contains"; an arm with neither is the default.
type adhocBranchInput struct {
	Equals   string `json:"equals"`
	Contains string `json:"contains"`
	Next     string `json:"next"`
}

// RunAdhocFlowTool runs a multi-round delegation plan — fan out, branch on the
// results, fan out again — inside ONE tool call, on the orchestration engine.
type RunAdhocFlowTool struct{}

// NewRunAdhocFlowTool constructs the run_adhoc_flow tool.
func NewRunAdhocFlowTool() RunAdhocFlowTool { return RunAdhocFlowTool{} }

func (RunAdhocFlowTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "run_adhoc_flow",
		// The FIRST LINE is what the lazy catalog shows; keep it a complete sentence
		// under 200 chars.
		Description: "Run a multi-round subagent plan in ONE call: fan out in parallel, branch on the results, fan out again.\n" +
			"Steps run from the first one. A \"parallel\" step runs its `tasks` (same fields as run_subagent's " +
			"`tasks`) concurrently and continues at `next` (\"\" = stop). A \"branch\" step matches the output of " +
			"the parallel step named in `on` against each arm's `equals`/`contains` (an arm with neither is the " +
			"default) and continues at that arm's `next`. An \"end\" step stops. A parallel step's output (what " +
			"`on` matches and what later tasks read as {{last}} or {{node.<step id>}}) is the answering legs' " +
			"replies; with exactly one leg it is that reply verbatim, so `equals` and `json_field` need a " +
			"single-leg step. A step whose legs ALL fail stops the run; a partial failure is reported and the run " +
			"goes on. Each parallel-step execution is one round, capped by `max_rounds` (default and maximum 3); " +
			"all rounds share one subagent budget. Returns JSON {run_id, status, steps:[{id,status,output}], final}; " +
			"status is done|failed|skipped|cancelled, and run_id works with get_view kind=flowrun.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "steps": {
      "type": "array",
      "minItems": 1,
      "description": "The plan. The first step is the entry.",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "description": "Unique step id (referenced by next/on and {{node.<id>}})." },
          "type": { "type": "string", "enum": ["parallel", "branch", "end"] },
          "tasks": {
            "type": "array",
            "description": "parallel only: the subagent legs, run concurrently.",
            "items": {
              "type": "object",
              "properties": {
                "target": { "type": "string", "description": "Profile id (\"explore\" | \"planner\" | \"coder\" | \"reviewer\") or an existing agent's name/id." },
                "task": { "type": "string", "description": "Self-contained instruction; may use {{last}} / {{node.<step id>}}." },
                "context": { "type": "string", "enum": ["isolated", "inherited"] },
                "model": { "type": "string" },
                "objective": { "type": "string" },
                "output_format": { "type": "string" },
                "boundaries": { "type": "string" }
              },
              "required": ["target", "task"],
              "additionalProperties": false
            }
          },
          "next": { "type": "string", "description": "parallel only: the step after this one (\"\" = stop)." },
          "on": { "type": "string", "description": "branch only: id of the parallel step whose output is matched." },
          "json_field": { "type": "string", "description": "branch only: match this top-level JSON field of that output instead of the raw text (single-leg step only)." },
          "branches": {
            "type": "array",
            "description": "branch only: routing arms, first match wins; an arm with neither equals nor contains is the default.",
            "items": {
              "type": "object",
              "properties": {
                "equals": { "type": "string", "description": "Case-insensitive, trimmed exact match (single-leg step only)." },
                "contains": { "type": "string", "description": "Case-insensitive substring match." },
                "next": { "type": "string", "description": "Step to continue at (\"\" = stop)." }
              },
              "additionalProperties": false
            }
          }
        },
        "required": ["id", "type"],
        "additionalProperties": false
      }
    },
    "max_rounds": { "type": "integer", "minimum": 1, "maximum": 3, "description": "Max parallel-step executions in this run (default 3)." }
  },
  "required": ["steps"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"steps":[{"id":"scan","type":"parallel","tasks":[{"target":"reviewer","task":"Review internal/db for data races.","output_format":"JSON {\"verdict\":\"PASS\"|\"FAIL\",\"findings\":[...]}"}],"next":"triage"},{"id":"triage","type":"branch","on":"scan","json_field":"verdict","branches":[{"equals":"FAIL","next":"fix"},{"next":""}]},{"id":"fix","type":"parallel","tasks":[{"target":"coder","task":"Fix these findings: {{node.scan}}"}],"next":""}],"max_rounds":2}`),
		},
	}
}

func (RunAdhocFlowTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	spec, err := parseAdhocFlowInput(input)
	if err != nil {
		return "", err
	}
	run := RunAdhocFlowFrom(ctx)
	if run == nil {
		return "", fmt.Errorf("ad-hoc flows are not available in this context")
	}
	res, err := run(ctx, spec)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode run_adhoc_flow result: %w", err)
	}
	return string(out), nil
}

// parseAdhocFlowInput decodes and validates a run_adhoc_flow call into a spec.
// Decoding is strict: an unknown field — a nested "tasks" inside a leg above all —
// is refused rather than silently dropped, so a leg can never look like it fans
// out further.
func parseAdhocFlowInput(input json.RawMessage) (AdhocFlowSpec, error) {
	var in adhocFlowInput
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return AdhocFlowSpec{}, argErrFor("run_adhoc_flow", err)
	}
	if len(in.Steps) == 0 {
		return AdhocFlowSpec{}, fmt.Errorf("\"steps\" needs at least one step")
	}
	if in.MaxRounds < 0 {
		return AdhocFlowSpec{}, fmt.Errorf("\"max_rounds\" must be positive; got %d", in.MaxRounds)
	}

	byID := make(map[string]adhocStepInput, len(in.Steps))
	for i, s := range in.Steps {
		id := strings.TrimSpace(s.ID)
		if id == "" {
			return AdhocFlowSpec{}, fmt.Errorf("steps[%d]: \"id\" is required", i)
		}
		if strings.HasPrefix(id, adhocReservedPrefix) {
			return AdhocFlowSpec{}, fmt.Errorf("steps[%d]: id %q must not start with %q (reserved)", i, id, adhocReservedPrefix)
		}
		if _, dup := byID[id]; dup {
			return AdhocFlowSpec{}, fmt.Errorf("steps[%d]: duplicate id %q", i, id)
		}
		s.ID = id
		byID[id] = s
	}

	spec := AdhocFlowSpec{MaxRounds: in.MaxRounds}
	for i, s := range in.Steps {
		step, err := parseAdhocStep(s, byID)
		if err != nil {
			return AdhocFlowSpec{}, fmt.Errorf("steps[%d] (%s): %w", i, strings.TrimSpace(s.ID), err)
		}
		spec.Steps = append(spec.Steps, step)
	}
	return spec, nil
}

// parseAdhocStep validates one step against its type. Fields that do not apply
// to the type are refused, never ignored: a "next" on a branch step, say, would
// look accepted while the arms decide the route.
func parseAdhocStep(s adhocStepInput, byID map[string]adhocStepInput) (AdhocFlowStep, error) {
	step := AdhocFlowStep{ID: strings.TrimSpace(s.ID), Type: strings.ToLower(strings.TrimSpace(s.Type))}
	ref := func(field, id string) (string, error) {
		id = strings.TrimSpace(id)
		if id == "" {
			return "", nil
		}
		if _, ok := byID[id]; !ok {
			return "", fmt.Errorf("%s references unknown step %q", field, id)
		}
		return id, nil
	}
	isBranchOnly := s.On != "" || s.JSONField != "" || len(s.Branches) > 0

	switch step.Type {
	case AdhocStepParallel:
		if isBranchOnly {
			return step, fmt.Errorf("\"on\", \"json_field\" and \"branches\" only apply to a branch step")
		}
		if len(s.Tasks) == 0 {
			return step, fmt.Errorf("a parallel step needs at least one task")
		}
		// The legs go through run_subagent's own fan-out validation, so the two
		// tools accept exactly the same leg shapes.
		legs, err := buildFanOutSpec(runSubagentInput{Tasks: s.Tasks}, RunAgentSpec{})
		if err != nil {
			return step, err
		}
		step.Tasks = legs.Tasks
		if step.Next, err = ref("next", s.Next); err != nil {
			return step, err
		}

	case AdhocStepBranch:
		if len(s.Tasks) > 0 || strings.TrimSpace(s.Next) != "" {
			return step, fmt.Errorf("\"tasks\" and \"next\" do not apply to a branch step; route with \"branches\"")
		}
		on, err := ref("on", s.On)
		if err != nil {
			return step, err
		}
		if on == "" {
			return step, fmt.Errorf("\"on\" is required: name the parallel step whose output this branch matches")
		}
		target := byID[on]
		if strings.ToLower(strings.TrimSpace(target.Type)) != AdhocStepParallel {
			return step, fmt.Errorf("\"on\" must name a parallel step; %q is a %s step", on, target.Type)
		}
		if len(s.Branches) == 0 {
			return step, fmt.Errorf("a branch step needs at least one arm in \"branches\"")
		}
		step.On = on
		step.JSONField = strings.TrimSpace(s.JSONField)
		if err := parseAdhocBranches(&step, s.Branches, ref); err != nil {
			return step, err
		}
		// A multi-leg step's output is several replies, which neither an exact
		// match nor a JSON field lookup can ever hit — such a branch would silently
		// always take its default arm.
		if (step.MatchMode == "equals" || step.JSONField != "") && len(target.Tasks) != 1 {
			return step, fmt.Errorf("\"equals\" and \"json_field\" need a single-leg \"on\" step (%q has %d legs); use \"contains\" to match any leg's reply", on, len(target.Tasks))
		}

	case AdhocStepEnd:
		if isBranchOnly || len(s.Tasks) > 0 || strings.TrimSpace(s.Next) != "" {
			return step, fmt.Errorf("an end step takes no other fields")
		}

	default:
		return step, enumErr("type", s.Type, AdhocStepParallel, AdhocStepBranch, AdhocStepEnd)
	}
	return step, nil
}

// parseAdhocBranches fills a branch step's arms and its uniform match mode. The
// engine applies one match mode per branch node, so mixing "equals" and
// "contains" in one step is refused rather than half-honoured.
func parseAdhocBranches(step *AdhocFlowStep, arms []adhocBranchInput, ref func(field, id string) (string, error)) error {
	defaults := 0
	for i, a := range arms {
		eq, co := strings.TrimSpace(a.Equals), strings.TrimSpace(a.Contains)
		if eq != "" && co != "" {
			return fmt.Errorf("branches[%d]: set \"equals\" or \"contains\", not both", i)
		}
		next, err := ref(fmt.Sprintf("branches[%d].next", i), a.Next)
		if err != nil {
			return err
		}
		mode, value := "", ""
		switch {
		case eq != "":
			mode, value = "equals", eq
		case co != "":
			mode, value = "contains", co
		default:
			defaults++
			if defaults > 1 {
				return fmt.Errorf("branches[%d]: only one default arm (no equals/contains) is allowed", i)
			}
		}
		if mode != "" {
			if step.MatchMode != "" && step.MatchMode != mode {
				return fmt.Errorf("branches[%d]: all arms of one step must use the same match (\"equals\" or \"contains\")", i)
			}
			step.MatchMode = mode
		}
		step.Branches = append(step.Branches, AdhocFlowBranch{Value: value, Next: next})
	}
	if step.MatchMode == "" {
		return fmt.Errorf("a branch step needs at least one arm with \"equals\" or \"contains\"")
	}
	return nil
}
