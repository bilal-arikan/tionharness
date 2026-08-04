package db

import (
	"context"
)

// BoardChangeEvent describes a single kanban card change. It is delivered to the
// board hook (see SetBoardHook) so board-triggered automations can react. Op is
// one of BoardOpCreate/BoardOpMove/BoardOpUpdate/BoardOpDelete. For a move,
// FromState/ToState hold the old and new columns; create leaves FromState empty
// and delete leaves ToState empty.
type BoardChangeEvent struct {
	TaskID       string
	Title        string
	Op           string
	FromState    string
	ToState      string
	OwnerAgentID string
	Tags         []string
	Priority     string
}

// BoardChangeFn observes board card changes. The store calls it after releasing
// its lock; implementations must return promptly (dispatch async).
type BoardChangeFn func(ev BoardChangeEvent)

// SetBoardHook registers (or clears, with nil) the board-change observer. Wired
// once at workspace boot by the manager to the AutomationEngine.
func (d *DB) SetBoardHook(fn BoardChangeFn) {
	d.boardHookMu.Lock()
	d.boardHook = fn
	d.boardHookMu.Unlock()
}

// fireBoardHook dispatches a board-change event to the registered observer (if
// any). Called after the store lock is released.
func (d *DB) fireBoardHook(ev BoardChangeEvent) {
	d.boardHookMu.RLock()
	fn := d.boardHook
	d.boardHookMu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

func (d *DB) persistTaskLocked(t Task) error {
	return dbPersistLocked(d, d.tasks, dirTasks, t.ID, t)
}

// CreateTask inserts a new task and returns the stored row.
func (d *DB) CreateTask(ctx context.Context, t Task) (Task, error) {
	t.ID = d.nextID(idTask)
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	if t.BoardState == "" {
		t.BoardState = BoardTodo
	}
	if t.Dependencies == "" {
		t.Dependencies = "[]"
	}
	d.mu.Lock()
	err := d.persistTaskLocked(t)
	d.mu.Unlock()
	if err != nil {
		return t, err
	}
	d.fireBoardHook(BoardChangeEvent{
		TaskID: t.ID, Title: t.Title, Op: BoardOpCreate,
		ToState: t.BoardState, OwnerAgentID: t.OwnerAgentID, Tags: t.Tags, Priority: t.Priority,
	})
	return t, nil
}

// GetTask loads a task by id.
func (d *DB) GetTask(ctx context.Context, id string) (Task, error) {
	return dbGet(d, d.tasks, id)
}

// ListTasks returns all tasks (including archived), newest first.
func (d *DB) ListTasks(ctx context.Context) ([]Task, error) {
	return dbList(d, d.tasks, func(a, b Task) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// ListActiveTasks returns only non-archived tasks, newest first — the active
// board. Used by the board-facing surfaces (the /api/tasks list, the get_view
// board projection, the list_tasks tool) so archived cards drop off the board
// without being deleted. ListTasks still returns everything for integrity paths.
func (d *DB) ListActiveTasks(ctx context.Context) ([]Task, error) {
	return dbFilter(d, d.tasks,
		func(t Task) bool { return !t.Archived },
		func(a, b Task) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// UpdateTask edits the mutable fields of a task (title/description/prompt/owner/state).
// It fires the board hook afterwards: as a move when the board state changed,
// otherwise as a plain update (so board automations with op update/any can react).
func (d *DB) UpdateTask(ctx context.Context, t Task) error {
	d.mu.Lock()
	cur, ok := d.tasks[t.ID]
	if !ok {
		d.mu.Unlock()
		return ErrNotFound
	}
	oldBoard := cur.BoardState
	cur.Title = t.Title
	cur.Description = t.Description
	cur.Prompt = t.Prompt
	cur.OwnerAgentID = t.OwnerAgentID
	cur.FlowID = t.FlowID
	cur.BoardState = t.BoardState
	if t.Dependencies != "" {
		cur.Dependencies = t.Dependencies
	}
	cur.Priority = t.Priority
	cur.Tags = t.Tags
	cur.ArtifactIDs = t.ArtifactIDs
	cur.Progress = t.Progress
	cur.StartDate = t.StartDate
	cur.DueDate = t.DueDate
	cur.UpdatedAt = now()
	err := d.persistTaskLocked(cur)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	ev := BoardChangeEvent{TaskID: cur.ID, Title: cur.Title, OwnerAgentID: cur.OwnerAgentID, Tags: cur.Tags, Priority: cur.Priority}
	if cur.BoardState != oldBoard {
		ev.Op, ev.FromState, ev.ToState = BoardOpMove, oldBoard, cur.BoardState
	} else {
		ev.Op, ev.ToState = BoardOpUpdate, cur.BoardState
	}
	d.fireBoardHook(ev)
	return nil
}

// MoveTask changes only a task's board state (kanban drag/drop). Fires the board
// hook as a move when the column actually changed.
func (d *DB) MoveTask(ctx context.Context, id, boardState string) error {
	d.mu.Lock()
	t, ok := d.tasks[id]
	if !ok {
		d.mu.Unlock()
		return ErrNotFound
	}
	oldBoard := t.BoardState
	t.BoardState = boardState
	t.UpdatedAt = now()
	err := d.persistTaskLocked(t)
	title, owner, tags, prio := t.Title, t.OwnerAgentID, t.Tags, t.Priority
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if oldBoard != boardState {
		d.fireBoardHook(BoardChangeEvent{
			TaskID: id, Title: title, Op: BoardOpMove,
			FromState: oldBoard, ToState: boardState, OwnerAgentID: owner, Tags: tags, Priority: prio,
		})
	}
	return nil
}

// SetTaskArchived flips a task's Archived flag (reversible soft-hide, unlike
// DeleteTask). Archiving is used by the "done → archive" board automation to
// clear finished cards off the active board without an LLM call. It fires the
// board hook as a plain update so other board rules can observe it, but archiving
// itself never spawns — so an archive rule cannot re-fire on its own event. A
// no-op (already in the requested state) neither persists nor fires.
func (d *DB) SetTaskArchived(ctx context.Context, id string, archived bool) error {
	d.mu.Lock()
	t, ok := d.tasks[id]
	if !ok {
		d.mu.Unlock()
		return ErrNotFound
	}
	if t.Archived == archived {
		d.mu.Unlock()
		return nil
	}
	t.Archived = archived
	t.UpdatedAt = now()
	err := d.persistTaskLocked(t)
	title, owner, board, tags, prio := t.Title, t.OwnerAgentID, t.BoardState, t.Tags, t.Priority
	d.mu.Unlock()
	if err != nil {
		return err
	}
	d.fireBoardHook(BoardChangeEvent{
		TaskID: id, Title: title, Op: BoardOpUpdate,
		ToState: board, OwnerAgentID: owner, Tags: tags, Priority: prio,
	})
	return nil
}

// DeleteTask removes a task and all of its runs.
func (d *DB) DeleteTask(ctx context.Context, id string) error {
	d.mu.Lock()
	t, ok := d.tasks[id]
	if !ok {
		d.mu.Unlock()
		return ErrNotFound
	}
	delete(d.tasks, id)
	if err := removeFile(d.dir(dirTasks, id+".json")); err != nil {
		d.mu.Unlock()
		return err
	}
	// Cascade: remove runs belonging to this task.
	for rid, r := range d.runs {
		if r.TaskID == id {
			delete(d.runs, rid)
			_ = removeFile(d.dir(dirRuns, rid+".json"))
		}
	}
	title, board, owner, tags, prio := t.Title, t.BoardState, t.OwnerAgentID, t.Tags, t.Priority
	d.mu.Unlock()
	d.fireBoardHook(BoardChangeEvent{
		TaskID: id, Title: title, Op: BoardOpDelete,
		FromState: board, OwnerAgentID: owner, Tags: tags, Priority: prio,
	})
	return nil
}
