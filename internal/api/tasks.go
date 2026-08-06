package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	// By default the board shows only active cards; archived cards are hidden but
	// kept. ?archived=1 (or true) returns everything, for an "archived" view.
	list := ws(r).DB.ListActiveTasks
	if v := r.URL.Query().Get("archived"); v == "1" || v == "true" {
		list = ws(r).DB.ListTasks
	}
	tasks, err := list(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if tasks == nil {
		tasks = []db.Task{}
	}
	q := r.URL.Query()
	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Optional filters (none by default → legacy full board).
	boardState := q.Get("boardState")
	priority := q.Get("priority")
	owner := q.Get("ownerAgentId")
	wantTags := tools.SplitTags(q.Get("tags"))
	matches := make([]db.Task, 0, len(tasks))
	for _, tk := range tasks {
		if boardState != "" && tk.BoardState != boardState {
			continue
		}
		if priority != "" && tk.Priority != priority {
			continue
		}
		if owner != "" && tk.OwnerAgentID != owner {
			continue
		}
		if len(wantTags) > 0 && !tools.HasAllTags(tk.Tags, wantTags) {
			continue
		}
		matches = append(matches, tk)
	}
	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	if field != "" {
		less, err := tools.SortByField(matches, field, asc,
			func(tk db.Task) int64 { return tk.UpdatedAt },
			func(tk db.Task) int64 { return tk.CreatedAt },
			func(tk db.Task) string { return tk.Title })
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sort.SliceStable(matches, less)
	}
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
}

type createTaskReq struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Prompt       string   `json:"prompt"`
	OwnerAgentID string   `json:"ownerAgentId"`
	FlowID       string   `json:"flowId"`
	BoardState   string   `json:"boardState"`
	Dependencies string   `json:"dependencies"` // JSON array of task IDs
	Priority     string   `json:"priority"`
	Tags         []string `json:"tags"`
	ArtifactIDs  []string `json:"artifactIds"`
	Progress     int      `json:"progress"`
	StartDate    string   `json:"startDate"`
	DueDate      string   `json:"dueDate"`
}

// placeholderTitle derives an instant, single-line title from a card's content,
// used until the async AI title lands (see handleCreateTask). First line only,
// capped to a sensible length with an ellipsis — mirrors the frontend excerpt.
func placeholderTitle(source string) string {
	source = strings.TrimSpace(source)
	if i := strings.IndexAny(source, "\r\n"); i >= 0 {
		source = strings.TrimSpace(source[:i])
	}
	r := []rune(source)
	if len(r) > 60 {
		return strings.TrimSpace(string(r[:60])) + "…"
	}
	return source
}

// clampProgress keeps a progress value within [0,100].
func clampProgress(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[createTaskReq](w, r)
	if !ok {
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
	if !db.ValidPriority(req.Priority) {
		writeError(w, http.StatusBadRequest, "invalid priority")
		return
	}

	// Title strategy: when the user omits a title we DON'T block the create on an
	// LLM call. Instead we stamp an instant placeholder — a truncated excerpt of
	// the content — and generate the real AI title in the background (see below),
	// swapping it in and broadcasting a board event once it lands. This keeps card
	// creation snappy so the board can render the new card immediately.
	title := req.Title
	titleSource := req.Description
	if titleSource == "" {
		titleSource = req.Prompt
	}
	autoTitle := req.Title == "" && titleSource != ""
	if autoTitle {
		title = placeholderTitle(titleSource) // instant content excerpt
	}

	wsp := ws(r)
	task, err := wsp.DB.CreateTask(ctx, db.Task{
		Title:        title,
		Description:  req.Description,
		Prompt:       req.Prompt,
		OwnerAgentID: req.OwnerAgentID,
		FlowID:       req.FlowID,
		BoardState:   req.BoardState,
		Dependencies: req.Dependencies,
		Priority:     req.Priority,
		Tags:         req.Tags,
		ArtifactIDs:  req.ArtifactIDs,
		Progress:     clampProgress(req.Progress),
		StartDate:    req.StartDate,
		DueDate:      req.DueDate,
	})
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("task created", "task", task.ID, "title", task.Title, "owner", req.OwnerAgentID)
	// Broadcast so other open windows (Network live-mode, future badge
	// listeners) refresh without polling — mirrors the updateTask branch.
	publishEntityChange(wsp, "board", "Görev oluşturuldu: "+task.Title, task.BoardState,
		map[string]string{"view": "board", "taskId": task.ID, "op": "create"})

	// Async AI titling (best-effort). Runs on a detached context so it survives
	// the request returning; only applies the AI title if the card still carries
	// the placeholder (never clobbers a title the user edited, nor a deleted card).
	if autoTitle && wsp.Runtime != nil {
		placeholder := title
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			newTitle, genErr := wsp.Runtime.TitleFor(bgCtx, req.OwnerAgentID, titleSource)
			if genErr != nil {
				s.logger.Warn("async task title generation degraded", "task", task.ID, "error", genErr)
			}
			newTitle = strings.TrimSpace(newTitle)
			if newTitle == "" || newTitle == placeholder {
				return // nothing better to apply
			}
			cur, err := wsp.DB.GetTask(bgCtx, task.ID)
			if err != nil {
				return // deleted meanwhile
			}
			if cur.Title != placeholder {
				return // user renamed it in the meantime — don't clobber
			}
			cur.Title = newTitle
			if err := wsp.DB.UpdateTask(bgCtx, cur); err != nil {
				s.logger.Warn("async task title update failed", "task", task.ID, "error", err)
				return
			}
			publishEntityChange(wsp, "board", "Görev başlığı güncellendi: "+newTitle, cur.BoardState,
				map[string]string{"view": "board", "taskId": cur.ID, "op": "update"})
		}()
	}

	writeJSON(w, http.StatusCreated, task)
}

type updateTaskReq struct {
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	Prompt       *string   `json:"prompt"`
	OwnerAgentID *string   `json:"ownerAgentId"`
	FlowID       *string   `json:"flowId"`
	BoardState   *string   `json:"boardState"`
	Dependencies *string   `json:"dependencies"` // JSON array of task IDs
	Priority     *string   `json:"priority"`
	Tags         *[]string `json:"tags"`
	ArtifactIDs  *[]string `json:"artifactIds"`
	Progress     *int      `json:"progress"`
	StartDate    *string   `json:"startDate"`
	DueDate      *string   `json:"dueDate"`
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
	// Snapshot the fields the Network screen renders from, so we know exactly
	// what actually changed (vs. just which fields the request touched) and can
	// publish a single, focused "board" event downstream.
	oldTitle := task.Title
	oldOwner := task.OwnerAgentID
	oldFlowID := task.FlowID
	oldPriority := task.Priority
	oldTags := append([]string(nil), task.Tags...)
	oldProgress := task.Progress

	req, ok := bindJSON[updateTaskReq](w, r)
	if !ok {
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
	if req.Priority != nil {
		if !db.ValidPriority(*req.Priority) {
			writeError(w, http.StatusBadRequest, "invalid priority")
			return
		}
		task.Priority = *req.Priority
	}
	if req.Tags != nil {
		task.Tags = *req.Tags
	}
	if req.ArtifactIDs != nil {
		task.ArtifactIDs = *req.ArtifactIDs
	}
	if req.Progress != nil {
		task.Progress = clampProgress(*req.Progress)
	}
	if req.StartDate != nil {
		task.StartDate = *req.StartDate
	}
	if req.DueDate != nil {
		task.DueDate = *req.DueDate
	}

	if err := wsp.DB.UpdateTask(r.Context(), task); writeDBError(w, err, "task not found") {
		return
	}
	// Board move → generic "board" change event so other windows badge the
	// Görevler view + workspace label (the user's own window clears it on view).
	// This covers the most visually loud change; non-BoardState edits below
	// only publish when the move signal didn't already fire (no spam).
	if req.BoardState != nil && *req.BoardState != oldBoard {
		publishEntityChange(wsp, "board", "Görev taşındı: "+task.Title, *req.BoardState,
			map[string]string{"view": "board", "taskId": task.ID, "op": "move"})
	} else {
		// Detect *actual* changes to network-rendering fields so a no-op PATCH
		// (request carries a field but its value is identical) doesn't trigger
		// a useless refresh elsewhere.
		changed := task.Title != oldTitle ||
			task.OwnerAgentID != oldOwner ||
			task.FlowID != oldFlowID ||
			task.Priority != oldPriority ||
			task.Progress != oldProgress ||
			!equalStringSlice(task.Tags, oldTags)
		if changed {
			publishEntityChange(wsp, "board", "Görev güncellendi: "+task.Title, task.BoardState,
				map[string]string{"view": "board", "taskId": task.ID, "op": "update"})
		}
	}
	writeJSON(w, http.StatusOK, task)
}

// equalStringSlice reports whether two string slices contain the same elements
// in the same order. Used to detect *real* tag-list changes (vs. a PATCH that
// re-sent an identical array).
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type archiveTaskReq struct {
	Archived bool `json:"archived"`
}

// handleArchiveTask flips a task's Archived flag (reversible soft-hide). Archiving
// drops the card off the active board without deleting it; unarchiving restores it.
// This is the manual counterpart to the "done → archive" board automation.
func (s *Server) handleArchiveTask(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	id := r.PathValue("id")
	req, ok := bindJSON[archiveTaskReq](w, r)
	if !ok {
		return
	}
	task, err := wsp.DB.GetTask(ctx, id)
	if writeDBError(w, err, "task not found") {
		return
	}
	if err := wsp.DB.SetTaskArchived(ctx, id, req.Archived); writeDBError(w, err, "task not found") {
		return
	}
	verb := "arşivlendi"
	op := "archive"
	if !req.Archived {
		verb = "arşivden çıkarıldı"
		op = "unarchive"
	}
	s.logger.Info("task archive toggled", "task", id, "archived", req.Archived)
	publishEntityChange(wsp, "board", "Görev "+verb+": "+task.Title, task.BoardState,
		map[string]string{"view": "board", "taskId": id, "op": op})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "archived": req.Archived})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	id := r.PathValue("id")
	// Read the task first so the broadcast carries its title (deleted rows no
	// longer exist when listeners process the event a moment later).
	task, err := wsp.DB.GetTask(ctx, id)
	if writeDBError(w, err, "task not found") {
		return
	}
	if err := wsp.DB.DeleteTask(ctx, id); writeDBError(w, err, "task not found") {
		return
	}
	s.logger.Info("task deleted", "task", id, "title", task.Title)
	// Mirror handleUpdateTask's "board" notify so the Network screen drops
	// the node immediately (live-mode SSE subscriptions refresh on notify).
	publishEntityChange(wsp, "board", "Görev silindi: "+task.Title, task.BoardState,
		map[string]string{"view": "board", "taskId": id, "op": "delete"})
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "result": "deleted"})
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
