package api

import (
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/memory"
)

// handleListMemories returns an agent's memories, optionally filtered by ?kind=.
func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	// Default to the user-facing display kinds so the always-in-context core
	// blocks (core_persona/core_human) don't leak into the flat list — they have
	// their own card. An explicit ?kind= still passes through for debugging.
	kinds := memory.DisplayKinds()
	if k := r.URL.Query().Get("kind"); k != "" {
		kinds = []string{k}
	}

	mems, err := ws(r).Runtime.Memory().List(r.Context(), agentID, kinds...)
	if writeDBError(w, err, "") {
		return
	}
	if mems == nil {
		mems = []db.KnowledgeSource{}
	}
	writeJSON(w, http.StatusOK, mems)
}

type createMemoryReq struct {
	Content string `json:"content"`
	Kind    string `json:"kind"`
}

// handleCreateMemory stores a user-provided memory (defaults to a document).
func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	var req createMemoryReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.Kind == "" {
		req.Kind = db.MemoryDocument
	}
	if _, err := wsp.DB.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	mem, err := wsp.Runtime.Memory().Remember(r.Context(), agentID, req.Kind, req.Content)
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("memory added", "agent", agentID, "kind", req.Kind, "id", mem.ID)
	writeJSON(w, http.StatusCreated, mem)
}

// handleGetCore returns the agent's MemGPT-style core memory, split into its
// persona and human sections (each "" when unset).
func (s *Server) handleGetCore(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	persona, human, err := ws(r).Runtime.Memory().ReadCoreSections(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"persona": persona, "human": human})
}

// putCoreReq carries the two sections. Each is a pointer so a client can update
// just one section; a non-nil empty string clears that section.
type putCoreReq struct {
	Persona *string `json:"persona"`
	Human   *string `json:"human"`
}

// handlePutCore replaces the agent's core sections (upsert). Only the sections
// present in the body are written; the single-row-per-section invariant is
// preserved by the store. Returns the resulting persona/human.
func (s *Server) handlePutCore(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	var req putCoreReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if _, err := wsp.DB.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	mem := wsp.Runtime.Memory()
	if req.Persona != nil {
		if err := mem.WriteCore(r.Context(), agentID, memory.CorePersona, *req.Persona); writeDBError(w, err, "") {
			return
		}
	}
	if req.Human != nil {
		if err := mem.WriteCore(r.Context(), agentID, memory.CoreHuman, *req.Human); writeDBError(w, err, "") {
			return
		}
	}
	persona, human, err := mem.ReadCoreSections(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("core memory written", "agent", agentID)
	writeJSON(w, http.StatusOK, map[string]string{"persona": persona, "human": human})
}

// handleDeleteMemory removes a memory by id.
func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := ws(r).Runtime.Memory().Delete(r.Context(), id)
	if writeDBError(w, err, "memory not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "result": "deleted"})
}

// handleReflect triggers the agent's reflection (dream cycle) over its journal.
func (s *Server) handleReflect(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	reflection, err := ws(r).Runtime.Reflect(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, reflection)
}

type recallReq struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// handleRecall previews which memories an agent would recall for a query — a
// debugging/inspection aid for the memory system.
func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	var req recallReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	hits, err := ws(r).Runtime.Memory().Recall(r.Context(), agentID, req.Query, req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, map[string]any{
			"id":      h.Source.ID,
			"kind":    h.Source.Kind,
			"content": h.Source.Content,
			"score":   h.Score,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
