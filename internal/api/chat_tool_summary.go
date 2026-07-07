package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// Bounds for the recent-tool-activity recap folded back into the history. Only
// the last few assistant turns get a recap, each turn lists at most N tools, and
// each tool's output is truncated — so an agent can answer "what did you just do /
// what did that return" without re-bloating context with full tool I/O.
const (
	toolSummaryRecentTurns = 4   // assistant turns (newest) that get a recap
	toolSummaryMaxTools    = 10  // tool lines per turn before eliding the rest
	toolSummaryMaxOutput   = 240 // runes kept of each tool's output
)

// histToolStep is the minimal shape parsed from a message's serialized Steps
// trace — just the tool name, input and (truncated) output. Mirrors the relevant
// fields of agent.TurnStep without importing it.
type histToolStep struct {
	Kind string `json:"kind"`
	Tool string `json:"tool"`
	// CallName is the exact (namespaced) name the provider used on the claude-cli
	// path; empty for native/bare tools. Preferred over Tool in the recap so the
	// model sees the real callable name (mcp__tionswarm_extended__list_tasks) and
	// does not re-call the bare form (which the CLI rejects). See agent.TurnStep.
	CallName string          `json:"callName"`
	Input    json.RawMessage `json:"input"`
	Output   string          `json:"output"`
	IsError  bool            `json:"isError"`
}

// appendRecentToolSummaries folds a compact recap of each recent assistant turn's
// tool activity into that turn's text, so the model can see WHICH tools it ran and
// what they returned in the last few turns. The stored tool I/O lives only in the
// message's Steps trace (dropped when history → provider messages), so without
// this an agent literally cannot answer "what did that command output". Bounded
// (last N turns, M tools/turn, truncated output) to keep the token cost small.
//
// Works on a copy; the stored messages are never mutated.
func appendRecentToolSummaries(history []db.Message) []db.Message {
	// Indices of the newest assistant turns that should get a recap.
	recap := make(map[int]bool)
	seen := 0
	for i := len(history) - 1; i >= 0 && seen < toolSummaryRecentTurns; i-- {
		if history[i].Role == providers.RoleAssistant {
			recap[i] = true
			seen++
		}
	}
	if len(recap) == 0 {
		return history
	}

	out := make([]db.Message, len(history))
	copy(out, history)
	for i := range out {
		if !recap[i] || out[i].Steps == "" {
			continue
		}
		if block := toolRecapBlock(out[i].Steps); block != "" {
			out[i].Text = strings.TrimSpace(out[i].Text + "\n\n" + block)
		}
	}
	return out
}

// toolRecapBlock parses a serialized Steps trace and renders a compact
// <recent_tool_activity> block, or "" when the turn ran no tools.
func toolRecapBlock(stepsJSON string) string {
	var steps []histToolStep
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return ""
	}
	lines := make([]string, 0, toolSummaryMaxTools)
	elided := 0
	for _, st := range steps {
		// Tool invocations (and file-mutation diff cards) are the actionable I/O.
		if st.Kind != "tool" && st.Kind != "diff" {
			continue
		}
		if st.Tool == "" {
			continue
		}
		if len(lines) >= toolSummaryMaxTools {
			elided++
			continue
		}
		lines = append(lines, formatToolRecapLine(st))
	}
	if len(lines) == 0 {
		return ""
	}
	if elided > 0 {
		lines = append(lines, fmt.Sprintf("- … (+%d more tool call(s))", elided))
	}
	return "<recent_tool_activity>\n" + strings.Join(lines, "\n") + "\n</recent_tool_activity>"
}

// formatToolRecapLine renders one tool step as "- Tool(argHint) → result".
func formatToolRecapLine(st histToolStep) string {
	arg := toolArgHint(st.Input)
	// Prefer the exact callable name (namespaced on the CLI path) so the model can
	// re-call the tool verbatim; fall back to the bare name for native tools.
	name := st.CallName
	if name == "" {
		name = st.Tool
	}
	head := name
	if arg != "" {
		head = fmt.Sprintf("%s(%s)", name, arg)
	}
	result := truncateRunes(strings.TrimSpace(st.Output), toolSummaryMaxOutput)
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

// toolArgHint pulls a short, human-recognizable argument from a tool's JSON input
// (the command / path / pattern / url / query), falling back to "".
func toolArgHint(input json.RawMessage) string {
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
				return truncateRunes(strings.TrimSpace(s), 80)
			}
		}
	}
	return ""
}

// truncateRunes caps s to max runes (Turkish-safe), appending "…" when cut.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}
