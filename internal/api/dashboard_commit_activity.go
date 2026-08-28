package api

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

const dashboardCommitMaxWeeks = 52

type gitCommand func(context.Context, string, ...string) (string, error)

func runDashboardGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitCmdTimeout)
	defer cancel()

	full := append([]string{"-C", dir}, args...)
	cmd := proc.CommandContext(ctx, "git", full...)
	proc.TreeKill(cmd)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return strings.TrimSpace(string(out)), err
}

func isNotGitRepository(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && strings.Contains(strings.ToLower(string(exitErr.Stderr)), "not a git repository")
}

func commitActivityWeeks(raw string) int {
	weeks := dashboardCommitMaxWeeks
	if raw == "" {
		return weeks
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return weeks
	}
	if n < 1 {
		return 1
	}
	if n > dashboardCommitMaxWeeks {
		return dashboardCommitMaxWeeks
	}
	return n
}

func commitActivity(ctx context.Context, dir string, weeks int, now time.Time, run gitCommand) ([]daySeriesPoint, bool, error) {
	days := weeks * 7
	points := bucketCommitsByLocalDay(nil, days, now)

	inside, err := run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if isNotGitRepository(err) {
			return points, false, nil
		}
		return nil, false, err
	}
	if inside != "true" {
		return points, false, nil
	}

	location := now.Location()
	oldest := now.AddDate(0, 0, -(days - 1))
	start := time.Date(oldest.Year(), oldest.Month(), oldest.Day(), 0, 0, 0, 0, location)
	out, err := run(ctx, dir, "log", "--format=%ct", "--since="+start.Format(time.RFC3339))
	if err != nil {
		return nil, true, err
	}
	if out == "" {
		return points, true, nil
	}

	stamps := make([]int64, 0, strings.Count(out, "\n")+1)
	for _, line := range strings.Split(out, "\n") {
		stamp, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
		if err != nil {
			return nil, true, err
		}
		stamps = append(stamps, stamp)
	}
	return bucketCommitsByLocalDay(stamps, days, now), true, nil
}

func bucketCommitsByLocalDay(stamps []int64, days int, now time.Time) []daySeriesPoint {
	keys := dayKeys(days, now)
	index := make(map[string]int, len(keys))
	points := make([]daySeriesPoint, len(keys))
	for i, key := range keys {
		index[key] = i
		points[i] = daySeriesPoint{Day: key}
	}
	for _, stamp := range stamps {
		day := time.Unix(stamp, 0).In(now.Location()).Format("2006-01-02")
		if i, ok := index[day]; ok {
			points[i].Value++
		}
	}
	return points
}

func (s *Server) handleDashboardCommitActivity(w http.ResponseWriter, r *http.Request) {
	weeks := commitActivityWeeks(r.URL.Query().Get("weeks"))
	projectDir := ws(r).Runtime.WorkspaceDefaultDir()
	points, isGitRepo, err := commitActivity(r.Context(), projectDir, weeks, time.Now(), runDashboardGit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "git command failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"weeks":        weeks,
		"commitsByDay": points,
		"isGitRepo":    isGitRepo,
	})
}
