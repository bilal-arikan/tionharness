package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// noteInputTransformations journals thinking blocks the API dropped from a
// request (Fable 5.1 "preserved thinking", providers.Response.InputTransformations).
// The native client asks the API to DROP rather than fail when a block no longer
// matches the conversation prefix, so a turn survives an in-flight history edit —
// but every such drop costs the model its earlier reasoning and restarts the
// cache from that point, so it must be visible: one debug event per response,
// reason in Name, dropped paths in Detail, and a warning in the log.
func (r *Runtime) noteInputTransformations(ctx context.Context, agent db.Agent, resp *providers.Response) {
	if resp == nil || len(resp.InputTransformations) == 0 {
		return
	}
	paths := make([]string, 0, len(resp.InputTransformations))
	reasons := map[string]int{}
	for _, t := range resp.InputTransformations {
		paths = append(paths, t.Path+" ("+t.Reason+")")
		reasons[t.Reason]++
	}
	first := resp.InputTransformations[0].Reason
	r.logger.Warn("provider dropped thinking blocks from the request",
		"agent", agent.ID, "model", resp.Model, "count", len(paths), "reason", first)
	r.emitDebug(ctx, db.DebugEvent{
		Type:    db.DebugThinkingDropped,
		AgentID: agent.ID,
		Model:   resp.Model,
		Name:    first,
		Calls:   len(paths),
		Detail:  strings.Join(paths, ", "),
	})
}
