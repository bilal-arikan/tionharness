package db

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// ---- Agents ----

func (d *DB) persistAgentLocked(a Agent) error {
	d.agents[a.ID] = a
	return atomicWriteJSON(d.dir(dirAgents, a.ID+".json"), a)
}

// AgentPath returns the absolute path of an agent's on-disk JSON file (one
// file per agent under the workspace store's agents/ folder).
func (d *DB) AgentPath(agentID string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.agents[agentID]; !ok {
		return "", ErrNotFound
	}
	return d.dir(dirAgents, agentID+".json"), nil
}

// mutateAgentLocked loads an agent under the write lock, applies fn to it, bumps
// UpdatedAt, and persists it — centralizing the lock/lookup/mutate/persist dance
// shared by every agent mutator. Returns ErrNotFound when the agent is absent.
func (d *DB) mutateAgentLocked(id string, fn func(*Agent)) (Agent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	fn(&a)
	a.UpdatedAt = now()
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return a, nil
}

// CreateAgent inserts a new agent and returns the stored row.
func (d *DB) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	a.ID = d.nextID(idAgent)
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.PermissionMode == "" {
		a.PermissionMode = "auto"
	}
	if a.AllowedTools == "" {
		a.AllowedTools = "[]"
	}
	if a.BlockedTools == "" {
		a.BlockedTools = "[]"
	}
	if a.ToolOverrides == "" {
		a.ToolOverrides = "{}"
	}
	if a.Skills == nil {
		a.Skills = []string{}
	}
	// MCPEnabled is intentionally NOT defaulted here. Booleans cannot tell
	// "caller didn't set" from "caller set false", so default-on at the DB
	// layer would silently override explicit opt-outs. Each creation path
	// (POST /api/agents, market/ingest pack install, workspace-template
	// seeding, self-management create_agent, e2e harness) is responsible for
	// flipping false→true before calling CreateAgent — see the per-caller
	// "default-on" comments next to each db.Agent literal. To turn tools off
	// for an existing agent, call UpdateAgentTools (POST /api/agents/{id}/tools).

	d.mu.Lock()
	defer d.mu.Unlock()
	return a, d.persistAgentLocked(a)
}

// DeleteAgent marks an agent deleted and drops the forward-looking records that
// can no longer fire without it:
//   - schedules bound to it (AgentID),
//   - automations targeting it (TargetAgentID cleared, not deleted),
//   - tasks it owns (OwnerAgentID) together with their runs.
//
// The agent row itself is KEPT (Deleted=true) and so are the sessions it owns.
// Conversations are history: destroying them to remove their author loses the
// user's record of what happened, and every consumer that resolves a message's
// agent id would fall back to a raw id (or, worse, to a different agent). The
// row survives so that history still renders the real name/avatar/colour with a
// "deleted" marker, while ListAgents hides it from rosters and pickers.
//
// Removing the schedules here only clears the persisted rows; callers that run a
// live cron registry (the API server) must reload the scheduler afterwards so
// the in-memory jobs drop too.
func (d *DB) DeleteAgent(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return ErrNotFound
	}
	a.Deleted = true
	a.DeletedAt = now()
	a.UpdatedAt = a.DeletedAt
	d.agents[id] = a
	if err := atomicWriteJSON(d.dir(dirAgents, id+".json"), a); err != nil {
		return err
	}
	// Cascade: schedules deliver prompts to this agent, so they can no longer fire.
	for scid, sc := range d.schedules {
		if sc.AgentID == id {
			delete(d.schedules, scid)
			_ = removeFile(d.dir(dirSchedules, scid+".json"))
		}
	}
	// Cascade: automations targeting this agent lose their target so they don't
	// fire a dangling reference at runtime. The automation stays intact (name,
	// trigger, budget, history) and can be pointed at a new agent later.
	for aid, a := range d.automations {
		if a.TargetAgentID == id {
			a.TargetAgentID = ""
			a.UpdatedAt = now()
			if err := atomicWriteJSON(d.dir(dirAutomations, aid+".json"), a); err != nil {
				return err
			}
			d.automations[aid] = a
		}
	}
	// Cascade: tasks owned by this agent (and their runs) — they cannot be
	// delivered without an owner.
	deletedTasks := make(map[string]bool)
	for tid, t := range d.tasks {
		if t.OwnerAgentID == id {
			delete(d.tasks, tid)
			_ = removeFile(d.dir(dirTasks, tid+".json"))
			deletedTasks[tid] = true
		}
	}
	for rid, r := range d.runs {
		if r.AgentID == id || deletedTasks[r.TaskID] {
			delete(d.runs, rid)
			_ = removeFile(d.dir(dirRuns, rid+".json"))
		}
	}
	return nil
}

// RemoveSkillFromAgents strips a skill slug from every agent's skill selection
// and persists the agents that changed. It is called after a skill is deleted so
// no agent keeps a dangling reference to a skill that no longer exists. Returns
// the number of agents that were updated.
func (d *DB) RemoveSkillFromAgents(ctx context.Context, slug string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	updated := 0
	for aid, a := range d.agents {
		if len(a.Skills) == 0 {
			continue
		}
		kept := make([]string, 0, len(a.Skills))
		for _, s := range a.Skills {
			if s != slug {
				kept = append(kept, s)
			}
		}
		if len(kept) == len(a.Skills) {
			continue // slug not referenced by this agent
		}
		a.Skills = kept
		a.UpdatedAt = now()
		if err := d.persistAgentLocked(a); err != nil {
			return updated, err
		}
		d.agents[aid] = a
		updated++
	}
	return updated, nil
}

// GetAgent loads an agent by id.
func (d *DB) GetAgent(ctx context.Context, id string) (Agent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	return a, nil
}

// ListAgents returns the live agents, newest first. Deleted agents are excluded:
// this backs every server-side "pick an agent" path (defaults, budget rollups,
// market publish, graph), none of which may ever select a deleted one.
func (d *DB) ListAgents(ctx context.Context) ([]Agent, error) {
	return d.listAgents(false)
}

// ListAgentsWithDeleted also returns agents marked deleted (flagged, so the
// caller can tell them apart). The UI roster endpoint needs them: a past
// conversation must still render its author's real name and avatar with a
// "deleted" marker rather than falling back to a raw id — while the client
// filters them out of its own pickers.
func (d *DB) ListAgentsWithDeleted(ctx context.Context) ([]Agent, error) {
	return d.listAgents(true)
}

func (d *DB) listAgents(includeDeleted bool) ([]Agent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Agent, 0, len(d.agents))
	for _, a := range d.agents {
		if a.Deleted && !includeDeleted {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// AgentProfilePatch carries the editable identity fields for UpdateAgent. A nil
// pointer leaves that field untouched, so callers can do partial updates.
type AgentProfilePatch struct {
	Name           *string
	Soul           *string
	Identity       *string
	Provider       *string
	Model          *string
	ThinkingLevel  *string
	PermissionMode *string
	Avatar         *string
	Color          *string
	// Skills is the agent's ordered skill-slug selection. Non-nil replaces the
	// whole list (an empty slice clears it).
	Skills *[]string
	// CoordinatorMode / CoordinatorWorkflow are the agent's coordinator DEFAULTS
	// for the sessions it opens (see Agent.CoordinatorMode). Pointers so an
	// explicit false is distinguishable from "not in this patch"; changing them
	// never touches sessions that already exist.
	CoordinatorMode     *bool
	CoordinatorWorkflow *string
	// CoordinatorPrompt is the agent's coordinator-only prompt block (see
	// Agent.CoordinatorPrompt). Pointer so clearing it ("") is distinguishable
	// from "not in this patch".
	CoordinatorPrompt *string
}

// UpdateAgent applies a partial profile patch to an existing agent and persists
// it. Only non-nil patch fields are written.
func (d *DB) UpdateAgent(ctx context.Context, agentID string, p AgentProfilePatch) (Agent, error) {
	return d.mutateAgentLocked(agentID, func(a *Agent) {
		if p.Name != nil {
			a.Name = *p.Name
		}
		if p.Soul != nil {
			a.Soul = *p.Soul
		}
		if p.Identity != nil {
			a.Identity = *p.Identity
		}
		if p.Provider != nil {
			a.Provider = *p.Provider
		}
		if p.Model != nil {
			a.Model = *p.Model
		}
		if p.ThinkingLevel != nil {
			a.ThinkingLevel = *p.ThinkingLevel
		}
		if p.PermissionMode != nil {
			a.PermissionMode = *p.PermissionMode
		}
		if p.Avatar != nil {
			a.Avatar = *p.Avatar
		}
		if p.Color != nil {
			a.Color = *p.Color
		}
		if p.Skills != nil {
			a.Skills = *p.Skills
		}
		if p.CoordinatorMode != nil {
			a.CoordinatorMode = *p.CoordinatorMode
		}
		if p.CoordinatorWorkflow != nil {
			a.CoordinatorWorkflow = *p.CoordinatorWorkflow
		}
		if p.CoordinatorPrompt != nil {
			a.CoordinatorPrompt = *p.CoordinatorPrompt
		}
	})
}

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
	return d.writeSessionHeaderLocked(s)
}

// mutateSessionLocked loads a session under the write lock, applies fn, and
// persists it. Unlike the agent variant it does not touch UpdatedAt, leaving
// that to fn — some session mutations (e.g. rolling summary) are not "edits".
// Returns ErrNotFound when the session is absent.
func (d *DB) mutateSessionLocked(id string, fn func(*Session)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return ErrNotFound
	}
	fn(&s)
	return d.persistSessionLocked(s)
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
// Adding a message must go through appendMessageLocked, which stays O(1).
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
	defer d.mu.Unlock()
	return d.createSessionLocked(s)
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
	if s.SchemaVersion == 0 {
		s.SchemaVersion = SessionSchemaVersion
	}
	// Seed the session's model snapshot from its agent's configured model so
	// the header answers "which model?" in O(1) without scanning messages.
	if s.Model == "" && s.AgentID != "" {
		if a, ok := d.agents[s.AgentID]; ok {
			s.Model = a.Model
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
		if a, ok := d.agents[s.AgentID]; ok && a.CoordinatorMode {
			s.CoordinatorMode = true
			if s.CoordinatorWorkflow == "" {
				s.CoordinatorWorkflow = a.CoordinatorWorkflow
			}
		}
	}
	d.messages[s.ID] = nil
	return s, d.persistSessionLocked(s)
}

func (d *DB) getOrCreateKindSession(agentID, kind, title string) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.sessions {
		if s.AgentID == agentID && s.Kind == kind {
			return s, nil
		}
	}
	return d.createSessionLocked(Session{AgentID: agentID, Kind: kind, Title: title})
}

// GetOrCreateSourceSession returns (creating if absent) the session that owns a
// specific source entity's run history, keyed by (kind, sourceID) — e.g. one
// "task" session per task or one "flow" session per flow. Each run appends a
// turn, so the entity's whole execution history reads as a single transcript.
func (d *DB) GetOrCreateSourceSession(ctx context.Context, kind, sourceID, agentID, title string) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.sessions {
		if s.Kind == kind && s.SourceID == sourceID {
			return s, nil
		}
	}
	return d.createSessionLocked(Session{AgentID: agentID, Kind: kind, SourceID: sourceID, Title: title})
}

// SetSessionSummary persists the rolling compaction summary for a session.
func (d *DB) SetSessionSummary(ctx context.Context, sessionID, summary string, msgCount int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Summary = summary
		s.SummaryMsgCount = msgCount
	})
}

// SetSessionTitle persists a (re)generated title for a session.
func (d *DB) SetSessionTitle(ctx context.Context, sessionID, title string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Title = title
		s.UpdatedAt = now()
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
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.State = state
		s.UpdatedAt = now()
	})
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
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.RunState = state
		s.RunStateAt = at
	})
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

// SetSessionPinned pins/unpins a session to the top of the sidebar list. Does not
// bump UpdatedAt (pinning is a view preference, not activity).
func (d *DB) SetSessionPinned(ctx context.Context, sessionID string, pinned bool) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Pinned = pinned
	})
}

// SetMessageFeedback sets (or clears, when rating==0 and note=="") a user rating
// on an assistant message, rewriting the session's JSONL file. Returns ErrNotFound
// if the session or message is absent.
func (d *DB) SetMessageFeedback(ctx context.Context, sessionID, messageID string, rating int, note string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	msgs := d.messages[sessionID]
	idx := -1
	for i := range msgs {
		if msgs[i].ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	if rating == 0 && note == "" {
		msgs[idx].Feedback = nil
	} else {
		msgs[idx].Feedback = &MessageFeedback{Rating: rating, Note: note, At: now()}
	}
	d.messages[sessionID] = msgs
	return d.writeSessionFileLocked(s)
}

// SetSessionCLIResume records the claude-cli resume state for a session: the
// (rotated) CLI session id to --resume next turn, and how many of the session's
// messages the CLI has already seen (so the next turn sends only the delta). Does
// not bump UpdatedAt — bookkeeping must not reorder the session list.
func (d *DB) SetSessionCLIResume(ctx context.Context, sessionID, cliSessionID string, sentMsgCount int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CLISessionID = cliSessionID
		s.CLISentMsgCount = sentMsgCount
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

// SetCoordinatorMode turns a session's coordinator capability on or off (M2). It
// touches ONLY CoordinatorMode: Role stays whatever the session's lineage is, so
// enabling it on a worker produces a mid-level node (worker + coordinator) rather
// than severing its link to its parent. Disabling also clears the LEGACY Role
// value, otherwise IsCoordinator() would keep returning true on an old session
// and the toggle would silently do nothing.
func (d *DB) SetCoordinatorMode(ctx context.Context, sessionID string, enabled bool) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorMode = enabled
		if !enabled && s.Role == SessionRoleCoordinator {
			s.Role = ""
		}
	})
}

// SetSessionCoordinatorLineage stamps a freshly spawned worker's place in its
// coordinator tree: its parent, the tree root, and its depth below that root.
// Written once at spawn time, never edited afterwards — the tree shape is fixed
// at creation, which is what makes RootCoordinator()/depth safe to trust for
// tree-wide budgeting.
func (d *DB) SetSessionCoordinatorLineage(ctx context.Context, sessionID, parentID, rootID string, depth int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorSessionID = parentID
		s.RootCoordinatorSessionID = rootID
		s.CoordinatorDepth = depth
	})
}

// SetCoordinatorReportPending records whether a mid-level node still owes its
// coordinator an upward report (see Session.CoordinatorReportPending).
func (d *DB) SetCoordinatorReportPending(ctx context.Context, sessionID string, pending bool) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorReportPending = pending
	})
}

// ClaimCoordinatorReport atomically takes ownership of a session's outstanding
// upward report: it clears CoordinatorReportPending and returns whether THIS caller
// is the one that flipped it. Only the winner may send the report.
//
// A plain read-then-clear is not enough. Two settle backstops can be armed for the
// same session (one per drain exit), and a backstop can run alongside the agent's
// own report_to_coordinator — each would pass its own "does it still owe one?"
// check and send, so the coordinator above would receive the same task reported
// twice, with different statuses. The store lock makes the flip indivisible.
func (d *DB) ClaimCoordinatorReport(ctx context.Context, sessionID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return false, ErrNotFound
	}
	if !s.CoordinatorReportPending {
		return false, nil // someone else already reported
	}
	s.CoordinatorReportPending = false
	return true, d.persistSessionLocked(s)
}

// ListPendingCoordinatorReports returns every non-archived session that still owes
// its coordinator a report. Read at boot to re-arm the settle backstop for nodes
// whose owed report would otherwise be forgotten across a restart.
func (d *DB) ListPendingCoordinatorReports(ctx context.Context) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []Session
	for _, s := range d.sessions {
		if s.CoordinatorReportPending && s.State != "archived" && s.CoordinatorSessionID != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// ListCoordinatorTree returns every session in the coordinator tree that sessionID
// belongs to, INCLUDING the root and sessionID itself, in breadth-first order from
// the root. Accepts any member of the tree (root, mid-level node, or leaf) and
// normalizes to the root first — the same contract as ListFlowRunTree (_Docs/62),
// so a UI can hand it whatever session the user happens to be looking at.
//
// One pass over the session map builds the parent→children index: walking
// CoordinatorSessionID per node would be O(depth) lookups per node, and this runs
// on every coordinator turn (the live worker-status block).
func (d *DB) ListCoordinatorTree(ctx context.Context, sessionID string) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	start, ok := d.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	rootID := start.RootCoordinator()
	if rootID == "" {
		return []Session{start}, nil // neither coordinator nor worker: a tree of one
	}
	root, ok := d.sessions[rootID]
	if !ok {
		// The root was deleted out from under its subtree. Treat the caller as the
		// root so the surviving nodes stay reachable — returning an empty tree here
		// would read as "no workers" to a coordinator still waiting on them.
		root, rootID = start, start.ID
	}
	children := map[string][]Session{}
	for _, s := range d.sessions {
		if s.CoordinatorSessionID != "" {
			children[s.CoordinatorSessionID] = append(children[s.CoordinatorSessionID], s)
		}
	}
	out := []Session{root}
	queue := []string{rootID}
	seen := map[string]bool{rootID: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		kids := children[cur]
		sortSessionsByCreation(kids)
		for _, k := range kids {
			if seen[k.ID] {
				continue // defensive: a hand-edited parent cycle must not hang the walk
			}
			seen[k.ID] = true
			out = append(out, k)
			queue = append(queue, k.ID)
		}
	}
	return out, nil
}

// ListCoordinatorAncestors returns the chain from sessionID's ROOT down to its
// direct parent (root first, parent last); empty for a root or an ordinary
// session. This is the breadcrumb a worker walks upward to see who it reports to.
func (d *DB) ListCoordinatorAncestors(ctx context.Context, sessionID string) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	var chain []Session
	seen := map[string]bool{sessionID: true} // bounds a hand-edited parent cycle
	for cur := s.CoordinatorSessionID; cur != ""; {
		if seen[cur] {
			break
		}
		seen[cur] = true
		p, ok := d.sessions[cur]
		if !ok {
			break
		}
		chain = append(chain, p)
		cur = p.CoordinatorSessionID
	}
	// Collected parent-first; callers want root-first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// sortSessionsByCreation orders siblings oldest-first so a tree walk is stable
// across calls. CreatedAt has second resolution, so ties fall back to the id
// (monotonic per store) rather than leaving sibling order to map iteration.
func sortSessionsByCreation(ss []Session) {
	sort.Slice(ss, func(i, j int) bool {
		if ss[i].CreatedAt != ss[j].CreatedAt {
			return ss[i].CreatedAt < ss[j].CreatedAt
		}
		return ss[i].ID < ss[j].ID
	})
}

// SetSessionCoordinatorWorkflow records the selected coordinator recipe (M5) on a
// session plus the resolved per-session notify-loop cap override (0 = keep the
// workspace default). An empty slug clears the selection.
func (d *DB) SetSessionCoordinatorWorkflow(ctx context.Context, sessionID, slug string, maxTurns int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorWorkflow = slug
		s.CoordinatorMaxTurns = maxTurns
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.sessions[sessionID]; !ok {
		return ErrNotFound
	}
	delete(d.sessions, sessionID)
	delete(d.messages, sessionID)
	d.deleteSessionFilesLocked(sessionID)
	return os.RemoveAll(d.dir(dirSessions, sessionID))
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
	return s, nil
}

// ListSessions returns sessions for an agent (or all if agentID is empty),
// most recently updated first.
func (d *DB) ListSessions(ctx context.Context, agentID string) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Session, 0, len(d.sessions))
	for _, s := range d.sessions {
		if agentID == "" || s.AgentID == agentID {
			out = append(out, s)
		}
	}
	// Pinned sessions float to the top; within each group, most-recently-updated
	// first. A view preference, so it never changes the underlying activity order.
	// Tie-break on ID so equal-UpdatedAt sessions keep a STABLE order across calls
	// (the source map iterates in random order, so without this the list reshuffles
	// on every poll).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// ---- Messages ----

// AddMessage appends a message to a session and bumps the session counter.
func (d *DB) AddMessage(ctx context.Context, m Message) (Message, error) {
	// Respect a caller-supplied ID (used so a streamed reply and its crash sidecar
	// share one identity, making recovery idempotent); otherwise allocate one.
	if m.ID == "" {
		m.ID = newID()
	}
	m.CreatedAt = now()
	if m.ToolCalls == "" {
		m.ToolCalls = "[]"
	}
	if m.Steps == "" {
		m.Steps = "[]"
	}
	// Ensure the participant fields are populated before the line is persisted, so
	// the on-disk transcript is canonical (author/recipient recorded, not derived).
	m.NormalizeParticipants()

	d.mu.Lock()
	s, ok := d.sessions[m.SessionID]
	if !ok {
		d.mu.Unlock()
		return m, ErrNotFound
	}
	d.messages[m.SessionID] = append(d.messages[m.SessionID], m)
	s.MessageCount++
	// Sum this message's executed tool calls into the session's lifetime tool
	// counter (backs counter automations with metric "tool"). Only assistant
	// messages carry tool steps; a user/system append contributes 0.
	toolDelta := 0
	if m.Role == "assistant" {
		toolDelta = countToolSteps(m.Steps)
		s.ToolCallCount += toolDelta
	}
	s.UpdatedAt = m.CreatedAt
	// An agent reply marks the session unread; the UI clears it when opened.
	if m.Role == "assistant" {
		s.Unread = true
	}
	// Keep the participant roster in sync: any agent that authors a message or is
	// addressed by one joins the thread. The human "user" and broadcast ("*") are
	// implicit and never stored in the roster.
	s.Participants = addParticipant(s.Participants, m.AuthorKind, m.AuthorID)
	s.Participants = addParticipant(s.Participants, AuthorAgent, m.RecipientID)
	d.sessions[s.ID] = s
	// Hot path: append only the new message line (O(1)) instead of rewriting the
	// whole conversation file (which was O(n) per message → O(n²) per session).
	// The header line keeps a stale MessageCount/UpdatedAt on disk; both are
	// recomputed from the message lines on load and refreshed by the next full
	// rewrite (title/summary change).
	appendErr := d.appendMessageLocked(s.ID, m)
	// Snapshot the totals for the activity signal, then release the lock BEFORE
	// firing the hook (the observer dispatches on its own goroutine, which will
	// itself take the store lock).
	sig := ActivitySignal{
		SessionID:    s.ID,
		MessageTotal: s.MessageCount,
		MessageDelta: 1,
		ToolTotal:    s.ToolCallCount,
		ToolDelta:    toolDelta,
	}
	d.mu.Unlock()
	d.fireActivityHook(sig)
	return m, appendErr
}

// addParticipant appends an agent id to a session's participant roster when it is
// a real, not-yet-present agent participant. Only AuthorAgent ids join: the human
// "user", the broadcast marker "*", and empty ids are implicit and never stored.
func addParticipant(list []string, kind, id string) []string {
	if kind != AuthorAgent || id == "" || id == UserParticipantID || id == BroadcastRecipientID {
		return list
	}
	for _, x := range list {
		if x == id {
			return list
		}
	}
	return append(list, id)
}

// appendMessageLocked appends a single encoded message line to a session's
// transcript file, creating it on the first message (a session's directory is
// made at creation time, but messages.jsonl only appears once it has one).
func (d *DB) appendMessageLocked(sessionID string, m Message) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return err
	}
	path := d.dir(dirSessions, sessionID, sessionMsgsFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// DeleteMessage removes a single message from a session by id and rewrites the
// session's JSONL file. Returns ErrNotFound if the session or message is absent.
func (d *DB) DeleteMessage(ctx context.Context, sessionID, messageID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	msgs := d.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	d.messages[sessionID] = append(msgs[:idx:idx], msgs[idx+1:]...)
	if s.MessageCount > 0 {
		s.MessageCount--
	}
	d.sessions[s.ID] = s
	return d.writeSessionFileLocked(s)
}

// DeleteMessagesFrom removes the message with the given id and every message
// after it (a conversation "rewind" back to a checkpoint), then rewrites the
// session's JSONL file. Returns the number of messages removed, or ErrNotFound
// if the session or message is absent. File changes made by past turns are NOT
// reverted — this only truncates the transcript.
func (d *DB) DeleteMessagesFrom(ctx context.Context, sessionID, messageID string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return 0, ErrNotFound
	}
	msgs := d.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, ErrNotFound
	}
	removed := len(msgs) - idx
	// Truncate in place; the three-index slice caps cap so the dropped tail is
	// not aliased and can be GC'd.
	d.messages[sessionID] = msgs[:idx:idx]
	s.MessageCount = idx
	// Recompute the lifetime tool counter over the retained messages so a truncate
	// (rewind) rolls it back in step with MessageCount.
	s.ToolCallCount = 0
	for _, m := range msgs[:idx] {
		if m.Role == "assistant" {
			s.ToolCallCount += countToolSteps(m.Steps)
		}
	}
	// If the truncation point falls before the summarized boundary, the rolling
	// summary now describes messages that no longer exist. Reset it so the next
	// turn re-derives context from the (shorter) live transcript instead of a
	// stale summary. Loud on purpose — we do not keep a dangling summary.
	if s.SummaryMsgCount > idx {
		s.Summary = ""
		s.SummaryMsgCount = 0
	}
	d.sessions[s.ID] = s
	return removed, d.writeSessionFileLocked(s)
}

// ListMessages returns messages for a session in chronological order.
func (d *DB) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	msgs := d.messages[sessionID]
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

// ---- session loading (boot) ----

// loadSessions reads every sessions/<id>/ directory: the header from
// session.json and the transcript from messages.jsonl, migrating a legacy
// combined session.jsonl on the way.
func (d *DB) loadSessions() error {
	entries, err := os.ReadDir(d.dir(dirSessions))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(d.dir(dirSessions), e.Name())
		s, msgs, err := readSessionDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if s.ID == "" { // empty/headerless session — nothing usable
			continue
		}
		d.sessions[s.ID] = s
		d.messages[s.ID] = msgs
	}
	return nil
}

// readSessionDir loads one session from the split layout, falling back to the
// legacy combined file (and converting it) when no header file is present.
func readSessionDir(dir string) (Session, []Message, error) {
	b, err := os.ReadFile(filepath.Join(dir, sessionHeaderFile))
	if os.IsNotExist(err) {
		return migrateLegacySession(dir)
	}
	if err != nil {
		return Session{}, nil, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, nil, err // header corruption is fatal
	}
	msgs, err := readMessagesFile(filepath.Join(dir, sessionMsgsFile))
	if err != nil {
		return Session{}, nil, err
	}
	return reconcileHeader(s, msgs), msgs, nil
}

// migrateLegacySession converts a combined session.jsonl (header on line 1,
// messages after) into the split layout. Write order is what makes it
// crash-safe: the transcript lands first, the header second — and the header is
// the marker the loader keys on — so a crash before that point simply leaves the
// legacy file in place for the next boot to redo. Dropping the legacy file last
// is best-effort; a leftover copy is inert once session.json exists.
func migrateLegacySession(dir string) (Session, []Message, error) {
	path := filepath.Join(dir, legacySessionFile)
	lines, err := readJSONLines(path)
	if err != nil {
		return Session{}, nil, err // includes IsNotExist → caller skips the dir
	}
	if len(lines) == 0 {
		return Session{}, nil, nil
	}
	var s Session
	if err := json.Unmarshal(lines[0], &s); err != nil {
		return Session{}, nil, err // header corruption is fatal
	}
	msgs, err := decodeMessages(lines[1:])
	if err != nil {
		return Session{}, nil, err
	}
	s = reconcileHeader(s, msgs)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return Session{}, nil, err
		}
	}
	if err := atomicWriteBytes(filepath.Join(dir, sessionMsgsFile), buf.Bytes()); err != nil {
		return Session{}, nil, err
	}
	buf.Reset()
	if err := enc.Encode(s); err != nil {
		return Session{}, nil, err
	}
	if err := atomicWriteBytes(filepath.Join(dir, sessionHeaderFile), buf.Bytes()); err != nil {
		return Session{}, nil, err
	}
	_ = os.Remove(path)
	return s, msgs, nil
}

// readMessagesFile reads a transcript file. A session with no messages yet has
// no file at all, which is not an error.
func readMessagesFile(path string) ([]Message, error) {
	lines, err := readJSONLines(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeMessages(lines)
}

// readJSONLines returns the file's non-empty lines, copied out of the scanner's
// reused buffer.
func readJSONLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large message lines
	var lines [][]byte
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		b := make([]byte, len(line))
		copy(b, line)
		lines = append(lines, b)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func decodeMessages(lines [][]byte) ([]Message, error) {
	msgs := make([]Message, 0, len(lines))
	for i, line := range lines {
		var m Message
		if err := json.Unmarshal(line, &m); err != nil {
			// A torn trailing line (crash mid-append) is tolerated by dropping it;
			// corruption on any earlier line is real and fatal.
			if i == len(lines)-1 {
				break
			}
			return nil, err
		}
		// Back-fill the participant fields for messages stored before the model
		// (idempotent once set), so consumers never see empty AuthorKind on legacy
		// transcripts. No disk rewrite — this is an in-memory projection.
		m.NormalizeParticipants()
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// reconcileHeader makes in-memory state authoritative over the stored header.
// The append hot path deliberately leaves the header's counters stale (it never
// rewrites it), so they are recomputed from the actual messages. The participant
// roster is rebuilt the same way — an agent that joined via the append path
// never reached the header — so it self-heals across a restart.
func reconcileHeader(s Session, msgs []Message) Session {
	s.MessageCount = len(msgs)
	s.ToolCallCount = 0
	for _, m := range msgs {
		s.Participants = addParticipant(s.Participants, m.AuthorKind, m.AuthorID)
		s.Participants = addParticipant(s.Participants, AuthorAgent, m.RecipientID)
		if m.Role == "assistant" {
			s.ToolCallCount += countToolSteps(m.Steps)
		}
	}
	if n := len(msgs); n > 0 && msgs[n-1].CreatedAt > s.UpdatedAt {
		s.UpdatedAt = msgs[n-1].CreatedAt
	}
	return s
}
