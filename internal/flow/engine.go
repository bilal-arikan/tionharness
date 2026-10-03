package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Runner is what the engine needs from its host: a model call for llm nodes,
// a judge for route nodes in judge mode, a checker for criteria mode and a
// trigger for automation nodes.
type Runner interface {
	// RunLLM runs one llm node with its rendered prompt and returns the reply
	// text. visit is 1-based.
	RunLLM(ctx context.Context, node Node, prompt string, visit int) (string, error)
	// Judge picks the index of the option that describes value, or -1 when
	// unsure. options are the labelled arms in edge order.
	Judge(ctx context.Context, node Node, value string, options []string) (int, error)
	// Check reports, per criterion, whether value satisfies it. An error (or a
	// short slice) sends the route down its default arm.
	Check(ctx context.Context, node Node, value string, criteria []string) ([]bool, error)
	// Trigger fires the node's automation with the rendered payload and
	// returns a short description of what happened (the node's recorded
	// output). The flow's last output passes through unchanged.
	Trigger(ctx context.Context, node Node, payload string) (string, error)
}

// Event is one live node-lifecycle frame (start / done / error).
type Event struct {
	Phase      string `json:"phase"`
	NodeID     string `json:"nodeId"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Index      int    `json:"index"` // 1-based step order within the run
	Visit      int    `json:"visit"`
	Output     string `json:"output,omitempty"`
	Edge       string `json:"edge,omitempty"`   // arm taken by a route node
	Detail     string `json:"detail,omitempty"` // criteria verdicts / trigger result
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

// Step is the persisted record of one executed node.
type Step struct {
	Index      int    `json:"index"`
	NodeID     string `json:"nodeId"`
	Type       string `json:"type"`
	Title      string `json:"title,omitempty"`
	Visit      int    `json:"visit"`
	Input      string `json:"input,omitempty"`  // rendered prompt / judged value
	Output     string `json:"output,omitempty"` // node output
	Edge       string `json:"edge,omitempty"`   // route: arm taken ("*" = default)
	Detail     string `json:"detail,omitempty"` // route criteria verdicts, trigger result
	StartedAt  int64  `json:"startedAt"`        // unix ms
	EndedAt    int64  `json:"endedAt"`          // unix ms
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

// Result is what one run produced, including the partial trace on failure.
type Result struct {
	Output  string
	Steps   []Step
	Outputs map[string]string
	Last    string
	Err     error
}

// ErrStepCap is returned when a run exceeds the graph's step cap.
var ErrStepCap = errors.New("flow exceeded its step cap")

// Run executes g on input. It never panics on a bad graph: callers are
// expected to Validate first, but a structural surprise surfaces as Result.Err.
// onEvent (may be nil) receives one frame before and after every node.
func Run(ctx context.Context, g Graph, input string, r Runner, onEvent func(Event)) Result {
	g = g.Normalized()
	res := Result{Outputs: map[string]string{}, Last: input}
	emit := func(ev Event) {
		if onEvent != nil {
			onEvent(ev)
		}
	}
	in, ok := g.Input()
	if !ok {
		res.Err = fmt.Errorf("graph has no input node")
		return res
	}
	visits := map[string]int{}
	cur := in
	maxSteps := g.EffectiveMaxSteps()
	for step := 1; ; step++ {
		if step > maxSteps {
			res.Err = fmt.Errorf("%w (%d)", ErrStepCap, maxSteps)
			return res
		}
		if err := ctx.Err(); err != nil {
			res.Err = err
			return res
		}
		visits[cur.ID]++
		visit := visits[cur.ID]
		vars := Vars{Input: input, Last: res.Last, Outputs: res.Outputs, Visit: visit, Step: step}
		started := time.Now()
		st := Step{Index: step, NodeID: cur.ID, Type: cur.Type, Title: cur.Title, Visit: visit, StartedAt: started.UnixMilli()}
		emit(Event{Phase: "start", NodeID: cur.ID, Type: cur.Type, Title: cur.Title, Index: step, Visit: visit})

		var out, edgeLabel, nextID, detail string
		var err error
		switch cur.Type {
		case NodeInput:
			out = input
			nextID = firstOut(g, cur.ID)
		case NodeLLM:
			prompt := strings.TrimSpace(cur.Prompt)
			if prompt == "" {
				prompt = "{{input}}"
			}
			st.Input = Render(prompt, vars)
			out, err = r.RunLLM(ctx, cur, st.Input, visit)
			nextID = firstOut(g, cur.ID)
		case NodeTransform:
			out = Render(cur.Template, vars)
			nextID = firstOut(g, cur.ID)
		case NodeOutput:
			tpl := strings.TrimSpace(cur.Template)
			if tpl == "" {
				tpl = "{{last}}"
			}
			out = Render(tpl, vars)
		case NodeRoute:
			st.Input = routeValue(cur, res.Last)
			var e Edge
			e, edgeLabel, detail, err = pickEdge(ctx, g, cur, st.Input, visit, r)
			out = res.Last
			nextID = e.To
		case NodeTrigger:
			// Pass-through: the automation gets the rendered payload, the flow
			// keeps its last output so a trigger can sit anywhere in the chain.
			tpl := strings.TrimSpace(cur.Template)
			if tpl == "" {
				tpl = "{{last}}"
			}
			st.Input = Render(tpl, vars)
			out = res.Last
			if r != nil {
				detail, err = r.Trigger(ctx, cur, st.Input)
			} else {
				err = errors.New("no runner to fire the automation")
			}
			nextID = firstOut(g, cur.ID)
		default:
			err = fmt.Errorf("node %q has unknown type %q", cur.ID, cur.Type)
		}
		ended := time.Now()
		st.EndedAt = ended.UnixMilli()
		st.DurationMs = ended.Sub(started).Milliseconds()
		st.Output = out
		st.Edge = edgeLabel
		st.Detail = detail
		if err != nil {
			st.Error = err.Error()
			res.Steps = append(res.Steps, st)
			emit(Event{Phase: "error", NodeID: cur.ID, Type: cur.Type, Title: cur.Title, Index: step, Visit: visit, Error: err.Error(), Detail: detail, DurationMs: st.DurationMs})
			res.Err = fmt.Errorf("node %q: %w", cur.ID, err)
			return res
		}
		res.Steps = append(res.Steps, st)
		if cur.Type == NodeTrigger {
			// {{node.<trigger>}} reads what the trigger did; {{last}} stays.
			res.Outputs[cur.ID] = detail
		} else {
			res.Outputs[cur.ID] = out
		}
		res.Last = out
		emit(Event{Phase: "done", NodeID: cur.ID, Type: cur.Type, Title: cur.Title, Index: step, Visit: visit, Output: out, Edge: edgeLabel, Detail: detail, DurationMs: st.DurationMs})
		if cur.Type == NodeOutput {
			res.Output = out
			return res
		}
		next, ok := g.NodeByID(nextID)
		if !ok {
			res.Err = fmt.Errorf("node %q has no successor", cur.ID)
			return res
		}
		cur = next
	}
}

func firstOut(g Graph, id string) string {
	for _, e := range g.Edges {
		if e.From == id {
			return e.To
		}
	}
	return ""
}

// routeValue is the text a route node matches on: the last output, or one of
// its top-level JSON fields in json mode.
func routeValue(n Node, last string) string {
	if n.Mode == ModeJSON {
		if v, ok := jsonTopField(last, n.JSONField); ok {
			return v
		}
	}
	return last
}

// jsonTopField reads a top-level field of a JSON object as text. A JSON object
// embedded in prose (code fences, a leading sentence) is accepted by taking the
// outermost {...} span.
func jsonTopField(raw, field string) (string, bool) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return "", false
	}
	v, ok := obj[field]
	if !ok {
		return "", false
	}
	var str string
	if err := json.Unmarshal(v, &str); err == nil {
		return str, true
	}
	return strings.TrimSpace(string(v)), true
}

// pickEdge chooses the arm a route node takes. Beyond its visit cap the node
// takes its default arm so a loop always exits; a labelled arm that matches wins
// before the default; no match and no default is an error. The third result is
// a human-readable detail (criteria verdicts) for the step record.
func pickEdge(ctx context.Context, g Graph, n Node, value string, visit int, r Runner) (Edge, string, string, error) {
	out := g.Outgoing(n.ID)
	var def *Edge
	var arms []Edge
	for i := range out {
		if out[i].When == "" {
			def = &out[i]
			continue
		}
		arms = append(arms, out[i])
	}
	if visit > MaxVisitsOf(n) {
		if def == nil {
			return Edge{}, "", "", fmt.Errorf("visit cap %d reached and the route has no default arm", MaxVisitsOf(n))
		}
		return *def, "* (visit cap)", "", nil
	}
	switch n.Mode {
	case ModeJudge:
		options := make([]string, len(arms))
		for i, a := range arms {
			options[i] = a.When
		}
		pick := -1
		if r != nil && len(options) > 0 {
			var err error
			pick, err = r.Judge(ctx, n, value, options)
			if err != nil {
				pick = -1
			}
		}
		if pick >= 0 && pick < len(arms) {
			return arms[pick], arms[pick].When, "", nil
		}
		if def == nil {
			return Edge{}, "", "", fmt.Errorf("judge could not pick an arm and the route has no default arm")
		}
		return *def, "*", "", nil
	case ModeCriteria:
		return pickCriteriaEdge(ctx, n, value, arms, def, r)
	}
	for _, a := range arms {
		if matchArm(n.Mode, value, a.When) {
			return a, a.When, "", nil
		}
	}
	if def == nil {
		return Edge{}, "", "", fmt.Errorf("no arm matched %q and the route has no default arm", truncate(value, 80))
	}
	return *def, "*", "", nil
}

// pickCriteriaEdge runs the checker over the node's criteria: every criterion
// holding takes the "pass" arm, anything else the "fail" arm; a missing arm or
// an unavailable checker falls back to the default arm. The detail lists the
// verdicts so the run trace and the observer can see WHICH criterion failed.
func pickCriteriaEdge(ctx context.Context, n Node, value string, arms []Edge, def *Edge, r Runner) (Edge, string, string, error) {
	armByLabel := func(label string) *Edge {
		for i := range arms {
			if strings.EqualFold(arms[i].When, label) {
				return &arms[i]
			}
		}
		return nil
	}
	fallback := func(detail string) (Edge, string, string, error) {
		if def == nil {
			return Edge{}, "", detail, fmt.Errorf("criteria could not be checked and the route has no default arm")
		}
		return *def, "*", detail, nil
	}
	if r == nil || len(n.Criteria) == 0 {
		return fallback("checker unavailable")
	}
	verdicts, err := r.Check(ctx, n, value, n.Criteria)
	if err != nil || len(verdicts) < len(n.Criteria) {
		if err == nil {
			err = errors.New("short verdict list")
		}
		return fallback("checker unavailable: " + err.Error())
	}
	passed := 0
	var failed []string
	for i, c := range n.Criteria {
		if verdicts[i] {
			passed++
		} else {
			failed = append(failed, c)
		}
	}
	detail := fmt.Sprintf("%d/%d criteria met", passed, len(n.Criteria))
	if len(failed) > 0 {
		detail += "; failed: " + strings.Join(failed, " | ")
	}
	label := ArmFail
	if passed == len(n.Criteria) {
		label = ArmPass
	}
	if arm := armByLabel(label); arm != nil {
		return *arm, arm.When, detail, nil
	}
	return fallback(detail)
}

func matchArm(mode, value, when string) bool {
	switch mode {
	case ModeEquals, ModeJSON:
		return strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(when))
	case ModeRegex:
		re, err := regexp.Compile(when)
		return err == nil && re.MatchString(value)
	default:
		return strings.Contains(strings.ToLower(value), strings.ToLower(when))
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
