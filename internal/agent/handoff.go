package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/prompts"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// handoffGitTimeout bounds the git calls used to snapshot the working tree for a
// handoff so a slow/hung git can never stall the reset.
const handoffGitTimeout = 3 * time.Second

// HandoffReason records why a context reset happened (for the tombstone/log).
const (
	HandoffReasonManual = "manual" // user ran /handoff
	HandoffReasonAgent  = "agent"  // the agent called the handoff_session tool
	HandoffReasonAuto   = "auto"   // the runtime auto-reset after a context-limit turn
)

// HandoffOptions tunes a context reset.
type HandoffOptions struct {
	Reason    string // one of HandoffReason*; defaults to manual
	CreatedBy string // provenance for the spawned session ("" = user/API)
	// AllowRunningWorkers bypasses the coordinator running-workers guard. Left
	// false by default so a handoff never silently orphans an in-flight fleet;
	// set it only when the caller has knowingly decided to reset anyway.
	AllowRunningWorkers bool
}

// HandoffResult is what a successful context reset returns: the fresh session that
// continues the work, the handoff artifact written into the old session, and the
// generated handoff text (so a caller can show it in a chat reply).
type HandoffResult struct {
	NewSessionID string
	AgentName    string
	ArtifactID   string
	HandoffText  string
}

// HandoffSession performs a context reset: it generates a structured handoff
// document from the session's full history, persists it as an artifact in the OLD
// session (and optionally to a file in the working dir), spawns a FRESH session
// seeded with the handoff so a clean-window agent continues the work, links the
// new session back to the old one, and drops a tombstone in the old session. This
// is the Anthropic "context reset" pattern — unlike in-place compaction it gives
// the next agent a genuinely clean slate, which is what defeats context anxiety.
func (r *Runtime) HandoffSession(ctx context.Context, session db.Session, agent db.Agent, opts HandoffOptions) (HandoffResult, error) {
	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		reason = HandoffReasonManual
	}

	// Guard: a coordinator with still-running workers must not be handed off.
	// Handoff snapshots THIS session's transcript into a fresh window, but it
	// neither carries over nor stops the workers this session spawned — resetting
	// now would orphan them (their results never fold back in). And because the
	// handoff summary is a SYNCHRONOUS provider call, a wedged worker could stall
	// the reset itself. Make the caller settle the fleet first (stop_worker or
	// wait for it to finish) instead of silently abandoning it.
	if !opts.AllowRunningWorkers && session.IsCoordinator() {
		workers, werr := r.ListWorkers(ctx, session.ID)
		if werr != nil {
			return HandoffResult{}, fmt.Errorf("handoff öncesi worker durumu okunamadı: %w", werr)
		}
		var busy []WorkerInfo
		for _, wk := range workers {
			if wk.Running || wk.Delegating {
				busy = append(busy, wk)
			}
		}
		if len(busy) > 0 {
			return HandoffResult{}, &RunningWorkersError{Workers: busy}
		}
	}

	provider, err := r.providers.Get(agent.ProviderRef())
	if err != nil {
		return HandoffResult{}, err
	}
	// Out-of-loop path: pin this app's CLI homes before BuildHandoff's direct
	// provider.Complete, mirroring guardedComplete (else the CLI falls back to the
	// ambient home and can fail auth even when TionHarness is logged in).
	if err := r.PinCLIHome(provider); err != nil {
		return HandoffResult{}, err
	}
	history, err := r.db.ListMessages(ctx, session.ID)
	if err != nil {
		return HandoffResult{}, err
	}
	// Only the turns after the most recent compaction are re-read: everything
	// before that boundary is already carried by session.Summary, which is passed
	// alongside. Rendering the full transcript here would ship the same content
	// twice and let long-folded detail crowd out the recent work.
	pending := conversation.PendingAfterSummary(history, session.SummaryMsgCount)
	rendered := conversation.RenderTranscript(pending)
	if strings.TrimSpace(rendered) == "" && strings.TrimSpace(session.Summary) == "" {
		return HandoffResult{}, fmt.Errorf("nothing to hand off: session has no conversation yet")
	}

	env := r.handoffEnv(ctx, session, history)
	// Carry the claude-home on ctx too so the fold core self-pins (defense in
	// depth alongside the explicit PinClaudeHome above).
	ctx = conversation.WithClaudeHome(ctx, r.claudeHomeDir())
	handoffText, err := conversation.BuildHandoff(ctx, r.db, provider, agent, session.Summary, rendered, env, r.readPrompt("handoff"))
	if err != nil {
		return HandoffResult{}, fmt.Errorf("generate handoff: %w", err)
	}

	// Persist the handoff as a first-class artifact in the OLD session, so it is
	// visible/downloadable in the UI and survives the reset.
	sink := r.NewArtifactSink(session.ID, agent.ID)
	title := "Handoff — " + handoffTitle(session)
	ref, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{
		Title:   title,
		Kind:    "markdown",
		Content: handoffText,
	})
	if err != nil {
		return HandoffResult{}, fmt.Errorf("write handoff artifact: %w", err)
	}
	_ = r.db.SetSessionHandoffArtifact(ctx, session.ID, ref.ID)

	// Optionally also write the handoff to a file on disk, mirroring the Anthropic
	// "progress file the next session reads" pattern. Best-effort: a file error
	// must not abort the reset (the artifact + inline copy already carry the state).
	filePath := r.writeHandoffFile(env.WorkingDir, handoffText)

	// Spawn a FRESH session seeded with the handoff so a clean-window agent
	// continues from the "Next Concrete Step". The continuation prompt embeds the
	// handoff inline (no tool round-trip needed) plus recovery pointers.
	cont := buildContinuationPrompt(r.readPrompt("continuation"), session.ID, ref.ID, filePath, handoffText)
	spawn, err := r.SpawnSession(ctx, agent.ID, cont, handoffContinuationSpawnOpts(session, opts))
	if err != nil {
		return HandoffResult{}, fmt.Errorf("spawn continuation session: %w", err)
	}

	// Cross-window live refresh: TWO sessions change here. The old session gets
	// a tombstone (transcript reload) and the new session appears in the
	// sidebar/activity feed. We publish BOTH explicitly so a sibling window
	// following the handoff chain updates without polling — the inner
	// SpawnSession call would only emit op="spawn", which is the wrong semantic
	// for a handoff continuation (it's not a fresh user-driven spawn).
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": session.ID, "op": "handoff"},
	})
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": spawn.SessionID, "op": "create", "kind": "spawned"},
	})

	// Tombstone the old session so the thread reads as handed off and deep-links
	// to its continuation.
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		AgentID:   agent.ID,
		Role:      "assistant",
		Text: fmt.Sprintf("↪ **Context reset (%s)** — bu oturum bağlam sınırına ulaştı; iş temiz bir pencerede devam ediyor.\n\n"+
			"- Devam oturumu: `%s`\n- Handoff artifact: `%s`%s",
			reason, spawn.SessionID, ref.ID, fileLine(filePath)),
	}); err != nil {
		r.logger.Warn("handoff: failed to record tombstone", "session", session.ID, "error", err)
	}

	r.logger.Info("handoff: context reset",
		"from", session.ID, "to", spawn.SessionID, "agent", agent.ID, "reason", reason, "artifact", ref.ID)

	return HandoffResult{
		NewSessionID: spawn.SessionID,
		AgentName:    spawn.AgentName,
		ArtifactID:   ref.ID,
		HandoffText:  handoffText,
	}, nil
}

// maybeAutoHandoff performs an automatic context reset after an autonomous turn
// that hit the context limit. It is a no-op unless HandoffAuto is enabled, the
// turn actually overflowed (reactive compaction fired), and the reset chain is
// still under the configured depth cap. Best-effort: a failure is logged and the
// session simply continues under ordinary compaction.
func (r *Runtime) maybeAutoHandoff(ctx context.Context, sessionID string, agent db.Agent, overflowed bool) {
	if !overflowed || !r.tun.HandoffAuto() || strings.TrimSpace(sessionID) == "" {
		return
	}
	session, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	if depth := r.handoffChainDepth(ctx, session); depth >= r.tun.HandoffMaxChain() {
		r.logger.Warn("handoff: chain depth cap reached; falling back to compaction",
			"session", sessionID, "depth", depth, "cap", r.tun.HandoffMaxChain())
		return
	}
	if _, err := r.HandoffSession(ctx, session, agent, HandoffOptions{
		Reason:    HandoffReasonAuto,
		CreatedBy: agent.ID,
	}); err != nil {
		r.logger.Warn("handoff: auto reset failed", "session", sessionID, "error", err)
	}
}

// handoffChainDepth counts how many context resets precede this session by walking
// the ParentSessionID lineage. Bounded by the chain cap so a corrupt cycle can't
// loop forever.
func (r *Runtime) handoffChainDepth(ctx context.Context, session db.Session) int {
	depth := 0
	cur := session
	for i := 0; i < r.tun.HandoffMaxChain()+1; i++ {
		parent := strings.TrimSpace(cur.ParentSessionID)
		if parent == "" {
			break
		}
		depth++
		p, err := r.db.GetSession(ctx, parent)
		if err != nil {
			break
		}
		cur = p
	}
	return depth
}

// handoffEnv gathers the environment snapshot for a handoff: the session's
// working directory, the git branch/status there, its active checklist and its
// existing artifacts. Best-effort — any piece that can't be gathered is simply
// omitted.
//
// The checklist comes from the transcript the caller already loaded (no second
// read) via the shared view.LatestTodos. HandoffEnv.Todos had been declared and
// rendered since the field was introduced but NEVER populated: every handoff
// shipped without the one piece of state a resuming agent most needs — what was
// already done and what is still open. A COMPLETED list is kept here (unlike the
// system-prompt block, which hides it), because "these are done" is precisely
// what stops a fresh agent redoing them.
func (r *Runtime) handoffEnv(ctx context.Context, session db.Session, history []db.Message) conversation.HandoffEnv {
	dir := strings.TrimSpace(session.WorkingDir)
	if dir == "" {
		dir = r.WorkspaceDefaultDir()
	}
	env := conversation.HandoffEnv{
		WorkingDir: dir,
		GitBranch:  gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD"),
		GitStatus:  gitOut(dir, "status", "--short", "--branch"),
		Todos:      view.LatestTodos(history).RenderChecklist(),
	}
	if arts, err := r.db.ListArtifacts(ctx, session.ID); err == nil && len(arts) > 0 {
		var b strings.Builder
		for _, a := range arts {
			fmt.Fprintf(&b, "  - %s (%s, id %s)\n", a.Title, a.Kind, a.ID)
		}
		env.Artifacts = strings.TrimRight(b.String(), "\n")
	}
	return env
}

// writeHandoffFile optionally persists the handoff to <workdir>/.tionharness/handoff.md
// when HandoffWriteFile is on. Returns the written path, or "" when disabled or on
// any error (the reset never depends on the file succeeding).
func (r *Runtime) writeHandoffFile(dir, content string) string {
	if !r.tun.HandoffWriteFile() || strings.TrimSpace(dir) == "" {
		return ""
	}
	outDir := filepath.Join(dir, ".tionharness")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		r.logger.Warn("handoff: mkdir failed", "dir", outDir, "error", err)
		return ""
	}
	path := filepath.Join(outDir, "handoff.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.logger.Warn("handoff: write file failed", "path", path, "error", err)
		return ""
	}
	return path
}

// buildContinuationPrompt is the opening user turn for the fresh session: it tells
// the agent it is resuming after a context reset, embeds the handoff inline so it
// has full state immediately, and points to the old session + artifact for
// verbatim recovery of anything the handoff did not capture. tmpl is the
// registry "continuation" template (blank/invalid falls back to the default);
// the optional {{fileNote}} slot renders the on-disk handoff file when written.
func buildContinuationPrompt(tmpl, oldSessionID, artifactID, filePath, handoffText string) string {
	if prompts.Validate("continuation", tmpl) != nil {
		tmpl = prompts.Default("continuation")
	}
	fileNote := ""
	if strings.TrimSpace(filePath) != "" {
		fileNote = fmt.Sprintf(" (and file `%s`)", filePath)
	}
	return prompts.Render(tmpl, map[string]string{
		"handoff":    handoffText,
		"oldSession": oldSessionID,
		"artifact":   artifactID,
		"fileNote":   fileNote,
	})
}

// continuationKind picks the session kind for a handoff continuation. A chat
// parent (kind "chat" or legacy "") yields a "chat" continuation so it appears
// under the sidebar's "Sohbet" filter next to the thread it continues. Any other
// parent kind falls back to "spawned" — the continuation is always a single-agent
// human-continuable transcript, and inheriting a non-writable kind (task/flow/
// schedule) or a tree kind (worker/flow-coordinator, which also carry back-links
// this fresh session lacks) would misfile or misrender it.
func continuationKind(parentKind string) string {
	switch strings.TrimSpace(parentKind) {
	case "", "chat":
		return "chat"
	default:
		return "spawned"
	}
}

// handoffContinuationSpawnOpts builds the SpawnOptions for a /handoff continuation
// session. It carries forward what makes the continuation the SAME work in a clean
// window — the parent's working dir plus its COORDINATOR capability and workflow
// recipe (CoordinatorMode + CoordinatorWorkflow + CoordinatorMaxTurns), so the new
// session keeps the same multi-agent setup (coordinator prompt + spawn_worker/…
// tools + selected recipe). Unlike a sub-coordinator SPAWN — where the recipe is
// deliberately NOT inherited to stop a recursive recipe repeating down the tree —
// a handoff is one logical session continuing, so inheriting is correct. Tree/worker
// LINEAGE is intentionally dropped (no Role/CoordinatorSessionID/Root/Depth): the
// continuation starts as its own fresh top-level coordinator, not a worker still
// reporting to the old (now handed-off) tree.
func handoffContinuationSpawnOpts(session db.Session, opts HandoffOptions) SpawnOptions {
	return SpawnOptions{
		CreatedBy:       opts.CreatedBy,
		ParentSessionID: session.ID,
		Title:           "↪ " + handoffTitle(session),
		// Keep a chat handoff in the "Sohbet" sidebar filter next to its parent; other
		// parent kinds fall back to the writable "spawned" kind (see continuationKind).
		Kind:                continuationKind(session.Kind),
		WorkingDir:          strings.TrimSpace(session.WorkingDir),
		CoordinatorMode:     session.CoordinatorMode,
		CoordinatorWorkflow: strings.TrimSpace(session.CoordinatorWorkflow),
		CoordinatorMaxTurns: session.CoordinatorMaxTurns,
	}
}

// handoffTitle derives a short label for the handoff artifact + continuation
// session from the source session's title (falling back to its id).
func handoffTitle(session db.Session) string {
	t := strings.TrimSpace(session.Title)
	// Strip a leading reset marker so chained handoffs don't accrete "↪ ↪ …".
	t = strings.TrimSpace(strings.TrimPrefix(t, "↪"))
	if t == "" {
		return session.ID
	}
	return t
}

// RunningWorkersError is returned by HandoffSession when a coordinator still has
// running/delegating workers. It is a user-actionable precondition, not a server
// fault, so the API layer maps it to 409 + an in-thread notice instead of a 500.
type RunningWorkersError struct {
	Workers []WorkerInfo
}

func (e *RunningWorkersError) Error() string {
	return fmt.Sprintf(
		"handoff engellendi: bu koordinatör oturumunda %d worker hâlâ çalışıyor (%s). "+
			"Handoff worker'ları devretmez veya durdurmaz; önce onları durdur (stop_worker) "+
			"ya da bitmelerini bekle, sonra tekrar dene.",
		len(e.Workers), formatBusyWorkers(e.Workers))
}

// formatBusyWorkers renders a short "SES.. (agent) [delegating], SES.. (agent)"
// list of the still-running workers for the coordinator handoff guard message.
func formatBusyWorkers(ws []WorkerInfo) string {
	parts := make([]string, 0, len(ws))
	for _, w := range ws {
		label := w.SessionID
		if name := strings.TrimSpace(w.AgentName); name != "" {
			label += " (" + name + ")"
		}
		if w.Delegating {
			label += " [delegating]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}

// fileLine renders the optional handoff-file bullet for the tombstone message.
func fileLine(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return "\n- Handoff dosyası: `" + path + "`"
}

// gitOut runs a time-bounded git command in dir and returns its trimmed stdout,
// or "" when dir is not a repo / git is unavailable. Best-effort.
func gitOut(dir string, args ...string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), handoffGitTimeout)
	defer cancel()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
