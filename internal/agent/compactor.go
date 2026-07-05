package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools/compact"
)

// toolSummarySystemPrompt steers the cheap model used by System B: an
// intent-aware compressor, not a creative summarizer. It must keep every fact
// the calling agent could act on and invent nothing.
const toolSummarySystemPrompt = `You compress a tool's raw output for an autonomous agent's context window. Preserve every fact the agent could act on: file paths, identifiers, error messages, line numbers, counts, and final results. Remove boilerplate, decoration, and repetition. Never invent information not present in the output. Reply in the same language as the output. Output only the compressed result — no preamble, no commentary.`

// intentInputRunes caps how much of the tool input is echoed into the summary
// prompt as "intent" context, so a huge argument blob can't dominate.
const intentInputRunes = 300

// compactToolResult applies the two independent, parallel tool-output token
// optimization systems to a successful tool result before it re-enters the
// model context:
//
//   - System A (deterministic): free, rule-based shrink — always when enabled.
//   - System B (LLM summary): an intent-aware cheap-model pass, only when
//     enabled AND the (post-A) output still exceeds the configured threshold.
//
// Either may run alone or both in sequence. Error results are never touched
// (the agent needs the failure verbatim). Any failure in System B falls back to
// the System A output, so compaction can never break a turn.
func (r *Runtime) compactToolResult(ctx context.Context, agent db.Agent, toolName string, input json.RawMessage, res providers.ToolResult) providers.ToolResult {
	if res.IsError || strings.TrimSpace(res.Content) == "" {
		return res
	}
	content := res.Content

	// Per-model effective budget: scale the byte thresholds to this agent's model
	// window (Option B), so a big-context model tolerates larger tool output before
	// compaction. Falls back to the configured/default budget when unknown.
	// 0,0 → EffectiveBudget uses its package-default fraction/ceil; tool-output
	// threshold scaling does not need the live conversation-budget knobs.
	budget := conversation.EffectiveBudget(agent.Provider, agent.Model, r.tun.ContextBudgetTokens(), 0, 0)

	// System A — deterministic, dependency-free.
	if r.tun.CompactDeterministic() {
		out, st := compact.Compact(content, compact.Options{
			Enabled:  true,
			Dedupe:   true,
			MaxLines: r.tun.CompactMaxLines(),
			MaxBytes: r.tun.CompactMaxBytesFor(budget),
		})
		if st.Applied {
			r.logger.Debug("tool output compacted (A)",
				"agent", agent.ID, "tool", toolName,
				"before", st.BeforeBytes, "after", st.AfterBytes, "saved", st.Saved())
			// Persist the free savings into today's usage rollup (standalone meter)
			// and into the originating session's lifetime rollup (blank sid = no-op).
			if err := r.db.AddCompactionSavings(ctx, agent.ID, st.Saved()); err != nil {
				r.logger.Warn("record compaction savings failed", "agent", agent.ID, "error", err)
			}
			if sid := SessionIDFrom(ctx); sid != "" {
				_ = r.db.AddSessionCompactionSavings(ctx, sid, agent.ID, st.Saved())
			}
		}
		content = out
	}

	// System B — LLM intent-aware summary, gated by size.
	if r.tun.CompactLLM() && len(content) > r.tun.CompactLLMThresholdFor(budget) {
		summary, err := r.summarizeToolOutput(ctx, agent, toolName, input, content)
		switch {
		case err != nil:
			r.logger.Warn("tool output summary failed; keeping deterministic output",
				"agent", agent.ID, "tool", toolName, "error", err)
		case strings.TrimSpace(summary) == "":
			r.logger.Warn("tool output summary empty; keeping deterministic output",
				"agent", agent.ID, "tool", toolName)
		default:
			savedB := len(content) - len(summary)
			r.logger.Debug("tool output summarized (B)",
				"agent", agent.ID, "tool", toolName, "before", len(content), "after", len(summary), "saved", savedB)
			// Persist System B's gross output reduction into today's rollup (standalone
			// meter; the summary call's own token cost is already recorded as KindCompact).
			if err := r.db.AddLLMCompactionSavings(ctx, agent.ID, savedB); err != nil {
				r.logger.Warn("record llm compaction savings failed", "agent", agent.ID, "error", err)
			}
			if sid := SessionIDFrom(ctx); sid != "" {
				_ = r.db.AddSessionLLMCompactionSavings(ctx, sid, agent.ID, savedB)
			}
			content = summary
		}
	}

	res.Content = content
	return res
}

// summarizeToolOutput runs System B: it asks a cheap model (the title-model
// override when configured, else the agent's own model) to compress output
// while preserving everything relevant to the tool's purpose. Attributed to
// KindCompact for usage accounting; never gates the daily budget (mirrors the
// other auxiliary calls — titling, summaries, reflection).
func (r *Runtime) summarizeToolOutput(ctx context.Context, agent db.Agent, toolName string, input json.RawMessage, output string) (string, error) {
	// Model resolution chain: dedicated compaction model → title model → the
	// agent's own model. The dedicated knob lets the user pin a cheap model for
	// summaries without affecting auto-titling. Provider is always the agent's.
	model := agent.Model
	if title := r.tun.TitleModel(); title != "" {
		model = title
	}
	if cm := r.tun.CompactModel(); cm != "" {
		model = cm
	}

	intent := toolName
	if s := strings.TrimSpace(string(input)); s != "" && s != "{}" {
		intent = fmt.Sprintf("%s (input: %s)", toolName, truncateRunes(oneLine(s), intentInputRunes))
	}

	resp, err := r.guardedComplete(WithCallKind(ctx, KindCompact), agent, providers.Request{
		Model:  model,
		System: toolSummarySystemPrompt,
		Messages: []providers.Message{{
			Role: providers.RoleUser,
			Text: fmt.Sprintf("Tool: %s\n\nCompress this tool output, preserving everything relevant to that tool's purpose:\n\n%s", intent, output),
		}},
	}, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}
