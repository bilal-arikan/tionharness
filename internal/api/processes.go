package api

import (
	"net/http"
	"strconv"
	"strings"

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
	if wsp := ws(r); wsp != nil {
		wsID = wsp.ID
	}
	entries := procwatch.Default().List(f)
	out := make([]procwatch.Entry, 0, len(entries))
	for _, e := range entries {
		if wsID != "" && e.Owner.WorkspaceID != "" && e.Owner.WorkspaceID != wsID {
			continue
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
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
