package worktree

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

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
		{},              // base status
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

func TestResolveBaseRef(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		results    []runnerResult
		want       string
		wantError  string
	}{
		{name: "develop HEAD", results: []runnerResult{{out: "develop\n"}, {}}, want: "develop"},
		{name: "explicit remote ref", configured: "origin/release", results: []runnerResult{{out: "abc123\n"}}, want: "origin/release"},
		{name: "invalid explicit ref", configured: "missing", results: []runnerResult{{err: errors.New("exit 128")}}, wantError: "invalid worktree base ref"},
		{name: "detached HEAD", results: []runnerResult{{err: errors.New("exit 1")}}, wantError: "HEAD is detached"},
		{name: "unborn HEAD", results: []runnerResult{{out: "develop\n"}, {err: errors.New("exit 128")}}, wantError: "is unborn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{results: tt.results}
			got, err := (Git{RepoRoot: "repo", Runner: runner}).ResolveBaseRef(context.Background(), tt.configured)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ResolveBaseRef() = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}
}

func TestMergeDirtyBasePreservesWorktreeAndBranch(t *testing.T) {
	for _, status := range []string{"M  staged.go\n", " M unstaged.go\n", "?? untracked.go\n"} {
		runner := &fakeRunner{results: []runnerResult{{out: "develop\n"}, {out: status}}}
		git := Git{RepoRoot: "repo", Runner: runner}
		err := git.Merge(context.Background(), "task/tsk1", "trees/tsk1", "develop")
		if !errors.Is(err, ErrDirty) {
			t.Fatalf("status %q: error = %v, want ErrDirty", status, err)
		}
		if got := len(runner.calls); got != 2 {
			t.Fatalf("status %q triggered merge/remove/delete: %v", status, runner.calls)
		}
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

func TestGitFailureOutputIsBoundedAndRecorded(t *testing.T) {
	warnings := strings.Repeat("warning: a very noisy CRLF conversion warning\r\n", 500)
	warnings += "fatal: final diagnostic"
	runner := &fakeRunner{results: []runnerResult{
		{out: "abc123\n"},
		{out: warnings, err: errors.New("exit status 128")},
	}}
	git := Git{RepoRoot: "repo", Runner: runner}
	store := &memoryStore{task: db.Task{ID: "TSK490"}}
	lifecycle := &Lifecycle{Store: store, Git: git, BaseRef: "main", WorktreeRoot: "trees"}
	err := lifecycle.Handle(context.Background(), db.BoardChangeEvent{
		TaskID: "TSK490", Op: db.BoardOpMove, ToState: db.BoardTodo,
	})
	if err == nil {
		t.Fatal("expected provision error")
	}
	for name, text := range map[string]string{"error": err.Error(), "last error": store.task.WorktreeLastError} {
		if len(text) > gitErrorOutputLimit+256 {
			t.Fatalf("%s length = %d, want bounded near %d", name, len(text), gitErrorOutputLimit)
		}
		if !utf8.ValidString(text) {
			t.Errorf("%s contains invalid UTF-8", name)
		}
		for _, want := range []string{"git worktree add", "warning: a very noisy", "bytes omitted", "fatal: final diagnostic"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q: %.200q", name, want, text)
			}
		}
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
	provisions       int
	resolved         string
	resolveErr       error
	provisionBaseRef string
	mergeErr         error
	discardErr       error
}

func (f *fakeOperations) ResolveBaseRef(_ context.Context, configured string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	if f.resolved != "" {
		return f.resolved, nil
	}
	return configured, nil
}
func (f *fakeOperations) Provision(_ context.Context, _, _, baseRef string) error {
	f.provisions++
	f.provisionBaseRef = baseRef
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

func TestLifecycleStoresResolvedBaseRef(t *testing.T) {
	store := &memoryStore{task: db.Task{ID: "TSK1"}}
	ops := &fakeOperations{resolved: "develop"}
	lifecycle := &Lifecycle{Store: store, Git: ops, WorktreeRoot: "trees"}
	event := db.BoardChangeEvent{TaskID: "TSK1", Op: db.BoardOpMove, ToState: db.BoardTodo}
	if err := lifecycle.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if store.task.WorktreeBaseRef != "develop" || ops.provisionBaseRef != "develop" {
		t.Fatalf("resolved base ref not persisted/reused: task=%q provision=%q", store.task.WorktreeBaseRef, ops.provisionBaseRef)
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
