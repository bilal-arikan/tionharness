package worktree

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

type runnerResult struct {
	out string
	err error
}

type fakeRunner struct {
	results []runnerResult
	calls   []string
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(append([]string{dir, name}, args...), " "))
	if len(f.results) == 0 {
		return "", fmt.Errorf("unexpected command %s", f.calls[len(f.calls)-1])
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r.out, r.err
}

func TestHappyPathWithFakeRunner(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{
		{},              // worktree add
		{out: "main\n"}, // current branch
		{},              // base dirty check
		{},              // merge
		{},              // worktree remove
		{},              // branch delete
	}}
	git := Git{RepoRoot: "repo", Runner: runner}
	if err := git.Provision(context.Background(), "task/tsk1", "trees/tsk1", "main"); err != nil {
		t.Fatal(err)
	}
	if err := git.Merge(context.Background(), "task/tsk1", "trees/tsk1", "main"); err != nil {
		t.Fatal(err)
	}
	if got := len(runner.calls); got != 6 {
		t.Fatalf("calls = %d, want 6: %v", got, runner.calls)
	}
}

func TestMergeConflictPreservesWorktree(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{
		{out: "main\n"}, {}, {out: "CONFLICT in file", err: errors.New("exit 1")}, {out: "file.go\n"},
	}}
	git := Git{RepoRoot: "repo", Runner: runner}
	err := git.Merge(context.Background(), "task/tsk1", "trees/tsk1", "main")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if got := len(runner.calls); got != 4 {
		t.Fatalf("merge conflict performed cleanup; calls = %v", runner.calls)
	}
}

func TestDirtyWorktreeRefusesDiscard(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{out: " M important.go\n"}}}
	git := Git{RepoRoot: "repo", Runner: runner}
	err := git.Discard(context.Background(), "task/tsk1", "trees/tsk1", "main")
	if !errors.Is(err, ErrDirty) {
		t.Fatalf("error = %v, want ErrDirty", err)
	}
	if got := len(runner.calls); got != 1 {
		t.Fatalf("dirty discard performed cleanup; calls = %v", runner.calls)
	}
}

type memoryStore struct{ task db.Task }

func (s *memoryStore) GetTask(context.Context, string) (db.Task, error) { return s.task, nil }
func (s *memoryStore) SetTaskWorktree(_ context.Context, _ string, branch, path, baseRef, state, lastError string) error {
	s.task.WorktreeBranch = branch
	s.task.WorktreePath = path
	s.task.WorktreeBaseRef = baseRef
	s.task.WorktreeState = state
	s.task.WorktreeLastError = lastError
	return nil
}

type fakeOperations struct {
	provisions int
	mergeErr   error
	discardErr error
}

func (f *fakeOperations) Provision(context.Context, string, string, string) error {
	f.provisions++
	return nil
}
func (f *fakeOperations) Merge(context.Context, string, string, string) error { return f.mergeErr }
func (f *fakeOperations) Discard(context.Context, string, string, string) error {
	return f.discardErr
}

func TestDoubleDeliveryIsIdempotent(t *testing.T) {
	store := &memoryStore{task: db.Task{ID: "TSK1"}}
	ops := &fakeOperations{}
	lifecycle := &Lifecycle{Store: store, Git: ops, BaseRef: "main", WorktreeRoot: "trees"}
	event := db.BoardChangeEvent{TaskID: "TSK1", Op: db.BoardOpMove, ToState: db.BoardTodo}
	if err := lifecycle.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if ops.provisions != 1 {
		t.Fatalf("provision calls = %d, want 1", ops.provisions)
	}
}

func TestLifecycleRecordsConflictAndDirtyRefusal(t *testing.T) {
	store := &memoryStore{task: db.Task{
		ID: "TSK1", WorktreeBranch: "task/tsk1", WorktreePath: "trees/tsk1",
		WorktreeBaseRef: "main", WorktreeState: db.WorktreeProvisioned,
	}}
	ops := &fakeOperations{mergeErr: fmt.Errorf("%w: file conflict", ErrConflict)}
	lifecycle := &Lifecycle{Store: store, Git: ops}
	mergeEvent := db.BoardChangeEvent{TaskID: "TSK1", Op: db.BoardOpMove, ToState: db.BoardDone}
	if err := lifecycle.Handle(context.Background(), mergeEvent); !errors.Is(err, ErrConflict) {
		t.Fatalf("merge error = %v, want ErrConflict", err)
	}
	if store.task.WorktreeState != db.WorktreeConflict || !strings.Contains(store.task.WorktreeLastError, "file conflict") {
		t.Fatalf("conflict not recorded: %+v", store.task)
	}

	ops.mergeErr = nil
	ops.discardErr = fmt.Errorf("%w: uncommitted changes", ErrDirty)
	discardEvent := db.BoardChangeEvent{TaskID: "TSK1", Op: db.BoardOpMove, ToState: db.BoardCancelled}
	if err := lifecycle.Handle(context.Background(), discardEvent); !errors.Is(err, ErrDirty) {
		t.Fatalf("discard error = %v, want ErrDirty", err)
	}
	if store.task.WorktreeState != db.WorktreeConflict || !strings.Contains(store.task.WorktreeLastError, "uncommitted") {
		t.Fatalf("dirty refusal not recorded: %+v", store.task)
	}
}
