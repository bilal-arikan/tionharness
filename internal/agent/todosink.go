package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/progress"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// todoSink is a tools.TodoSink that persists the agent's working checklist to a
// durable progress file (internal/progress) so the list survives a fresh
// session load. The file is PER-SESSION (keyed by session id under the workspace
// store) — it is NOT shared across sessions that happen to use the same working
// directory. Each session keeps its own checklist so two sessions on the same
// project never overwrite each other's progress. Best-effort.
type todoSink struct {
	publish   func(events.Event)
	db        *db.DB
	sessionID string
	agentID   string
	dir       string // resolved per-session directory
}

// ProgressDir resolves where a session's persistent progress file lives: a
// PER-SESSION directory under the store, keyed by session id. Progress is
// session-specific — it is never keyed by the working directory, so sessions
// sharing a project dir keep independent checklists.
//
// Single source of truth for both the persist (NewTodoSink) and the read paths
// (resume context block + the session-detail progress card) so they always agree.
func (r *Runtime) ProgressDir(sessionID string) string {
	return filepath.Join(r.db.Root(), "progress", sessionID)
}

// NewTodoSink builds a todo sink bound to a session + agent for this workspace,
// resolving where the progress file lives via ProgressDir (explicit project dir,
// else a per-session fallback). Mirrors NewArtifactSink: the native loop installs
// it as a fallback; the CLI Interaction bridge sets it on the run.
func (r *Runtime) NewTodoSink(sessionID, agentID string) tools.TodoSink {
	return &todoSink{publish: r.publish, db: r.db, sessionID: sessionID, agentID: agentID, dir: r.ProgressDir(sessionID)}
}

// LoadTodos returns the checklist last persisted to the progress file, backing
// todo_write's compact `set` update form (status-only changes merged server-side).
func (s *todoSink) LoadTodos(_ context.Context) ([]tools.TodoSinkItem, bool, error) {
	rec, ok, err := progress.Load(s.dir)
	if err != nil || !ok || len(rec.Todos) == 0 {
		return nil, false, err
	}
	items := make([]tools.TodoSinkItem, len(rec.Todos))
	for i, t := range rec.Todos {
		items[i] = tools.TodoSinkItem{Content: t.Content, Status: t.Status, Category: t.Category, Steps: t.Steps}
	}
	return items, true, nil
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
