package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// coordination_stall.go — judge-based protection against the coordinator "stall"
// (spawn-hallucination) freeze, see _Docs/47. A long-running coordinator can
// narrate a spawn it never issued ("Round 15 açıldı — 2 kol", "SES144 açıldı",
// "spawned 3 workers … [running]") while making NO spawn_worker call, then sit
// idle forever waiting for workers that were never created.
//
// The classification used to be a prose regex; it missed the freeze once a
// coordinator drifted to its own vocabulary (the WS17/SES101 incident). The regex
// is replaced by a cheap LLM judge, gated behind a purely deterministic condition
// (turn made no coordination tool call AND no worker is running) so the paid call
// only ever fires on a genuine idle turn. Two layers act on the verdict:
//
//  1. Turn-end guard (guardCoordinatorStall) — runs right after every coordinator
//     turn; on a positive verdict it injects a corrective note and re-arms one more
//     turn in the SAME drain batch. Bounded by the per-coordinator nudge cap.
//  2. Staleness sweeper (StartCoordinatorStallSweeper) — the long-horizon backstop
//     for a coordinator whose turn-end budget was spent or whose freeze was missed
//     (e.g. a restart between turns): it periodically judges any live coordinator
//     slot that has been silent past the staleness window with no running worker.
//  3. Hard-halt escalation (escalateCoordinatorStallHalt) — when either layer above
//     confirms the stall PERSISTS, it stops auto-turning the wedged coordinator
//     (slot.stallHalted) and posts a one-shot user notice, instead of nudging forever
//     or failing silent. Layers 1–2 still run. TWO independent counters trip it:
//     the CONSECUTIVE in-memory nudge budget (slot.spawnHallucStreak vs
//     CoordinatorStallMaxNudges), and the CUMULATIVE persisted tally
//     (Session.StallNudges vs CoordinatorStallHaltTotal). The second exists because
//     the first is blind to relapse: a coordinator that stalls, is nudged into one
//     real tool call (zeroing the streak), then stalls again never reaches the nudge
//     cap and would loop forever. The persisted tally also survives a restart, which
//     the streak does not. Any turn that genuinely calls a coordination tool clears
//     BOTH, so the cumulative tier measures relapses-without-recovery.
//
// The layers are gated asymmetrically around a freshly delivered worker result: the
// judge and the corrective nudge ALWAYS run (the turn right after a worker note is
// where phantom spawns concentrate — WS19/SES427), while the hard halt is suppressed
// for as long as the note is fresh (halting there was the WS24/SES34 false halt).

const (
	// DefaultCoordinatorStallSweepMin is the staleness window (minutes): the sweeper
	// only judges a coordinator that has been silent this long with no running
	// worker, so a coordinator mid-synthesis is never disturbed.
	DefaultCoordinatorStallSweepMin = 5
	// DefaultCoordinatorStallMaxNudges caps consecutive corrective nudges before the
	// runtime stops arguing with a wedged model. Past the cap, a coordinator STILL
	// judged to be phantom-spawning is hard-halted: auto-turns stop and the user is
	// notified (escalateCoordinatorStallHalt), rather than the runtime nudging forever.
	DefaultCoordinatorStallMaxNudges = 2
)

// coordStallSweepInterval is how often the sweeper scans live coordinator slots.
// Coarse on purpose: the staleness window (minutes) is the real gate.
const coordStallSweepInterval = 60 * time.Second

// coordStallNote is the corrective message injected on a positive stall verdict.
// Unambiguous: describing a spawn is not spawning it.
const coordStallNote = "<coordination-guard>\n" +
	"Your previous message described spawning workers (or reported them as running), but that turn made NO spawn_worker (and no list_workers) tool call — so NO worker session was actually created and none is running. Do NOT narrate delegation; describing a spawn is not spawning it.\n" +
	"If the plan still has work to delegate, CALL the spawn_worker tool for each worker in THIS turn (and use list_workers to check real status). If nothing remains to delegate, say so plainly and conclude.\n" +
	"</coordination-guard>"

// stallJudgeSystemPrompt instructs the cheap classifier. Terse and strict: it must
// return ONE JSON object so the reply parses deterministically. Language-agnostic
// by design — the whole point is to survive vocabulary drift a regex cannot.
const stallJudgeSystemPrompt = `You are auditing one turn of a multi-agent COORDINATOR.
You are told: the coordinator's latest message, and the fact that this turn made NO worker-spawn / coordination tool call and NO worker is currently running under it.
Decide: does the message CLAIM (in ANY language) to have just started, spawned, opened, or launched worker(s) / branches / sub-tasks — or report them as running / in-progress — when in reality none was started this turn?
- stalled=true if it narrates delegation as done or underway (e.g. "spawned 3 workers", "Round 5 opened - 2 arms", "SES144 acildi", "[running]").
- stalled=false if it plainly concludes, reports already-finished work, asks the user a question, or narrates only its own non-delegated actions.
Reply with STRICT JSON and nothing else: {"stalled": true} or {"stalled": false}.`

// guardCoordinatorStall runs after every coordinator turn (from runCoordinatorTurn).
// A real coordination tool call clears the nudge streak and returns. Otherwise, when
// the master toggle is on and no worker is running, it asks the judge whether the
// turn narrated a phantom spawn; on a true verdict it injects the corrective note and
// re-arms one more turn via slot.pending (read by drainCoordinator right after this
// returns). Bounded by CoordinatorStallMaxNudges. Fails safe on a judge error: it
// does NOT nudge, leaving the case to the sweeper, so a judge outage cannot spam
// re-arms.
func (r *Runtime) guardCoordinatorStall(coordSessionID, agentID string, agent db.Agent, text string, steps []TurnStep) {
	slot := r.coordSlotFor(coordSessionID)
	// Both entry points (the runtime's own coordinator loop and the user chat turn via
	// GuardCoordinatorStall) leave the slot in the same shape: flagged as a coordinator
	// (which relaxes the sweeper's hadWorkers gate) and stamped with this turn's time.
	// runCoordinatorTurn stamps the same value immediately before calling in, so this is
	// a no-op there; for a chat-driven coordinator it is the only stamp there is, and
	// without it the sweeper would never consider the session (lastTurnUnix==0).
	slot.mu.Lock()
	slot.coordinatorMode = true
	slot.lastTurnUnix = time.Now().Unix()
	slot.mu.Unlock()
	// A real coordination tool call this turn means the model is executing, not
	// narrating — clear any streak (and any prior hard-halt) and never correct.
	if turnCalledCoordinationTool(steps) {
		slot.mu.Lock()
		slot.spawnHallucStreak = 0
		slot.stallHalted = false
		slot.mu.Unlock()
		// The PERSISTED tally is cleared by the same recovery signal, so the cumulative
		// halt tier below measures relapses-without-recovery rather than a lifetime
		// total that only ever grows. Best-effort: a store error here must not deny the
		// coordinator its (successful) turn, and the tier stays conservative either way.
		r.clearStallTally(coordSessionID)
		return
	}
	if !r.tun.CoordinatorStallGuard() {
		return
	}
	// A worker actively running means the coordinator is legitimately waiting, not
	// stalled — never disturb it.
	if slot.workers.Load() > 0 {
		return
	}
	// A just-delivered worker result legitimately leaves zero running workers while
	// the coordinator digests the result and decides the next round. HALTING that turn
	// produced the WS24/SES34 false halt, so a fresh worker note still suppresses the
	// hard halt below. It must NOT suppress the judge: WS19/SES427 froze on exactly
	// this turn — right after a <task-notification>, the coordinator wrote "TSK103
	// handed to an independent validator" with an empty toolCalls list, and the blanket
	// exemption meant the guard never even looked. The turn that most often phantom-spawns
	// was the guard's blind spot. Judge + nudge now always run; only the escalation is
	// held back while the note is fresh. Fresh means the most recent inbound message is
	// that worker note; any newer user or runtime prompt supersedes it.
	freshWorkerNote := r.hasRecentWorkerNoteInbound(coordSessionID, time.Now())

	// The judge fires on any idle, no-worker turn — including one whose nudge budget is
	// already spent, because confirming the stall PERSISTS is what justifies the hard
	// halt below. Fails safe on a judge error: no nudge, no halt, leave it to the
	// sweeper, so a judge outage can neither spam re-arms nor wrongly halt.
	stalled, err := r.judgeCoordinatorStalled(context.Background(), agent, text)
	if err != nil {
		r.logger.Warn("coordination: stall judge failed; deferring to sweeper", "coordinator", coordSessionID, "error", err)
		return
	}
	if !stalled {
		return
	}

	// Confirmed phantom spawn. First detections inject a corrective nudge and re-arm one
	// more turn (bounded by CoordinatorStallMaxNudges). Once that budget is spent and the
	// coordinator is STILL stalling, escalate to a hard halt: stop auto-turning it and
	// tell the user why (FND-99caeb31), instead of silently deferring to the sweeper.
	slot.mu.Lock()
	spent := slot.spawnHallucStreak >= r.tun.CoordinatorStallMaxNudges()
	slot.mu.Unlock()
	if spent && !freshWorkerNote {
		r.escalateCoordinatorStallHalt(coordSessionID, agentID, agent, slot, "nudge budget spent")
		return
	}
	// This nudge is about to be counted; the cumulative tier reads the tally AFTER the
	// bump so the Nth stall is the one that halts (not the N+1st). The in-memory streak
	// alone cannot catch the relapse pattern this tier exists for: a coordinator that
	// stalls, gets nudged into one real call (zeroing the streak), then stalls again
	// would loop forever with the streak never reaching the nudge cap.
	total := r.injectStallNudge(coordSessionID, agentID, slot, true /* re-arm this batch */)
	// Neither halt tier may fire while a worker note is fresh (WS24/SES34): the nudge is
	// the whole correction here, and the coordinator keeps its turn to act on it. Once
	// the note ages out of the grace window a still-stalling coordinator is judged —
	// and halted — normally.
	if freshWorkerNote {
		return
	}
	if limit := r.tun.CoordinatorStallHaltTotal(); limit > 0 && total >= limit {
		r.escalateCoordinatorStallHalt(coordSessionID, agentID, agent, slot,
			fmt.Sprintf("cumulative stall threshold reached (%d/%d)", total, limit))
	}
}

// GuardCoordinatorStall is the exported turn-end phantom-spawn guard, for callers
// outside this package. The runtime's own coordinator loop reaches the guard directly
// (runCoordinatorTurn), so a coordinator driven by an ordinary USER chat turn was
// never checked at all: the WS27/SES90 session narrated two spawns from a kind="chat"
// turn with no tool call in its journal and nothing caught it. The API chat path calls
// this after the turn so both kinds of coordinator turn get identical protection.
//
// The caller decides eligibility: invoke it only for a session whose coordinator mode
// is on.
func (r *Runtime) GuardCoordinatorStall(coordSessionID, agentID string, agent db.Agent, text string, steps []TurnStep) {
	r.guardCoordinatorStall(coordSessionID, agentID, agent, text, steps)
}

// hasRecentWorkerNoteInbound reports whether the most recent inbound message is a
// worker RESULT delivered inside the grace window — the state in which a coordinator
// with no running worker is legitimately digesting rather than frozen.
//
// A standalone <coordination-status> message is not a result. A status appended to
// a <task-notification>, however, is still the last worker's real result and keeps
// the normal grace window. This distinction prevents the piggyback from turning the
// final result into an immediate false-positive spawn nudge.
func (r *Runtime) hasRecentWorkerNoteInbound(coordSessionID string, now time.Time) bool {
	var inbound db.Message
	err := r.db.StreamMessages(context.Background(), coordSessionID, func(msg db.Message) bool {
		// The guard's own corrective note is skipped: it is injected BECAUSE of the
		// state being measured here, so letting it count as the newest inbound would
		// make the first nudge silently cancel the halt suppression it just earned.
		if msg.Role == "user" && msg.Origin != "coordination-guard" {
			inbound = msg
		}
		return true
	})
	if err != nil || inbound.Origin != "worker-note" {
		return false
	}
	if strings.Contains(inbound.Text, "<coordination-status>") &&
		!strings.Contains(inbound.Text, "<task-notification>") {
		return false
	}
	age := now.Sub(time.Unix(inbound.CreatedAt, 0))
	return age >= 0 && age <= r.tun.CoordinatorWorkerNoteGrace()
}

// escalateCoordinatorStallHalt is the hard-halt escalation (FND-99caeb31): once a
// stall tier fires and the coordinator is STILL judged to be narrating phantom
// spawns, it marks the slot halted (the drain loop then stops re-arming and skips the
// idle-reconcile turn) and posts a SINGLE user-facing notice explaining why auto-turns
// stopped and how to resume. One-shot via slot.stallHalted so neither the turn-end
// guard nor the every-60s sweeper can spam the notice. The sweeper is intentionally
// left running as the long-horizon backstop — this is an added escalation layer, not a
// replacement. A later turn that actually calls a coordination tool clears the flag.
//
// Two tiers reach it: the consecutive nudge budget (slot.spawnHallucStreak vs
// CoordinatorStallMaxNudges) and the cumulative persisted tally (Session.StallNudges
// vs CoordinatorStallHaltTotal). `reason` names which one, so the log and the debug
// journal say what actually tripped rather than always claiming the nudge budget.
func (r *Runtime) escalateCoordinatorStallHalt(coordSessionID, agentID string, agent db.Agent, slot *coordSlot, reason string) {
	slot.mu.Lock()
	already := slot.stallHalted
	slot.stallHalted = true
	slot.pending = false
	streak := slot.spawnHallucStreak
	slot.mu.Unlock()
	if already {
		return
	}
	r.logger.Warn("coordination: phantom-spawn stall confirmed; halting coordinator auto-turns",
		"coordinator", coordSessionID, "streak", streak, "reason", reason)
	r.emitDebug(WithSessionID(context.Background(), coordSessionID), db.DebugEvent{
		Type:    db.DebugError,
		AgentID: agentID,
		Detail:  "coordinator stall halt (" + reason + "): auto-turns stopped",
		Err:     true,
	})
	r.publish(events.Event{
		Type:  events.TypeCoordination,
		Level: "warn",
		Title: "🧭 Koordinatör durduruldu — hayalet spawn",
		Body: "Koordinatör (" + agent.Name + ") worker başlattığını anlatıyor ama gerçek bir spawn_worker çağrısı yapmıyor; " +
			"düzeltici uyarılar sonuç vermedi. Otomatik koordinatör turları durduruldu. Worker bildirimleri hâlâ kaydediliyor. " +
			"Devam etmek için oturuma manuel bir mesaj gönderin (ör. \"spawn_worker'ı gerçekten çağır\").",
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
}

// CoordinatorStallHalted reports whether a coordinator session is currently in the
// hard-halt state (auto-turns stopped after a persistent phantom-spawn stall). Read
// from the in-memory slot WITHOUT creating one — a session that has never coordinated
// has no slot and is trivially not halted, so a plain read must not allocate a slot
// for every info-panel poll. The UI reads this to render a persistent "durduruldu"
// badge; after a process restart the slot is fresh (false) and the sweeper re-detects
// a still-frozen coordinator within its window, re-arming the badge.
func (r *Runtime) CoordinatorStallHalted(coordSessionID string) bool {
	v, ok := r.coordSlots.Load(coordSessionID)
	if !ok {
		return false
	}
	slot, _ := v.(*coordSlot)
	if slot == nil {
		return false
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return slot.stallHalted
}

// ResumeCoordinatorFromStall is the user-facing "Devam ettir" action behind the halt
// badge: it clears the hard-halt state and the nudge streak (a human is back in the
// loop, so the coordinator gets a genuine fresh budget) and kicks one coordinator
// turn. If the model stalls again the guard re-halts (one-shot notice), so the button
// can be pressed repeatedly without spamming. Errors if the session is not a live
// coordinator, so the caller surfaces a real failure instead of silently no-oping.
func (r *Runtime) ResumeCoordinatorFromStall(ctx context.Context, coordSessionID string) error {
	sess, err := r.db.GetSession(ctx, coordSessionID)
	if err != nil {
		return err
	}
	if !sess.IsCoordinator() {
		return fmt.Errorf("session %s is not a coordinator", coordSessionID)
	}
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	slot.stallHalted = false
	slot.spawnHallucStreak = 0
	slot.mu.Unlock()
	// The persisted tally is part of "a genuine fresh budget": leaving it at or above
	// the cumulative threshold would re-halt the coordinator on its very next stall,
	// making the button look broken. A human in the loop resets both counters.
	r.clearStallTally(coordSessionID)
	r.logger.Info("coordination: stall halt cleared by user; resuming coordinator", "coordinator", coordSessionID)
	r.enqueueCoordinatorTurn(coordSessionID)
	return nil
}

// clearStallTally resets the PERSISTED cumulative stall counter after a coordinator
// turn that actually drove workers. Best-effort by design: the caller is on the
// success path of a healthy turn, so a store failure is logged and ignored rather
// than propagated — the only consequence is that the cumulative tier stays armed a
// little longer, which errs toward halting a wedged coordinator, not past one.
func (r *Runtime) clearStallTally(coordSessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.db.SetSessionStallNudges(ctx, coordSessionID, 0); err != nil {
		r.logger.Warn("coordination: failed to clear stall counter", "coordinator", coordSessionID, "error", err)
	}
}

// injectStallNudge records the corrective note, bumps the nudge streak, and (when
// rearm) sets slot.pending so the coordinator gets one more turn to act on it. The
// sweeper passes rearm=false and separately kicks a fresh turn via
// enqueueCoordinatorTurn (the coordinator is idle, not mid-drain).
//
// Returns the new CUMULATIVE (persisted) stall count, which the cumulative halt tier
// compares against its threshold. Returns 0 when the counter could not be persisted —
// a value no threshold matches, so a store outage can never manufacture a halt.
func (r *Runtime) injectStallNudge(coordSessionID, agentID string, slot *coordSlot, rearm bool) int {
	slot.mu.Lock()
	slot.spawnHallucStreak++
	streak := slot.spawnHallucStreak
	if rearm {
		slot.pending = true
	}
	slot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := r.recordInjectedUserNote(ctx, coordSessionID, "coordination-guard", coordStallNote); err != nil {
		r.logger.Warn("coordination: failed to record stall note", "coordinator", coordSessionID, "error", err)
	}
	// The slot streak above is consecutive and in-memory: a clean coordination call
	// zeroes it and a restart loses it entirely. Persist a cumulative tally next to
	// StuckTurns so a later escalation tier — and a forensic pass over session.json —
	// can see how often this coordinator has phantom-spawned across its whole life.
	total, err := r.db.BumpSessionStallNudges(ctx, coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: failed to persist stall counter", "coordinator", coordSessionID, "error", err)
		total = 0 // unknown tally: report a value no threshold matches rather than a stale one
	}
	r.logger.Warn("coordination: coordinator narrated a spawn with no tool call; injected corrective note",
		"coordinator", coordSessionID, "streak", streak, "totalStalls", total)
	r.emitDebug(WithSessionID(context.Background(), coordSessionID), db.DebugEvent{
		Type:    db.DebugError,
		AgentID: agentID,
		Detail:  "coordinator stall: claimed workers with no spawn_worker/list_workers call",
		Err:     true,
	})
	return total
}

// judgeCoordinatorStalled asks a cheap model whether `text` claims a spawn that
// never happened. Synchronous (the turn-end caller re-arms on a true verdict, so the
// decision must be in hand before it returns), bounded by a short timeout. A cheaper
// model is preferred, same policy as lessons/summaries: the coordinator's own model.
func (r *Runtime) judgeCoordinatorStalled(ctx context.Context, agent db.Agent, text string) (bool, error) {
	if r.stallJudgeFn != nil {
		return r.stallJudgeFn(ctx, agent, text)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false, nil
	}
	model := agent.Model
	jctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := r.guardedComplete(WithCallKind(jctx, KindReflect), agent, providers.Request{
		Model:     model,
		System:    stallJudgeSystemPrompt,
		MaxTokens: 30,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: "Coordinator's latest message:\n\n" + truncateRunes(text, 4000)},
		},
	}, false)
	if err != nil {
		return false, err
	}
	return parseStallVerdict(resp.Text), nil
}

// parseStallVerdict extracts the boolean from the judge's JSON reply, tolerant of
// surrounding prose (scans the first {...} object). Unparseable → false, so a
// garbled verdict never triggers a nudge.
func parseStallVerdict(s string) bool {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j <= i {
		return false
	}
	var v struct {
		Stalled bool `json:"stalled"`
	}
	if err := json.Unmarshal([]byte(s[i:j+1]), &v); err != nil {
		return false
	}
	return v.Stalled
}

// StartCoordinatorStallSweeper launches the staleness backstop: every
// coordStallSweepInterval it judges any live coordinator slot that has gone silent
// past the staleness window with no running worker (see sweepCoordinatorStallsAt).
// A no-op while the master toggle or the sweeper window is off. Stops when ctx is
// cancelled.
func (r *Runtime) StartCoordinatorStallSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(coordStallSweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.sweepCoordinatorStallsAt(ctx, time.Now().Unix())
			}
		}
	}()
}

// sweepCoordinatorStallsAt judges every stall-candidate coordinator slot as of
// `now`. Split from StartCoordinatorStallSweeper with an explicit `now` for
// deterministic testing. A candidate is a live slot that is not running, has (had)
// workers with none running now, went silent past the window, and still has nudge
// budget. On a positive verdict it injects the note and kicks a fresh turn; when the
// budget is spent it escalates to a loud, observable warning instead of nudging on.
func (r *Runtime) sweepCoordinatorStallsAt(ctx context.Context, now int64) {
	if !r.tun.CoordinatorStallGuard() {
		return
	}
	window, enabled := r.tun.CoordinatorStallSweep()
	if !enabled {
		return
	}
	windowSec := int64(window / time.Second)
	maxNudges := r.tun.CoordinatorStallMaxNudges()

	type cand struct {
		id   string
		slot *coordSlot
	}
	var cands []cand
	r.coordSlots.Range(func(k, v any) bool {
		id, _ := k.(string)
		slot, _ := v.(*coordSlot)
		if id == "" || slot == nil {
			return true
		}
		if !slotIsStallCandidate(slot, r.sessionTurnBusy(id), now, windowSec) {
			return true
		}
		cands = append(cands, cand{id, slot})
		return true
	})

	for _, c := range cands {
		r.judgeAndNudgeStall(ctx, c.id, c.slot, maxNudges)
	}
}

// slotIsStallCandidate is the pure, deterministic gate the sweeper applies before
// spending a judge call: an idle coordinator that has no worker running now and has
// been silent at least `windowSec`. lastTurnUnix==0 (never ran a real turn — e.g. a
// stubbed test) is excluded. turnBusy comes from the admission queue (a turn of ANY
// kind holds the session) — a busy session is alive, not stalled.
//
// hadWorkers used to be required, which made the sweeper structurally blind to the
// case it exists for: a coordinator that only ever NARRATED spawns never spawned a
// worker, so it could never become a candidate. The gate is therefore relaxed for a
// slot known to be in coordinator mode; any other slot still needs a real worker in
// its history before the sweeper spends a judge call on it.
func slotIsStallCandidate(slot *coordSlot, turnBusy bool, now, windowSec int64) bool {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if turnBusy || slot.driving {
		return false
	}
	if !slot.hadWorkers && !slot.coordinatorMode {
		return false
	}
	if slot.workers.Load() > 0 {
		return false
	}
	if slot.lastTurnUnix == 0 || now-slot.lastTurnUnix < windowSec {
		return false
	}
	return true
}

// judgeAndNudgeStall runs the judge on a candidate coordinator's latest assistant
// message and acts on the verdict: nudge + fresh turn when budget remains, or a loud
// warning when it is spent (so a permanently wedged coordinator is observable rather
// than silently frozen).
func (r *Runtime) judgeAndNudgeStall(ctx context.Context, coordSessionID string, slot *coordSlot, maxNudges int) {
	sess, err := r.db.GetSession(ctx, coordSessionID)
	if err != nil {
		return
	}
	agent, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
		return
	}
	msgs, err := r.db.ListMessages(ctx, coordSessionID)
	if err != nil {
		return
	}
	text := lastAssistantText(msgs)
	if strings.TrimSpace(text) == "" {
		return
	}
	stalled, jerr := r.judgeCoordinatorStalled(ctx, agent, text)
	if jerr != nil || !stalled {
		return
	}
	slot.mu.Lock()
	spent := slot.spawnHallucStreak >= maxNudges
	slot.mu.Unlock()
	if spent {
		// Nudges exhausted and the stall still confirmed: escalate to the hard halt
		// (one-shot user notice + stop auto-turns), the same path the turn-end guard
		// takes — so a freeze the sweeper is the first to catch is surfaced to the user
		// rather than left as a silent log line.
		r.escalateCoordinatorStallHalt(coordSessionID, agent.ID, agent, slot, "nudge budget spent")
		return
	}
	// Same cumulative tier as the turn-end guard: the sweeper is often the layer that
	// SEES the relapse pattern (each stretch dies with its process, so only the
	// persisted tally connects them), and kicking a fresh turn into a coordinator that
	// has already crossed the threshold is exactly what the tier exists to stop.
	total := r.injectStallNudge(coordSessionID, agent.ID, slot, false /* not mid-drain; kick below */)
	if limit := r.tun.CoordinatorStallHaltTotal(); limit > 0 && total >= limit {
		r.escalateCoordinatorStallHalt(coordSessionID, agent.ID, agent, slot,
			fmt.Sprintf("cumulative stall threshold reached in sweep (%d/%d)", total, limit))
		return
	}
	r.enqueueCoordinatorTurn(coordSessionID)
}

// lastAssistantText returns the text of the most recent assistant message, or "".
func lastAssistantText(msgs []db.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return msgs[i].Text
		}
	}
	return ""
}
