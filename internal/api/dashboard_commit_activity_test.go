package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDashboardCommitActivityUsesSelectedProjectDir(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	projectDir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	} {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "tracked.txt"), []byte("activity\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	for _, args := range [][]string{{"add", "tracked.txt"}, {"commit", "-m", "test activity"}} {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	// The workspace sandbox is deliberately not a repository. The dashboard must
	// query the selected project directory instead.
	if _, isGitRepo, err := commitActivity(context.Background(), wsp.SandboxRoot(), 1, time.Now(), runDashboardGit); err != nil {
		t.Fatalf("check sandbox repository: %v", err)
	} else if isGitRepo {
		t.Fatal("test workspace sandbox unexpectedly is a git repository")
	}
	wsp.Runtime.SetDefaultWorkDir(projectDir)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/commit-activity?weeks=1", nil)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleDashboardCommitActivity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		CommitsByDay []daySeriesPoint `json:"commitsByDay"`
		IsGitRepo    bool             `json:"isGitRepo"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.IsGitRepo {
		t.Fatal("selected project repository reported as absent")
	}
	var commits int64
	for _, point := range got.CommitsByDay {
		commits += point.Value
	}
	if commits != 1 {
		t.Fatalf("commit count = %d, want 1: %+v", commits, got.CommitsByDay)
	}
}

func TestCommitActivityWeeksClamps(t *testing.T) {
	for raw, want := range map[string]int{"": 52, "abc": 52, "0": 1, "1": 1, "12": 12, "53": 52} {
		if got := commitActivityWeeks(raw); got != want {
			t.Errorf("commitActivityWeeks(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestCommitActivityNotRepositoryAndEmptyLog(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name string
		run  gitCommand
	}{
		{"not repository", func(context.Context, string, ...string) (string, error) {
			return "", &exec.ExitError{Stderr: []byte("fatal: not a git repository (or any of the parent directories): .git")}
		}},
		{"empty log", func(_ context.Context, _ string, args ...string) (string, error) {
			if args[0] == "rev-parse" {
				return "true", nil
			}
			return "", nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			points, isGitRepo, err := commitActivity(context.Background(), t.TempDir(), 2, now, tc.run)
			if err != nil {
				t.Fatal(err)
			}
			if len(points) != 14 {
				t.Fatalf("got %d buckets, want 14", len(points))
			}
			if isGitRepo != (tc.name == "empty log") {
				t.Fatalf("isGitRepo = %v", isGitRepo)
			}
			for _, point := range points {
				if point.Value != 0 {
					t.Fatalf("bucket %s = %d, want zero", point.Day, point.Value)
				}
			}
		})
	}
}

func TestCommitActivityDistinguishesRevParseFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"executable missing", exec.ErrNotFound},
		{"process start error", &exec.Error{Name: "git", Err: errors.New("process start failed")}},
		{"permission stderr", &exec.ExitError{Stderr: []byte("fatal: cannot open .git/FETCH_HEAD: Permission denied")}},
		{"unknown stderr", &exec.ExitError{Stderr: []byte("fatal: unexpected rev-parse failure")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := func(context.Context, string, ...string) (string, error) { return "", tc.err }
			if _, _, err := commitActivity(context.Background(), ".", 1, time.Now(), run); !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestCommitActivityRealNonGitDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "non-git")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	points, isGitRepo, err := commitActivity(context.Background(), dir, 1, time.Now(), runDashboardGit)
	if err != nil {
		t.Fatal(err)
	}
	if isGitRepo || len(points) != 7 {
		t.Fatalf("isGitRepo = %v, buckets = %d", isGitRepo, len(points))
	}
}

func TestCommitActivityUsesLocalCalendarBoundariesAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, location)
	boundary := time.Date(2026, 3, 4, 0, 0, 0, 0, location)
	before := boundary.Add(-time.Second)

	run := func(_ context.Context, _ string, args ...string) (string, error) {
		if args[0] == "rev-parse" {
			return "true", nil
		}
		wantSince := "--since=" + boundary.Format(time.RFC3339)
		if args[len(args)-1] != wantSince {
			t.Errorf("since = %q, want %q", args[len(args)-1], wantSince)
		}
		return strconvFormat(boundary.Unix()) + "\n" + strconvFormat(before.Unix()), nil
	}
	points, isGitRepo, err := commitActivity(context.Background(), ".", 1, now, run)
	if err != nil {
		t.Fatal(err)
	}
	if !isGitRepo {
		t.Fatal("repository reported as absent")
	}
	if len(points) != 7 || points[0].Day != "2026-03-04" || points[0].Value != 1 {
		t.Fatalf("boundary bucket = %+v", points)
	}
	if points[4].Day != "2026-03-08" {
		t.Fatalf("DST calendar day drifted: %+v", points)
	}
}

func TestCommitActivityPropagatesLogFailureAndCancellation(t *testing.T) {
	logErr := errors.New("log failed")
	run := func(ctx context.Context, _ string, args ...string) (string, error) {
		if args[0] == "rev-parse" {
			return "true", nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", logErr
	}

	if _, _, err := commitActivity(context.Background(), ".", 1, time.Now(), run); !errors.Is(err, logErr) {
		t.Fatalf("command error = %v, want %v", err, logErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledRun := func(ctx context.Context, _ string, _ ...string) (string, error) {
		return "", ctx.Err()
	}
	if _, _, err := commitActivity(ctx, ".", 1, time.Now(), canceledRun); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
}

func TestCommitActivityPropagatesRevParseTimeoutAndCancellation(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			run := func(context.Context, string, ...string) (string, error) { return "", want }
			if _, _, err := commitActivity(context.Background(), ".", 1, time.Now(), run); !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
		})
	}
}

func strconvFormat(n int64) string {
	return strconv.FormatInt(n, 10)
}
