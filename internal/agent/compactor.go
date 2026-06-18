package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools/compact"
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

	// System A — deterministic, dependency-free.
	if r.tun.CompactDeterministic() {
		out, st := compact.Compact(content, compact.Options{
			Enabled:  true,
			Dedupe:   true,
			MaxLines: r.tun.CompactMaxLines(),
			MaxBytes: r.tun.CompactMaxBytes(),
		})
		if st.Applied {
			r.logger.Debug("tool output compacted (A)",
				"agent", agent.ID, "tool", toolName,
				"before", st.BeforeBytes, "after", st.AfterBytes, "saved", st.Saved())
		}
		content = out
	}

	// System B — LLM intent-aware summary, gated by size.
	if r.tun.CompactLLM() && len(content) > r.tun.CompactLLMThreshold() {
		summary, err := r.summarizeToolOutput(ctx, agent, toolName, input, content)
		switch {
		case err != nil:
			r.logger.Warn("tool output summary failed; keeping deterministic output",
				"agent", agent.ID, "tool", toolName, "error", err)
		case strings.TrimSpace(summary) == "":
			r.logger.Warn("tool output summary empty; keeping deterministic output",
				"agent", agent.ID, "tool", toolName)
		default:
			r.logger.Debug("tool output summarized (B)",
				"agent", agent.ID, "tool", toolName, "before", len(content), "after", len(summary))
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
	model := agent.Model
	if override := r.tun.TitleModel(); override != "" {
		model = override
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
