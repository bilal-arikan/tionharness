package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// processListLimit is the default page size for the process ledger read. The
// ledger itself is bounded (procwatch.DefaultHistory), so this only keeps a
// panel refresh small; ?limit=0 returns everything retained.
const processListLimit = 200

// handleListProcesses serves the native processes TionHarness spawned on the
// agents' behalf — running first-class, recently finished from the bounded
// history (internal/procwatch).
//
// Query params: status (repeatable/comma-separated: running|succeeded|failed|
// killed|timed_out), kind (same shape: shell|shell_background|code|provider|
// mcp|hook|external), session, agent, limit.
//
// Scoping: entries are filtered to the ACTIVE workspace, plus the ones that
// carry no workspace at all. An MCP server or a startup probe is spawned outside
// any session and would otherwise be invisible in every workspace — and a
// process the panel cannot show is exactly what this feature exists to prevent.
func (s *Server) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := processListLimit
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			limit = n
		}
	}
	f := procwatch.Filter{
		SessionID: strings.TrimSpace(q.Get("session")),
		AgentID:   strings.TrimSpace(q.Get("agent")),
		Limit:     limit,
	}
	for _, v := range csvValues(q["status"]) {
		f.Statuses = append(f.Statuses, procwatch.Status(v))
	}
	for _, v := range csvValues(q["kind"]) {
		f.Kinds = append(f.Kinds, procwatch.Kind(v))
	}

	wsID := ""
	var store *db.DB
	if wsp := ws(r); wsp != nil {
		wsID = wsp.ID
		store = wsp.DB
	}
	entries := procwatch.Default().List(f)
	out := make([]procwatch.Entry, 0, len(entries))
	agents := agentOwnerResolver(r.Context(), store)
	for _, e := range entries {
		if wsID != "" && e.Owner.WorkspaceID != "" && e.Owner.WorkspaceID != wsID {
			continue
		}
		agents(&e.Owner)
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// agentOwnerResolver fills in the agent behind an owner that carries only a
// session. Processes spawned by a CLI subprocess reach the ledger through the
// Interaction MCP bridge, which knows the run's session but not its agent
// (stampRunSession); without this they would all render as "unknown agent" in
// the panel even though the session identifies one.
//
// The returned func memoizes per session id, so one panel refresh costs one
// lookup per DISTINCT session rather than one per row. A session or agent that
// no longer exists (deleted while its process outlived it) resolves to nothing
// and the row keeps the session alone — the process is still real, and hiding it
// over a missing name is the one outcome this panel must not produce.
func agentOwnerResolver(ctx context.Context, store *db.DB) func(*procwatch.Owner) {
	if store == nil {
		return func(*procwatch.Owner) {}
	}
	type resolved struct{ id, name string }
	cache := map[string]resolved{}
	return func(o *procwatch.Owner) {
		if o.SessionID == "" || o.AgentName != "" {
			return
		}
		got, ok := cache[o.SessionID]
		if !ok {
			if sess, err := store.GetSession(ctx, o.SessionID); err == nil {
				got.id = sess.AgentID
				if ag, err := store.GetAgent(ctx, sess.AgentID); err == nil {
					got.name = ag.Name
				}
			}
			cache[o.SessionID] = got
		}
		if o.AgentID == "" {
			o.AgentID = got.id
		}
		o.AgentName = got.name
	}
}

// handleStopProcess terminates one tracked process. The response says which of
// the three outcomes happened rather than flattening them into 200/404:
// stopped=true (the signal went out), stopped=false with a reason (the entry is
// already finished, or it was registered without a stop path). A process that
// refuses to die is not reported here — the entry reaches its terminal status
// when the site that started it reaps it.
func (s *Server) handleStopProcess(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := procwatch.Default().Stop(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	resp := map[string]any{"id": id, "stopped": ok}
	if !ok {
		resp["reason"] = "this process is already finished or cannot be stopped from here"
	}
	writeJSON(w, http.StatusOK, resp)
}

// csvValues flattens repeated query params that may themselves be
// comma-separated ("?status=running&status=failed" and "?status=running,failed"
// mean the same thing), dropping blanks.
func csvValues(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
