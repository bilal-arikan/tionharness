package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/prompts"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// HandoffEnv carries the environment snapshot folded into a handoff artifact's
// "Environment State" section — the durable, non-transcript facts the next agent
// needs to resume in a clean window (working dir, git branch, artifacts).
// All fields are optional; empty ones are simply omitted from the rendered block.
type HandoffEnv struct {
	WorkingDir string // cwd the fs/shell tools operate in
	GitBranch  string // current git branch at reset time, if known
	GitStatus  string // short git status snapshot, if gathered
	Todos      string // active todo checklist, if any
	Artifacts  string // existing artifacts (id + title), if any
}

// rendered turns the env snapshot into a markdown block appended to the
// transcript handed to the model, so the generated artifact's Environment State
// section is grounded in real values rather than guessed from the conversation.
func (e HandoffEnv) rendered() string {
	var b strings.Builder
	add := func(label, val string) {
		if v := strings.TrimSpace(val); v != "" {
			fmt.Fprintf(&b, "- %s: %s\n", label, v)
		}
	}
	add("Working directory", e.WorkingDir)
	add("Git branch", e.GitBranch)
	if s := strings.TrimSpace(e.GitStatus); s != "" {
		fmt.Fprintf(&b, "- Git status:\n```\n%s\n```\n", s)
	}
	if s := strings.TrimSpace(e.Todos); s != "" {
		fmt.Fprintf(&b, "- Active todos:\n%s\n", s)
	}
	if s := strings.TrimSpace(e.Artifacts); s != "" {
		fmt.Fprintf(&b, "- Existing artifacts:\n%s\n", s)
	}
	if b.Len() == 0 {
		return "(no environment snapshot)"
	}
	return b.String()
}

// The handoff prompt template lives in the central prompt registry
// (internal/prompts, key "handoff"): a context-reset artifact that lets a FRESH
// agent in a clean window resume long-running work without restarting. It
// extends the rolling-summary's 8 sections with the reset-critical parts — an
// explicit DONE/PENDING split and a single "next concrete step" — which is what
// defeats "context anxiety". Placeholders: {{summary}} (existing rolling
// summary), {{transcript}} (turns to fold), {{environment}} (env snapshot).

// BuildHandoff generates a context-reset handoff document from a session's
// existing rolling summary, its rendered transcript, and an environment snapshot.
// It reuses the same provider-call core as compaction (recorded under
// UsageKindCompact) but with the continuation-oriented handoff prompt.
// promptTmpl is the (possibly workspace-overridden) template; blank or invalid
// falls back to the registry default. The result is the handoff markdown body
// (without a title); the caller persists it as an artifact and seeds a fresh
// session with it.
func BuildHandoff(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, existingSummary, rendered string, env HandoffEnv, promptTmpl string) (string, error) {
	existing := strings.TrimSpace(existingSummary)
	if existing == "" {
		existing = "(none)"
	}
	if strings.TrimSpace(rendered) == "" {
		return "", fmt.Errorf("cannot build handoff from an empty transcript")
	}
	if prompts.Validate("handoff", promptTmpl) != nil {
		promptTmpl = prompts.Default("handoff")
	}
	resp, err := provider.Complete(ctx, providers.Request{
		Model:     agent.Model,
		MaxTokens: compactMaxOutputTokens,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompts.Render(promptTmpl, map[string]string{
				"summary":     existing,
				"transcript":  rendered,
				"environment": env.rendered(),
			})},
		},
	})
	if err != nil {
		return "", err
	}
	recordCompaction(ctx, database, agent, resp.Usage)
	return strings.TrimSpace(resp.Text), nil
}

// RenderTranscript flattens stored turns to the "role: text" transcript the
// handoff prompt expects. Exposed so the agent package can build a handoff from a
// session's full message history without re-implementing the rendering.
func RenderTranscript(msgs []db.Message) string {
	return renderDBMessages(msgs)
}
