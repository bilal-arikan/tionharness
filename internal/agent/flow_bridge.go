package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// agentFlowBridge implements tools.FlowBridge for one calling agent.
type agentFlowBridge struct {
	rt    *Runtime
	actor string
}

func (b agentFlowBridge) target(agentID string) string {
	if strings.TrimSpace(agentID) == "" {
		return b.actor
	}
	return agentID
}

func (b agentFlowBridge) load(ctx context.Context, agentID string) (db.Flow, error) {
	a, err := b.rt.db.GetAgent(ctx, b.target(agentID))
	if err != nil {
		return db.Flow{}, fmt.Errorf("no agent with id %q (use list_agents)", b.target(agentID))
	}
	if a.System {
		return db.Flow{}, fmt.Errorf("agent %q is a system agent and has no flow", a.Name)
	}
	return b.rt.db.EnsureAgentFlow(ctx, a.ID)
}

// FlowInfoFor renders the agent-facing view of a flow (shared with the API).
func (r *Runtime) FlowInfoFor(ctx context.Context, f db.Flow, withVersions bool) (tools.FlowInfo, error) {
	g, err := flow.Parse(f.Graph)
	if err != nil {
		return tools.FlowInfo{}, err
	}
	pol := f.Policy.Normalized()
	info := tools.FlowInfo{ID: f.ID, AgentID: f.AgentID, Name: f.Name, Version: f.Version, Shape: g.Summary(), Note: f.Note,
		Policy: fmt.Sprintf("%s (every %d runs, min confidence %.2f, max %d nodes)", pol.Mode, pol.EveryRuns, pol.MinConfidence, pol.MaxNodes),
		Graph:  json.RawMessage(flow.Encode(g))}
	info.Stats.Runs, info.Stats.Success, info.Stats.Failure = f.Stats.Runs, f.Stats.Success, f.Stats.Failure
	info.Stats.LastRunAt, info.Stats.LastRunID = f.Stats.LastRunAt, f.Stats.LastRunID
	if f.Stats.Runs > 0 {
		info.Stats.AvgMs = f.Stats.TotalMs / int64(f.Stats.Runs)
		info.Stats.AvgTokens = f.Stats.TotalTokens / int64(f.Stats.Runs)
	}
	if withVersions {
		vs, _ := r.db.ListFlowVersions(ctx, f.ID)
		for _, v := range vs {
			author := v.Author.Kind
			if v.Author.ID != "" {
				author += ":" + v.Author.ID
			}
			info.Versions = append(info.Versions, tools.FlowVersionInfo{Version: v.Version, Author: author, Reason: v.Reason, Diff: v.Diff, CreatedAt: v.CreatedAt})
		}
	}
	return info, nil
}

func (b agentFlowBridge) GetFlow(ctx context.Context, agentID string, withVersions bool) (tools.FlowInfo, error) {
	f, err := b.load(ctx, agentID)
	if err != nil {
		return tools.FlowInfo{}, err
	}
	return b.rt.FlowInfoFor(ctx, f, withVersions)
}

func (b agentFlowBridge) EditFlow(ctx context.Context, agentID string, raw json.RawMessage, reason string) (tools.FlowInfo, error) {
	f, err := b.load(ctx, agentID)
	if err != nil {
		return tools.FlowInfo{}, err
	}
	ops, err := flow.ParseOps(raw)
	if err != nil {
		return tools.FlowInfo{}, err
	}
	if _, err := b.rt.ApplyFlowOps(ctx, f.ID, ops, db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: b.actor}, reason, ""); err != nil {
		return tools.FlowInfo{}, err
	}
	f, _ = b.rt.db.GetFlow(ctx, f.ID)
	return b.rt.FlowInfoFor(ctx, f, false)
}

func (b agentFlowBridge) RevertFlow(ctx context.Context, agentID string, version int, reason string) (tools.FlowInfo, error) {
	f, err := b.load(ctx, agentID)
	if err != nil {
		return tools.FlowInfo{}, err
	}
	if _, err := b.rt.RevertFlow(ctx, f.ID, version, db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: b.actor}, reason); err != nil {
		return tools.FlowInfo{}, err
	}
	f, _ = b.rt.db.GetFlow(ctx, f.ID)
	return b.rt.FlowInfoFor(ctx, f, false)
}

func (b agentFlowBridge) ListFlowRuns(ctx context.Context, agentID string, limit int, withSteps bool) ([]tools.FlowRunInfo, error) {
	f, err := b.load(ctx, agentID)
	if err != nil {
		return nil, err
	}
	runs, err := b.rt.db.ListFlowRuns(ctx, f.ID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]tools.FlowRunInfo, 0, len(runs))
	for _, run := range runs {
		item := tools.FlowRunInfo{ID: run.ID, Status: run.Status, Version: run.Version, SessionID: run.SessionID, DurationMs: run.DurationMs,
			Tokens: run.Usage.InputTokens + run.Usage.OutputTokens, Feedback: run.Feedback,
			Input: truncateRunes(run.Input, 400), Output: truncateRunes(run.Output, 400), Error: run.Error, CreatedAt: run.CreatedAt}
		if withSteps {
			item.Steps = run.Steps
		}
		out = append(out, item)
	}
	return out, nil
}

func (b agentFlowBridge) UpdatePrompts(ctx context.Context, soul, identity *string, reason string) (string, error) {
	if err := b.rt.UpdateAgentPrompts(ctx, b.actor, soul, identity, db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: b.actor}, reason, ""); err != nil {
		return "", err
	}
	return "Prompt updated; it takes effect on your next turn. The previous text is kept as a prompt version.", nil
}
