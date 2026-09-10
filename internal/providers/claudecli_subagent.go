package providers

import (
	"strings"
	"time"
)

// This file folds a CLI-native subagent's events into the trace step of the
// Agent call that launched it.
//
// claude-cli (2.1.211+) tags every assistant/user event that belongs to a
// subagent with parent_tool_use_id = the tool_use id of the launching Agent
// call; with CLAUDE_CODE_FORWARD_SUBAGENT_TEXT the subagent's text and thinking
// blocks are forwarded too, so the whole nested transcript is on the wire. The
// parser keeps those events OUT of the main trace (a subagent's intermediate
// text is not the reply) and appends them to the parent step's SubSteps
// instead, re-publishing the parent as a running card after each addition so
// the delegation is visible while it runs — the same shape run_subagent's
// live card has on the native path.

// Environment claude-cli reads for the native subagent menu.
const (
	// envSubagentDepth caps how deep native subagents may nest. 1 = the main
	// conversation may spawn, a subagent may not: TionHarness only opens the
	// launcher for read-only research, and a research helper has no business
	// fanning out further.
	envSubagentDepth = "CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1"
	// envForwardSubagentText asks the CLI to forward subagent text and thinking
	// blocks (default: only their tool_use/tool_result), so the folded transcript
	// is complete rather than tool-calls-only.
	envForwardSubagentText = "CLAUDE_CODE_FORWARD_SUBAGENT_TEXT=1"
)

// nativeSubagentEnv returns the env entries a claude-cli launch needs when the
// native Agent launcher is on the menu; nil otherwise.
func nativeSubagentEnv(req Request) []string {
	if !req.CLINativeSubagents {
		return nil
	}
	return []string{envSubagentDepth, envForwardSubagentText}
}

// subRef locates a nested tool step: the parent's index in resp.Trace and the
// step's index inside that parent's SubSteps.
type subRef struct{ parent, sub int }

// subagentParent resolves the trace index of the Agent step an event belongs
// to. Only a DIRECT child of a known launcher is folded; an unknown parent (a
// nested spawn whose launcher lives inside another subagent, or an id the
// parser never saw) falls back to the top-level handling so nothing is lost.
func (p *cliStreamParser) subagentParent(ev *cliEvent) (int, bool) {
	if ev.ParentToolUseID == "" {
		return 0, false
	}
	idx, ok := p.toolIdx[ev.ParentToolUseID]
	return idx, ok
}

// feedSubagentAssistant folds one subagent assistant event into parent's
// SubSteps. Usage is NOT merged here: the result envelope carries the
// authoritative aggregate across every internal call, subagents included, and
// overrides the running counters when it arrives.
func (p *cliStreamParser) feedSubagentAssistant(parent int, ev *cliEvent) {
	p.sawModelTurn = true
	if ev.Message == nil {
		return
	}
	step := &p.resp.Trace[parent]
	// The launching call needs a stable card id so every partial republish and
	// the final tool_result emission replace the same card in the UI.
	if step.ID == "" {
		step.ID = ev.ParentToolUseID
	}
	added := false
	for _, b := range ev.Message.Content {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				step.SubSteps = append(step.SubSteps, TraceStep{Kind: "text", Text: t})
				added = true
			}
		case "thinking":
			if t := strings.TrimSpace(b.Thinking); t != "" {
				step.SubSteps = append(step.SubSteps, TraceStep{Kind: "thinking", Text: t})
				added = true
			}
		case "tool_use":
			step.SubSteps = append(step.SubSteps, TraceStep{Kind: "tool", Tool: b.Name, Input: b.Input})
			if b.ID != "" {
				p.subToolIdx[b.ID] = subRef{parent: parent, sub: len(step.SubSteps) - 1}
				p.toolStart[b.ID] = time.Now()
			}
			added = true
		case "server_tool_use":
			step.SubSteps = append(step.SubSteps, TraceStep{Kind: "tool", Tool: b.Name, Input: b.Input, Output: "(executed server-side)"})
			added = true
		case "web_search_tool_result":
			step.SubSteps = append(step.SubSteps, TraceStep{Kind: "tool", Tool: webSearchName, Output: renderWebToolResult(b.Content, "result")})
			added = true
		case "web_fetch_tool_result":
			step.SubSteps = append(step.SubSteps, TraceStep{Kind: "tool", Tool: webFetchName, Output: renderWebToolResult(b.Content, "document")})
			added = true
		default:
			p.noteUnknownBlock(b.Type)
		}
	}
	if added {
		p.emitSubagentLive(parent)
	}
}

// feedSubagentToolResult fills a nested tool step's result. Returns false when
// the id is not a nested step so the caller can try the main trace.
func (p *cliStreamParser) feedSubagentToolResult(b cliBlock) bool {
	ref, ok := p.subToolIdx[b.ToolUseID]
	if !ok {
		return false
	}
	sub := &p.resp.Trace[ref.parent].SubSteps[ref.sub]
	sub.Output = CapToolOutput(toolResultText(b.Content))
	sub.IsError = b.IsError
	if start, ok := p.toolStart[b.ToolUseID]; ok {
		sub.DurMs = time.Since(start).Milliseconds()
		delete(p.toolStart, b.ToolUseID)
	}
	p.emitSubagentLive(ref.parent)
	return true
}

// emitSubagentLive republishes the launching step as a RUNNING card carrying the
// sub-steps gathered so far. Skipped once the final step went out (its
// tool_result arrived), so a late-forwarded sub-event cannot reopen a closed
// card. The copy detaches SubSteps so the consumer never aliases the parser's
// growing slice.
func (p *cliStreamParser) emitSubagentLive(parent int) {
	if p.onEvent == nil || p.emitted[parent] {
		return
	}
	live := p.resp.Trace[parent]
	live.Running = true
	live.SubSteps = append([]TraceStep(nil), live.SubSteps...)
	p.dispatchTrace(live)
}
