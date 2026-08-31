package db

import (
	"context"
	"fmt"
	"log/slog"
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

// SetTaskWorktree records card lifecycle metadata without firing a board event.
// Git work is deliberately completed before this method is called, so no DB
// lock is held while an external process runs.
func (d *DB) SetTaskWorktree(ctx context.Context, id, branch, path, baseRef, state, lastError string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tasks[id]
	if !ok {
		return ErrNotFound
	}
	t.WorktreeBranch = branch
	t.WorktreePath = path
	t.WorktreeBaseRef = baseRef
	t.WorktreeState = state
	t.WorktreeLastError = lastError
	t.UpdatedAt = now()
	return d.persistTaskLocked(t)
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
	if !IsValidBoardKey(boardState) {
		return fmt.Errorf("invalid board state %q", boardState)
	}
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

// DeleteTask removes a task.
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
	title, board, owner, tags, prio := t.Title, t.BoardState, t.OwnerAgentID, t.Tags, t.Priority
	d.mu.Unlock()
	d.fireBoardHook(BoardChangeEvent{
		TaskID: id, Title: title, Op: BoardOpDelete,
		FromState: board, OwnerAgentID: owner, Tags: tags, Priority: prio,
	})
	return nil
}

// MigrateBoardColumns reconciles tasks' BoardState against a board-column
// rename/delete. BoardColumnDef has no stable id — only Key — so a rename is
// detected positionally: same index, different Key. Any old key absent from
// newCols by either match (i.e. neither renamed nor still present verbatim)
// is treated as deleted, and its tasks are moved to the first column of
// newCols (falling back to BoardTodo if newCols is empty) so cards are never
// silently dropped off the board. Persist + migration run under a single
// write lock so a save can never land half-applied. Returns the number of
// tasks whose BoardState changed.
func (d *DB) MigrateBoardColumns(ctx context.Context, oldCols, newCols []BoardColumnDef) (moved int, err error) {
	// Positional rename map: oldCols[i].Key -> newCols[i].Key when the key
	// actually changed at that index and the replacement key is new. Existing
	// keys can shift into the same index after a deletion and are not renames.
	oldKeys := make(map[string]bool, len(oldCols))
	for _, c := range oldCols {
		oldKeys[c.Key] = true
	}
	renamed := make(map[string]string, len(oldCols))
	for i := 0; i < len(oldCols) && i < len(newCols); i++ {
		if oldCols[i].Key != newCols[i].Key && !oldKeys[newCols[i].Key] {
			renamed[oldCols[i].Key] = newCols[i].Key
		}
	}
	stillPresent := make(map[string]bool, len(newCols))
	for _, c := range newCols {
		stillPresent[c.Key] = true
	}
	fallback := BoardTodo
	if len(newCols) > 0 {
		fallback = newCols[0].Key
	}

	// resolve maps an old key to where its tasks should land, or "" if the
	// column is unchanged (still present verbatim, no migration needed).
	resolve := func(oldKey string) string {
		if stillPresent[oldKey] {
			return ""
		}
		if newKey, ok := renamed[oldKey]; ok {
			return newKey
		}
		// Old key no longer present and not a rename target: its column was
		// deleted. Route orphaned tasks to the first remaining column rather
		// than losing them.
		return fallback
	}

	type migration struct {
		fromState string
		toState   string
		ev        BoardChangeEvent
	}
	var toMigrate []migration

	d.mu.Lock()
	for _, t := range d.tasks {
		to := resolve(t.BoardState)
		if to == "" || to == t.BoardState {
			continue
		}
		fromState := t.BoardState
		t.BoardState = to
		t.UpdatedAt = now()
		if perr := d.persistTaskLocked(t); perr != nil {
			d.mu.Unlock()
			return moved, perr
		}
		d.tasks[t.ID] = t
		toMigrate = append(toMigrate, migration{
			fromState: fromState,
			toState:   to,
			ev: BoardChangeEvent{
				TaskID: t.ID, Title: t.Title, Op: BoardOpMove,
				FromState: fromState, ToState: to,
				OwnerAgentID: t.OwnerAgentID, Tags: t.Tags, Priority: t.Priority,
			},
		})
	}
	moved = len(toMigrate)
	d.mu.Unlock()

	if moved > 0 {
		slog.Info("board columns changed: migrated tasks", "moved", moved, "renamed", len(renamed))
	}
	for _, m := range toMigrate {
		d.fireBoardHook(m.ev)
	}
	return moved, nil
}
