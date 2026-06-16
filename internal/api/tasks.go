package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
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
	BoardState   string `json:"boardState"`
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
	// On the board, a task is created from a prompt alone; the title is
	// auto-generated when omitted. Require at least one of the two.
	if req.Title == "" && req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt or title is required")
		return
	}
	if req.BoardState != "" && !db.ValidBoardState(req.BoardState) {
		writeError(w, http.StatusBadRequest, "invalid board state")
		return
	}

	title := req.Title
	if title == "" {
		gen, err := ws(r).Runtime.TitleFor(ctx, req.OwnerAgentID, req.Prompt)
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
		BoardState:   req.BoardState,
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
	BoardState   *string `json:"boardState"`
}

// handleUpdateTask edits any subset of a task's mutable fields (PATCH-like PUT).
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	task, err := wsp.DB.GetTask(r.Context(), id)
	if writeDBError(w, err, "task not found") {
		return
	}

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
	if req.BoardState != nil {
		if !db.ValidBoardState(*req.BoardState) {
			writeError(w, http.StatusBadRequest, "invalid board state")
			return
		}
		task.BoardState = *req.BoardState
	}

	if err := wsp.DB.UpdateTask(r.Context(), task); writeDBError(w, err, "task not found") {
		return
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

// handleRunTask executes a task immediately ("run now") with its owner agent.
func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	run, err := wsp.Runtime.RunTask(r.Context(), id, "manual")
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	} else if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// handleListTaskRuns returns the run history for a task.
func (s *Server) handleListTaskRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := ws(r).DB.ListRuns(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "") {
		return
	}
	if runs == nil {
		runs = []db.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}
