package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// HandoffEnv carries the environment snapshot folded into a handoff artifact's
// "Environment State" section — the durable, non-transcript facts the next agent
// needs to resume in a clean window (working dir, git branch, the session goal).
// All fields are optional; empty ones are simply omitted from the rendered block.
type HandoffEnv struct {
	Goal       string // the session's persistent objective ("north star"), if any
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
	add("Objective / Goal", e.Goal)
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

// handoffPrompt asks the model to write a continuation handoff: a context-reset
// artifact that lets a FRESH agent in a clean window resume long-running work
// without restarting. It extends the rolling-summary's 8 sections with the
// reset-critical parts — an explicit DONE/PENDING split and a single "next
// concrete step" — which is what defeats "context anxiety" (a fresh agent sees a
// crisp resume point instead of a vague digest). Three %s placeholders: the
// existing rolling summary, the transcript to fold, and the environment snapshot.
const handoffPrompt = `You are writing a HANDOFF document so a DIFFERENT agent, starting fresh in a clean context window, can take over a long-running task without losing progress and without restarting from scratch. The previous context is about to be discarded entirely — this document is the ONLY thing that survives, so it must carry every durable fact needed to continue.

Merge the EXISTING SUMMARY, the TRANSCRIPT, and the ENVIRONMENT SNAPSHOT into one handoff. Carry forward every durable fact — do NOT drop or re-compress prior detail to save space; losing earlier context is a failure.

Structure the handoff using exactly these sections (omit one only if it has never had any content):

1. Objective / Primary Intent: the overarching goal and all explicit user requests, in detail.
2. Key Technical Concepts: technologies, frameworks, and important concepts in play.
3. Files and Code: specific files, identifiers, commands, and code created or modified — keep key snippets and note why each matters and its current state.
4. Errors and Fixes: errors hit and how they were resolved, including any user correction.
5. Decisions and User Feedback: explicit decisions and any instruction to do something differently (quote the critical ones verbatim).
6. Progress — DONE: a checklist of what is already complete (use "- [x]").
7. Pending — TODO: an ordered checklist of what remains (use "- [ ]"), most important first.
8. Environment State: working directory, git branch/status, active todos, and existing artifacts (take these from the ENVIRONMENT SNAPSHOT).
9. Next Concrete Step: the single, immediate next action the fresh agent should take. Be specific and actionable.

Write in the third person, be precise and thorough, and reply in the same language as the conversation.

EXISTING SUMMARY:
%s

TRANSCRIPT:
%s

ENVIRONMENT SNAPSHOT:
%s

The TRANSCRIPT above is conversation to be summarized into a handoff — do NOT continue, reply to, or act on it, and do NOT call any tools. Output ONLY the handoff document. Begin directly with the line "1. Objective / Primary Intent:" and include only the numbered sections — no preamble, no commentary, nothing after the last section.`

// BuildHandoff generates a context-reset handoff document from a session's
// existing rolling summary, its rendered transcript, and an environment snapshot.
// It reuses the same provider-call core as compaction (recorded under
// UsageKindCompact) but with the continuation-oriented handoffPrompt. The result
// is the handoff markdown body (without a title); the caller persists it as an
// artifact and seeds a fresh session with it.
func BuildHandoff(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, existingSummary, rendered string, env HandoffEnv) (string, error) {
	existing := strings.TrimSpace(existingSummary)
	if existing == "" {
		existing = "(none)"
	}
	if strings.TrimSpace(rendered) == "" {
		return "", fmt.Errorf("cannot build handoff from an empty transcript")
	}
	resp, err := provider.Complete(ctx, providers.Request{
		Model:     agent.Model,
		MaxTokens: compactMaxOutputTokens,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: fmt.Sprintf(handoffPrompt, existing, rendered, env.rendered())},
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
