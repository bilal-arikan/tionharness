package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/tools"
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

	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return HandoffResult{}, err
	}
	history, err := r.db.ListMessages(ctx, session.ID)
	if err != nil {
		return HandoffResult{}, err
	}
	rendered := conversation.RenderTranscript(history)
	if strings.TrimSpace(rendered) == "" {
		return HandoffResult{}, fmt.Errorf("nothing to hand off: session has no conversation yet")
	}

	env := r.handoffEnv(ctx, session)
	handoffText, err := conversation.BuildHandoff(ctx, r.db, provider, agent, session.Summary, rendered, env)
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
	cont := buildContinuationPrompt(session.ID, ref.ID, filePath, handoffText)
	spawn, err := r.SpawnSession(ctx, agent.ID, cont, SpawnOptions{
		CreatedBy:       opts.CreatedBy,
		ParentSessionID: session.ID,
		Title:           "↪ " + handoffTitle(session),
		// Continue in the same directory the parent worked in (its explicit
		// override, else the workspace default seeded by SpawnSession).
		WorkingDir: strings.TrimSpace(session.WorkingDir),
	})
	if err != nil {
		return HandoffResult{}, fmt.Errorf("spawn continuation session: %w", err)
	}

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

// handoffEnv gathers the environment snapshot for a handoff: the session goal, its
// working directory, the git branch/status there, and the session's existing
// artifacts. Best-effort — any piece that can't be gathered is simply omitted.
func (r *Runtime) handoffEnv(ctx context.Context, session db.Session) conversation.HandoffEnv {
	dir := strings.TrimSpace(session.WorkingDir)
	if dir == "" {
		dir = r.WorkspaceDefaultDir()
	}
	env := conversation.HandoffEnv{
		WorkingDir: dir,
		GitBranch:  gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD"),
		GitStatus:  gitOut(dir, "status", "--short", "--branch"),
	}
	if !session.GoalDone {
		env.Goal = strings.TrimSpace(session.Goal)
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

// writeHandoffFile optionally persists the handoff to <workdir>/.swarmgo/handoff.md
// when HandoffWriteFile is on. Returns the written path, or "" when disabled or on
// any error (the reset never depends on the file succeeding).
func (r *Runtime) writeHandoffFile(dir, content string) string {
	if !r.tun.HandoffWriteFile() || strings.TrimSpace(dir) == "" {
		return ""
	}
	outDir := filepath.Join(dir, ".swarmgo")
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
// verbatim recovery of anything the handoff did not capture.
func buildContinuationPrompt(oldSessionID, artifactID, filePath, handoffText string) string {
	var b strings.Builder
	b.WriteString("You are continuing a long-running task in a FRESH context window (a context reset / handoff). ")
	b.WriteString("Your previous session reached its context limit; this is a clean slate. ")
	b.WriteString("Do NOT restart from scratch — continue from the \"Next Concrete Step\" in the handoff below.\n\n")
	b.WriteString("# Handoff\n")
	b.WriteString(handoffText)
	b.WriteString("\n\n---\n")
	fmt.Fprintf(&b, "Recovery: the previous session id is `%s`. For exact pre-reset detail not captured above "+
		"(a code snippet, error message, file contents, or a specific decision), do not guess — use "+
		"`conversation_search` with session_id=\"%s\", or re-open the referenced files with your file tools. ",
		oldSessionID, oldSessionID)
	fmt.Fprintf(&b, "This handoff is also saved as artifact `%s`", artifactID)
	if strings.TrimSpace(filePath) != "" {
		fmt.Fprintf(&b, " (and file `%s`)", filePath)
	}
	b.WriteString(".")
	return b.String()
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
