package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/memory"
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

	req, ok := bindJSON[createMemoryReq](w, r)
	if !ok {
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

// handleGetCore returns the agent's MemGPT-style core memory as an ordered list
// of named blocks (label, description, content, charLimit, readOnly). Defaults to
// the persona/human pair when the agent has defined no blocks.
func (s *Server) handleGetCore(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	blocks, err := ws(r).Runtime.Memory().ReadCoreBlocks(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	if blocks == nil {
		blocks = []memory.BlockView{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

// putCoreReq carries content writes keyed by block label. Only the labels present
// are written; the single-row-per-label invariant is preserved by the store. The
// read-only flag is NOT enforced here — humans edit any block through the UI; the
// flag only stops the agent (tool layer).
type putCoreReq struct {
	Blocks map[string]string `json:"blocks"`
}

// handlePutCore writes core block content by label. Unknown labels and over-limit
// content are 400s with an actionable message. Returns the resulting blocks.
func (s *Server) handlePutCore(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	req, ok := bindJSON[putCoreReq](w, r)
	if !ok {
		return
	}
	if _, err := wsp.DB.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	mem := wsp.Runtime.Memory()
	for label, content := range req.Blocks {
		if err := mem.WriteCore(r.Context(), agentID, label, content); err != nil {
			if writeCoreWriteError(w, err) {
				return
			}
			if writeDBError(w, err, "") {
				return
			}
		}
	}
	blocks, err := mem.ReadCoreBlocks(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("core memory written", "agent", agentID, "blocks", len(req.Blocks))
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

// defineCoreBlockReq defines or updates a block. CharLimit 0 means the default.
type defineCoreBlockReq struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	CharLimit   int    `json:"charLimit"`
	ReadOnly    bool   `json:"readOnly"`
}

// handleDefineCoreBlock creates or updates a named core block's definition (seeds
// persona/human first when the agent had none). Returns the resulting blocks.
func (s *Server) handleDefineCoreBlock(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	req, ok := bindJSON[defineCoreBlockReq](w, r)
	if !ok {
		return
	}
	if _, err := wsp.DB.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	mem := wsp.Runtime.Memory()
	err := mem.DefineCoreBlock(r.Context(), agentID, db.CoreBlock{
		Label:       req.Label,
		Description: req.Description,
		CharLimit:   req.CharLimit,
		ReadOnly:    req.ReadOnly,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	blocks, err := mem.ReadCoreBlocks(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("core block defined", "agent", agentID, "label", req.Label)
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

// handleDeleteCoreBlock removes a block definition and its content. Returns the
// resulting blocks (which fall back to persona/human if none remain).
func (s *Server) handleDeleteCoreBlock(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	label := r.PathValue("label")
	mem := ws(r).Runtime.Memory()
	if err := mem.DeleteCoreBlock(r.Context(), agentID, label); writeDBError(w, err, "") {
		return
	}
	blocks, err := mem.ReadCoreBlocks(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("core block deleted", "agent", agentID, "label", label)
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

// writeCoreWriteError maps the typed core-write errors to a 400 with an
// actionable message. Returns true when it handled (wrote) the error.
func writeCoreWriteError(w http.ResponseWriter, err error) bool {
	var full *memory.CoreBlockFullError
	if errors.As(err, &full) {
		writeError(w, http.StatusBadRequest, full.Error())
		return true
	}
	if errors.Is(err, memory.ErrUnknownCoreBlock) {
		writeError(w, http.StatusBadRequest, err.Error())
		return true
	}
	return false
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

	req, ok := bindJSON[recallReq](w, r)
	if !ok {
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
