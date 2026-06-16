package api

import (
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
)

// handleListMemories returns an agent's memories, optionally filtered by ?kind=.
func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	kinds := []string{}
	if k := r.URL.Query().Get("kind"); k != "" {
		kinds = append(kinds, k)
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
	writeJSON(w, http.StatusCreated, mem)
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
