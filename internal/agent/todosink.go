package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/events"
	"github.com/bilal-arikan/swarmgo/internal/progress"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// todoSink is a tools.TodoSink that persists the agent's working checklist to a
// durable progress file (internal/progress) so the list survives across
// sessions. When the session has a working directory the file lives in the
// project (<cwd>/.swarmgo/progress.json) — git-committable and portable; with no
// cwd it falls back to a per-agent file under the workspace store so an agent
// still resumes its own progress. Persistence is best-effort.
type todoSink struct {
	publish   func(events.Event)
	db        *db.DB
	sessionID string
	agentID   string
	dir       string // resolved project/fallback directory
}

// NewTodoSink builds a todo sink bound to a session + agent for this workspace,
// resolving where the progress file lives from cwd (or the store fallback).
// Mirrors NewArtifactSink: the native loop installs it as a fallback; the CLI
// Interaction bridge sets it on the run.
func (r *Runtime) NewTodoSink(sessionID, agentID, cwd string) tools.TodoSink {
	dir := cwd
	if dir == "" {
		dir = filepath.Join(r.db.Root(), "progress", agentID)
	}
	return &todoSink{publish: r.publish, db: r.db, sessionID: sessionID, agentID: agentID, dir: dir}
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
