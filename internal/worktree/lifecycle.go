package worktree

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/bilal-arikan/tionharness/internal/db"
)

type TaskStore interface {
	GetTask(context.Context, string) (db.Task, error)
	SetTaskWorktree(context.Context, string, string, string, string, string, string) error
}

type Operations interface {
	ResolveBaseRef(context.Context, string) (string, error)
	Provision(context.Context, string, string, string) error
	Merge(context.Context, string, string, string) error
	Discard(context.Context, string, string, string) error
}

type Lifecycle struct {
	Store        TaskStore
	Git          Operations
	BaseRef      string
	WorktreeRoot string
	mu           sync.Mutex
}

// Handle serializes lifecycle events so redelivery cannot race past the stored
// state check. The mutex never covers a DB operation and DB never shells out.
func (l *Lifecycle) Handle(ctx context.Context, ev db.BoardChangeEvent) error {
	if ev.Op != db.BoardOpCreate && ev.Op != db.BoardOpMove {
		return nil
	}
	switch ev.ToState {
	case db.BoardTodo, db.BoardDone, db.BoardCancelled, db.BoardFailed:
	default:
		return nil
	}
	if l.Store == nil || l.Git == nil {
		return errors.New("worktree lifecycle dependencies are nil")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	task, err := l.Store.GetTask(ctx, ev.TaskID)
	if err != nil {
		return err
	}
	state := task.WorktreeState
	if state == "" {
		state = db.WorktreeNone
	}

	switch ev.ToState {
	case db.BoardTodo:
		if state != db.WorktreeNone {
			return nil
		}
		return l.provision(ctx, task)
	case db.BoardDone:
		if state == db.WorktreeMerged {
			return nil
		}
		if state != db.WorktreeProvisioned && state != db.WorktreeConflict {
			return nil
		}
		return l.merge(ctx, task)
	default:
		if state == db.WorktreeDiscarded {
			return nil
		}
		if state != db.WorktreeProvisioned && state != db.WorktreeConflict {
			return nil
		}
		return l.discard(ctx, task)
	}
}

func (l *Lifecycle) provision(ctx context.Context, task db.Task) error {
	baseRef, err := l.Git.ResolveBaseRef(ctx, l.BaseRef)
	if err != nil {
		persistErr := l.Store.SetTaskWorktree(ctx, task.ID, "", "", "", db.WorktreeNone, err.Error())
		return errors.Join(err, persistErr)
	}
	branch := "task/" + branchPart(task.ID)
	path := filepath.Join(l.WorktreeRoot, branchPart(task.ID))
	if err := l.Git.Provision(ctx, branch, path, baseRef); err != nil {
		persistErr := l.Store.SetTaskWorktree(ctx, task.ID, branch, path, baseRef, db.WorktreeNone, err.Error())
		return errors.Join(err, persistErr)
	}
	return l.Store.SetTaskWorktree(ctx, task.ID, branch, path, baseRef, db.WorktreeProvisioned, "")
}

func (l *Lifecycle) merge(ctx context.Context, task db.Task) error {
	err := l.Git.Merge(ctx, task.WorktreeBranch, task.WorktreePath, task.WorktreeBaseRef)
	if err == nil {
		return l.Store.SetTaskWorktree(ctx, task.ID, task.WorktreeBranch, task.WorktreePath, task.WorktreeBaseRef, db.WorktreeMerged, "")
	}
	state := db.WorktreeProvisioned
	if errors.Is(err, ErrConflict) {
		state = db.WorktreeConflict
	}
	persistErr := l.Store.SetTaskWorktree(ctx, task.ID, task.WorktreeBranch, task.WorktreePath, task.WorktreeBaseRef, state, err.Error())
	return errors.Join(err, persistErr)
}

func (l *Lifecycle) discard(ctx context.Context, task db.Task) error {
	err := l.Git.Discard(ctx, task.WorktreeBranch, task.WorktreePath, task.WorktreeBaseRef)
	if err == nil {
		return l.Store.SetTaskWorktree(ctx, task.ID, task.WorktreeBranch, task.WorktreePath, task.WorktreeBaseRef, db.WorktreeDiscarded, "")
	}
	persistErr := l.Store.SetTaskWorktree(ctx, task.ID, task.WorktreeBranch, task.WorktreePath, task.WorktreeBaseRef, task.WorktreeState, err.Error())
	return errors.Join(err, persistErr)
}

func branchPart(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		panic(fmt.Sprintf("task id %q produces an empty branch name", id))
	}
	return b.String()
}
