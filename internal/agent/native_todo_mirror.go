package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// mirrorNativeTodos persists the LAST checklist a CLI-native tool wrote this
// turn (claude-cli TodoWrite, codex update_plan) to the session's todo sink —
// the same progress file the bridged todo_write writes through the Interaction
// server. Without this the native call would render a card but leave the
// durable progress file stale, which is exactly why those natives used to be
// suppressed. Only the final list matters: every native write replaces the
// whole list, so the last one is the state the turn ended in. No-op without a
// sink on ctx or without a native checklist in the trace; the bridged
// todo_write is skipped because it already persisted itself when it ran.
func (r *Runtime) mirrorNativeTodos(ctx context.Context, trace []providers.TraceStep) {
	sink := tools.TodoSinkFrom(ctx)
	if sink == nil {
		return
	}
	items := nativeChecklistFromTrace(trace)
	if len(items) == 0 {
		return
	}
	if err := sink.SaveTodos(ctx, items); err != nil {
		r.logger.Warn("native checklist mirror: persist failed", "error", err)
	}
}

// nativeChecklistFromTrace returns the last checklist a CLI-NATIVE tool wrote in
// trace, in sink form; nil when the turn wrote none. Bridged todo_write steps
// are ignored (they persisted themselves), as are errored calls.
func nativeChecklistFromTrace(trace []providers.TraceStep) []tools.TodoSinkItem {
	var last []TodoItem
	for _, st := range trace {
		if st.Kind != "tool" || st.IsError {
			continue
		}
		tool := bareToolName(st.Tool)
		if tool == "todo_write" || !isChecklistTool(tool) {
			continue
		}
		if todos := todoStepItems(st.Input, st.Output); len(todos) > 0 {
			last = todos
		}
	}
	if len(last) == 0 {
		return nil
	}
	items := make([]tools.TodoSinkItem, 0, len(last))
	for _, t := range last {
		items = append(items, tools.TodoSinkItem{Content: t.Content, Status: t.Status})
	}
	return items
}

// bareToolName strips the Interaction MCP namespace a claude-cli trace carries
// (both tiers) so a bridged call and a native one compare by their plain name.
func bareToolName(tool string) string {
	if s := strings.TrimPrefix(tool, interactionToolPrefix); s != tool {
		return s
	}
	return strings.TrimPrefix(tool, extendedToolPrefix)
}
