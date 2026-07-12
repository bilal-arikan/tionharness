package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// btwSystemPrompt turns the agent into a read-only side consultant for ONE
// question. Three properties define the feature (_Docs/60-BTW-YAN-SOHBET.md):
//
//   - It sees the conversation, but the exchange is NEVER written back into it.
//   - It has NO tools: it cannot read files, run commands, or edit anything. It
//     answers purely from the context it was handed.
//   - It does not touch, continue, or comment on the main task in progress.
//
// The rules are stated in the system prompt AND repeated in the user turn
// because agentic CLI providers (claude-cli) carry a large base prompt that can
// otherwise drown out an appended system instruction — the same reason
// titler.go repeats its instruction.
const btwSystemPrompt = `You are answering a SIDE QUESTION ("btw") asked during an ongoing session.

Rules:
- You have NO tools. You cannot read files, run commands, search, or edit anything. Answer only from the conversation context you were given.
- Answer ONLY the side question. Do not continue, resume, comment on, or take over the main task in the conversation.
- This exchange is discarded: it is NOT added to the conversation history, so do not refer to it in the future and do not ask the user to remember it.
- Be direct and concise. If the context does not contain what you need, say so plainly instead of guessing or inventing a tool call.`

// btwUserPreamble repeats the contract inside the user turn (see above).
const btwUserPreamble = "Side question (btw) — answer from context only. You have no tools, and this exchange will NOT be added to the conversation history. Do not resume or comment on the main task; just answer the question.\n\nQuestion:\n"

// maxBtwQuestionRunes caps the side question so a stray paste cannot turn a
// cheap consultation into an expensive turn.
const maxBtwQuestionRunes = 8000

// BtwAnswer is the result of one side-chat consultation. Nothing here is
// persisted to the session: the caller returns it to the UI and drops it.
type BtwAnswer struct {
	Text  string
	Model string
	Usage providers.Usage
}

// AskBtw runs ONE tool-less, off-transcript consultation for the session.
//
// history is the SAME budgeted message list the main chat turn would send (so
// the side question sees the code the agent read and the decisions it made), and
// system/systemDynamic are that turn's composed context. The side-chat contract
// is enforced structurally, not by prompt alone:
//
//	Request.Tools is left nil       → the provider is given no tools at all.
//	Nothing is written to the DB    → the main history stays byte-identical, so
//	                                  the next real turn still hits the prompt cache.
//
// The question is appended as a final user message; the assistant reply is
// returned to the caller and never appended anywhere.
func (r *Runtime) AskBtw(ctx context.Context, agent db.Agent, sessionID, system, systemDynamic string, history []providers.Message, question string) (BtwAnswer, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return BtwAnswer{}, errors.New("empty btw question")
	}

	// KindBtw stamps the call as auxiliary: usage is metered to the session under
	// its own kind, and the cache-break probe ignores it (isConversationKind).
	// autonomous=false — this is user-initiated, so the autonomy brake must not
	// block it.
	req := buildBtwRequest(agent, system, systemDynamic, history, question)
	resp, err := r.guardedComplete(WithSessionID(WithCallKind(ctx, KindBtw), sessionID), agent, req, false)
	if err != nil {
		return BtwAnswer{}, err
	}
	return BtwAnswer{
		Text:  strings.TrimSpace(resp.Text),
		Model: resp.Model,
		Usage: resp.Usage,
	}, nil
}

// buildBtwRequest composes the side chat's provider request. Pure (no I/O), so the
// side-chat contract is unit-testable without a live provider.
//
// Two invariants live here and are covered by btw_test.go:
//   - Request.Tools stays nil — the side chat cannot call a tool, by construction
//     rather than by asking the model nicely not to.
//   - history is copied, never appended to in place — a Go append() into a caller's
//     slice with spare capacity would write the btw question into the backing array
//     the main conversation is about to reuse.
func buildBtwRequest(agent db.Agent, system, systemDynamic string, history []providers.Message, question string) providers.Request {
	// The side-chat instructions lead the dynamic (volatile) suffix rather than the
	// static prefix: the static prefix is the session's cached system prompt, and
	// rewriting it here would invalidate the very prompt cache the main conversation
	// depends on.
	dynamic := strings.TrimSpace(btwSystemPrompt + "\n\n" + strings.TrimSpace(systemDynamic))

	messages := make([]providers.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, providers.Message{
		Role: providers.RoleUser,
		Text: btwUserPreamble + truncateRunes(strings.TrimSpace(question), maxBtwQuestionRunes),
	})

	return providers.Request{
		// The agent's own model: unlike titling/summarizing, this answer is read by
		// the USER and must be as good as a normal reply. The saving comes from the
		// exchange never entering the history, not from a weaker model.
		Model:         agent.Model,
		System:        system,
		SystemDynamic: dynamic,
		Messages:      messages,
		// Tools: deliberately left nil — see the doc comment above.
	}
}
