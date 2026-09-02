package api

import (
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Bounds for the recent-tool-activity recap injected into the volatile dynamic
// suffix. Only the last few assistant turns are recapped, each turn lists at
// most N tools, and each tool's output is truncated — so an agent can answer
// "what did you just do / what did that return" without re-bloating context
// with full tool I/O.
const (
	toolSummaryRecentTurns = 4   // assistant turns (newest) that get a recap
	toolSummaryMaxTools    = 10  // tool lines per turn before eliding the rest
	toolSummaryMaxOutput   = 240 // runes kept of each tool's output
)

// recentToolActivityBlock renders ONE compact <recent_tool_activity> block
// covering the newest assistant turns' tool I/O, for the volatile dynamic
// suffix. The stored tool I/O lives only in each message's Steps trace (dropped
// when history → provider messages), so without this an agent literally cannot
// answer "what did that command output".
//
// It deliberately does NOT fold the recap into the history messages themselves
// (the previous design): a recap embedded in a past assistant turn changes that
// turn's bytes when it later drops out of the newest-N window, invalidating the
// rolling prompt-cache breakpoint on the conversation history. As a dynamic
// block it rides after the cache breakpoint and the history stays byte-stable.
// Returns "" when none of the recapped turns ran tools.
func recentToolActivityBlock(history []db.Message) string {
	// Newest assistant turns, oldest→newest, with their age in assistant turns.
	type turn struct {
		age   int // 0 = latest assistant turn
		steps string
	}
	var turns []turn
	age := 0
	for i := len(history) - 1; i >= 0 && age < toolSummaryRecentTurns; i-- {
		if history[i].Role != providers.RoleAssistant {
			continue
		}
		if history[i].Steps != "" {
			turns = append([]turn{{age: age, steps: history[i].Steps}}, turns...)
		}
		age++
	}
	var sections []string
	for _, t := range turns {
		lines := toolRecapLines(t.steps)
		if len(lines) == 0 {
			continue
		}
		label := "[latest assistant turn]"
		if t.age > 0 {
			label = fmt.Sprintf("[%d assistant turn(s) ago]", t.age)
		}
		sections = append(sections, label+"\n"+strings.Join(lines, "\n"))
	}
	if len(sections) == 0 {
		return ""
	}
	return "<recent_tool_activity>\n" + strings.Join(sections, "\n") + "\n</recent_tool_activity>"
}

// toolRecapLines parses a serialized Steps trace and renders its compact recap
// lines, or nil when the turn ran no tools. The parsing and rendering live in
// package tools (ParseRecapSteps/RecapLines) so get_session_info renders another session's
// activity from the same code path instead of a second, drifting parser; a
// malformed trace is ignored HERE — this recap is an optimisation for the
// prompt, and failing a turn over an unreadable past trace would help nobody.
func toolRecapLines(stepsJSON string) []string {
	steps, err := tools.ParseRecapSteps(stepsJSON)
	if err != nil {
		return nil
	}
	return tools.RecapLines(steps, tools.RecapOpts{
		MaxTools:  toolSummaryMaxTools,
		MaxOutput: toolSummaryMaxOutput,
	})
}

// truncateRunes caps s to max runes (Turkish-safe), appending "…" when cut.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}
