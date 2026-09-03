package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// chatTurn carries the state one chat turn threads through its phases. It exists
// so runChatTurn can read as an ordered sequence of phases instead of one long
// body; every phase keeps the exact control flow it had inline (each returns a
// "keep going" bool where the inline code used an early return).
//
// ctx is deliberately a field, not a parameter: the inline code assigned to a
// single turn-scoped ctx variable from inside the per-agent loop (compaction
// prompt, attachment root, claude home, context overhead, PreCompact and
// native-compact seams), so those extensions outlive the agent that installed
// them and are seen by the next agent and the next stop pass. Threading ctx as a
// value parameter would silently drop that.
type chatTurn struct {
	s           *Server
	wsp         *workspace.Workspace
	database    *db.DB
	req         chatReq
	run         *chatRun
	runID       string
	clientGone  context.Context
	clientMsgID string
	// sse is the legacy SSE sink (run.emit); a no-op when no writer is installed
	// (queue worker path). All UI rides the hub regardless.
	sse func(event string, data any)

	ctx     context.Context
	session db.Session
	agents  []db.Agent

	firstTurn bool
	// freshSession is captured before the user message is appended: primes
	// cross-session context on a fresh session's first turn.
	freshSession bool
	// started flips once the user message is persisted (the turn is committed);
	// emitted flips once a terminal event was published by the success path.
	started    bool
	emitted    bool
	inflightID string
}

func (t *chatTurn) clearInflight() {
	t.withGeneration(func() {
		_ = t.database.ClearInflightExpected(t.req.SessionID, t.runID, t.inflightID, t.run.generation)
	})
}

// withGeneration is the single fence for run-owned durable, hub and live-sink
// mutations. Holding this session's gate closes the check/write race with a newer
// detached run taking ownership without serializing unrelated sessions.
func (t *chatTurn) withGeneration(write func()) bool {
	return t.s.runs.withCurrent(t.run, write)
}

func (t *chatTurn) emitLifecycleSteps(steps []agent.TurnStep) {
	for _, st := range steps {
		step := st
		t.withGeneration(func() { t.sse("step", step) })
	}
}

func (t *chatTurn) persistBlockedPrompt(steps []agent.TurnStep, reason string) bool {
	var persistErr error
	current := t.withGeneration(func() {
		blockMsg, err := t.database.AddMessage(t.ctx, db.Message{
			ID:        uuid.NewString(),
			SessionID: t.session.ID,
			Role:      providers.RoleAssistant,
			AgentID:   t.agents[0].ID,
			Text:      reason,
			Steps:     marshalSteps(steps),
		})
		if err != nil {
			persistErr = err
			return
		}
		t.sse("reply", map[string]any{"replyMessage": blockMsg})
		t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindReply, blockMsg, false)
		terminal := map[string]any{"sessionTitle": strings.TrimSpace(t.session.Title), "clientMsgId": t.clientMsgID}
		t.sse("done", terminal)
		t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindTurnDone, terminal, false)
		t.s.hub.Commit(t.wsp.ID, t.session.ID)
		t.emitted = true
	})
	if current && persistErr != nil {
		t.failTurn(t.agents[0].ID, "hook_block_persist", persistErr.Error())
	}
	return current
}

// failPreflight reports a pre-flight failure (before the turn commits) to the
// legacy SSE sink AND the hub, so every window clears its "thinking" state and
// shows why.
func (t *chatTurn) failPreflight(reason, detail string) {
	t.s.logger.Error("chat turn preflight failed", "session", t.req.SessionID, "reason", reason, "detail", detail)
	payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": t.clientMsgID}
	write := func() {
		t.run.emit("error", payload)
		t.s.publishHub(t.wsp.ID, t.req.SessionID, sessionhub.KindTurnError, payload, false)
		t.s.hub.Commit(t.wsp.ID, t.req.SessionID)
	}
	if t.run.generation == 0 {
		write()
		return
	}
	t.withGeneration(write)
}

// preflight loads the session, claims the per-session turn slot, resolves the
// responding agents and persists the incoming user message. The returned release
// func is the turn slot's (nil when the caller already holds it) and must be
// deferred by the caller even when ok is false — the slot is claimed before the
// later failure paths. ok is false when the turn must not continue.
func (t *chatTurn) preflight() (release func(), ok bool) {
	session, err := t.database.GetSession(t.ctx, t.req.SessionID)
	if err != nil {
		t.failPreflight("session_not_found", "session not found")
		return nil, false
	}
	// Every session runs at most ONE turn at a time: claim the per-session turn slot
	// (blocking until any in-flight turn finishes) so this turn never overlaps a
	// concurrent direct /chat/stream call, a queued inbox turn, a scheduler wake, a
	// peer delivery, or (for a coordinator) an auto turn. Worker notifications
	// arriving mid-turn coalesce and trigger one auto turn on release. Claiming for
	// ALL sessions — not just coordinators — is what closes the plain-session
	// concurrent-turn race (_Docs/58).
	//
	// The send-queue worker claims the slot BEFORE it pops this message (so it stays
	// visible in the queue tray while it waits) and hands ownership over via
	// turnSlotHeld; claiming again here would deadlock the turn behind itself.
	if !t.req.turnSlotHeld {
		release = t.wsp.Runtime.BeginSessionUserTurn(session.ID)
		// From the claim above until this function returns, the slot is covered by
		// NOBODY's defer: the caller only arms its own `defer release()` once
		// preflight returns. A panic in the rest of this body would therefore leak
		// the slot permanently — runTurnGuarded recovers, so the process survives
		// and every later turn on this session blocks forever. Release it here on
		// the panic path ONLY (recover() is nil on every normal return, so the
		// caller's defer stays the single, unchanged release point) and re-panic so
		// the failure is still reported.
		defer func() {
			if r := recover(); r != nil {
				release()
				panic(r)
			}
		}()
	}
	// Generation ownership starts only after the session slot is truly ours.
	// A registered run waiting above cannot invalidate the current slot owner.
	t.s.runs.activate(t.run)
	t.firstTurn = t.s.isFirstUntitledTurn(session)
	t.freshSession = session.MessageCount == 0

	// Resolve the ordered list of responding agents (default → session agent).
	agents := t.s.resolveTurnAgents(t.ctx, t.database, session, t.req.AgentIDs)
	if len(agents) == 0 {
		t.failPreflight("agent_not_found", "agent not found")
		return release, false
	}
	// A fresh session opened by @mentioning an agent adopts it as the main agent.
	session = t.s.adoptMentionedAgent(t.ctx, t.database, session, t.req.AgentIDs, agents)
	t.session = session
	t.agents = agents
	// Label the run with the responding agent's provider so the Session Info panel
	// can show which kind of background process is running (e.g. "claude-cli").
	t.run.setProvider(agents[0].Provider)
	// Record whether "Yönlendir" (mid-turn steer) can actually reach this turn, so
	// the control endpoint can tell the client to queue instead of silently
	// dropping it. claude-cli delivers a steer only at a permission-prompt tool
	// boundary, which exists solely in "ask"/"read-only" modes; "auto" runs the CLI
	// with --dangerously-skip-permissions (no such boundary). Native providers drain
	// the steer channel in the tool loop regardless of mode. Effective mode = the
	// request override when set, else the agent's own mode.
	effMode := agents[0].PermissionMode
	if t.req.PermissionMode != "" {
		effMode = t.req.PermissionMode
	}
	t.run.setSteerable(steerableForTurn(agents[0].Provider, effMode))

	// Persist the incoming user message once. Stamp the routed recipient agent
	// (agents[0]) so a multi-agent thread's history can show which agent each
	// question was directed at — the "@name" in the text is only informational and
	// does not route. Harmless in a 1:1 session (labelling only kicks in with 2+
	// agents).
	var userMsg db.Message
	if !t.withGeneration(func() {
		userMsg, err = t.database.AddMessage(t.ctx, db.Message{
			SessionID:   session.ID,
			Role:        providers.RoleUser,
			AgentID:     agents[0].ID,
			Text:        t.req.Message,
			Attachments: t.req.Attachments,
		})
	}) {
		return release, false
	}
	if err != nil {
		t.failPreflight("persist_error", err.Error())
		return release, false
	}
	// The turn is now committed (user message persisted); arm the terminal-event
	// guard so a later failure still notifies the frontend.
	t.started = true
	// Every file attached to a chat turn becomes a session artifact (origin chat).
	t.s.captureAttachmentArtifacts(t.ctx, t.database, session.ID, agents[0].ID, t.req.Attachments)

	t.withGeneration(func() {
		t.sse("meta", map[string]any{"userMessage": userMsg, "runId": t.runID})
		// Put the user message onto the session hub so EVERY window watching this
		// session (not just the one that submitted) renders it live, in order.
		t.s.publishHub(t.wsp.ID, session.ID, sessionhub.KindUserMessage, userMsg, false)
	})
	return release, true
}

// wireInteractive installs the interactive prompts (ask_user, multi-question
// ask, permission approvals) and the session-scoped grants onto the turn context.
func (t *chatTurn) wireInteractive() {
	// Wire the interactive asker: the ask_user tool emits a transient "ask" step
	// and blocks here until the client POSTs an answer (or the turn is stopped).
	// The tool loop runs in this same goroutine, so emitting via sse is safe.
	t.ctx = tools.WithAsker(t.ctx, func(ctx context.Context, question string, options []string) (string, error) {
		// Resolve-once interaction on the session hub: EVERY window renders the card
		// and the first to answer wins (CAS). clientGone still aborts a detached turn
		// whose user navigated away so the goroutine never leaks.
		pi := t.s.openInteraction(t.wsp.ID, t.session.ID, "ask", map[string]any{"question": question, "options": options})
		return t.s.waitInteraction(ctx, t.clientGone, pi)
	})

	// Multi-question asker: ask_user with several questions emits ONE interaction
	// carrying all of them; every window renders a combined form and any POSTs a JSON
	// array of answers, which FormatMultiAnswer folds into a single labeled block.
	t.ctx = tools.WithMultiAsker(t.ctx, func(ctx context.Context, questions []tools.AskQuestion) (string, error) {
		pi := t.s.openInteraction(t.wsp.ID, t.session.ID, "ask", map[string]any{"questions": questions})
		ans, err := t.s.waitInteraction(ctx, t.clientGone, pi)
		if err != nil {
			return "", err
		}
		return tools.FormatMultiAnswer(questions, ans), nil
	})

	// Session-scoped permission grants + the approval prompter for write/exec
	// tools under "ask" mode. The prompter emits a dedicated StepPermission card
	// and blocks on the same answer channel as ask_user; "Always allow" is
	// recorded in the session grants so it isn't re-asked. Also wired onto the run
	// so the CLI permission-prompt tool shares the same grant set.
	grants := t.s.grants.forSession(t.wsp.ID, t.session.ID)
	t.run.setGrants(grants)
	t.ctx = tools.WithGrants(t.ctx, grants)
	// Session-scoped use_skill dedupe. The epoch is the session's rolling-summary
	// fold count: after a fold the earlier skill body is no longer in the window,
	// so the next load must serve the real text again. Wired onto the run as well
	// so the CLI bridge's use_skill shares the same ledger as the native tool.
	skillLedger := t.s.skillLedgers.forSession(t.wsp.ID, t.session.ID)
	t.run.setSkillLedger(skillLedger, t.session.CompactionCount)
	t.ctx = tools.WithSkillLedger(t.ctx, skillLedger, t.session.CompactionCount)
	t.ctx = tools.WithPermissionPrompter(t.ctx, func(ctx context.Context, tool, risk, arg string, options []string) (string, error) {
		pi := t.s.openInteraction(t.wsp.ID, t.session.ID, "permission", map[string]any{
			"tool": tool, "reason": risk, "text": arg, "options": options,
		})
		return t.s.waitInteraction(ctx, t.clientGone, pi)
	})
}

// runPromptLifecycle fires the once-per-turn lifecycle hooks (SessionStart on a
// fresh session, UserPromptSubmit always) and returns the context they injected.
// stop is true when the turn must end here — either a hook vetoed the prompt
// (handled, not a failure) or persisting that veto failed.
func (t *chatTurn) runPromptLifecycle() (lifecycleContext string, stop bool) {
	// Lifecycle hooks (Claude Code parity), fired once per user turn BEFORE any
	// agent runs. SessionStart primes a fresh session; UserPromptSubmit sees the
	// submitted prompt and may inject context (e.g. a caveman "respond terse"
	// ruleset) or BLOCK the turn entirely. Injected context is folded into every
	// responding agent's dynamic system prompt below. Fail-open by construction.
	if t.freshSession {
		ss := t.wsp.Runtime.RunLifecycleHooks(t.ctx, t.session.ID, db.HookSessionStart, agent.LifecycleExtras{Source: "startup"})
		t.emitLifecycleSteps(ss.Steps)
		lifecycleContext = ss.Context
	}
	ups := t.wsp.Runtime.RunLifecycleHooks(t.ctx, t.session.ID, db.HookUserPromptSubmit, agent.LifecycleExtras{Prompt: t.req.Message})
	t.emitLifecycleSteps(ups.Steps)
	if c := strings.TrimSpace(ups.Context); c != "" {
		lifecycleContext = strings.TrimSpace(lifecycleContext + "\n\n" + c)
	}
	if !ups.Block {
		return lifecycleContext, false
	}
	// A UserPromptSubmit hook vetoed this prompt: persist the reason as the
	// assistant reply (so it survives reload) and end the turn without calling
	// any model. Not a failure — an intentional, hook-driven stop.
	reason := strings.TrimSpace(ups.Reason)
	if reason == "" {
		reason = "Prompt bir hook tarafından engellendi."
	}
	t.persistBlockedPrompt(ups.Steps, reason)
	return lifecycleContext, true
}

// runStopPasses drives the agents to an answer, re-running the whole pass when a
// Stop hook blocks. Returns false when the turn ended early (a failure inside a
// pass already reported itself).
func (t *chatTurn) runStopPasses(lifecycleContext string) bool {
	// Stop-hook continuation loop (Claude Code parity): after the agents answer, a
	// Stop hook may block to force another pass (e.g. "you forgot to run the tests").
	// stopContinue carries the block reason into the next pass as injected context;
	// bounded by maxStopPasses, and StopHookActive lets the hook detect it is already
	// in a forced continuation and relent.
	const maxStopPasses = 3
	stopContinue := ""
	for stopPass := 0; ; stopPass++ {
		passContext := lifecycleContext
		if stopContinue != "" {
			passContext = strings.TrimSpace(lifecycleContext + "\n\n[A Stop hook asked you to keep working before ending the turn]\n" + stopContinue)
		}

		// Each agent answers in turn, re-reading the (growing) history so later
		// agents see the earlier replies.
		for i, agentRow := range t.agents {
			if !t.runAgentPass(i, agentRow, passContext) {
				return false
			}
		}

		// Stop lifecycle hook (Claude Code parity): the main agent(s) finished this
		// pass. The hook may inject audit context and — via decision:"block" — force
		// one more pass with the reason as guidance (StopHookActive marks a forced
		// continuation so the hook can relent; maxStopPasses is the hard backstop).
		stop := t.wsp.Runtime.RunLifecycleHooks(t.ctx, t.session.ID, db.HookStop,
			agent.LifecycleExtras{StopHookActive: stopPass > 0})
		for _, st := range stop.Steps {
			step := st
			t.withGeneration(func() { t.sse("step", step) })
		}
		if !stop.Block || stopPass >= maxStopPasses {
			break
		}
		stopContinue = stop.Reason
	}
	return true
}

// prepareAgentRuntime applies the per-turn overrides onto this pass's local agent
// copy and resolves the provider it will run on. ok is false when the turn was
// already failed and must return.
func (t *chatTurn) prepareAgentRuntime(agentRow *db.Agent) (providers.Provider, bool) {
	// Per-turn reasoning override (local copy only — never persisted). Here
	// "" keeps its own distinct meaning — "no override, use the agent's own
	// level" — but anything else must be a tier this agent's model actually
	// supports, exactly as on the agent write path. Silently running the turn
	// at the stored level would hide that the request had no effect.
	if t.req.ThinkingLevel != "" {
		if terr := providers.ValidateThinkingLevelForProvider(agentRow.Provider, agentRow.Model, t.req.ThinkingLevel); terr != nil {
			t.failTurn(agentRow.ID, "invalid_thinking_level", terr.Error())
			return nil, false
		}
		agentRow.ThinkingLevel = t.req.ThinkingLevel
	}
	if t.req.PermissionMode != "" {
		agentRow.PermissionMode = t.req.PermissionMode
	}
	provider, perr := t.s.providers.Get(agentRow.ProviderRef())
	if perr != nil {
		t.failTurn(agentRow.ID, "provider_unavailable", perr.Error())
		return nil, false
	}
	// Pin this workspace's claude-home before Prepare's rolling compaction,
	// which folds via a direct provider.Complete (summarizeRendered) that
	// bypasses guardedComplete. Without this the claude-cli provider falls
	// back to the global claude-home and fails auth even when the workspace
	// is logged in (mirrors the manual /compact path in summary.go).
	if herr := t.wsp.Runtime.PinCLIHome(provider); herr != nil {
		t.failTurn(agentRow.ID, "provider_unavailable", herr.Error())
		return nil, false
	}
	return provider, true
}

// agentTurnPrep is everything prepareAgentRequest resolved for one agent pass.
type agentTurnPrep struct {
	llmReq     providers.Request
	leadSteps  []agent.TurnStep
	resumePlan cliResumePlan
	// rawHistory is the un-annotated message list (stable indices), the basis for
	// the claude-cli resume delta and the CLI compaction boundary.
	rawHistory []db.Message
}

// prepareAgentRequest reads the history, refreshes the session, folds the context
// if needed and composes the provider request for one agent pass. ok is false when
// the turn was already failed and must return.
func (t *chatTurn) prepareAgentRequest(agentRow db.Agent, provider providers.Provider, passContext string) (agentTurnPrep, bool) {
	var out agentTurnPrep
	history, herr := t.database.ListMessages(t.ctx, t.session.ID)
	if herr != nil {
		t.failTurn(agentRow.ID, "history_error", herr.Error())
		return out, false
	}
	// Re-read the session so this pass sees the freshest metadata (title, CLI
	// resume ids a previous agent wrote). A failure here means the session no
	// longer exists (deleted mid-turn, store swapped): abort the turn instead of
	// continuing with the zero value, whose empty ID would persist this reply
	// under no session and publish to a hub scope nobody watches.
	refreshed, serr := t.database.GetSession(t.ctx, t.session.ID)
	if serr != nil {
		t.failTurn(agentRow.ID, "session_not_found", serr.Error())
		return out, false
	}
	t.session = refreshed
	// Raw (un-annotated) message list — the basis for the claude-cli resume delta
	// (stable indices, unlike the annotated history below). Includes this turn's
	// just-added user message.
	rawHistory := history
	// Annotate the history with each assistant turn's author so this agent can
	// tell who said what in a thread shared by several agents (no-op for a
	// single-agent session). multiAgent gates the explanatory system note.
	history, multiAgent := t.s.labelMultiAgentHistory(t.ctx, t.database, agentRow.ID, history)
	// Recap of recent turns' tool I/O (the trace is otherwise dropped when
	// history → provider messages). Rendered as a volatile dynamic block —
	// NOT folded into the history — so the history messages stay byte-stable
	// for the rolling prompt-cache breakpoint.
	toolRecap := recentToolActivityBlock(history)
	// The user's 👍/👎 on earlier turns. Same volatile-block treatment as the
	// tool recap (never folded into the history): a rating can be added, flipped
	// or cleared at any moment, and rewriting a cached history message for it
	// would bust the rolling prompt-cache breakpoint.
	feedbackRecap := recentFeedbackBlock(history)
	// Carry this workspace's editable compaction prompt onto the turn context.
	t.ctx = t.wsp.Runtime.FoldContext(t.ctx, agentRow)
	t.ctx = conversation.WithAttachmentRoot(t.ctx, t.wsp.SandboxRoot())
	t.ctx = conversation.WithClaudeHome(t.ctx, t.wsp.Runtime.ClaudeHomeDir())
	// Budget the fold against the TRUE per-turn footprint: the non-message
	// context (static prefix + tool/skill catalogs + eager schemas + artifacts)
	// ships every turn but Prepare only folds messages, so without this a large
	// static prefix keeps the message-only estimate under budget while the real
	// footprint runs over — the fold never fires (the 127%-but-never-compacted
	// coordinator case). systemFillers is the same basis the context meter uses.
	overhead, stepBase, oerr := t.s.contextOverheadTokens(t.ctx, t.wsp, t.session, history, multiAgent)
	if oerr != nil {
		t.failTurn(agentRow.ID, "context_overhead_failed", "context overhead: "+oerr.Error())
		return out, false
	}
	t.ctx = conversation.WithContextOverhead(t.ctx, overhead)
	t.ctx = conversation.WithContextOverheadStepBase(t.ctx, stepBase)
	// PreCompact lifecycle hook (Claude Code parity): Prepare invokes this just
	// before it folds older turns into the rolling summary. Fire-and-forget audit.
	t.ctx = conversation.WithPreCompact(t.ctx, func(trigger string) {
		pc := t.wsp.Runtime.RunLifecycleHooks(t.ctx, t.session.ID, db.HookPreCompact, agent.LifecycleExtras{Trigger: trigger})
		for _, st := range pc.Steps {
			step := st
			t.withGeneration(func() { t.sse("step", step) })
		}
	})
	// Native-compaction seam: under autoCompactMode native/auto the gate asks
	// the CLI to compact its own window before folding history ourselves. The
	// error travels back unwrapped — conversation treats any non-nil result as
	// "not compacted" and falls back to the rolling fold.
	t.ctx = conversation.WithNativeCompact(t.ctx, func(ctx context.Context) (conversation.NativeCompactResult, error) {
		result, err := t.s.runNativeCompact(ctx, t.wsp, t.session, rawHistory, nativeCompactAuto)
		return conversation.NativeCompactResult{SuccessDebugPersisted: result.SuccessDebugPersisted}, err
	})
	prep, cerr := t.s.convo.Prepare(t.ctx, t.database, provider, t.session, agentRow, history)
	if cerr != nil {
		t.failTurn(agentRow.ID, "compaction_failed", "compaction failed: "+cerr.Error())
		return out, false
	}
	if prep.Compacted {
		t.wsp.Runtime.DropWarmCLISession(t.session.ID)
	}
	if prep.NativeCompacted {
		t.session, cerr = t.database.GetSession(t.ctx, t.session.ID)
		if cerr != nil {
			t.failTurn(agentRow.ID, "session_reload_failed", "reload session after native compaction: "+cerr.Error())
			return out, false
		}
	}

	llmReq := t.s.composeTurnRequest(t.ctx, t.wsp, t.session, agentRow, t.agents, t.req.Message, prep, t.freshSession, multiAgent, toolRecap, feedbackRecap, passContext)
	// Prompt-epoch drift step: if the static context changed since the frozen
	// snapshot, surface a context_change step at the head of the turn (once per
	// drift episode). Emitted live and prepended to the persisted trace so the
	// chat history shows when the change landed. The agent already read the diff
	// via the dynamic-suffix note composeTurnRequest injected.
	leadSteps := consumeContextChangeLead(t.wsp.Runtime, t.session.ID, agentRow.ID)
	// Auto-compaction visibility: Prepare folds older history into the rolling
	// summary silently, inside this already-serialized turn (it holds the inbox
	// slot, so it cannot and must not re-enter the send-queue like the manual
	// /compact command — that would self-deadlock on the serial slot). What it
	// lacked was on-screen presence. Surface it as a lead step on the SAME hub
	// channel the manual command uses (live SSE + cross-window publish + persisted
	// trace via leadSteps below) so the fold shows up like any other turn event.
	if prep.Compacted {
		leadSteps = append([]agent.TurnStep{compactionLeadStep(prep.Fold, provider)}, leadSteps...)
	}
	// The mirror case: the turn was over budget, the fold was attempted, and its
	// summarizer call failed. Prepare no longer kills the turn for that (one
	// transient 429 on the fold provider used to destroy the user's turn), so the
	// turn continues with an UNCOMPACTED context. That has to be on screen — an
	// oversized context that nobody announced is the failure this step exists to
	// prevent.
	if prep.FoldFailed {
		leadSteps = append([]agent.TurnStep{foldFailedLeadStep(prep.FoldError)}, leadSteps...)
	}
	for _, st := range leadSteps {
		step := st
		t.withGeneration(func() {
			t.sse("step", step)
			t.wsp.Runtime.EmitSessionStep(t.session.ID, step)
			t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindStep, step, false)
		})
	}
	// Safe CLI resume: Claude retains its existing opt-in/persistent-session
	// gates; Codex additionally requires one participant, the default persona
	// and a durable scoped home. A fold always leaves the compacted request intact
	// and starts fresh from TionHarness summary + recent tail.
	resumePlan := t.s.planCLIResume(t.ctx, provider, len(t.agents), t.session, agentRow, rawHistory, prep.Compacted, &llmReq)
	if resumePlan.retireNativeRecovery {
		if rerr := t.database.RetireSessionCLINativeCompactionRecovery(t.ctx, t.session.ID); rerr != nil {
			t.failTurn(agentRow.ID, "persist_error", "retire CLI native compaction recovery: "+rerr.Error())
			return out, false
		}
		resumePlan.nativeCompactionRecovery = false
		resumePlan.retireNativeRecovery = false
		t.session.CLISessionID = ""
		t.session.CLISentMsgCount = 0
		t.session.CLICompactMsgCount = 0
		t.session.CLINativeCompactionPending = false
	}

	out.llmReq = llmReq
	out.leadSteps = leadSteps
	out.resumePlan = resumePlan
	out.rawHistory = rawHistory
	return out, true
}

// installAgentSinks binds every per-agent sink (artifacts, notify, navigate,
// session, todos, wake, skills, shell, spawn, delegation, the self-management
// bridge and the Interaction MCP endpoint) onto both tool paths — the native loop
// via the returned context, the CLI via the run — and returns the per-agent tool
// context.
func (t *chatTurn) installAgentSinks(agentRow db.Agent) context.Context {
	session := t.session
	// Attach a per-agent artifact sink so create_artifact / update_artifact
	// persist content stamped with this session + agent — both on the native
	// tool path (via context) and the CLI path (via the run, used by the
	// Interaction MCP backend).
	sink := newArtifactSink(t.database, session.ID, agentRow.ID, t.wsp.Runtime.Emit)
	t.run.setArtifacts(sink)
	turnCtx := tools.WithArtifacts(t.ctx, sink)
	// Attach a per-agent notify sink so the notify tool can raise a desktop
	// notification on both tool paths (native via context, CLI via the run, used
	// by the Interaction MCP backend). It publishes an "agent" event onto the
	// workspace bus → SSE → OS toast (per device prefs).
	nsink := newNotifySink(session.ID, agentRow.ID, t.wsp.Runtime.Emit)
	// Notification lifecycle hook (Claude Code parity): fire for every notify call
	// so an audit/relay hook (e.g. forward to Slack) sees agent notifications.
	nsink.onNotify = func(spec tools.NotifySpec) {
		t.wsp.Runtime.RunLifecycleHooks(turnCtx, session.ID, db.HookNotification,
			agent.LifecycleExtras{Message: strings.TrimSpace(spec.Title + " " + spec.Body)})
	}
	t.run.setNotify(nsink)
	turnCtx = tools.WithNotify(turnCtx, nsink)
	// focus_view shares the same sink (it implements NavigateSink too): an agent
	// can drive the UI to a view/entity on both tool paths.
	t.run.setNav(nsink)
	turnCtx = tools.WithNavigate(turnCtx, nsink)
	// Session sink: one sink for every session-scoped mutation — title, working
	// dir, tags, archive — on both tool
	// tools and the session-edit tools alike; each mutation emits a "session"
	// event so open windows refresh live.
	ssink := t.wsp.Runtime.NewSessionSink(session.ID)
	t.run.setSession(ssink)
	turnCtx = tools.WithSession(turnCtx, ssink)
	// Persistent progress: bind a todo sink so todo_write persists the checklist
	// to the project's progress file on both tool paths (native via context, CLI
	// via the run). Keyed to this session's working dir. Gated by ProgressPersist.
	if t.s.tun.ProgressPersist() {
		todoSink := t.wsp.Runtime.NewTodoSink(session.ID, agentRow.ID)
		t.run.setTodoSink(todoSink)
		turnCtx = tools.WithTodoSink(turnCtx, todoSink)
	}
	// Stamp the session id so the runtime can resolve this session's WorkingDir
	// (cwd) for the fs/shell sandbox and provider cwd.
	turnCtx = agent.WithSessionID(turnCtx, session.ID)

	// Wire the self-wake scheduler for THIS agent + session, on both tool paths:
	// the native built-in reads it from the context; the CLI path reaches it via
	// the run (used by the Interaction MCP schedule_wake dispatch). A wake arms a
	// one-shot schedule that re-delivers a prompt into this session as a fresh
	// turn, so the conversation continues on its own.
	respondingID := agentRow.ID
	wakeFn := func(wctx context.Context, delaySeconds int, prompt, reason string) (string, error) {
		return t.wsp.Runtime.ScheduleWake(wctx, session.ID, respondingID, prompt, reason, delaySeconds)
	}
	turnCtx = tools.WithWakeScheduler(turnCtx, wakeFn)
	t.run.setWakeScheduler(wakeFn)

	// use_skill (CLI path): mirror the native built-in for claude-cli agents,
	// which reach TionHarness skills only through the Interaction MCP bridge. The
	// loader enforces the same per-agent allowlist as the native use_skill tool,
	// so a restricted skill stays unreachable unless assigned/shared.
	skillAgent := agentRow
	t.run.setSkillLoader(func(slug string) (string, error) {
		return t.wsp.Runtime.LoadSkillForAgent(skillAgent, slug)
	})
	// skill_search (CLI path): same per-agent allowlist; lets a claude-cli agent
	// discover on-demand/conditional skills not in its appended catalog. (SK-2)
	t.run.setSkillSearcher(func(query string, limit int) []tools.SkillHit {
		return t.wsp.Runtime.SearchSkillsForAgent(skillAgent, query, limit)
	})
	// SK-3 (CLI path): loading a skill auto-grants its declared allowed-tools.
	t.run.setSkillAllowed(func(slug string) []string {
		return t.wsp.Runtime.SkillAllowedToolsForAgent(skillAgent, slug)
	})

	// shell (CLI path): mirror the native built-in for claude-cli agents, which
	// reach TionHarness tools only through the Interaction MCP bridge. When shell is
	// enabled, install a sandboxed runner so the bridge's shell dispatch runs
	// commands through TionHarness's PowerShell shell — letting the CLI's own POSIX
	// Bash be safely disallowed. nil when shell is off (then Bash stays allowed).
	t.run.setShellRunner(t.wsp.Runtime.NewShellRunner())

	// Spawn (CLI path): mirror the native built-in for claude-cli agents, which
	// reach TionHarness tools only through the Interaction MCP bridge. Install a
	// per-agent spawn tool on the run so the bridge's spawn_session dispatch can
	// launch independent sessions. Self-management is always on now; a fresh
	// instance per turn resets the per-turn spawn budget.
	t.run.setSpawnTool(tools.NewSpawnSessionTool(respondingID, t.s.tun.SpawnMaxPerTurn(),
		func(sctx context.Context, target, prompt, modelOverride string) (tools.SpawnResult, error) {
			// Inherit this chat session's working directory, so a spawn from a
			// session pinned to repo A does not silently open in the workspace
			// default directory.
			res, err := t.wsp.Runtime.SpawnSession(sctx, target, prompt, agent.SpawnOptions{
				ModelOverride: modelOverride,
				CreatedBy:     respondingID,
				WorkingDir:    t.wsp.Runtime.SessionWorkdir(session.ID),
			})
			return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName, Queued: res.Queued, QueuePosition: res.QueuePosition}, err
		}))

	// run_subagent (CLI path): mirror the native delegation built-in so a
	// claude-cli agent can hand a self-contained sub-task to another agent and
	// get the answer back IN THIS turn (unlike spawn_session's fire-and-forget
	// into a separate session). nil when delegation is off. Bound to this agent
	// so depth / cycle / per-turn budget guards apply.
	t.run.setRunAgent(t.wsp.Runtime.RunSubagentRunner(agentRow, t.run.autonomous))

	// Self-management bridge (CLI path, CLI-3): claude-cli has no native
	// activate_tools loop, so advertise the responding agent's lazy
	// self-management tools up front through the Interaction MCP and dispatch
	// them through the same registry the native loop uses. Empty when
	// self-manage is off. Re-point the CLI MCP endpoint for THIS agent turn so
	// its allowlist carries the static interaction tools + the bridged tools.
	bridgeDefs, bridgeCall := t.wsp.Runtime.BridgeTools(turnCtx, agentRow)
	t.run.setBridge(bridgeDefs, bridgeCall)
	// Visibility-aware CLI wire split: full→core (eager), summary/name-only→
	// extended (deferred), hidden→neither. Installed on the run so tools/list
	// (Tools) classifies identically to the allowlist built here.
	visOf := t.wsp.Runtime.ToolVisibilityFunc(turnCtx, agentRow)
	t.run.setTierVis(visOf)
	// Effective tool filter (workspace DisabledTools + agent denylist), installed
	// so the Interaction bridge's tools/list and the allowlist drop tools the
	// native ToolCatalog would (e.g. a workspace disabling PowerShell to force Bash).
	allowOf := t.wsp.Runtime.ToolAllowedFunc(turnCtx, agentRow)
	t.run.setToolAllowed(allowOf)
	if url := t.s.interactionURL(); url != "" {
		names := filterAllowedNames(interactionAdvertisedNames(t.s.tun, false), allowOf)
		coreNames, extNames := splitInteractionTiers(names, bridgeDefs, visOf)
		// Stable per-(session,agent) Bearer token (not run.token): keeps the CLI
		// mcp-config byte-identical across turns so a persistent process stays warm
		// (Doc 52 §3-D). bindActive resolves it to this in-flight run.
		tok := t.s.runs.interactionToken(t.wsp.ID, t.req.SessionID, agentRow.ID)
		t.s.runs.bindActive(tok, t.run)
		turnCtx = tools.WithInteractionEndpoint(turnCtx, url, tok, coreNames, extNames)
	}
	return turnCtx
}

// streamSink builds the per-step callback for one agent pass: it fans each step
// out to the legacy SSE sink, the process-wide bus and the session hub, and
// accumulates the partial answer + persistable trace for the crash sidecar.
func (t *chatTurn) streamSink(turnCtx context.Context, partial *strings.Builder, kept *[]agent.TurnStep, snapshot func()) func(agent.TurnStep) {
	sessionID := t.session.ID
	return func(st agent.TurnStep) {
		t.withGeneration(func() {
			agent.ObserveActivityStep(turnCtx, st)
			t.sse("step", st)
			// Mirror the step onto the process-wide bus so OTHER windows viewing
			// this session render it live too. The originating window ignores the
			// bus copy (it owns the run and streams over its own per-request SSE);
			// the emitter drops high-frequency/interactive kinds itself.
			t.wsp.Runtime.EmitSessionStep(sessionID, st)
			switch st.Kind {
			case agent.StepDelta:
				partial.WriteString(st.Text)
				// Ephemeral token growth: best-effort live, seq 0, not retained.
				t.s.publishHub(t.wsp.ID, sessionID, sessionhub.KindDelta, st, true)
			case agent.StepToolDelta:
				t.s.publishHub(t.wsp.ID, sessionID, sessionhub.KindToolDelta, st, true)
			case agent.StepTombstone:
				t.s.publishHub(t.wsp.ID, sessionID, sessionhub.KindTombstone, st, true)
			case agent.StepAsk, agent.StepPermission, agent.StepPlan:
				// Interactive prompts are handled by the Phase 2 interaction CAS
				// (interaction_open/resolved), not broadcast as plain hub steps —
				// otherwise a passive window would show a card it cannot resolve.
			default:
				if st.Running || st.Append {
					break
				}
				*kept = append(*kept, st)
				// Durable activity (thinking/tool/todo/diff/recovery/error/…) →
				// seq'd on the hub so every window renders it live and a reconnect
				// gap-fills it from the ring.
				t.s.publishHub(t.wsp.ID, sessionID, sessionhub.KindStep, st, false)
			}
			snapshot()
		})
	}
}

// runAgentPass runs one agent's answer end to end: overrides + provider, request
// preparation, sink wiring, the streamed completion and whichever persistence
// path the outcome takes. Returns false when the turn ended early.
func (t *chatTurn) runAgentPass(i int, agentRow db.Agent, passContext string) bool {
	provider, ok := t.prepareAgentRuntime(&agentRow)
	if !ok {
		return false
	}

	t.withGeneration(func() {
		t.sse("agent", map[string]any{"agentId": agentRow.ID, "index": i})
		// Mirror agent-start onto the hub so late-joining windows know which
		// agent is answering (multi-agent threads render each turn's author).
		t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindAgentStart, map[string]any{"agentId": agentRow.ID, "index": i}, false)
	})

	prep, ok := t.prepareAgentRequest(agentRow, provider, passContext)
	if !ok {
		return false
	}
	turnCtx := t.installAgentSinks(agentRow)

	agentStart := time.Now()
	// Pre-allocate the reply id so the streaming crash sidecar and the final
	// persisted message share one identity (recovery is then idempotent).
	replyID := uuid.NewString()
	t.inflightID = replyID
	// Tag every debug event emitted during this turn with the reply id, so the
	// per-message debug panel can fetch exactly this message's spend/latency.
	turnCtx = agent.WithTurnID(turnCtx, replyID)
	// Durable Ask (MVP): opt this interactive turn into durable ask_user
	// suspend/resume. At a clean suspend point the native loop returns an
	// *askSuspend sentinel instead of blocking the asker; the intercept below
	// parks it to disk + opens a durable card. The blocking asker (wired above)
	// still handles non-clean asks (parallel/PTC) as the fallback.
	turnCtx = agent.WithDurableAsk(turnCtx)
	// Snapshot the in-flight reply to disk on a throttle: the partial answer
	// text (accumulated from streaming deltas) plus the persistable trace so
	// far. A mid-turn process death leaves this sidecar for boot to reclaim.
	var partial strings.Builder
	var kept []agent.TurnStep
	var lastSnap time.Time
	snapshot := func() {
		if time.Since(lastSnap) < 600*time.Millisecond {
			return
		}
		lastSnap = time.Now()
		// streamSink invokes snapshots while holding the generation fence.
		_ = t.database.WriteInflight(db.InflightTurn{
			MessageID:  replyID,
			RunID:      t.runID,
			Generation: t.run.generation,
			SessionID:  t.session.ID,
			AgentID:    agentRow.ID,
			StartedAt:  agentStart.Unix(),
			Text:       partial.String(),
			Steps:      marshalSteps(kept),
		})
	}
	resp, steps, cerr := t.wsp.Runtime.CompleteWithToolsStream(turnCtx, agentRow, provider, prep.llmReq, false,
		t.streamSink(turnCtx, &partial, &kept, snapshot))
	// Prompt-cache break card: the completion's usage revealed that this turn
	// re-paid the whole cached prefix because the model or the prompt/tool
	// schemas changed. Appended to the lead so it sits at the head of the
	// persisted trace (that is where the cold prefix was paid) on every exit
	// path below — success, provider error and durable-ask suspend alike.
	leadSteps := append(prep.leadSteps, consumeCacheBreakLead(t.wsp.Runtime, t.session.ID)...)
	// Durable Ask suspend: the native loop parked at a clean ask_user point.
	// This is NOT an error — persist the suspend snapshot (lead + loop trace),
	// open a durable card keyed to the ask id, clear the crash sidecar (the wait
	// is now durable, not a mid-turn orphan), and return the goroutine cleanly.
	// The answer endpoint re-drives the turn via ResumeAsk.
	if cerr != nil && !errors.Is(context.Cause(t.ctx), agent.ErrTurnHardTimeout) && !errors.Is(context.Cause(t.ctx), agent.ErrTurnIdleTimeout) {
		snapSteps := append(append([]agent.TurnStep{}, leadSteps...), steps...)
		if ask, suspended, perr := t.wsp.Runtime.SuspendAskFromError(context.WithoutCancel(t.ctx), agentRow, t.session.ID, prep.llmReq, snapSteps, cerr); suspended {
			if perr != nil {
				t.s.logger.Error("durable ask: persist suspend failed", "session", t.session.ID, "error", perr)
			} else {
				t.withGeneration(func() {
					_ = t.database.ClearInflightExpected(t.req.SessionID, t.runID, t.inflightID, t.run.generation)
					t.s.openDurableAskCard(t.wsp.ID, t.session.ID, ask, false)
					t.s.hub.Commit(t.wsp.ID, t.session.ID)
					t.s.logger.Info("durable ask: turn suspended", "session", t.session.ID, "ask", ask.ID)
					t.sse("ask_suspended", map[string]any{"askId": ask.ID})
				})
				return false
			}
		}
	}
	if cerr != nil {
		t.persistInterruptedTurn(agentRow, replyID, agentStart, leadSteps, kept, partial.String(), cerr)
		return false
	}

	return t.persistAgentReply(agentRow, prep, replyID, agentStart, leadSteps, steps, resp)
}

// persistInterruptedTurn saves whatever the agent produced before the stream
// failed (or the user stopped it) as the assistant reply, then publishes the
// terminal error to every window.
func (t *chatTurn) persistInterruptedTurn(agentRow db.Agent, replyID string, agentStart time.Time, leadSteps, kept []agent.TurnStep, partial string, cerr error) {
	t.withGeneration(func() {
		// Distinguish a manual Stop (run.cancel cancelled ctx) from a genuine
		// provider failure. Preserve the partial trace accumulated so far.
		cause := context.Cause(t.ctx)
		stopped := errors.Is(cause, context.Canceled)
		detail, reason := chatTurnFailure(cause, cerr)
		switch reason {
		case reasonTurnHardTimeout:
			t.s.logger.Info("chat turn hit hard timeout", "session", t.session.ID, "agent", agentRow.ID)
		case reasonTurnIdleTimeout:
			t.s.logger.Info("chat turn stalled: no step within the inactivity window",
				"session", t.session.ID, "agent", agentRow.ID, "idle", t.s.chatTurnIdle().String())
		case reasonStopped:
			t.s.logger.Info("chat turn stopped by user", "session", t.session.ID, "agent", agentRow.ID)
		default:
			t.s.logger.Error("stream completion failed", "error", cerr, "agent", agentRow.ID)
		}
		trace := append(append(leadSteps, kept...), agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason})
		persistCtx := context.WithoutCancel(t.ctx)
		payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": t.clientMsgID}
		if msg, aerr := t.database.AddMessage(persistCtx, db.Message{
			ID:        replyID,
			SessionID: t.session.ID,
			Role:      providers.RoleAssistant,
			AgentID:   agentRow.ID,
			Text:      partial,
			Steps:     marshalSteps(trace),
			Cancelled: stopped, Interrupted: !stopped,
			DurationMs: time.Since(agentStart).Milliseconds(),
		}); aerr != nil {
			t.s.logger.Error("persist interrupted turn failed", "session", t.session.ID, "error", aerr)
		} else {
			payload["replyMessage"] = msg
			t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindReply, msg, false)
			t.s.hub.Commit(t.wsp.ID, t.session.ID)
		}
		_ = t.database.ClearInflightExpected(t.req.SessionID, t.runID, t.inflightID, t.run.generation)
		t.wsp.Runtime.AutoTagTurn(context.WithoutCancel(t.ctx), t.session.ID, trace, reason)
		t.sse("error", payload)
		t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindTurnError, payload, false)
		t.s.hub.Commit(t.wsp.ID, t.session.ID)
	})
}

// persistAgentReply saves the completed answer, carries the CLI resume/compaction
// state forward and publishes the reply. Returns false when persisting failed and
// the turn must end.
func (t *chatTurn) persistAgentReply(agentRow db.Agent, prep agentTurnPrep, replyID string, agentStart time.Time, leadSteps, steps []agent.TurnStep, resp *providers.Response) bool {
	boundary, compacted := cliCompactionBoundary(steps, len(prep.rawHistory))
	state := db.CLIReplyState{
		UpdateResume:          prep.resumePlan.active && resp.SessionID != "",
		ResumeSessionID:       resp.SessionID,
		ResumeSentMsgCount:    prep.resumePlan.sentCount + 1,
		UpdateCompactBoundary: compacted,
		CompactMsgCount:       boundary,
	}
	if prep.resumePlan.nativeCompactionRecovery {
		state.ClearNativeCompactionPending = true
		if !state.UpdateResume {
			state.RetireResume = true
			state.ResumeSentMsgCount = 0
		}
	}
	reply := db.Message{
		ID:         replyID,
		SessionID:  t.session.ID,
		AgentID:    agentRow.ID,
		Role:       providers.RoleAssistant,
		Text:       resp.Text,
		Steps:      marshalSteps(append(leadSteps, steps...)),
		Model:      resp.Model,
		StopReason: resp.StopReason,
		Usage:      messageUsage(resp.Usage),
		DurationMs: time.Since(agentStart).Milliseconds(),
		// Flag the boundary where the underlying CLI conversation restarted,
		// so the transcript can draw a divider above this turn (TSK514).
		CLIColdStart: prep.resumePlan.active && prep.resumePlan.coldStart,
	}
	var replyMsg db.Message
	var failureReason, failureDetail string
	current := t.withGeneration(func() {
		var aerr error
		if state.UpdateResume || state.UpdateCompactBoundary || state.ClearNativeCompactionPending {
			replyMsg, aerr = t.database.AddMessageWithCLIState(t.ctx, reply, state)
		} else {
			replyMsg, aerr = t.database.AddMessage(t.ctx, reply)
		}
		if aerr != nil {
			failureReason, failureDetail = "persist_error", aerr.Error()
			return
		}
		// The CLI state above was committed atomically with the reply. The remaining
		// block only reports a model mismatch; it performs no second store mutation.
		if state.UpdateResume && resp.Model != "" && resp.Model != agentRow.Model {
			t.s.logger.Info("cli-resume-model-mismatch",
				"session", t.session.ID, "agent", agentRow.ID,
				"agent_model", agentRow.Model, "cli_model", resp.Model)
		}
		// P1.1: update the session header's model snapshot when the actual
		// response model differs — keeps the header's O(1) answer current.
		if resp.Model != "" && resp.Model != t.session.Model {
			if merr := t.database.SetSessionModel(t.ctx, t.session.ID, resp.Model); merr != nil {
				t.s.logger.Warn("set session model failed", "session", t.session.ID, "error", merr)
			}
		}
		// Reply is durable now; drop this agent's sidecar before the next agent
		// (the top-level defer is the catch-all for early-return paths).
		_ = t.database.ClearInflightExpected(t.req.SessionID, t.runID, t.inflightID, t.run.generation)
		t.sse("reply", map[string]any{"replyMessage": replyMsg})
		// Canonical reply onto the hub: every window replaces its live-accumulated
		// bubble with this persisted, authoritative message (steps + usage + model).
		t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindReply, replyMsg, false)
		// This agent's turn is now in the persisted transcript → a fresh
		// subscriber need not replay it (only the next agent's in-flight tail).
		t.s.hub.Commit(t.wsp.ID, t.session.ID)
	})
	if !current {
		return false
	}
	if failureReason != "" {
		t.failTurn(agentRow.ID, failureReason, failureDetail)
		return false
	}

	t.s.logger.Info("chat turn completed",
		"session", t.session.ID, "agent", agentRow.Name, "provider", agentRow.Provider,
		"model", resp.Model, "in", resp.Usage.InputTokens, "out", resp.Usage.OutputTokens,
		"steps", len(steps), "stream", true,
		"dur", time.Since(agentStart).Round(time.Millisecond).String())

	// Auto-tag: derive session tags from this turn (tool-error / error /
	// archived) so an automation can later scan + repair them.
	t.wsp.Runtime.AutoTagTurn(t.ctx, t.session.ID, steps, "")

	// Phantom-spawn guard: a coordinator driven by a plain user chat turn never
	// passes through runCoordinatorTurn, so until now it was the one coordinator
	// turn kind nothing checked — and it narrated spawns it never made
	// (WS27/SES90).
	t.s.guardCoordinatorChatTurn(t.ctx, t.database, t.wsp.Runtime, t.session.ID, agentRow, resp.Text, steps)

	// Tag-triggered automations: signal that this session finished a turn. The
	// runtime dispatches it detached, so a tagged session completing can spawn a
	// follow-up (the automation loop) without blocking this turn. Fired per
	// responding agent so a multi-agent turn's last reply carries the result.
	t.wsp.Runtime.FireTurnFinished(t.session.ID, agentRow.ID, resp.Text)
	return true
}

// finishTurn auto-titles the session and publishes the terminal success events.
func (t *chatTurn) finishTurn() {
	// Title generation may invoke a provider. Keep it outside every registry and
	// session generation lock, then fence its persist with terminal effects.
	titleCandidate := t.s.autoTitleCandidate(t.ctx, t.wsp, t.firstTurn, t.agents[0].ID, t.session.ID, t.req.Message)
	t.withGeneration(func() {
		sessionTitle := t.s.persistAutoTitle(t.ctx, t.wsp, t.session.ID, titleCandidate)

		t.sse("done", map[string]any{"sessionTitle": sessionTitle})
		// Terminal success onto the hub: every window stops its live indicator and
		// picks up the (possibly new) session title. clientMsgId lets a queue observer
		// (legacy /chat + /chat/stream) recognise its own turn's completion.
		t.s.publishHub(t.wsp.ID, t.session.ID, sessionhub.KindTurnDone, map[string]any{"sessionTitle": sessionTitle, "clientMsgId": t.clientMsgID}, false)
		t.s.hub.Commit(t.wsp.ID, t.session.ID)

		// Publish a chat-completion event so other workspaces can flag activity with
		// a badge when the user is viewing a different workspace. The frontend uses
		// chat events only for the badge (not a duplicate desktop notification).
		title := strings.TrimSpace(t.session.Title)
		if sessionTitle != "" {
			title = sessionTitle
		}
		if title == "" {
			title = "Sohbet"
		}
		t.emitted = true
		t.wsp.Runtime.Emit(events.Event{
			Type:   "chat",
			Level:  "success",
			Title:  "Yanıt hazır: " + title,
			Body:   t.req.Message,
			Target: map[string]string{"view": "chat", "sessionId": t.session.ID},
		})
	})
}
