package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// handleChatStream runs one chat turn over Server-Sent Events, emitting each
// activity step the moment it occurs. A turn may involve several agents (via
// "@mention" routing); each answers in order, seeing the prior replies. Events:
//
//	meta  → { userMessage, contextTokens }      (once)
//	agent → { agentId, index }                  (before each agent's steps)
//	step  → a single TurnStep                   (zero or more, current agent)
//	reply → { replyMessage }                    (after each agent finishes)
//	done  → { sessionTitle }                    (terminal, success)
//	error → { error }                           (terminal, failure)
//
// Pre-flight failures use a normal JSON error; once streaming begins, errors
// are delivered as an `error` event.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[chatReq](w, r)
	if !ok {
		return
	}
	if req.SessionID == "" || (strings.TrimSpace(req.Message) == "" && len(req.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "sessionId and message (or attachments) are required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Register this turn so it can be stopped or steered while running. The
	// cancelable context ends the stream on "stop"; the steer channel feeds live
	// guidance into the tool loop.
	runID := uuid.NewString()
	// Detach the turn from the client connection. A page refresh or navigation
	// aborts the SSE fetch; if generation were tied to the request context it
	// would cancel mid-turn and the assistant reply would never be persisted —
	// so after reload the answer is gone and the message block "vanishes". By
	// detaching, generation runs to completion and persists regardless; only an
	// explicit "stop" control cancels it. The original request context is kept
	// as clientGone so an interactive ask_user (which needs a live client) does
	// not block forever once the user has navigated away.
	clientGone := r.Context()
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	run := s.runs.register(runID, req.SessionID, cancel)
	defer s.runs.unregister(runID)
	ctx = agent.WithSteer(ctx, run.steer)
	// Point any CLI subprocess (claude-cli, ...) at the in-process Interaction MCP
	// endpoint for this turn, carrying the per-run token so its ask_user/todo_write
	// calls correlate back here. No-op when the base URL is unknown.
	if url := s.interactionURL(); url != "" {
		// Provisional endpoint (pre-turn, no responding agent resolved yet): use the
		// static split. It is overwritten below per agent turn with the visibility-
		// aware split once agentRow + its registry are known.
		coreNames, extNames := splitInteractionTiers(interactionAdvertisedNames(s.tun, false), nil, nil)
		ctx = tools.WithInteractionEndpoint(ctx, url, run.token, coreNames, extNames)
	}

	wsp := ws(r)
	database := wsp.DB
	// Drop any crash-recovery sidecar when the turn returns by any normal path
	// (success, handled failure, client abort): only a true mid-turn process
	// death must leave it behind for the next boot to reclaim.
	defer database.ClearInflight(req.SessionID)

	// Ensure a terminal chat event fires even when generation fails after the
	// turn has begun: the frontend uses it to clear the post-reload "thinking"
	// indicator and reload the transcript. The success path sets emitted=true
	// and publishes its own richer event below.
	turnStarted := false
	emitted := false
	defer func() {
		if !turnStarted || emitted {
			return
		}
		wsp.Runtime.Emit(events.Event{
			Type:   "chat",
			Level:  "error",
			Title:  "Sohbet turu sonlandı",
			Target: map[string]string{"view": "chat", "sessionId": req.SessionID},
		})
	}()

	session, err := database.GetSession(ctx, req.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// A coordinator session runs at most ONE turn at a time: claim the turn slot
	// (blocking until any in-flight auto turn finishes) so this interactive turn
	// never overlaps an auto-triggered coordinator turn. Worker notifications
	// arriving mid-turn coalesce and trigger one auto turn on release.
	if session.Role == "coordinator" {
		release := wsp.Runtime.BeginCoordinatorUserTurn(session.ID)
		defer release()
	}
	firstTurn := s.isFirstUntitledTurn(session)
	// Captured before the user message is appended: primes cross-session context
	// on a fresh session's first turn.
	freshSession := session.MessageCount == 0

	// Resolve the ordered list of responding agents (default → session agent).
	agents := s.resolveTurnAgents(ctx, database, session, req.AgentIDs)
	if len(agents) == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	// A fresh session opened by @mentioning an agent adopts it as the main agent.
	session = s.adoptMentionedAgent(ctx, database, session, req.AgentIDs, agents)
	// Label the run with the responding agent's provider so the Session Info panel
	// can show which kind of background process is running (e.g. "claude-cli").
	run.setProvider(agents[0].Provider)

	// Persist the incoming user message once. Stamp the routed recipient agent
	// (agents[0]) so a multi-agent thread's history can show which agent each
	// question was directed at — the "@name" in the text is only informational and
	// does not route. Harmless in a 1:1 session (labelling only kicks in with 2+
	// agents).
	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID:   session.ID,
		Role:        providers.RoleUser,
		AgentID:     agents[0].ID,
		Text:        req.Message,
		Attachments: req.Attachments,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The turn is now committed (user message persisted); arm the terminal-event
	// guard so a later failure still notifies the frontend.
	turnStarted = true
	// Every file attached to a chat turn becomes a session artifact (origin chat).
	s.captureAttachmentArtifacts(ctx, database, session.ID, agents[0].ID, req.Attachments)

	// Begin the event stream.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Route every SSE write through the run's mutex-guarded writer so the stream
	// handler goroutine and the Interaction MCP handler goroutine (which emits
	// ask/todo steps for the CLI path) never race on the ResponseWriter.
	run.setWrite(func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	})
	defer run.clearWrite()
	sse := run.emit

	sse("meta", map[string]any{"userMessage": userMsg, "runId": runID})

	// Wire the interactive asker: the ask_user tool emits a transient "ask" step
	// and blocks here until the client POSTs an answer (or the turn is stopped).
	// The tool loop runs in this same goroutine, so emitting via sse is safe.
	ctx = tools.WithAsker(ctx, func(ctx context.Context, question string, options []string) (string, error) {
		sse("step", agent.TurnStep{Kind: agent.StepAsk, Text: question, Options: options})
		select {
		case ans := <-run.answer:
			return ans, nil
		case <-clientGone.Done():
			// The user navigated away; no one can answer. Surface an error so the
			// model proceeds on its own instead of blocking the detached turn
			// forever (and leaking this goroutine).
			return "", clientGone.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// Multi-question asker: ask_user with several questions emits ONE step carrying
	// all of them; the client renders a combined form and POSTs a JSON array of
	// answers, which FormatMultiAnswer folds into a single labeled block for the model.
	ctx = tools.WithMultiAsker(ctx, func(ctx context.Context, questions []tools.AskQuestion) (string, error) {
		sse("step", agent.TurnStep{Kind: agent.StepAsk, Questions: questions})
		select {
		case ans := <-run.answer:
			return tools.FormatMultiAnswer(questions, ans), nil
		case <-clientGone.Done():
			return "", clientGone.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// Session-scoped permission grants + the approval prompter for write/exec
	// tools under "ask" mode. The prompter emits a dedicated StepPermission card
	// and blocks on the same answer channel as ask_user; "Always allow" is
	// recorded in the session grants so it isn't re-asked. Also wired onto the run
	// so the CLI permission-prompt tool shares the same grant set.
	grants := s.grants.forSession(session.ID)
	run.setGrants(grants)
	ctx = tools.WithGrants(ctx, grants)
	ctx = tools.WithPermissionPrompter(ctx, func(ctx context.Context, tool, risk, arg string, options []string) (string, error) {
		sse("step", agent.TurnStep{Kind: agent.StepPermission, Tool: tool, Reason: risk, Text: arg, Options: options})
		select {
		case ans := <-run.answer:
			return ans, nil
		case <-clientGone.Done():
			return "", clientGone.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// Lifecycle hooks (Claude Code parity), fired once per user turn BEFORE any
	// agent runs. SessionStart primes a fresh session; UserPromptSubmit sees the
	// submitted prompt and may inject context (e.g. a caveman "respond terse"
	// ruleset) or BLOCK the turn entirely. Injected context is folded into every
	// responding agent's dynamic system prompt below. Fail-open by construction.
	var lifecycleContext string
	if freshSession {
		ss := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookSessionStart, agent.LifecycleExtras{Source: "startup"})
		for _, st := range ss.Steps {
			sse("step", st)
		}
		lifecycleContext = ss.Context
	}
	ups := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookUserPromptSubmit, agent.LifecycleExtras{Prompt: req.Message})
	for _, st := range ups.Steps {
		sse("step", st)
	}
	if c := strings.TrimSpace(ups.Context); c != "" {
		lifecycleContext = strings.TrimSpace(lifecycleContext + "\n\n" + c)
	}
	if ups.Block {
		// A UserPromptSubmit hook vetoed this prompt: persist the reason as the
		// assistant reply (so it survives reload) and end the turn without calling
		// any model. Not a failure — an intentional, hook-driven stop.
		reason := strings.TrimSpace(ups.Reason)
		if reason == "" {
			reason = "Prompt bir hook tarafından engellendi."
		}
		blockMsg, berr := database.AddMessage(ctx, db.Message{
			ID:        uuid.NewString(),
			SessionID: session.ID,
			Role:      providers.RoleAssistant,
			AgentID:   agents[0].ID,
			Text:      reason,
			Steps:     marshalSteps(ups.Steps),
		})
		if berr != nil {
			s.failTurn(ctx, database, sse, session.ID, agents[0].ID, "hook_block_persist", berr.Error())
			return
		}
		sse("reply", map[string]any{"replyMessage": blockMsg})
		sse("done", map[string]any{"sessionTitle": strings.TrimSpace(session.Title)})
		emitted = true
		return
	}

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
		for i, agentRow := range agents {
			// Per-turn reasoning override (local copy only — never persisted).
			if req.ThinkingLevel != "" {
				agentRow.ThinkingLevel = req.ThinkingLevel
			}
			if req.PermissionMode != "" {
				agentRow.PermissionMode = req.PermissionMode
			}
			provider, perr := s.providers.Get(agentRow.Provider)
			if perr != nil {
				s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "provider_unavailable", perr.Error())
				return
			}

			sse("agent", map[string]any{"agentId": agentRow.ID, "index": i})

			history, herr := database.ListMessages(ctx, session.ID)
			if herr != nil {
				s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "history_error", herr.Error())
				return
			}
			session, _ = database.GetSession(ctx, session.ID)
			// Raw (un-annotated) message list — the basis for the claude-cli resume delta
			// (stable indices, unlike the annotated history below). Includes this turn's
			// just-added user message.
			rawHistory := history
			// Annotate the history with each assistant turn's author so this agent can
			// tell who said what in a thread shared by several agents (no-op for a
			// single-agent session). multiAgent gates the explanatory system note.
			history, multiAgent := s.labelMultiAgentHistory(ctx, database, agentRow.ID, history)
			// Recap of recent turns' tool I/O (the trace is otherwise dropped when
			// history → provider messages). Rendered as a volatile dynamic block —
			// NOT folded into the history — so the history messages stay byte-stable
			// for the rolling prompt-cache breakpoint.
			toolRecap := recentToolActivityBlock(history)
			// Carry this workspace's editable compaction prompt onto the turn context.
			ctx = conversation.WithCompactPrompt(ctx, wsp.Runtime.CompactPromptTemplate())
			// PreCompact lifecycle hook (Claude Code parity): Prepare invokes this just
			// before it folds older turns into the rolling summary. Fire-and-forget audit.
			ctx = conversation.WithPreCompact(ctx, func(trigger string) {
				pc := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookPreCompact, agent.LifecycleExtras{Trigger: trigger})
				for _, st := range pc.Steps {
					sse("step", st)
				}
			})
			prep, cerr := s.convo.Prepare(ctx, database, provider, session, agentRow, history)
			if cerr != nil {
				s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "compaction_failed", "compaction failed: "+cerr.Error())
				return
			}

			llmReq := s.composeTurnRequest(ctx, wsp, session, agentRow, agents, req.Message, prep, freshSession, multiAgent, toolRecap, passContext)
			// Prompt-epoch drift step: if the static context changed since the frozen
			// snapshot, surface a context_change step at the head of the turn (once per
			// drift episode). Emitted live and prepended to the persisted trace so the
			// chat history shows when the change landed. The agent already read the diff
			// via the dynamic-suffix note composeTurnRequest injected.
			leadSteps := consumeContextChangeLead(wsp.Runtime, session.ID, agentRow.ID)
			for _, st := range leadSteps {
				sse("step", st)
				wsp.Runtime.EmitSessionStep(session.ID, st)
			}
			// claude-cli session resume (opt-in): when engaged, this trims llmReq to the
			// unseen delta and sets ResumeSessionID so the CLI reuses its warm cache.
			resumePlan := s.planClaudeResume(provider, len(agents), session, rawHistory, &llmReq)

			// Attach a per-agent artifact sink so create_artifact / update_artifact
			// persist content stamped with this session + agent — both on the native
			// tool path (via context) and the CLI path (via the run, used by the
			// Interaction MCP backend).
			sink := newArtifactSink(database, session.ID, agentRow.ID, wsp.Runtime.Emit)
			run.setArtifacts(sink)
			turnCtx := tools.WithArtifacts(ctx, sink)
			// Attach a per-agent notify sink so the notify tool can raise a desktop
			// notification on both tool paths (native via context, CLI via the run, used
			// by the Interaction MCP backend). It publishes an "agent" event onto the
			// workspace bus → SSE → OS toast (per device prefs).
			nsink := newNotifySink(session.ID, agentRow.ID, wsp.Runtime.Emit)
			// Notification lifecycle hook (Claude Code parity): fire for every notify call
			// so an audit/relay hook (e.g. forward to Slack) sees agent notifications.
			nsink.onNotify = func(spec tools.NotifySpec) {
				wsp.Runtime.RunLifecycleHooks(turnCtx, session.ID, db.HookNotification,
					agent.LifecycleExtras{Message: strings.TrimSpace(spec.Title + " " + spec.Body)})
			}
			run.setNotify(nsink)
			turnCtx = tools.WithNotify(turnCtx, nsink)
			// focus_view shares the same sink (it implements NavigateSink too): an agent
			// can drive the UI to a view/entity on both tool paths.
			run.setNav(nsink)
			turnCtx = tools.WithNavigate(turnCtx, nsink)
			// Session sink: one sink for every session-scoped mutation — goal (the same
			// db.Session.Goal the user edits), title, working dir, archive — on both tool
			// paths. It is a SessionSink (superset of GoalSink), so it serves the goal
			// tools and the session-edit tools alike; each mutation emits a "session"
			// event so open windows refresh live.
			ssink := wsp.Runtime.NewSessionSink(session.ID)
			run.setSession(ssink)
			turnCtx = tools.WithSession(turnCtx, ssink)
			// Persistent progress: bind a todo sink so todo_write persists the checklist
			// to the project's progress file on both tool paths (native via context, CLI
			// via the run). Keyed to this session's working dir. Gated by ProgressPersist.
			if s.tun.ProgressPersist() {
				todoSink := wsp.Runtime.NewTodoSink(session.ID, agentRow.ID)
				run.setTodoSink(todoSink)
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
				return wsp.Runtime.ScheduleWake(wctx, session.ID, respondingID, prompt, reason, delaySeconds)
			}
			turnCtx = tools.WithWakeScheduler(turnCtx, wakeFn)
			run.setWakeScheduler(wakeFn)

			// use_skill (CLI path): mirror the native built-in for claude-cli agents,
			// which reach TionSwarm skills only through the Interaction MCP bridge. The
			// loader enforces the same per-agent allowlist as the native use_skill tool,
			// so a restricted skill stays unreachable unless assigned/shared.
			skillAgent := agentRow
			run.setSkillLoader(func(slug string) (string, error) {
				return wsp.Runtime.LoadSkillForAgent(skillAgent, slug)
			})
			// skill_search (CLI path): same per-agent allowlist; lets a claude-cli agent
			// discover on-demand/conditional skills not in its appended catalog. (SK-2)
			run.setSkillSearcher(func(query string, limit int) []tools.SkillHit {
				return wsp.Runtime.SearchSkillsForAgent(skillAgent, query, limit)
			})
			// SK-3 (CLI path): loading a skill auto-grants its declared allowed-tools.
			run.setSkillAllowed(func(slug string) []string {
				return wsp.Runtime.SkillAllowedToolsForAgent(skillAgent, slug)
			})

			// shell (CLI path): mirror the native built-in for claude-cli agents, which
			// reach TionSwarm tools only through the Interaction MCP bridge. When shell is
			// enabled, install a sandboxed runner so the bridge's shell dispatch runs
			// commands through TionSwarm's PowerShell shell — letting the CLI's own POSIX
			// Bash be safely disallowed. nil when shell is off (then Bash stays allowed).
			run.setShellRunner(wsp.Runtime.NewShellRunner())

			// Spawn (CLI path): mirror the native built-in for claude-cli agents, which
			// reach TionSwarm tools only through the Interaction MCP bridge. Install a
			// per-agent spawn tool on the run so the bridge's spawn_session dispatch can
			// launch independent sessions. Self-management is always on now; a fresh
			// instance per turn resets the per-turn spawn budget.
			run.setSpawnTool(tools.NewSpawnSessionTool(respondingID, s.tun.SpawnMaxPerTurn(),
				func(sctx context.Context, target, prompt, modelOverride string) (tools.SpawnResult, error) {
					res, err := wsp.Runtime.SpawnSession(sctx, target, prompt, agent.SpawnOptions{ModelOverride: modelOverride, CreatedBy: respondingID})
					return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName}, err
				}))

			// run_subagent (CLI path): mirror the native delegation built-in so a
			// claude-cli agent can hand a self-contained sub-task to another agent and
			// get the answer back IN THIS turn (unlike spawn_session's fire-and-forget
			// into a separate session). nil when delegation is off. Bound to this agent
			// so depth / cycle / per-turn budget guards apply.
			run.setRunAgent(wsp.Runtime.RunSubagentRunner(agentRow, run.autonomous))

			// Self-management bridge (CLI path, CLI-3): claude-cli has no native
			// activate_tools loop, so advertise the responding agent's lazy
			// self-management tools up front through the Interaction MCP and dispatch
			// them through the same registry the native loop uses. Empty when
			// self-manage is off. Re-point the CLI MCP endpoint for THIS agent turn so
			// its allowlist carries the static interaction tools + the bridged tools.
			bridgeDefs, bridgeCall := wsp.Runtime.BridgeTools(turnCtx, agentRow)
			run.setBridge(bridgeDefs, bridgeCall)
			// Visibility-aware CLI wire split: full→core (eager), summary/name-only→
			// extended (deferred), hidden→neither. Installed on the run so tools/list
			// (Tools) classifies identically to the allowlist built here.
			visOf := wsp.Runtime.ToolVisibilityFunc(turnCtx, agentRow)
			run.setTierVis(visOf)
			if url := s.interactionURL(); url != "" {
				coreNames, extNames := splitInteractionTiers(interactionAdvertisedNames(s.tun, false), bridgeDefs, visOf)
				// Stable per-(session,agent) Bearer token (not run.token): keeps the CLI
				// mcp-config byte-identical across turns so a persistent process stays warm
				// (Doc 52 §3-D). bindActive resolves it to this in-flight run.
				tok := s.runs.interactionToken(req.SessionID, agentRow.ID)
				s.runs.bindActive(tok, run)
				turnCtx = tools.WithInteractionEndpoint(turnCtx, url, tok, coreNames, extNames)
			}

			agentStart := time.Now()
			// Pre-allocate the reply id so the streaming crash sidecar and the final
			// persisted message share one identity (recovery is then idempotent).
			replyID := uuid.NewString()
			// Tag every debug event emitted during this turn with the reply id, so the
			// per-message debug panel can fetch exactly this message's spend/latency.
			turnCtx = agent.WithTurnID(turnCtx, replyID)
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
				_ = database.WriteInflight(db.InflightTurn{
					MessageID: replyID,
					SessionID: session.ID,
					AgentID:   agentRow.ID,
					StartedAt: agentStart.Unix(),
					Text:      partial.String(),
					Steps:     marshalSteps(kept),
				})
			}
			resp, steps, cerr := wsp.Runtime.CompleteWithToolsStream(turnCtx, agentRow, provider, llmReq, false,
				func(st agent.TurnStep) {
					sse("step", st)
					// Mirror the step onto the process-wide bus so OTHER windows viewing
					// this session render it live too. The originating window ignores the
					// bus copy (it owns the run and streams over its own per-request SSE);
					// the emitter drops high-frequency/interactive kinds itself.
					wsp.Runtime.EmitSessionStep(session.ID, st)
					switch st.Kind {
					case agent.StepDelta:
						partial.WriteString(st.Text)
					case agent.StepAsk, agent.StepToolDelta, agent.StepTombstone, agent.StepPermission:
						// Transient (live-UI only) — never part of the persisted trace.
					default:
						kept = append(kept, st)
					}
					snapshot()
				},
			)
			if cerr != nil {
				// Distinguish a manual Stop (run.cancel cancelled ctx) from a genuine
				// provider failure. Either way, PRESERVE the partial trace accumulated so
				// far (kept) so the tools/text the agent already produced stay visible
				// instead of vanishing — append an error/stopped step at the end.
				stopped := ctx.Err() != nil
				detail := "provider error: " + cerr.Error()
				reason := "provider_error"
				if stopped {
					detail = "Tur manuel olarak durduruldu. O ana kadarki adımlar korundu."
					reason = "stopped"
					s.logger.Info("chat turn stopped by user", "session", session.ID, "agent", agentRow.ID)
				} else {
					s.logger.Error("stream completion failed", "error", cerr, "agent", agentRow.ID)
				}
				trace := append(append(leadSteps, kept...), agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason})
				// Persist with a detached context so a cancelled (stopped) ctx still saves.
				persistCtx := context.WithoutCancel(ctx)
				payload := map[string]any{"error": detail, "reason": reason}
				if msg, aerr := database.AddMessage(persistCtx, db.Message{
					ID:        replyID,
					SessionID: session.ID,
					Role:      providers.RoleAssistant,
					AgentID:   agentRow.ID,
					Text:      partial.String(),
					Steps:     marshalSteps(trace),
					// A user Stop is Cancelled (clean, intentional); a provider failure
					// keeps Interrupted (the "cut off" banner) as before.
					Cancelled:   stopped,
					Interrupted: !stopped,
					DurationMs:  time.Since(agentStart).Milliseconds(),
				}); aerr != nil {
					s.logger.Error("persist interrupted turn failed", "session", session.ID, "error", aerr)
				} else {
					payload["replyMessage"] = msg
					// Keep any files written before the stop as artifacts.
					s.captureFileArtifacts(persistCtx, database, session.ID, agentRow.ID, trace)
				}
				_ = database.ClearInflight(session.ID)
				// Auto-tag the turn failure (skips a clean user "stopped"), plus any real
				// tool error captured before the failure.
				wsp.Runtime.AutoTagTurn(context.WithoutCancel(ctx), session.ID, trace, reason)
				sse("error", payload)
				return
			}

			replyMsg, aerr := database.AddMessage(ctx, db.Message{
				ID:         replyID,
				SessionID:  session.ID,
				Role:       providers.RoleAssistant,
				AgentID:    agentRow.ID,
				Text:       resp.Text,
				Steps:      marshalSteps(append(leadSteps, steps...)),
				Model:      resp.Model,
				StopReason: resp.StopReason,
				Usage:      messageUsage(resp.Usage),
				DurationMs: time.Since(agentStart).Milliseconds(),
			})
			if aerr != nil {
				s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "persist_error", aerr.Error())
				return
			}
			// Persist the rotated claude-cli session id so the NEXT turn resumes it and
			// sends only the new delta. sentCount+1 accounts for this turn's assistant
			// reply, which the CLI already holds server-side (no need to resend it).
			if resumePlan.active && resp.SessionID != "" {
				if rerr := database.SetSessionCLIResume(ctx, session.ID, resp.SessionID, resumePlan.sentCount+1); rerr != nil {
					s.logger.Warn("persist cli resume state failed", "session", session.ID, "error", rerr)
				}
			}
			// Reply is durable now; drop this agent's sidecar before the next agent
			// (the top-level defer is the catch-all for early-return paths).
			_ = database.ClearInflight(session.ID)
			// Auto-capture any files the agent wrote this turn as artifacts.
			s.captureFileArtifacts(ctx, database, session.ID, agentRow.ID, steps)
			sse("reply", map[string]any{"replyMessage": replyMsg})

			s.logger.Info("chat turn completed",
				"session", session.ID, "agent", agentRow.Name, "provider", agentRow.Provider,
				"model", resp.Model, "in", resp.Usage.InputTokens, "out", resp.Usage.OutputTokens,
				"steps", len(steps), "stream", true,
				"dur", time.Since(agentStart).Round(time.Millisecond).String())

			// Auto-tag: derive session tags from this turn (tool-error / error / goal /
			// archived) so an automation can later scan + repair them.
			wsp.Runtime.AutoTagTurn(ctx, session.ID, steps, "")

			// Tag-triggered automations: signal that this session finished a turn. The
			// runtime dispatches it detached, so a tagged session completing can spawn a
			// follow-up (the automation loop) without blocking this turn. Fired per
			// responding agent so a multi-agent turn's last reply carries the result.
			wsp.Runtime.FireTurnFinished(session.ID, agentRow.ID, resp.Text)
		}

		// Stop lifecycle hook (Claude Code parity): the main agent(s) finished this
		// pass. The hook may inject audit context and — via decision:"block" — force
		// one more pass with the reason as guidance (StopHookActive marks a forced
		// continuation so the hook can relent; maxStopPasses is the hard backstop).
		stop := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookStop,
			agent.LifecycleExtras{StopHookActive: stopPass > 0})
		for _, st := range stop.Steps {
			sse("step", st)
		}
		if !stop.Block || stopPass >= maxStopPasses {
			break
		}
		stopContinue = stop.Reason
	}

	// Auto-title once, after the turn, using the first responding agent.
	sessionTitle := s.maybeAutoTitle(ctx, wsp, firstTurn, agents[0].ID, session.ID, req.Message)

	sse("done", map[string]any{"sessionTitle": sessionTitle})

	// Publish a chat-completion event so other workspaces can flag activity with
	// a badge when the user is viewing a different workspace. The frontend uses
	// chat events only for the badge (not a duplicate desktop notification).
	title := strings.TrimSpace(session.Title)
	if sessionTitle != "" {
		title = sessionTitle
	}
	if title == "" {
		title = "Sohbet"
	}
	emitted = true
	wsp.Runtime.Emit(events.Event{
		Type:   "chat",
		Level:  "success",
		Title:  "Yanıt hazır: " + title,
		Body:   req.Message,
		Target: map[string]string{"view": "chat", "sessionId": session.ID},
	})
}

// resolveTurnAgents turns the requested agent ids (from "@mention" routing) into
// an ordered, de-duplicated list of existing agents. Falls back to the session's
// default agent when none are given or valid.
func (s *Server) resolveTurnAgents(ctx context.Context, database *db.DB, session db.Session, ids []string) []db.Agent {
	var out []db.Agent
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		if a, err := database.GetAgent(ctx, id); err == nil {
			seen[id] = true
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		if a, err := database.GetAgent(ctx, session.AgentID); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// failTurn records a turn-level failure as a persisted assistant message so the
// error shows up in the chat hierarchy (with its detail) and survives a reload,
// then notifies the client via the SSE `error` event carrying that message.
// agentID is the responding agent (may be "" if none was selected yet); reason
// is a stable machine tag (provider_error, compaction_failed, …) shown as a
// badge; detail is the human-readable message.
func (s *Server) failTurn(ctx context.Context, database *db.DB, sse func(string, any), sessionID, agentID, reason, detail string) {
	// Surface the failure in the server log too — without this a turn that dies
	// before producing output (provider unavailable, compaction failure, …) is
	// invisible server-side and only visible as a red card in the UI.
	s.logger.Error("turn failed", "session", sessionID, "agent", agentID, "reason", reason, "detail", detail)
	step := agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason}
	payload := map[string]any{"error": detail, "reason": reason}
	if msg, err := database.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Steps:     marshalSteps([]agent.TurnStep{step}),
	}); err != nil {
		s.logger.Error("persist turn error failed", "session", sessionID, "error", err)
	} else {
		payload["replyMessage"] = msg
	}
	sse("error", payload)
}
