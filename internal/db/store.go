package db

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	// Resolve on the RETURNED value only (not before persistAgentLocked above):
	// callers should see the effective row (inheritance folded, populated
	// ProviderInstanceID), but a mutation that didn't touch a field must not
	// silently widen the on-disk write beyond what the patch actually changed
	// (_Docs/71 §3, "no bulk write").
	return d.resolveAgentLocked(a), nil
}

// mutateAgentLockedErr is mutateAgentLocked for mutators that validate under
// the lock: fn may refuse the write, in which case nothing is persisted.
func (d *DB) mutateAgentLockedErr(id string, fn func(*Agent) error) (Agent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	if err := fn(&a); err != nil {
		return Agent{}, err
	}
	a.UpdatedAt = now()
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return d.resolveAgentLocked(a), nil
}

// CreateAgent inserts a new agent and returns the stored row.
func (d *DB) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	a.ID = d.nextID(idAgent)
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.PermissionMode == "" {
		a.PermissionMode = "auto"
	}
	// ThinkingLevel has no valid empty value any more (see Agent.ThinkingLevel).
	// The API create path rejects a blank level outright; the remaining creation
	// paths (pack install, workspace templates, self-management create_agent,
	// system-agent seeding) may still omit it, and resolving it here with the
	// same rule the boot migration uses keeps them from writing a row that only
	// becomes valid after the next restart.
	if a.ThinkingLevel == "" {
		a.ThinkingLevel = LegacyThinkingLevelFor(a.Provider)
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

	// Every creation path is expected to have already synced Provider/
	// ProviderInstanceID (agent.SyncProviderFields, _Docs/71 §2.5), but this
	// backfill is the last-resort invariant guard for a caller that forgot to —
	// a brand-new row is exactly where filling it in is safe to persist
	// (unlike an existing row, there is no "unrelated field" risk).
	a = a.backfillProviderInstance()

	d.mu.Lock()
	defer d.mu.Unlock()
	// Inheritance: the parent must exist; overrides must name real fields. A
	// brand-new row cannot form a cycle, so only existence is checked here.
	if err := d.checkParentLocked("", a.ParentID); err != nil {
		return Agent{}, err
	}
	if err := ValidateOverrideKeys(a.Overrides); err != nil {
		return Agent{}, err
	}
	if a.ParentID == "" {
		a.Overrides = nil
	} else {
		a.Overrides = normalizeOverrides(a.Overrides)
	}
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return d.resolveAgentLocked(a), nil
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
	// A built-in (locked) row is owned by the compiled registry and re-seeded on
	// boot, so deleting it would only ever be undone. A workspace customisation
	// of a system role (System && !Locked) IS deletable: the role falls back to
	// the built-in, and the soft-deleted row keeps its sessions renderable.
	if a.Locked {
		return ErrSystemAgentDelete
	}
	a.Deleted = true
	a.DeletedAt = now()
	a.UpdatedAt = a.DeletedAt
	d.agents[id] = a
	if err := atomicWriteJSON(d.dir(dirAgents, id+".json"), a); err != nil {
		return err
	}
	// Children keep their effective values: they are re-pointed at this agent's
	// own parent (or become roots), with the removed layer's contribution folded
	// into their overrides.
	if err := d.reparentChildrenLocked(a); err != nil {
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
	// Cascade: tasks owned by this agent — they cannot be delivered without an owner.
	for tid, t := range d.tasks {
		if t.OwnerAgentID == id {
			delete(d.tasks, tid)
			_ = removeFile(d.dir(dirTasks, tid+".json"))
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
	return d.resolveAgentLocked(a), nil
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
		out = append(out, d.resolveAgentLocked(a))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// AgentProfilePatch carries the editable identity fields for UpdateAgent. A nil
// pointer leaves that field untouched, so callers can do partial updates.
type AgentProfilePatch struct {
	Name     *string
	Soul     *string
	Identity *string
	Provider *string
	// ProviderInstanceID patches Agent.ProviderInstanceID directly. Most
	// callers should go through agent.SyncProviderFields instead of setting
	// this (or Provider) by hand — see its doc comment for why.
	ProviderInstanceID *string
	Model              *string
	ThinkingLevel      *string
	// NativeWebSearch toggles the provider's own web search (see
	// Agent.NativeWebSearch). Pointer so an explicit false (turn it off) is
	// distinguishable from "not in this patch"; the stored field is itself a
	// pointer whose nil means enabled, so a patch can only set it, never unset it.
	NativeWebSearch *bool
	PermissionMode  *string
	// InboundPolicy is the recipient-side messaging policy (see
	// Agent.InboundPolicy): "accept" | "hold" | "refuse", or "" to reset to the
	// default (accept). An unknown value is rejected by UpdateAgent.
	InboundPolicy *string
	Avatar        *string
	Color         *string
	// Skills is the agent's ordered skill-slug selection. Non-nil replaces the
	// whole list (an empty slice clears it).
	Skills   *[]string
	Disabled *bool
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
	// ParentID re-parents the agent (see Agent.ParentID); "" detaches it into a
	// root that keeps its effective values. Validated for existence and cycles.
	ParentID *string
	// ResetFields lists override keys (InheritableFieldKeys) to drop so those
	// fields inherit again. Applied after the field writes above, so a patch
	// cannot both set and reset the same field meaningfully — reset wins.
	ResetFields []string
}

// UpdateAgent applies a partial profile patch to an existing agent and persists
// it. Only non-nil patch fields are written.
//
// Inheritance rules (see agent_inherit.go):
//   - a Locked (built-in) agent refuses every patch with ErrAgentLocked;
//   - on a child, every field the patch touches becomes an OVERRIDE, and
//     ResetFields drops overrides (the field goes back to inheriting; its raw
//     value is refreshed from the parent so the on-disk row stays readable);
//   - ParentID "" → X turns a root into a child that keeps behaving exactly as
//     before (every field becomes an override, to be reset one by one);
//     X → "" materialises the effective values into a root; X → Y keeps the
//     override set and re-resolves the rest against Y.
func (d *DB) UpdateAgent(ctx context.Context, agentID string, p AgentProfilePatch) (Agent, error) {
	// Validate before mutating: an unknown inbound policy must fail the write, not
	// be stored and then blow up on every later delivery.
	if p.InboundPolicy != nil {
		if _, err := ValidateInboundPolicy(*p.InboundPolicy); err != nil {
			return Agent{}, err
		}
	}
	if err := ValidateOverrideKeys(p.ResetFields); err != nil {
		return Agent{}, err
	}
	return d.mutateAgentLockedErr(agentID, func(a *Agent) error {
		if a.Locked {
			return ErrAgentLocked
		}
		// Parent change first, so the field writes below are classified against
		// the tree the agent ends up in.
		if p.ParentID != nil && *p.ParentID != a.ParentID {
			if err := d.checkParentLocked(a.ID, *p.ParentID); err != nil {
				return err
			}
			switch {
			case a.ParentID == "":
				// root → child: keep behaviour, own everything until reset.
				a.ParentID = *p.ParentID
				a.Overrides = InheritableFieldKeys()
			case *p.ParentID == "":
				// child → root: freeze the effective values.
				*a = materialize(d.resolveAgentLocked(*a))
			default:
				a.ParentID = *p.ParentID
			}
		}
		// A disabled customisation coming back must not compete with another
		// enabled customisation of the same role.
		if p.Disabled != nil && !*p.Disabled && a.Disabled && a.System && a.SystemKey != "" {
			if d.systemRoleTakenLocked(a.SystemKey, a.ID) {
				return ErrSystemRoleTaken
			}
		}
		mark := func(key string) {
			if a.ParentID != "" {
				a.Overrides = withOverride(a.Overrides, key)
			}
		}
		if p.Name != nil {
			a.Name = *p.Name
		}
		if p.Soul != nil {
			a.Soul = *p.Soul
			mark("soul")
		}
		if p.Identity != nil {
			a.Identity = *p.Identity
			mark("identity")
		}
		if p.Provider != nil {
			a.Provider = *p.Provider
			mark("provider")
		}
		if p.ProviderInstanceID != nil {
			a.ProviderInstanceID = *p.ProviderInstanceID
			mark("provider")
		}
		if p.Model != nil {
			a.Model = *p.Model
			mark("model")
		}
		if p.ThinkingLevel != nil {
			a.ThinkingLevel = *p.ThinkingLevel
			mark("thinkingLevel")
		}
		if p.NativeWebSearch != nil {
			v := *p.NativeWebSearch
			a.NativeWebSearch = &v
			mark("nativeWebSearch")
		}
		if p.PermissionMode != nil {
			a.PermissionMode = *p.PermissionMode
			mark("permissionMode")
		}
		if p.InboundPolicy != nil {
			a.InboundPolicy = *p.InboundPolicy
			mark("inboundPolicy")
		}
		if p.Avatar != nil {
			a.Avatar = *p.Avatar
			mark("avatar")
		}
		if p.Color != nil {
			a.Color = *p.Color
			mark("color")
		}
		if p.Skills != nil {
			a.Skills = *p.Skills
			mark("skills")
		}
		if p.Disabled != nil {
			a.Disabled = *p.Disabled
		}
		if p.CoordinatorMode != nil {
			a.CoordinatorMode = *p.CoordinatorMode
			mark("coordinatorMode")
		}
		if p.CoordinatorWorkflow != nil {
			a.CoordinatorWorkflow = *p.CoordinatorWorkflow
			mark("coordinatorWorkflow")
		}
		if p.CoordinatorPrompt != nil {
			a.CoordinatorPrompt = *p.CoordinatorPrompt
			mark("coordinatorPrompt")
		}
		if len(p.ResetFields) > 0 && a.ParentID != "" {
			d.resetOverridesLocked(a, p.ResetFields)
		}
		if a.ParentID == "" {
			a.Overrides = nil
		}
		return nil
	})
}

// resetOverridesLocked drops the named overrides from a and refreshes their
// raw values from the parent's effective row. Caller holds d.mu.
func (d *DB) resetOverridesLocked(a *Agent, keys []string) {
	parent, ok := d.agents[a.ParentID]
	var parentEffective Agent
	if ok {
		parentEffective = d.resolveAgentLocked(parent)
	}
	for _, key := range keys {
		a.Overrides = withoutOverride(a.Overrides, key)
		if !ok {
			continue
		}
		for _, f := range inheritableFields {
			if f.Key == key {
				f.copy(a, &parentEffective)
			}
		}
	}
}

// ClearAgentOverrides makes a child inherit every field again. A root agent is
// returned unchanged; a locked built-in refuses with ErrAgentLocked.
func (d *DB) ClearAgentOverrides(ctx context.Context, agentID string) (Agent, error) {
	return d.mutateAgentLockedErr(agentID, func(a *Agent) error {
		if a.Locked {
			return ErrAgentLocked
		}
		if a.ParentID == "" {
			return nil
		}
		d.resetOverridesLocked(a, InheritableFieldKeys())
		return nil
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

// persistSessionAfterWriteLocked is reserved for mutations whose in-memory
// state must not become visible unless the atomic header replacement succeeds.
// General session mutations intentionally retain persistSessionLocked's legacy
// publish-before-write semantics, including terminal run-state recovery.
func (d *DB) persistSessionAfterWriteLocked(s Session) error {
	if err := d.writeSessionHeaderLocked(s); err != nil {
		return err
	}
	d.sessions[s.ID] = s
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
	if !oneOf(s.ExecutionType, ExecutionInteractive, ExecutionSubagent, ExecutionWorker, ExecutionFlow, ExecutionSchedule, ExecutionAutomation, ExecutionSystem) {
		return fmt.Errorf("invalid executionType %q", s.ExecutionType)
	}
	if !oneOf(s.Category, CategoryChat, CategorySubagent, CategoryWorker, CategoryFlow, CategoryAutomation, CategorySystem) {
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
	for _, candidate := range allowed {
		if v == candidate {
			return true
		}
	}
	return false
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
		if s.Kind == "flow" {
			s.ExecutionType = ExecutionFlow
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
		if s.Kind == "flow" {
			s.Category = CategoryFlow
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
	d.mu.Lock()
	for _, s := range d.sessions {
		if s.AgentID == agentID && s.Kind == kind {
			d.mu.Unlock()
			return s, nil
		}
	}
	created, err := d.createSessionLocked(Session{AgentID: agentID, Kind: kind, Title: title})
	d.mu.Unlock()
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: created.ID, Op: SessionOpCreate, Session: created})
	}
	return created, err
}

// GetOrCreateSourceSession returns (creating if absent) the session that owns a
// specific source entity's run history, keyed by (kind, sourceID) — e.g. one
// "task" session per task or one "flow" session per flow. Each run appends a
// turn, so the entity's whole execution history reads as a single transcript.
func (d *DB) GetOrCreateSourceSession(ctx context.Context, kind, sourceID, agentID, title string) (Session, error) {
	d.mu.Lock()
	for _, s := range d.sessions {
		if s.Kind == kind && s.SourceID == sourceID {
			d.mu.Unlock()
			return s, nil
		}
	}
	created, err := d.createSessionLocked(Session{AgentID: agentID, Kind: kind, SourceID: sourceID, Title: title})
	d.mu.Unlock()
	if err == nil {
		d.fireSessionHook(SessionChangeEvent{SessionID: created.ID, Op: SessionOpCreate, Session: created})
	}
	return created, err
}

// SetSessionOriginRun fills the flow run id into a flow-origin session's Origin.
// The per-run transcript session is created BEFORE its FlowRun row exists (so the
// executions feed shows the run the instant it starts), which is the one case
// where the origin cannot be complete at creation; RunFlow calls this right after
// CreateFlowRun, before the first node runs. A no-op when the id is already set.
func (d *DB) SetSessionOriginRun(ctx context.Context, sessionID, runID string) error {
	changed := false
	updated, err := d.mutateSession(sessionID, func(s *Session) {
		if s.Origin == nil {
			o := deriveOrigin(*s)
			o.At = s.CreatedAt
			s.Origin = &o
		}
		if s.Origin.RunID == runID {
			return
		}
		s.Origin.RunID = runID
		changed = true
	})
	if err == nil && changed {
		d.fireSessionHook(SessionChangeEvent{SessionID: sessionID, Op: SessionOpOrigin, Session: updated})
	}
	return err
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

// SetSessionInboundPolicy overrides (or clears, when empty) the inbound-message
// policy for messages delivered INTO this session. An empty string falls back to
// the recipient agent's policy; an unknown value is rejected so a typo cannot
// silently become "accept".
func (d *DB) SetSessionInboundPolicy(ctx context.Context, sessionID, policy string) error {
	if policy != "" {
		if _, err := ValidateInboundPolicy(policy); err != nil {
			return err
		}
	}
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.InboundPolicy = policy
		s.UpdatedAt = now()
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

// SetMessageFeedback sets (or clears, when rating==0 and note=="") a user rating
// on an assistant message, rewriting the session's JSONL file. Returns ErrNotFound
// if the session or message is absent.
func (d *DB) SetMessageFeedback(ctx context.Context, sessionID, messageID string, rating int, note string) error {
	// Rewrites the transcript: same lock, and taken BEFORE d.mu (see
	// transcript_lock.go), so it can never interleave with a concurrent append.
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
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

// BeginSessionCLINativeCompaction durably marks the external CLI state as
// potentially changing. The CLI must not be invoked if this write fails.
func (d *DB) BeginSessionCLINativeCompaction(ctx context.Context, sessionID string) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		if s.CLINativeCompactionPending {
			return errors.New("CLI native compaction recovery is pending")
		}
		s.CLINativeCompactionPending = true
		return nil
	})
}

// SetSessionCLICompactionState atomically commits the CLI's rotated resume
// target, both transcript counters and recovery-marker clearance. A failed temp
// write or rename leaves the durable and in-memory marker set, forcing a cold
// next turn rather than reusing the old resume id and delta cursor.
func (d *DB) SetSessionCLICompactionState(ctx context.Context, sessionID, cliSessionID string, msgCount int) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		s.CLISessionID = cliSessionID
		s.CLISentMsgCount = msgCount
		if msgCount > s.CLICompactMsgCount {
			s.CLICompactMsgCount = msgCount
		}
		s.CLINativeCompactionPending = false
		return nil
	})
}

// ClearSessionCLINativeCompactionPending closes a failed or cancelled attempt.
// If this write fails the marker deliberately remains set and recovery stays
// fail-closed.
func (d *DB) ClearSessionCLINativeCompactionPending(ctx context.Context, sessionID string) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		s.CLINativeCompactionPending = false
		return nil
	})
}

// RetireSessionCLINativeCompactionRecovery atomically discards stale external
// resume authority when the pending recovery cannot run through a compatible
// resumer. A failed write leaves the old state and marker untouched.
func (d *DB) RetireSessionCLINativeCompactionRecovery(ctx context.Context, sessionID string) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		s.CLISessionID = ""
		s.CLISentMsgCount = 0
		s.CLICompactMsgCount = 0
		s.CLINativeCompactionPending = false
		return nil
	})
}

// SetSessionCLICompactBoundary records the transcript length the provider's own
// context was compacted at (see Session.CLICompactMsgCount). Monotonic: a later
// compaction always moves the boundary forward, and a stale/smaller value is
// ignored rather than rewinding the baseline. Does not bump UpdatedAt —
// bookkeeping must not reorder the session list.
func (d *DB) SetSessionCLICompactBoundary(ctx context.Context, sessionID string, msgCount int) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		if msgCount > s.CLICompactMsgCount {
			s.CLICompactMsgCount = msgCount
		}
		return nil
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
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return false, err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
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
	delete(d.messages, sessionID)
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
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Session, 0, len(d.sessions))
	for _, s := range d.sessions {
		s = normalizeSessionMeta(s)
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

	// Serialise on THIS session's transcript lock, not on the global store lock:
	// two appends to the same session are ordered, appends to different sessions
	// run concurrently, and no reader of any unrelated entity waits for a disk
	// write. Held across both the file write and the in-memory commit below, so
	// the order of lines in the file is the order of messages in RAM.
	tl := d.transcriptLock(m.SessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(m.SessionID)
	if err != nil {
		tl.Unlock()
		return m, err
	}

	d.mu.RLock()
	s, ok := d.sessions[m.SessionID]
	msgs := append([]Message(nil), d.messages[m.SessionID]...)
	d.mu.RUnlock()
	if !ok {
		tl.Unlock()
		return m, ErrNotFound
	}
	target, _, _, err := prepareCLIReplyTarget(s, msgs, m, CLIReplyState{})
	if err != nil {
		tl.Unlock()
		return m, err
	}
	dir := d.dir(dirSessions, m.SessionID)
	walPath := filepath.Join(dir, cliReplyWALFile)
	toolDelta := target.ToolCallCount - s.ToolCallCount
	wal := cliReplyWAL{Version: cliReplyWALVersion, TxnID: m.ID, Message: m}
	wal, err = d.reserveActivitySequence(dir, wal, 1, int64(toolDelta))
	if err != nil {
		tl.Unlock()
		return m, fmt.Errorf("prepare message recovery: %w", err)
	}

	// Persist BEFORE publishing in memory. The failure this ordering rules out is
	// the one 66d1324d had to repair with a rollback: a message that the UI shows
	// and automations fire for, which never reached the transcript and vanishes on
	// the next restart (the counters are recomputed from the file). Writing first
	// means a failed append leaves nothing to undo — the message was never visible,
	// no activity hook fired, and the caller gets the error.
	//
	// Hot path: append only the new message line (O(1)) instead of rewriting the
	// whole conversation file (which was O(n) per message → O(n²) per session).
	// The header line keeps a stale MessageCount/UpdatedAt on disk; both are
	// recomputed from the message lines on load and refreshed by the next full
	// rewrite (title/summary change).
	if appendErr := d.appendMessageLine(m.SessionID, m); appendErr != nil {
		// AddMessage's contract is "a failed append leaves nothing behind". Unlike
		// AddMessageWithCLIState — whose WAL is the recovery record for a multi-file
		// commit and is meant to outlive a crash — this WAL only covers the single
		// line that just failed to land. Leaving it would make every later operation
		// on this session replay a message the caller was already told did not
		// persist, and a permanently unwritable transcript would keep failing there.
		if removeErr := durableRemove(walPath); removeErr != nil {
			tl.Unlock()
			return m, errors.Join(appendErr, fmt.Errorf("retire message recovery: %w", removeErr))
		}
		tl.Unlock()
		return m, appendErr
	}
	sig := cliReplyActivitySignal(wal, target)
	if err := persistCLIReplyActivity(dir, sig); err != nil {
		tl.Unlock()
		return m, fmt.Errorf("persist message activity: %w", err)
	}
	if err := durableRemove(walPath); err != nil {
		tl.Unlock()
		return m, fmt.Errorf("retire message recovery: %w", err)
	}

	d.mu.Lock()
	s, ok = d.sessions[m.SessionID]
	if !ok {
		// Unreachable while the transcript lock is held (DeleteSession takes it
		// too), but a session that disappeared must not be resurrected in memory.
		d.mu.Unlock()
		tl.Unlock()
		return m, ErrNotFound
	}
	d.messages[m.SessionID] = append(d.messages[m.SessionID], m)
	s.MessageCount++
	// Sum this message's executed tool calls into the session's lifetime tool
	// counter (backs counter automations with metric "tool"). Only assistant
	// messages carry tool steps; a user/system append contributes 0.
	committedToolDelta := 0
	if m.Role == "assistant" {
		committedToolDelta = countToolSteps(m.Steps)
		s.ToolCallCount += committedToolDelta
	}
	s.UpdatedAt = m.CreatedAt
	// An agent reply marks the session unread; the UI clears it when opened.
	// A machine-written transcript is exempt: it is hidden from the default
	// sessions view, so an unread badge raised there could never be cleared.
	if m.Role == "assistant" && !IsMachineTranscriptKind(s.Kind) {
		s.Unread = true
	}
	// Keep the participant roster in sync: any agent that authors a message or is
	// addressed by one joins the thread. The human "user" and broadcast ("*") are
	// implicit and never stored in the roster.
	s.Participants = addParticipant(s.Participants, m.AuthorKind, m.AuthorID)
	s.Participants = addParticipant(s.Participants, AuthorAgent, m.RecipientID)
	d.sessions[s.ID] = s
	d.mu.Unlock()
	tl.Unlock()
	d.deliverRecoveredCLIReplyActivity(recovered, m.SessionID)
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		slog.Error("durable message activity delivery deferred", "component", "db", "session", m.SessionID, "event", sig.EventID, "error", err)
	}
	return m, nil
}

// CLIReplyState is session bookkeeping committed with an assistant reply.
// Update flags distinguish an absent update from a deliberate zero value.
type CLIReplyState struct {
	UpdateResume                 bool
	RetireResume                 bool
	ResumeSessionID              string
	ResumeSentMsgCount           int
	UpdateCompactBoundary        bool
	CompactMsgCount              int
	ClearNativeCompactionPending bool
}

// AddMessageWithCLIState persists a reply and its CLI resume/compaction state
// through a session-scoped WAL. Open replays any interrupted transaction, so
// every crash phase converges to exactly one reply and its matching CLI state.
func (d *DB) AddMessageWithCLIState(ctx context.Context, m Message, state CLIReplyState) (Message, error) {
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
	m.NormalizeParticipants()

	tl := d.transcriptLock(m.SessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(m.SessionID)
	if err != nil {
		tl.Unlock()
		return m, err
	}
	d.mu.RLock()
	s, ok := d.sessions[m.SessionID]
	msgs := append([]Message(nil), d.messages[m.SessionID]...)
	d.mu.RUnlock()
	if !ok {
		tl.Unlock()
		return m, ErrNotFound
	}
	if err := validateCLIReplyState(state); err != nil {
		tl.Unlock()
		return m, err
	}
	dir := d.dir(dirSessions, m.SessionID)
	walPath := filepath.Join(dir, cliReplyWALFile)
	if _, err := os.Stat(walPath); err == nil {
		tl.Unlock()
		return m, errors.New("CLI reply recovery transaction remains pending")
	} else if !os.IsNotExist(err) {
		tl.Unlock()
		return m, err
	}
	target, targetMsgs, _, err := prepareCLIReplyTarget(s, msgs, m, state)
	if err != nil {
		tl.Unlock()
		return m, err
	}
	toolDelta := target.ToolCallCount - s.ToolCallCount
	wal := cliReplyWAL{Version: cliReplyWALVersion, TxnID: m.ID, Message: m, State: state}
	wal, err = d.reserveActivitySequence(dir, wal, 1, int64(toolDelta))
	if err != nil {
		tl.Unlock()
		return m, fmt.Errorf("prepare CLI reply recovery: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnPrepared); err != nil {
			tl.Unlock()
			return m, err
		}
	}
	if err := writeDurableMessages(dir, targetMsgs); err != nil {
		tl.Unlock()
		return m, fmt.Errorf("persist CLI reply transcript: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnMessage); err != nil {
			tl.Unlock()
			return m, err
		}
	}
	if err := writeDurableSessionHeader(dir, target); err != nil {
		tl.Unlock()
		return m, fmt.Errorf("persist CLI reply state: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnHeader); err != nil {
			tl.Unlock()
			return m, err
		}
	}
	sig := cliReplyActivitySignal(wal, target)
	if err := persistCLIReplyActivity(dir, sig); err != nil {
		tl.Unlock()
		return m, fmt.Errorf("persist CLI reply activity: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnActivity); err != nil {
			tl.Unlock()
			return m, err
		}
	}
	if err := durableRemove(walPath); err != nil {
		tl.Unlock()
		return m, fmt.Errorf("retire CLI reply recovery: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnRetired); err != nil {
			tl.Unlock()
			return m, err
		}
	}
	d.mu.Lock()
	d.messages[m.SessionID] = targetMsgs
	d.sessions[target.ID] = target
	d.mu.Unlock()
	tl.Unlock()
	d.deliverRecoveredCLIReplyActivity(recovered, m.SessionID)
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		slog.Error("durable CLI reply activity delivery deferred", "component", "db", "session", m.SessionID, "event", sig.EventID, "error", err)
	}
	return m, nil
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

// appendMessageLine appends a single encoded message line to a session's
// transcript file, creating it on the first message (a session's directory is
// made at creation time, but messages.jsonl only appears once it has one).
//
// The caller must hold the session's transcript lock (or be boot, which is
// single-threaded). It deliberately does NOT require d.mu — that is the whole
// point of the split: the disk write happens outside the global lock.
func (d *DB) appendMessageLine(sessionID string, m Message) error {
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
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
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
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return 0, err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
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
		// The fold chain that produced that summary is gone with it; the next fold
		// starts a fresh one, so its ordinal must start over too.
		s.CompactionCount = 0
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
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(d.dir(dirSessions), e.Name()))
		}
	}
	// The dominant boot cost: two cold file opens per session (session.json +
	// messages.jsonl), each taxed ~15 ms by the AV filter driver on Windows. Read
	// them concurrently (see loadpar.go) and populate the maps serially after.
	type loadedSession struct {
		s    Session
		msgs []Message
		skip bool // absent or headerless directory — not an error
	}
	loaded, err := parallelLoad(dirs, func(dir string) (loadedSession, error) {
		if err := d.recoverCLIReplyTransaction(dir); err != nil {
			if !errors.Is(err, ErrCLIReplyRecoveryDegraded) {
				return loadedSession{}, err
			}
			slog.Error("session CLI reply recovery degraded", "component", "db", "session_dir", dir, "error", err)
		}
		s, msgs, err := readSessionDir(dir)
		if err != nil {
			// A directory that vanished between ReadDir and the open is skipped, as
			// before; any other read/parse failure stays fatal.
			if os.IsNotExist(err) {
				return loadedSession{skip: true}, nil
			}
			return loadedSession{}, err
		}
		if s.ID == "" { // empty/headerless session — nothing usable
			return loadedSession{skip: true}, nil
		}
		return loadedSession{s: s, msgs: msgs}, nil
	})
	if err != nil {
		return err
	}
	for _, l := range loaded {
		if l.skip {
			continue
		}
		d.sessions[l.s.ID] = l.s
		d.messages[l.s.ID] = l.msgs
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
	// Lineage backfill for headers written before schema version 4: derive the
	// origin IN MEMORY so every loaded session answers Lineage() the same way a
	// new one does. The header is not rewritten for this — it lands on disk only
	// when some later mutation persists the row anyway.
	if s.Origin == nil {
		o := deriveOrigin(s)
		o.At = s.CreatedAt
		s.Origin = &o
	}
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
