package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Summary kinds the chat composer can request on demand from the "/" palette.
const (
	SummaryBoard = "board"
	SummaryFlows = "flows"
	SummaryTools = "tools"
)

// The summary system prompt lives in the central registry (internal/prompts,
// key "summary"); readPrompt resolves the workspace override.

// maxSummaryItems caps how many rows of each kind feed one summary, bounding the
// prompt size (and cost).
const maxSummaryItems = 40

// summaryItemRunes caps each item's text so one long entry cannot dominate.
const summaryItemRunes = 200

// Summarize produces an on-demand overview of a slice of workspace/agent data.
// The "tools" kind is rendered deterministically (no model call); the others are
// summarized by a cheap model — the title-model override when configured,
// otherwise the agent's own model. Returns the summary text (markdown).
func (r *Runtime) Summarize(ctx context.Context, agentID, kind string) (string, error) {
	agent, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return "", err
	}

	if kind == SummaryTools {
		return r.toolsOverview(ctx, agent), nil
	}

	data, label, err := r.gatherSummaryData(ctx, agent, kind)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(data) == "" {
		return "_(boş — özetlenecek " + label + " yok)_", nil
	}

	agentCfg, summaryPrompt := r.resolveCompactorConfig(agent)
	resp, err := r.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindSummary), "summary", summaryPrompt), agentCfg, providers.Request{
		Model:  agentCfg.Model,
		System: summaryPrompt,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: fmt.Sprintf("Summarize the following %s for the user:\n\n%s", label, data)},
		},
	}, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

func (r *Runtime) resolveCompactorConfig(agent db.Agent) (db.Agent, string) {
	compactor, _, err := r.ResolveSystemAgent("overview-summarizer")
	if err != nil {
		r.logger.Warn("system compactor resolution failed; using embedded summary behavior", "error", err)
		return agent, r.readPrompt("summary")
	}

	agent.Model = adoptSystemAgentModel(r.logger, "overview-summarizer", agent.Provider, agent.Model, compactor.Model)
	agent.System = true
	agent.SystemKey = compactor.SystemKey
	return agent, compactor.Soul
}

// gatherSummaryData collects up to maxSummaryItems rows of the requested kind and
// renders them as a compact bullet list for the summarizer, plus a human label.
func (r *Runtime) gatherSummaryData(ctx context.Context, agent db.Agent, kind string) (data, label string, err error) {
	var sb strings.Builder
	switch kind {
	case SummaryBoard:
		tasks, e := r.db.ListTasks(ctx)
		if e != nil {
			return "", "görev", e
		}
		if len(tasks) > maxSummaryItems {
			tasks = tasks[:maxSummaryItems]
		}
		for _, t := range tasks {
			fmt.Fprintf(&sb, "- [%s] %s — %s\n", t.BoardState, t.Title, oneLine(t.Description))
		}
		return sb.String(), "görev", nil
	case SummaryFlows:
		flows, e := r.db.ListFlows(ctx)
		if e != nil {
			return "", "akış", e
		}
		if len(flows) > maxSummaryItems {
			flows = flows[:maxSummaryItems]
		}
		for _, f := range flows {
			fmt.Fprintf(&sb, "- %s (%s)\n", f.Name, f.ID)
		}
		return sb.String(), "akış", nil
	default:
		return "", "", fmt.Errorf("unknown summary kind: %q", kind)
	}
}

// toolsOverview lists the tools available to an agent (built-ins + MCP) with a
// short description each. Deterministic — no model call.
func (r *Runtime) toolsOverview(ctx context.Context, agent db.Agent) string {
	if !agent.MCPEnabled {
		return fmt.Sprintf("**%s** için araçlar kapalı (MCP/araç erişimi devre dışı).", agent.Name)
	}
	defs := r.ToolCatalog(ctx, agent)
	if len(defs) == 0 {
		return fmt.Sprintf("**%s** için izin verilen araç yok.", agent.Name)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "**%s** için kullanılabilir %d araç:\n\n", agent.Name, len(defs))
	for _, d := range defs {
		if desc := oneLine(d.Description); desc != "" {
			fmt.Fprintf(&sb, "- `%s` — %s\n", d.Name, desc)
		} else {
			fmt.Fprintf(&sb, "- `%s`\n", d.Name)
		}
	}
	return strings.TrimSpace(sb.String())
}

// oneLine flattens text to a single trimmed line capped at summaryItemRunes.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, summaryItemRunes)
}
