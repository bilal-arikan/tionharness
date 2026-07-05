package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// titleSystemPrompt instructs a provider to emit a short, bare title that
// summarizes a request or conversation. The wording is deliberately strict so
// providers that like to add preamble or quotes stay terse.
const titleSystemPrompt = `You generate short titles. Given a user's request, message, or conversation, reply with a concise title of 3 to 6 words that summarizes it. Rules: reply with ONLY the title — no surrounding quotes, no trailing punctuation, no markdown, no preamble. Maximum 60 characters. Write the title in the same language as the input.`

// maxTitleSourceRunes caps how much input text we feed the titler; the opening
// of a request is more than enough to summarize and keeps the call cheap.
const maxTitleSourceRunes = 2000

// GenerateTitle asks the agent's provider for a short title summarizing source.
// It is user-initiated (not budget-gated) but records usage like any call.
func (r *Runtime) GenerateTitle(ctx context.Context, agent db.Agent, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", errors.New("empty source for title")
	}
	// The instruction is repeated inside the user turn (not only the system
	// prompt) because agentic CLI providers, e.g. claude-cli, carry a large
	// base prompt that can otherwise drown out an appended system instruction.
	userPrompt := "Below is a request or conversation. Reply with ONLY a concise title of 3 to 6 words that summarizes it — no quotes, no trailing punctuation, no preamble, max 60 characters, same language as the content.\n\n---\n" +
		truncateRunes(source, maxTitleSourceRunes) + "\n---\n\nTitle:"

	// A settings override lets titles be generated with a cheaper/faster model
	// than the agent normally uses; empty falls back to the agent's model.
	model := agent.Model
	if override := r.tun.TitleModel(); override != "" {
		model = override
	}

	resp, err := r.guardedComplete(WithCallKind(ctx, KindTitle), agent, providers.Request{
		Model:  model,
		System: r.readPrompt("title"),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: userPrompt},
		},
	}, false)
	if err != nil {
		return "", err
	}
	return SanitizeTitle(resp.Text), nil
}

// TitleFor resolves a titling agent (the preferred one if given, otherwise the
// first available) and generates a title. If no agent exists or generation
// fails it falls back to a trimmed form of source, so the caller always gets a
// usable title; any underlying error is still returned for logging.
func (r *Runtime) TitleFor(ctx context.Context, preferredAgentID, source string) (string, error) {
	agent, err := r.resolveTitlerAgent(ctx, preferredAgentID)
	if err != nil {
		return FallbackTitle(source), err
	}
	title, genErr := r.GenerateTitle(ctx, agent, source)
	if genErr != nil || title == "" {
		return FallbackTitle(source), genErr
	}
	return title, nil
}

// resolveTitlerAgent picks an agent to perform titling with: the preferred
// agent when it exists, otherwise the most recently created agent.
func (r *Runtime) resolveTitlerAgent(ctx context.Context, preferredAgentID string) (db.Agent, error) {
	if preferredAgentID != "" {
		if a, err := r.db.GetAgent(ctx, preferredAgentID); err == nil {
			return a, nil
		}
	}
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return db.Agent{}, err
	}
	if len(agents) == 0 {
		return db.Agent{}, errors.New("no agent available for titling")
	}
	return agents[0], nil
}

// SanitizeTitle normalizes a raw model reply into a single clean title line:
// first line only, surrounding quotes and trailing punctuation stripped, capped
// to a sensible length.
func SanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.Trim(s, "\"'`“”‘’ ")
	s = strings.TrimRight(s, ".。!！?？:： ")
	s = strings.TrimSpace(s)
	return truncateRunes(s, 60)
}

// FallbackTitle derives a plain title from arbitrary source text when no model
// title is available (no agents, or generation failed).
func FallbackTitle(source string) string {
	source = strings.TrimSpace(source)
	if i := strings.IndexAny(source, "\r\n"); i >= 0 {
		source = strings.TrimSpace(source[:i])
	}
	if source == "" {
		return "Untitled"
	}
	return truncateRunes(source, 50)
}

// truncateRunes shortens s to at most max runes (Unicode-safe), trimming any
// trailing space left by the cut.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max]))
}
