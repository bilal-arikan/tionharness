package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RecapStep is the minimal shape parsed out of a message's serialized Steps
// trace: what was called, with which argument, and how it ended.
//
// It mirrors the relevant fields of agent.TurnStep WITHOUT importing it — and
// cannot import it: package agent depends on this package (agent.TurnStep
// embeds tools.AskQuestion and tools.ShellOptimization), so the dependency only
// runs one way. internal/agent/steprecap_parity_test.go guards the two shapes
// against drift, since a renamed JSON tag on TurnStep would otherwise silently
// decode into a zero value here.
type RecapStep struct {
	Kind string `json:"kind"`
	Tool string `json:"tool"`
	// CallName is the exact (namespaced) name the provider used on the claude-cli
	// path; empty for native/bare tools. Preferred over Tool in a recap line so a
	// model re-reading the trace calls mcp__tionharness_extended__list_tasks and
	// not the bare list_tasks the CLI rejects.
	CallName string          `json:"callName"`
	Input    json.RawMessage `json:"input"`
	Output   string          `json:"output"`
	IsError  bool            `json:"isError"`
	// Reason is the stable machine tag carried by an error/recovery step
	// ("provider_error", "budget_exceeded", "permission_denied", ...).
	Reason string `json:"reason"`
	// Running marks a step that had not finished when the trace was captured. It
	// is only ever set in an in-flight (mid-turn) snapshot — a persisted step is
	// always terminal.
	Running bool `json:"running"`
}

// RecapOpts bounds a rendered recap.
type RecapOpts struct {
	// MaxTools caps how many tool lines are rendered before the rest are elided
	// into a "+N more" line. 0 means no cap.
	MaxTools int
	// MaxOutput is how many runes of each tool's output are kept. 0 omits tool
	// output entirely, leaving only the call and its ok/error/running state —
	// which is what a CROSS-SESSION inspection wants: it answers "what did that
	// agent do", not "what did it see", so another session's tool results never
	// land in this context.
	MaxOutput int
}

// recapMaxArgHint bounds the argument excerpt in a recap line.
const recapMaxArgHint = 80

// ParseRecapSteps decodes a serialized Steps trace. An empty trace is not an
// error (a plain reply runs no tools), but a non-empty malformed one is: a
// corrupt trace must be reportable instead of rendering as a turn that simply
// did nothing.
func ParseRecapSteps(stepsJSON string) ([]RecapStep, error) {
	trimmed := strings.TrimSpace(stepsJSON)
	if trimmed == "" {
		return nil, nil
	}
	var steps []RecapStep
	if err := json.Unmarshal([]byte(trimmed), &steps); err != nil {
		return nil, fmt.Errorf("persisted_steps_invalid: %w", err)
	}
	return steps, nil
}

// RecapLines renders a trace's tool invocations as compact single lines
// ("- Tool(argHint) → result"), or nil when the turn ran no tools.
func RecapLines(steps []RecapStep, opts RecapOpts) []string {
	var lines []string
	elided := 0
	for _, st := range steps {
		if !st.isToolCall() {
			continue
		}
		if opts.MaxTools > 0 && len(lines) >= opts.MaxTools {
			elided++
			continue
		}
		lines = append(lines, st.recapLine(opts.MaxOutput))
	}
	if len(lines) == 0 {
		return nil
	}
	if elided > 0 {
		lines = append(lines, fmt.Sprintf("- … (+%d more tool call(s))", elided))
	}
	return lines
}

// RecapToolCounts summarises which tools a turn used and how often, in
// first-call order: "Read×3, Bash×2, Edit". It uses the BARE tool name (not the
// namespaced CallName) because this line answers "what kind of work was this",
// where the namespace is noise; RecapLines keeps the exact callable name.
func RecapToolCounts(steps []RecapStep) string {
	var order []string
	count := map[string]int{}
	for _, st := range steps {
		if !st.isToolCall() {
			continue
		}
		if _, seen := count[st.Tool]; !seen {
			order = append(order, st.Tool)
		}
		count[st.Tool]++
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		if n := count[name]; n > 1 {
			parts = append(parts, fmt.Sprintf("%s×%d", name, n))
		} else {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
}

// RecapLastStep labels the final step of a trace — what the turn was doing when
// the trace was captured. Empty for an empty trace.
func RecapLastStep(steps []RecapStep) string {
	if len(steps) == 0 {
		return ""
	}
	st := steps[len(steps)-1]
	label := st.Kind
	if label == "" {
		label = "step"
	}
	if name := st.callName(); name != "" {
		label += " " + name
	}
	if st.Reason != "" {
		label += " (" + st.Reason + ")"
	}
	switch {
	case st.Running:
		label += " [running]"
	case st.IsError:
		label += " [error]"
	}
	return label
}

// RecapErrors lists a trace's failures: turn-level error/recovery steps by their
// machine tag, and tool calls that came back IsError by name. Returns nil when
// the turn had none — a silent empty string would make a failed turn read like a
// clean one.
func RecapErrors(steps []RecapStep) []string {
	var out []string
	for _, st := range steps {
		switch {
		case st.Kind == "error" || st.Kind == "recovery":
			label := st.Reason
			if label == "" {
				label = st.Kind
			}
			if name := st.callName(); name != "" {
				label = name + ": " + label
			}
			out = append(out, label)
		case st.isToolCall() && st.IsError:
			out = append(out, st.callName()+" failed")
		}
	}
	return out
}

// isToolCall reports whether a step is an actionable invocation: a tool call or
// the diff card a file mutation renders as.
func (st RecapStep) isToolCall() bool {
	return (st.Kind == "tool" || st.Kind == "diff") && st.Tool != ""
}

// callName is the name to show for a step: the exact namespaced callable when
// the provider used one, else the bare tool name.
func (st RecapStep) callName() string {
	if st.CallName != "" {
		return st.CallName
	}
	return st.Tool
}

// recapLine renders one tool step. maxOutput 0 omits the result entirely and
// reports only the call's state.
func (st RecapStep) recapLine(maxOutput int) string {
	head := st.callName()
	if arg := recapArgHint(st.Input); arg != "" {
		head = fmt.Sprintf("%s(%s)", head, arg)
	}
	if maxOutput <= 0 {
		switch {
		case st.Running:
			return "- " + head + " [running]"
		case st.IsError:
			return "- " + head + " [error]"
		}
		return "- " + head
	}
	result := truncRunes(strings.TrimSpace(st.Output), maxOutput)
	if st.IsError {
		if result == "" {
			result = "error"
		} else {
			result = "error: " + result
		}
	}
	if result == "" {
		result = "(no output)"
	}
	// Keep each line single-line so the recap stays compact.
	result = strings.ReplaceAll(result, "\n", " ⏎ ")
	return fmt.Sprintf("- %s → %s", head, result)
}

// recapArgHint pulls a short, human-recognizable argument out of a tool's JSON
// input (the command / path / pattern / url / query), falling back to "".
func recapArgHint(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "old_string"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return truncRunes(strings.TrimSpace(s), recapMaxArgHint)
			}
		}
	}
	return ""
}

// truncRunes caps s to max runes (Unicode-safe), appending "…" when cut.
func truncRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}
