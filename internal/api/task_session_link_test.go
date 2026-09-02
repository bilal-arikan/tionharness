package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// decodeTask reads a Task out of a JSON response body.
func decodeTask(t *testing.T, body []byte) db.Task {
	t.Helper()
	var task db.Task
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatalf("decode task: %v (body %s)", err, body)
	}
	return task
}

func TestGetTaskReturnsOneCard(t *testing.T) {
	handler, wsp := taskRoutesFixture(t)
	made, err := wsp.DB.CreateTask(context.Background(), db.Task{Title: "card", Prompt: "do it"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	rec := doJSON(t, handler, http.MethodGet, "/api/tasks/"+made.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	got := decodeTask(t, rec.Body.Bytes())
	if got.ID != made.ID || got.Title != "card" || got.Prompt != "do it" {
		t.Fatalf("got %+v, want the created card", got)
	}
}

func TestGetTaskUnknownIDIs404(t *testing.T) {
	handler, _ := taskRoutesFixture(t)
	rec := doJSON(t, handler, http.MethodGet, "/api/tasks/TSK-nope", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkTaskSession(t *testing.T) {
	handler, wsp := taskRoutesFixture(t)
	ctx := context.Background()
	task, err := wsp.DB.CreateTask(ctx, db.Task{Title: "card"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Title: "worker"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	t.Run("links and returns the card", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/sessions",
			map[string]string{"sessionId": sess.ID}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		got := decodeTask(t, rec.Body.Bytes())
		if len(got.SessionIDs) != 1 || got.SessionIDs[0] != sess.ID {
			t.Fatalf("sessionIds = %v, want [%s]", got.SessionIDs, sess.ID)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		// A retried call must not grow the list — an external driver that times
		// out and retries would otherwise duplicate the link.
		rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/sessions",
			map[string]string{"sessionId": sess.ID}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if got := decodeTask(t, rec.Body.Bytes()); len(got.SessionIDs) != 1 {
			t.Fatalf("sessionIds = %v after re-link, want exactly one", got.SessionIDs)
		}
	})

	t.Run("rejects an unknown session", func(t *testing.T) {
		// A card pointing at a nonexistent session is worse than no link.
		rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/sessions",
			map[string]string{"sessionId": "SES-nope"}, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejects an empty session id", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/sessions",
			map[string]string{"sessionId": ""}, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown task is 404", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodPost, "/api/tasks/TSK-nope/sessions",
			map[string]string{"sessionId": sess.ID}, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestUpdateTaskPreservesSessionLinks guards the server-owned invariant: a
// client PUTs whatever it last read, so a payload with no sessionIds must not
// erase links added since it read the card.
func TestUpdateTaskPreservesSessionLinks(t *testing.T) {
	handler, wsp := taskRoutesFixture(t)
	ctx := context.Background()
	task, err := wsp.DB.CreateTask(ctx, db.Task{Title: "card", BoardState: db.BoardTodo})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Title: "worker"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := wsp.DB.LinkTaskSession(ctx, task.ID, sess.ID); err != nil {
		t.Fatalf("link: %v", err)
	}

	rec := doJSON(t, handler, http.MethodPut, "/api/tasks/"+task.ID,
		map[string]any{"title": "renamed", "boardState": db.BoardTodo}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", rec.Code, rec.Body.String())
	}

	after, err := wsp.DB.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if len(after.SessionIDs) != 1 || after.SessionIDs[0] != sess.ID {
		t.Fatalf("sessionIds = %v after update, want the link preserved", after.SessionIDs)
	}
	if after.Title != "renamed" {
		t.Fatalf("title = %q, want the update to have applied", after.Title)
	}
}
