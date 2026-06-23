package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := ws(r).DB.ListTasks(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if tasks == nil {
		tasks = []db.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

type createTaskReq struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	OwnerAgentID string `json:"ownerAgentId"`
	FlowID       string `json:"flowId"`
	BoardState   string `json:"boardState"`
	Dependencies string `json:"dependencies"` // JSON array of task IDs
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ctx := r.Context()
	req.Title = strings.TrimSpace(req.Title)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.Description = strings.TrimSpace(req.Description)
	// On the board, a task is created from a description (or prompt); the title is
	// auto-generated when omitted. Require at least one source of content.
	if req.Title == "" && req.Description == "" && req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "title, description or prompt is required")
		return
	}
	if req.BoardState != "" && !db.IsValidBoardKey(req.BoardState) {
		writeError(w, http.StatusBadRequest, "invalid board state")
		return
	}

	title := req.Title
	if title == "" {
		// Generate the title from the description (board) or prompt (legacy/agents).
		source := req.Description
		if source == "" {
			source = req.Prompt
		}
		gen, err := ws(r).Runtime.TitleFor(ctx, req.OwnerAgentID, source)
		if err != nil {
			s.logger.Warn("task title generation degraded", "error", err)
		}
		title = gen
	}

	task, err := ws(r).DB.CreateTask(ctx, db.Task{
		Title:        title,
		Description:  req.Description,
		Prompt:       req.Prompt,
		OwnerAgentID: req.OwnerAgentID,
		FlowID:       req.FlowID,
		BoardState:   req.BoardState,
		Dependencies: req.Dependencies,
	})
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("task created", "task", task.ID, "title", task.Title, "owner", req.OwnerAgentID)
	writeJSON(w, http.StatusCreated, task)
}

type updateTaskReq struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	Prompt       *string `json:"prompt"`
	OwnerAgentID *string `json:"ownerAgentId"`
	FlowID       *string `json:"flowId"`
	BoardState   *string `json:"boardState"`
	Dependencies *string `json:"dependencies"` // JSON array of task IDs
}

// handleUpdateTask edits any subset of a task's mutable fields (PATCH-like PUT).
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	task, err := wsp.DB.GetTask(r.Context(), id)
	if writeDBError(w, err, "task not found") {
		return
	}
	oldBoard := task.BoardState

	var req updateTaskReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Prompt != nil {
		task.Prompt = *req.Prompt
	}
	if req.OwnerAgentID != nil {
		task.OwnerAgentID = *req.OwnerAgentID
	}
	if req.FlowID != nil {
		task.FlowID = *req.FlowID
	}
	if req.BoardState != nil {
		if !db.IsValidBoardKey(*req.BoardState) {
			writeError(w, http.StatusBadRequest, "invalid board state")
			return
		}
		task.BoardState = *req.BoardState
	}
	if req.Dependencies != nil {
		task.Dependencies = *req.Dependencies
	}

	if err := wsp.DB.UpdateTask(r.Context(), task); writeDBError(w, err, "task not found") {
		return
	}
	// Board move → generic "board" change event so other windows badge the
	// Görevler view + workspace label (the user's own window clears it on view).
	if req.BoardState != nil && *req.BoardState != oldBoard {
		publishEntityChange(wsp, "board", "Görev taşındı: "+task.Title, *req.BoardState,
			map[string]string{"view": "board", "taskId": task.ID})
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	err := ws(r).DB.DeleteTask(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "task not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("id"), "result": "deleted"})
}

// handleGenerateTaskTitle (re)generates a task's title from its prompt (falling
// back to description/title) and persists it.
func (s *Server) handleGenerateTaskTitle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	task, err := wsp.DB.GetTask(ctx, id)
	if writeDBError(w, err, "task not found") {
		return
	}

	source := strings.TrimSpace(task.Prompt)
	if source == "" {
		source = strings.TrimSpace(task.Description)
	}
	if source == "" {
		source = strings.TrimSpace(task.Title)
	}
	if source == "" {
		writeError(w, http.StatusBadRequest, "no content to generate a title from")
		return
	}

	title, genErr := wsp.Runtime.TitleFor(ctx, task.OwnerAgentID, source)
	if genErr != nil {
		s.logger.Warn("task title generation degraded", "task", id, "error", genErr)
	}
	task.Title = title
	if err := wsp.DB.UpdateTask(ctx, task); writeDBError(w, err, "task not found") {
		return
	}
	writeJSON(w, http.StatusOK, task)
}
