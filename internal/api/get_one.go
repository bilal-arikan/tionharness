package api

import (
	"net/http"
)

// get_one.go — single-item reads for the resource families that had list +
// create + update + delete but no way to fetch ONE row back.
//
// The gap mattered for anything driving the API from outside: after POST
// /api/tasks returned a card, the only way to re-read it was GET /api/tasks and
// filter client-side — O(all rows) per read, and it races a concurrent writer
// (the list is a snapshot; the row you want may have moved by the time you scan
// for it). The stores already exposed every getter used here; only the HTTP
// handlers were missing.
//
// Shape follows handleGetProvider (providers.go) and handleGetFlow (flows.go):
// the bare entity as JSON, 404 via writeDBError when absent. Deliberately NOT
// wrapped in the {items,total,...} listing envelope — that is the list contract
// (listparams.go), and a single read has no page to describe.

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, err := ws(r).DB.GetTask(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "task not found") {
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	agent, err := ws(r).DB.GetAgent(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "agent not found") {
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	sc, err := ws(r).DB.GetSchedule(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "schedule not found") {
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) handleGetAutomation(w http.ResponseWriter, r *http.Request) {
	au, err := ws(r).DB.GetAutomation(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "automation not found") {
		return
	}
	writeJSON(w, http.StatusOK, au)
}

func (s *Server) handleGetHook(w http.ResponseWriter, r *http.Request) {
	h, err := ws(r).DB.GetHook(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "hook not found") {
		return
	}
	writeJSON(w, http.StatusOK, h)
}

// handleGetMCPServer returns one MCP server config.
//
// EnvConfig/HeadersConfig may hold raw credentials, and nothing redacts them —
// but that is the EXISTING behavior of GET /api/mcp-servers, which already ships
// both fields for every row. This endpoint therefore exposes nothing the list
// did not; redaction, if it is wanted, belongs on both and behind the auth gate
// (auth.go), not bolted onto the narrower read.
func (s *Server) handleGetMCPServer(w http.ResponseWriter, r *http.Request) {
	m, err := ws(r).DB.GetMCPServer(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "mcp server not found") {
		return
	}
	writeJSON(w, http.StatusOK, m)
}
