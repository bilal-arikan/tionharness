package db

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// ---- Sessions ----

// A session directory holds its header and its transcript in SEPARATE files:
//
//	session.json    the header — one JSON object
//	messages.jsonl  the transcript — one message per line, appended
//
// The split is what keeps a header-only edit — rename, tag, pin, mark-read,
// rolling summary — off the transcript: it rewrites a few hundred bytes instead
// of re-encoding every message in the session. Combined, those were O(messages)
// per metadata edit, so toggling "pinned" on a long thread rewrote megabytes
// while holding the store's write lock.
//
// LEGACY: a single session.jsonl carried the header on line 1 and the messages
// after it. It is still readable and is migrated to the split layout on load.
const (
	sessionHeaderFile = "session.json"
	sessionMsgsFile   = "messages.jsonl"
	legacySessionFile = "session.jsonl"
)

func (d *DB) persistSessionLocked(s Session) error {
	d.sessions[s.ID] = s
	d.markMutatedLocked()
	return d.writeSessionHeaderLocked(s)
}

// persistSessionAfterWriteLocked is reserved for mutations whose in-memory
// state must not become visible unless the atomic header replacement succeeds.
// General session mutations intentionally retain persistSessionLocked's legacy
// publish-before-write semantics, including terminal run-state recovery.
func (d *DB) persistSessionAfterWriteLocked(s Session) error {
	if err := d.writeSessionHeaderLocked(s); err != nil {
		return err
	}
	d.sessions[s.ID] = s
	d.markMutatedLocked()
	return nil
}

func (d *DB) mutateSessionAfterWriteLocked(id string, fn func(*Session) error) error {
	tl := d.transcriptLock(id)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(id)
	if err != nil {
		tl.Unlock()
		return err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, id)
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if err := fn(&s); err != nil {
		return err
	}
	return d.persistSessionAfterWriteLocked(s)
}

// mutateSessionLocked loads a session under the write lock, applies fn, and
// persists it. Unlike the agent variant it does not touch UpdatedAt, leaving
// that to fn — some session mutations (e.g. rolling summary) are not "edits".
// Returns ErrNotFound when the session is absent.
func (d *DB) mutateSessionLocked(id string, fn func(*Session)) error {
	_, err := d.mutateSession(id, fn)
	return err
}

// mutateSession is mutateSessionLocked returning the post-mutation row, for
// setters that fire the session hook AFTER every lock is released (the returned
// copy is what the hook carries; no lock is held by the time it fires).
func (d *DB) mutateSession(id string, fn func(*Session)) (Session, error) {
	tl := d.transcriptLock(id)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(id)
	if err != nil {
		tl.Unlock()
		return Session{}, err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, id)
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	fn(&s)
	return s, d.persistSessionLocked(s)
}

// writeSessionHeaderLocked writes ONLY the session header file. Every metadata
// mutation takes this path, so it must never touch the transcript.
func (d *DB) writeSessionHeaderLocked(s Session) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	return atomicWriteBytes(d.dir(dirSessions, s.ID, sessionHeaderFile), buf.Bytes())
}

// writeSessionMessagesLocked rewrites the whole transcript file. O(messages) —
// reserved for callers that changed message CONTENT (edit, delete, rewind).
// Adding a message must go through appendMessageLine, which stays O(1).
func (d *DB) writeSessionMessagesLocked(sessionID string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, m := range d.messages[sessionID] {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return atomicWriteBytes(d.dir(dirSessions, sessionID, sessionMsgsFile), buf.Bytes())
}

// writeSessionFileLocked (re)writes both of a session's files: header and full
// transcript. Only for message-content changes — a header-only edit must use
// persistSessionLocked instead.
func (d *DB) writeSessionFileLocked(s Session) error {
	if err := d.writeSessionHeaderLocked(s); err != nil {
		return err
	}
	return d.writeSessionMessagesLocked(s.ID)
}

// CreateSession inserts a new session.
func (d *DB) CreateSession(ctx context.Context, s Session) (Session, error) {
	d.mu.Lock()
	created, err := d.createSessionLocked(s)
	d.mu.Unlock()
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: created.ID, Op: SessionOpCreate, Session: created})
	}
	return created, err
}

func (d *DB) GetSessionByDispatchKey(ctx context.Context, key string) (Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, session := range d.sessions {
		if key != "" && session.DispatchKey == key {
			return session, nil
		}
	}
	return Session{}, ErrNotFound
}

// CreateChildSession validates and atomically creates an execution child linked
// to an existing parent. It is the sole creation path for delegated executions.
func (d *DB) CreateChildSession(ctx context.Context, s Session) (Session, error) {
	created, err := d.createChildSession(s)
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: created.ID, Op: SessionOpCreate, Session: created})
	}
	return created, err
}

func (d *DB) createChildSession(s Session) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.TrimSpace(s.ParentSessionID) == "" {
		return Session{}, fmt.Errorf("child session requires parentSessionId")
	}
	if _, ok := d.sessions[s.ParentSessionID]; !ok {
		return Session{}, fmt.Errorf("parent session %q: %w", s.ParentSessionID, ErrNotFound)
	}
	if err := validateSessionMeta(s); err != nil {
		return Session{}, err
	}
	return d.createSessionLocked(s)
}

func validateSessionMeta(s Session) error {
	if err := validateSessionEnums(s); err != nil {
		return err
	}
	if (s.TargetProfile == "") == (s.TargetAgentID == "") {
		return fmt.Errorf("child session requires exactly one targetProfile or targetAgentId")
	}
	return nil
}

func validateSessionEnums(s Session) error {
	if !oneOf(s.ExecutionType, ExecutionInteractive, ExecutionSubagent, ExecutionWorker, ExecutionSchedule, ExecutionAutomation, ExecutionSystem) {
		return fmt.Errorf("invalid executionType %q", s.ExecutionType)
	}
	if !oneOf(s.Category, CategoryChat, CategorySubagent, CategoryWorker, CategoryAutomation, CategorySystem) {
		return fmt.Errorf("invalid category %q", s.Category)
	}
	if !oneOf(s.ContextMode, ContextIsolated, ContextInherited) {
		return fmt.Errorf("invalid contextMode %q", s.ContextMode)
	}
	if !oneOf(s.Visibility, VisibilityUser, VisibilityInternal) {
		return fmt.Errorf("invalid visibility %q", s.Visibility)
	}
	return nil
}

func oneOf(v string, allowed ...string) bool {
	return slices.Contains(allowed, v)
}

func (d *DB) createSessionLocked(s Session) (Session, error) {
	s.ID = d.nextID(idSession)
	s.CreatedAt = now()
	s.UpdatedAt = s.CreatedAt
	if s.Kind == "" {
		s.Kind = "chat"
	}
	if s.State == "" {
		s.State = "active"
	}
	s = normalizeSessionMeta(s)
	if err := validateSessionEnums(s); err != nil {
		return Session{}, err
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = SessionSchemaVersion
	}
	// Stamp the origin — the single lineage source (models_session_origin.go).
	// A caller that knows more than the legacy fields carry (the flow run + node,
	// the automation and the session that tripped it) passes an explicit origin;
	// everyone else gets the same derivation the boot loader applies to old
	// headers, so a session's lineage never depends on which build created it.
	if s.Origin == nil {
		o := deriveOrigin(s)
		s.Origin = &o
	}
	if s.Origin.At == 0 {
		s.Origin.At = s.CreatedAt
	}
	if err := validateOrigin(s.Origin); err != nil {
		return Session{}, err
	}
	// Seed the session's model snapshot from its agent's configured model so
	// the header answers "which model?" in O(1) without scanning messages.
	if s.Model == "" && s.AgentID != "" {
		if a, ok := d.agents[s.AgentID]; ok {
			s.Model = d.resolveAgentLocked(a).Model
		}
	}
	// Seed coordinator mode from the agent's default, so an agent configured as a
	// coordinator (a template's PM/CTO) arrives ready instead of needing a toggle
	// on every new thread. Same shape as the model snapshot above: the agent holds
	// the default, the session holds the live value everything else reads.
	//
	// Deliberately skipped inside a coordinator TREE (a spawned worker): there the
	// spawner has already decided, and it is the only layer that knows the depth
	// budget — see Runtime.SpawnWorker, which folds the agent default in itself and
	// degrades to a plain worker at the depth limit. Without this exemption the db
	// would silently re-enable a mode the spawner deliberately withheld.
	if !s.CoordinatorMode && s.AgentID != "" && s.CoordinatorSessionID == "" && s.CoordinatorDepth == 0 {
		if raw, ok := d.agents[s.AgentID]; ok {
			if a := d.resolveAgentLocked(raw); a.CoordinatorMode {
				s.CoordinatorMode = true
				if s.CoordinatorWorkflow == "" {
					s.CoordinatorWorkflow = a.CoordinatorWorkflow
				}
			}
		}
	}
	d.messages[s.ID] = nil
	return s, d.persistSessionLocked(s)
}

func normalizeSessionMeta(s Session) Session {
	if s.ExecutionType == "" {
		s.ExecutionType = ExecutionInteractive
		if s.Kind == "worker" {
			s.ExecutionType = ExecutionWorker
		}
		if s.Kind == "schedule" {
			s.ExecutionType = ExecutionSchedule
		}
	}
	if s.Category == "" {
		s.Category = CategoryChat
		if s.Kind == "worker" {
			s.Category = CategoryWorker
		}
	}
	if s.ContextMode == "" {
		s.ContextMode = ContextIsolated
	}
	if s.Visibility == "" {
		s.Visibility = VisibilityUser
	}
	return s
}

func (d *DB) getOrCreateKindSession(agentID, kind, title string) (Session, error) {
	fresh := Session{AgentID: agentID, Kind: kind, Title: title}
	d.mu.Lock()
	for _, s := range d.sessions {
		if s.AgentID == agentID && s.Kind == kind {
			d.mu.Unlock()
			return s, nil
		}
	}
	created, err := d.createSessionLocked(fresh)
	d.mu.Unlock()
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: created.ID, Op: SessionOpCreate, Session: created})
	}
	return created, err
}

// GetOrCreateSourceSession returns (creating if absent) the session that owns a
// specific source entity's run history, keyed by (kind, sourceID) — e.g. one
// "task" session per task. Each run appends a
// turn, so the entity's whole execution history reads as a single transcript.
func (d *DB) GetOrCreateSourceSession(ctx context.Context, kind, sourceID, agentID, title string) (Session, error) {
	fresh := Session{AgentID: agentID, Kind: kind, SourceID: sourceID, Title: title}
	d.mu.Lock()
	for _, s := range d.sessions {
		if s.Kind == kind && s.SourceID == sourceID {
			d.mu.Unlock()
			return s, nil
		}
	}
	created, err := d.createSessionLocked(fresh)
	d.mu.Unlock()
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: created.ID, Op: SessionOpCreate, Session: created})
	}
	return created, err
}

// SetSessionSummary persists the rolling compaction summary for a session and
// bumps the fold counter, returning the ordinal of THIS fold (1 for the first).
// The ordinal is what the compaction debug event records as fold_index; a
// non-nil error means it was not persisted and the caller must not report one.
func (d *DB) SetSessionSummary(ctx context.Context, sessionID, summary string, msgCount int) (int, error) {
	foldIndex := 0
	err := d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Summary = summary
		s.SummaryMsgCount = msgCount
		s.CompactionCount++
		foldIndex = s.CompactionCount
	})
	if err != nil {
		return 0, err
	}
	return foldIndex, nil
}

// SetSessionTitle persists a (re)generated title for a session. Does not bump
// UpdatedAt: like tagging, a title is metadata, not activity. UpdatedAt is the
// session's LAST ACTIVITY stamp (AddMessage sets it to the message's CreatedAt),
// and the rota screen draws a session's bar from CreatedAt to it — so titling a
// long-finished session afterwards (manual rename or a generated title) used to
// stretch its bar all the way to "now".
func (d *DB) SetSessionTitle(ctx context.Context, sessionID, title string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Title = title
	})
}

// SetSessionWorkingDir sets (or clears, when empty) a session's working
// directory (cwd) for the built-in filesystem/shell tools. An empty string
// resets the session to the workspace default.
func (d *DB) SetSessionWorkingDir(ctx context.Context, sessionID, dir string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.WorkingDir = dir
	})
}

// SetSessionState sets a session's lifecycle state ("active"/"archived"). An
// archived session drops out of the active list + the cross-session context
// block but is never deleted. Bumps UpdatedAt so the change is reflected.
func (d *DB) SetSessionState(ctx context.Context, sessionID, state string) error {
	prev := ""
	updated, err := d.mutateSession(sessionID, func(s *Session) {
		prev = s.State
		s.State = state
		s.UpdatedAt = now()
	})
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: sessionID, Op: SessionOpState, Session: updated, PrevState: prev})
	}
	return err
}

// SetSessionTags replaces a session's free-form tags. Does not bump UpdatedAt
// (tagging is metadata, not activity, and must not reorder the sidebar list).
func (d *DB) SetSessionTags(ctx context.Context, sessionID string, tags []string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Tags = normalizeTags(tags)
	})
}

// SetSessionStuckTurns persists the consecutive bad-turn counter (self-healing
// Faz D). Does not bump UpdatedAt — the counter is bookkeeping, not activity.
func (d *DB) SetSessionStuckTurns(ctx context.Context, sessionID string, n int) error {
	if n < 0 {
		n = 0
	}
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.StuckTurns = n
	})
}

// SetSessionRunState persists how the session's last background work turn ended
// (the turn-outcome vocabulary: completed / failed / killed / timeout /
// incomplete), so a finished run is distinguishable from a live one after a
// restart. `at` is the unix second the outcome was decided. Does not bump
// UpdatedAt — the caller records the reply message, which is the real activity.
func (d *DB) SetSessionRunState(ctx context.Context, sessionID, state string, at int64) error {
	prev := ""
	updated, err := d.mutateSession(sessionID, func(s *Session) {
		prev = s.RunState
		s.RunState = state
		s.RunStateAt = at
	})
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: sessionID, Op: SessionOpRunState, Session: updated, PrevRunState: prev})
	}
	return err
}

// BumpSessionStallNudges increments the cumulative coordinator-stall counter and
// returns the new value. Persisted (unlike the in-memory slot streak) so a later
// escalation tier survives a restart. Does not bump UpdatedAt — bookkeeping.
func (d *DB) BumpSessionStallNudges(ctx context.Context, sessionID string) (int, error) {
	n := 0
	err := d.mutateSessionLocked(sessionID, func(s *Session) {
		s.StallNudges++
		n = s.StallNudges
	})
	return n, err
}

// SetSessionStallNudges overwrites the cumulative coordinator-stall counter. The
// counterpart of the bump above: a coordinator turn that genuinely drives workers
// (a real coordination tool call) clears the tally with 0, so the cumulative halt
// tier only ever fires on a coordinator that keeps relapsing without recovering.
// Does not bump UpdatedAt — bookkeeping, same as the bump.
func (d *DB) SetSessionStallNudges(ctx context.Context, sessionID string, n int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.StallNudges = n
	})
}

// SetSessionPinned pins/unpins a session to the top of the sidebar list. Does not
// bump UpdatedAt (pinning is a view preference, not activity).
func (d *DB) SetSessionPinned(ctx context.Context, sessionID string, pinned bool) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Pinned = pinned
	})
}

// SetSessionModel updates the session header's model snapshot — called after a
// turn when the response model differs from the session's recorded model (e.g. the
// agent was reconfigured mid-session). Does not bump UpdatedAt — bookkeeping only.
func (d *DB) SetSessionModel(ctx context.Context, sessionID, model string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Model = model
	})
}

// SetSessionHandoffArtifact records, on the PARENT session, the id of the handoff
// artifact written when work was reset into a fresh child session. Bookkeeping
// only, so it does not bump UpdatedAt (recording a reset must not reorder the list).
func (d *DB) SetSessionHandoffArtifact(ctx context.Context, sessionID, artifactID string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.HandoffArtifactID = artifactID
	})
}

// SetSessionAgent updates a session's default (main) agent — used when the first
// message of a fresh session @mentions an agent, pinning the thread to it.
func (d *DB) SetSessionAgent(ctx context.Context, sessionID, agentID string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.AgentID = agentID
	})
}

// MarkSessionRead clears a session's unread flag (without bumping UpdatedAt, so
// reading a thread never reorders the list).
func (d *DB) MarkSessionRead(ctx context.Context, sessionID string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Unread = false
	})
}

// DeleteSession removes a session, its messages, its on-disk folder, and any
// attachment uploads that belonged to it (so uploaded files don't outlive the
// session that referenced them).
func (d *DB) DeleteSession(ctx context.Context, sessionID string) error {
	return d.deleteSession(ctx, sessionID, os.RemoveAll)
}

func (d *DB) deleteSession(ctx context.Context, sessionID string, removeAll func(string) error) error {
	removed, err := d.deleteSessionUnderLocks(ctx, sessionID, removeAll)
	if err != nil {
		return err
	}
	// Both the transcript lock and d.mu are released by now (deferred inside
	// deleteSessionUnderLocks), which is the hook's contract — and the
	// trajectory lock order's (never under d.mu).
	d.dropTrajectoryForRoot(sessionID)
	d.fireSessionHook(SessionChangeEvent{SessionID: sessionID, Op: SessionOpDelete, Session: removed})
	return nil
}

// deleteSessionUnderLocks removes the session under the transcript lock and
// d.mu, returning the row as it was for the delete event.
func (d *DB) deleteSessionUnderLocks(ctx context.Context, sessionID string, removeAll func(string) error) (Session, error) {
	// Removing the session directory destroys the transcript file, so it takes the
	// transcript lock like any other writer — otherwise an append could recreate
	// messages.jsonl underneath a delete that is already in progress.
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return Session{}, err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
	return d.deleteSessionLocked(ctx, sessionID, removeAll)
}

// deleteSessionLocked does the store-side removal under d.mu and returns the row
// as it was, for the delete event. Caller holds the transcript lock.
func (d *DB) deleteSessionLocked(ctx context.Context, sessionID string, removeAll func(string) error) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	removed, ok := d.sessions[sessionID]
	if !ok {
		return Session{}, ErrNotFound
	}
	if err := removeSessionDirWithRetry(ctx, d.dir(dirSessions, sessionID), removeAll); err != nil {
		return Session{}, err
	}
	delete(d.sessions, sessionID)
	d.markMutatedLocked()
	delete(d.messages, sessionID)
	delete(d.transcriptCache, sessionID)
	delete(d.transcriptCheckpoints, sessionID)
	d.deleteSessionFilesLocked(sessionID)
	d.dropTranscriptLock(sessionID)
	return removed, nil
}

// deleteSessionFilesLocked removes a session's artifacts when the session is
// deleted: every artifact entity (JSON) belonging to it and the per-session file
// folder (workspace/artifacts/<sid>/) that holds their content/uploads, plus the
// session's transient render_template output (<root>/render/<sid>/), so none of
// it outlives the session. Caller holds d.mu.
func (d *DB) deleteSessionFilesLocked(sessionID string) {
	for id, a := range d.artifacts {
		if a.SessionID == sessionID {
			delete(d.artifacts, id)
			d.markMutatedLocked()
			_ = removeFile(d.dir(dirArtifacts, id+".json"))
		}
	}
	_ = os.RemoveAll(d.ArtifactsDir(sessionID))
	_ = os.RemoveAll(d.RenderDir(sessionID))
}

// SessionDir returns the absolute folder holding a session's JSONL file.
func (d *DB) SessionDir(sessionID string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.sessions[sessionID]; !ok {
		return "", ErrNotFound
	}
	return d.dir(dirSessions, sessionID), nil
}

// GetSession loads a session by id.
func (d *DB) GetSession(ctx context.Context, id string) (Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s, ok := d.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return normalizeSessionMeta(s), nil
}

// ListSessions returns sessions for an agent (or all if agentID is empty),
// most recently updated first.
func (d *DB) ListSessions(ctx context.Context, agentID string) ([]Session, error) {
	// The ordering (pinned first, newest-updated first, ID tie-break) and the
	// normalisation both live in sortedSessions, which memoises the whole list
	// against MutationGen; this only copies (and optionally filters) it so the
	// caller owns the slice it gets.
	all := d.sortedSessions()
	if agentID == "" {
		out := make([]Session, len(all))
		copy(out, all)
		return out, nil
	}
	out := make([]Session, 0, len(all))
	for _, s := range all {
		if s.AgentID == agentID {
			out = append(out, s)
		}
	}
	return out, nil
}

// ---- Messages ----

// ---- session loading (boot) ----
