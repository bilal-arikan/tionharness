package agent

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Prompt epoch — a frozen snapshot of a session's cacheable prompt prefix.
//
// The Anthropic prompt cache is prefix-based (tools → system → messages); a
// single changed byte in the tool schemas or the static system prompt re-writes
// everything downstream at the cache-write premium. The prefix is assembled from
// LIVE state every turn, so any mid-session drift — a skill install, a settings
// edit, an MCP tools/list_changed, a capability toggle, another agent joining
// the thread — silently busts the whole cache.
//
// The epoch layer (external-context-agent's "frozen snapshot" pattern, generalised)
// freezes the assembled prefix per (session, agent) at first use and serves the
// SAME bytes every turn. Live changes keep landing on disk immediately; the
// prompt adopts them only at moments where the cache is already (or about to
// be) invalid:
//
//   - a fresh session / handoff (no snapshot yet)
//   - a compaction fold (the history breakpoint is busted anyway)
//   - a model change (a different cache key entirely)
//   - a working-dir change (update_session promises "takes effect next turn")
//   - the participant set changing (multi-agent labelling rewrites history)
//   - a TTL-cold gap (nothing cached is left to protect)
//   - an explicit refresh (/refresh-context or update_session refresh_context)
//
// Between those points a drifted live prefix only raises a one-line stale note
// on the VOLATILE dynamic suffix (never the cached prefix). Safety is not
// prompt-driven: a disabled tool already fails closed at the registry, and the
// permission gate always evaluates live state.
//
// Snapshots live in memory (Runtime.epochCache) and in a per-session sidecar
// (prompt_epoch.json, next to session.jsonl) so they survive restarts — the
// provider-side cache does too, so a restart must not re-derive a different
// prefix. A corrupt sidecar fails open to live composition.

// promptEpochVersion is the sidecar format version.
const promptEpochVersion = 1

// promptEpochAdoptAfter is the idle gap after which pending changes are adopted
// for free: past the provider cache TTL (1h on the native anthropic path,
// providers.cacheTTL) nothing cached remains, so rebuilding the prefix costs no
// extra invalidation.
const promptEpochAdoptAfter = time.Hour

// PromptEpochStaleNote is the one-line notice injected into the VOLATILE dynamic
// suffix while the frozen snapshot lags live state (hermes' "the tool response
// shows live state" trade-off, made explicit to the agent). Shared by the chat,
// headless, and tool-loop paths; the marker tag also guards double-append.
const PromptEpochStaleNote = "<context_snapshot_note>Parts of your static context (persona, instructions, " +
	"skills or tool catalog) changed mid-session; this prompt still shows the session-start snapshot. Pending " +
	"changes apply on the next context refresh — automatic after a compaction or an idle gap, or on demand via " +
	"the /refresh-context chat command (or update_session with refresh_context). Behaviour is safe meanwhile: " +
	"disabled tools fail closed at execution, newly added tools become callable after the refresh.</context_snapshot_note>"

// promptEpochEntry is one (session, agent) snapshot. The exported-to-JSON fields
// persist in the sidecar; the lowercase ones are in-memory session state.
type promptEpochEntry struct {
	Model      string              `json:"model"`
	WorkDir    string              `json:"workDir,omitempty"`
	MultiAgent bool                `json:"multiAgent,omitempty"`
	System     string              `json:"system,omitempty"`
	Tools      []providers.ToolDef `json:"tools,omitempty"`
	CreatedAt  int64               `json:"createdAt"`
	LastUsedAt int64               `json:"lastUsedAt"`

	systemStale   bool  // live static prefix drifted from the frozen one
	toolsStale    bool  // live tool defs drifted from the frozen ones
	staleNotified bool  // "stale" debug event already emitted for this drift
	persistedUse  int64 // LastUsedAt value last flushed to the sidecar (write throttle)

	// systemChange / toolsChange are the computed diffs (frozen ↔ live) for the
	// current drift episode, recomputed each stale turn. They feed both the
	// agent-facing suffix note (every stale turn) and the chat context_change
	// step (once per episode). Kept separate because they are computed on
	// different paths (static prefix vs. tool loop) and merged on access.
	systemChange *ContextChange
	toolsChange  *ContextChange
	// changePending gates the one-shot chat step: set when a fresh drift is first
	// detected, cleared by ConsumeContextChange. Re-armed when the drift clears.
	changePending bool
}

// combinedChange merges the entry's system + tools diffs into one fresh value
// (non-mutating: the stored diffs are reused across turns until refresh).
func (e *promptEpochEntry) combinedChange() *ContextChange {
	if e.systemChange.Empty() && e.toolsChange.Empty() {
		return nil
	}
	out := &ContextChange{}
	for _, src := range []*ContextChange{e.systemChange, e.toolsChange} {
		if src == nil {
			continue
		}
		out.Added += src.Added
		out.Removed += src.Removed
		out.Truncated += src.Truncated
		for _, a := range src.Areas {
			out.appendArea(a)
		}
	}
	return out
}

// epochUsePersistEvery throttles the sidecar write that only refreshes
// LastUsedAt: the TTL-cold check runs at hour granularity, so persisting the
// timestamp every few minutes is plenty (a restart may then see it up to this
// much stale — worst case an epoch is adopted slightly early, which is safe).
const epochUsePersistEvery = 5 * time.Minute

// promptEpochFilePayload is the sidecar document (all agents of one session).
type promptEpochFilePayload struct {
	Version int                          `json:"version"`
	Entries map[string]*promptEpochEntry `json:"entries"`
}

// epochEntries returns the in-memory entry map for a session, loading the
// sidecar on first touch. Caller must hold epochMu.
func (r *Runtime) epochEntriesLocked(sessionID string) map[string]*promptEpochEntry {
	if r.epochCache == nil {
		r.epochCache = map[string]map[string]*promptEpochEntry{}
	}
	if m, ok := r.epochCache[sessionID]; ok {
		return m
	}
	m := map[string]*promptEpochEntry{}
	if raw, ok, err := r.db.ReadPromptEpoch(sessionID); err == nil && ok {
		var f promptEpochFilePayload
		// A corrupt or future-versioned sidecar fails open: start fresh (live
		// composition re-freezes this turn) rather than blocking the turn.
		if json.Unmarshal(raw, &f) == nil && f.Version == promptEpochVersion && f.Entries != nil {
			m = f.Entries
		}
	}
	r.epochCache[sessionID] = m
	return m
}

// persistEpochLocked writes a session's entries to the sidecar. Best-effort:
// a write failure only degrades restart continuity, never the turn. Caller
// must hold epochMu.
func (r *Runtime) persistEpochLocked(sessionID string) {
	m := r.epochCache[sessionID]
	if len(m) == 0 {
		_ = r.db.ClearPromptEpoch(sessionID)
		return
	}
	_ = r.db.WritePromptEpoch(sessionID, promptEpochFilePayload{Version: promptEpochVersion, Entries: m})
}

// EpochStaticSystem returns the static system prefix for one turn: the frozen
// snapshot when a valid epoch exists, else the freshly built live prefix (which
// becomes the new snapshot). build must be PURE — side effects belong at the
// call site, because a frozen turn never invokes it for the prefix (it is still
// called once per turn for drift detection, so it must also be cheap).
//
// forceAdopt marks a moment where the message-history cache is already busted
// (a compaction fold) so pending changes are adopted for free. The returned
// stale flag reports live drift the snapshot is holding back — the caller
// surfaces it on the volatile dynamic suffix (PromptEpochStaleNote).
func (r *Runtime) EpochStaticSystem(ctx context.Context, sessionID string, a db.Agent, multiAgent, forceAdopt bool, workDir string, build func() string) (system string, stale bool) {
	if sessionID == "" || !r.PromptEpochEnabled() {
		return build(), false
	}
	now := time.Now()

	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	entries := r.epochEntriesLocked(sessionID)
	e := entries[a.ID]

	reason := ""
	switch {
	case e == nil:
		reason = "created"
	case forceAdopt:
		reason = "compaction"
	case e.Model != a.Model:
		reason = "model-changed"
	case e.WorkDir != workDir:
		reason = "workdir-changed"
	case e.MultiAgent != multiAgent:
		reason = "participants-changed"
	case now.UnixMilli()-e.LastUsedAt > promptEpochAdoptAfter.Milliseconds():
		reason = "ttl-cold"
	}
	if reason != "" {
		fresh := &promptEpochEntry{
			Model:        a.Model,
			WorkDir:      workDir,
			MultiAgent:   multiAgent,
			System:       build(),
			CreatedAt:    now.UnixMilli(),
			LastUsedAt:   now.UnixMilli(),
			persistedUse: now.UnixMilli(),
		}
		entries[a.ID] = fresh
		r.persistEpochLocked(sessionID)
		// Stamp the session id: the compose paths call in before the turn stamps
		// ctx, and emitDebug drops events whose ctx carries no session.
		r.emitDebug(WithSessionID(ctx, sessionID), db.DebugEvent{Type: db.DebugEpoch, AgentID: a.ID, Name: reason, Detail: "static prefix (re)frozen; tool defs re-freeze on next tool turn"})
		return fresh.System, false
	}

	// Serving the frozen snapshot: detect (but do not adopt) live drift, and
	// compute the paragraph-level diff so the note/step can show WHAT changed.
	live := build()
	e.systemStale = live != e.System
	if e.systemStale {
		e.systemChange = diffSystemPrefix(e.System, live)
	} else {
		e.systemChange = nil
	}
	r.noteEpochStaleLocked(ctx, sessionID, a.ID, e)
	e.LastUsedAt = now.UnixMilli()
	if e.LastUsedAt-e.persistedUse > epochUsePersistEvery.Milliseconds() {
		e.persistedUse = e.LastUsedAt
		r.persistEpochLocked(sessionID)
	}
	return e.System, e.systemStale || e.toolsStale
}

// EpochToolDefs returns the frozen tool defs for one turn, freezing the live
// set on first use (the epoch entry is created by EpochStaticSystem, which every
// turn-compose path calls first; a missing entry falls open to live defs so the
// tool loop never depends on call order). The returned slice must be treated as
// read-only. stale reports live drift held back by the snapshot.
func (r *Runtime) EpochToolDefs(ctx context.Context, sessionID string, a db.Agent, build func() []providers.ToolDef) (defs []providers.ToolDef, stale bool) {
	if sessionID == "" || !r.PromptEpochEnabled() {
		return nil, false
	}
	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	e := r.epochEntriesLocked(sessionID)[a.ID]
	if e == nil || e.Model != a.Model {
		// No epoch for this agent yet (pure tool-loop entry without a composed
		// system, or a model flip mid-order): fail open to live defs and let the
		// next EpochStaticSystem call establish the snapshot.
		return nil, false
	}
	if e.Tools == nil {
		e.Tools = build()
		if e.Tools == nil {
			e.Tools = []providers.ToolDef{}
		}
		r.persistEpochLocked(sessionID)
		return e.Tools, e.systemStale || e.toolsStale
	}
	live := build()
	e.toolsStale = toolDefsSig(live) != toolDefsSig(e.Tools)
	if e.toolsStale {
		e.toolsChange = diffToolNames(toolDefNames(e.Tools), toolDefNames(live))
	} else {
		e.toolsChange = nil
	}
	r.noteEpochStaleLocked(ctx, sessionID, a.ID, e)
	return e.Tools, e.systemStale || e.toolsStale
}

// toolDefNames extracts the ordered tool names from a def list (for the human-
// readable added/removed tool diff; the byte-level signature stays toolDefsSig).
func toolDefNames(defs []providers.ToolDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

// RefreshPromptEpoch drops a session's snapshots so the next turn re-freezes
// from live state — the explicit adopt trigger behind the /refresh-context chat
// command and update_session's refresh_context field. A deliberate full-prefix
// re-write; safe to call for a session that has no epoch.
func (r *Runtime) RefreshPromptEpoch(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	if r.epochCache != nil {
		delete(r.epochCache, sessionID)
	}
	_ = r.db.ClearPromptEpoch(sessionID)
	r.emitDebug(WithSessionID(ctx, sessionID), db.DebugEvent{Type: db.DebugEpoch, Name: "refreshed", Detail: "snapshot cleared; next turn recomposes the prefix from live state"})
}

// PromptEpochStale reports whether the frozen snapshot for (session, agent) is
// currently holding back live drift — consulted by dynamic-suffix builders that
// run after the system/tool epoch calls (the headless path).
func (r *Runtime) PromptEpochStale(sessionID, agentID string) bool {
	if sessionID == "" || !r.PromptEpochEnabled() {
		return false
	}
	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	if m, ok := r.epochCache[sessionID]; ok {
		if e := m[agentID]; e != nil {
			return e.systemStale || e.toolsStale
		}
	}
	return false
}

// noteEpochStaleLocked emits the one-time "stale" debug event on a fresh drift
// and re-arms once the drift disappears (e.g. the change was reverted).
func (r *Runtime) noteEpochStaleLocked(ctx context.Context, sessionID, agentID string, e *promptEpochEntry) {
	stale := e.systemStale || e.toolsStale
	if stale && !e.staleNotified {
		e.staleNotified = true
		// Arm the one-shot chat step for this fresh drift episode; ConsumeContextChange
		// clears it after emitting once (subsequent stale turns only refresh the note).
		e.changePending = true
		what := "system"
		if e.toolsStale && e.systemStale {
			what = "system+tools"
		} else if e.toolsStale {
			what = "tools"
		}
		r.emitDebug(WithSessionID(ctx, sessionID), db.DebugEvent{Type: db.DebugEpoch, AgentID: agentID, Name: "stale", Detail: what + " drifted from the frozen snapshot; pending until the next refresh"})
	}
	if !stale {
		e.staleNotified = false
		e.changePending = false
	}
}

// PromptEpochContextNote returns the agent-facing dynamic-suffix note for a
// session/agent while the frozen snapshot lags live state: a compact diff of
// what changed when known, else the generic stale note, else "" when in sync.
// Wrapped in the shared <context_snapshot_note> marker so it never double-
// appends with the tool-loop's fallback and never busts the cached prefix.
func (r *Runtime) PromptEpochContextNote(sessionID, agentID string) string {
	if sessionID == "" || !r.PromptEpochEnabled() {
		return ""
	}
	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	e := r.epochEntry(sessionID, agentID)
	if e == nil || !(e.systemStale || e.toolsStale) {
		return ""
	}
	return e.combinedChange().SuffixNote()
}

// ConsumeContextChange returns the drift diff for the chat context_change step
// ONCE per drift episode, then disarms it (later stale turns only refresh the
// suffix note). Returns nil when no fresh change is pending.
func (r *Runtime) ConsumeContextChange(sessionID, agentID string) *ContextChange {
	if sessionID == "" || !r.PromptEpochEnabled() {
		return nil
	}
	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	e := r.epochEntry(sessionID, agentID)
	if e == nil || !e.changePending {
		return nil
	}
	e.changePending = false
	c := e.combinedChange()
	if c.Empty() {
		return nil
	}
	return c
}

// epochEntry returns the in-memory entry for (session, agent) without loading
// the sidecar (callers here run after EpochStaticSystem has established it).
// Caller must hold epochMu.
func (r *Runtime) epochEntry(sessionID, agentID string) *promptEpochEntry {
	if r.epochCache == nil {
		return nil
	}
	if m, ok := r.epochCache[sessionID]; ok {
		return m[agentID]
	}
	return nil
}

// toolDefsSig hashes the fields of a tool-def list that reach the wire (and thus
// the cache prefix). Same fields the provider serialises: name, description,
// schema, defer/strict flags — in order, since order is part of the prefix.
func toolDefsSig(defs []providers.ToolDef) uint64 {
	h := fnv.New64a()
	for _, t := range defs {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(t.Name))
		_, _ = h.Write([]byte{1})
		_, _ = h.Write([]byte(t.Description))
		_, _ = h.Write([]byte{2})
		_, _ = h.Write(t.InputSchema)
		_, _ = h.Write([]byte(strconv.FormatBool(t.DeferLoading) + strconv.FormatBool(t.Strict)))
	}
	return h.Sum64()
}

// mergeFrozenToolDefs merges the frozen base defs with the agent's own in-turn
// activations: frozen defs keep their exact bytes (and order) so the shipped
// tools block stays cache-stable, EXCEPT a def the agent explicitly activated
// this turn, which takes its live form (native-search mode flips DeferLoading
// off on activation); live defs the agent activated that the snapshot has never
// seen are appended at the end. Deliberate agent intent pays the one-time cache
// re-write; passive drift does not.
func mergeFrozenToolDefs(frozen, live []providers.ToolDef, active map[string]bool) []providers.ToolDef {
	if len(active) == 0 {
		return frozen
	}
	liveBy := make(map[string]providers.ToolDef, len(live))
	for _, d := range live {
		liveBy[d.Name] = d
	}
	out := make([]providers.ToolDef, 0, len(frozen)+len(active))
	seen := make(map[string]bool, len(frozen))
	for _, d := range frozen {
		if active[d.Name] {
			if ld, ok := liveBy[d.Name]; ok {
				d = ld
			}
		}
		out = append(out, d)
		seen[d.Name] = true
	}
	for _, d := range live {
		if active[d.Name] && !seen[d.Name] {
			out = append(out, d)
		}
	}
	return out
}
