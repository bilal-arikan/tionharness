package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/progress"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// todoSink is a tools.TodoSink that persists the agent's working checklist to a
// durable progress file (internal/progress) so the list survives across
// sessions. When the session has an explicit project working directory the file
// lives in the project (<cwd>/.tionswarm/progress.json) — git-committable, portable
// and SHARED across every session working on that same project. With no explicit
// project dir it falls back to a PER-SESSION file under the workspace store, so
// unrelated sessions (which would otherwise all share the workspace-default dir)
// keep their own progress instead of overwriting each other. Best-effort.
type todoSink struct {
	publish   func(events.Event)
	db        *db.DB
	sessionID string
	agentID   string
	dir       string // resolved project/per-session directory
}

// ProgressDir resolves where a session's persistent progress file lives:
//   - the session's EXPLICIT working directory (a real project) → SHARED across
//     all sessions on that project (the claude-progress cross-session convention).
//   - otherwise a PER-SESSION directory under the store, so sessions without a
//     project dir don't all collide on one workspace-default progress file.
//
// Single source of truth for both the persist (NewTodoSink) and the read paths
// (resume context block + the session-detail progress card) so they always agree.
func (r *Runtime) ProgressDir(sessionID string) string {
	if sessionID != "" {
		if s, err := r.db.GetSession(context.Background(), sessionID); err == nil {
			if d := strings.TrimSpace(s.WorkingDir); d != "" {
				if info, statErr := os.Stat(d); statErr == nil && info.IsDir() {
					return d
				}
			}
		}
	}
	return filepath.Join(r.db.Root(), "progress", sessionID)
}

// NewTodoSink builds a todo sink bound to a session + agent for this workspace,
// resolving where the progress file lives via ProgressDir (explicit project dir,
// else a per-session fallback). Mirrors NewArtifactSink: the native loop installs
// it as a fallback; the CLI Interaction bridge sets it on the run.
func (r *Runtime) NewTodoSink(sessionID, agentID string) tools.TodoSink {
	return &todoSink{publish: r.publish, db: r.db, sessionID: sessionID, agentID: agentID, dir: r.ProgressDir(sessionID)}
}

func (s *todoSink) SaveTodos(_ context.Context, todos []tools.TodoSinkItem) error {
	items := make([]progress.TodoItem, len(todos))
	var done, active, pending int
	for i, t := range todos {
		items[i] = progress.TodoItem{Content: t.Content, Status: t.Status, Category: t.Category, Steps: t.Steps}
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			active++
		default:
			pending++
		}
	}
	now := time.Now().Unix()
	// Preserve any existing rolling log; append a one-line summary for this update.
	rec, _, _ := progress.Load(s.dir)
	rec.UpdatedAt = now
	rec.SessionID = s.sessionID
	rec.AgentID = s.agentID
	rec.Todos = items
	rec.Log = append(rec.Log, progress.LogEntry{
		TS:        now,
		SessionID: s.sessionID,
		Note:      fmt.Sprintf("%d/%d completed (%d in progress, %d pending)", done, len(items), active, pending),
	})
	if err := progress.Save(s.dir, rec); err != nil {
		return err
	}
	if s.publish != nil {
		s.publish(events.Event{
			Type:   "progress",
			Level:  "info",
			Title:  "Progress saved",
			Body:   fmt.Sprintf("%d/%d completed", done, len(items)),
			Target: map[string]string{"sessionId": s.sessionID},
		})
	}
	return nil
}
