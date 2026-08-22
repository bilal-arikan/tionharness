package workspace_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/config"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

func TestMessageAppendEmitsSessionActivityForEveryAuthor(t *testing.T) {
	root := t.TempDir()
	cipher, err := config.LoadSecret(root)
	if err != nil {
		t.Fatalf("load cipher: %v", err)
	}
	bus := events.NewBus()
	manager, err := workspace.NewManager(
		root,
		providers.NewRegistry(),
		agent.NewTunables(),
		cipher,
		bus,
		logbuf.New(16),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(manager.Close)

	wsp, err := manager.Create("test", "", "test")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	session, err := wsp.DB.CreateSession(context.Background(), db.Session{})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	subID, eventCh := bus.Subscribe()
	t.Cleanup(func() { bus.Unsubscribe(subID) })

	for _, role := range []string{"user", "assistant"} {
		if _, err := wsp.DB.AddMessage(context.Background(), db.Message{
			SessionID: session.ID,
			Role:      role,
			Text:      role + " message",
		}); err != nil {
			t.Fatalf("add %s message: %v", role, err)
		}

		select {
		case event := <-eventCh:
			if event.Type != events.TypeSession || event.Target["sessionId"] != session.ID || event.Target["op"] != "message_activity" {
				t.Fatalf("%s event = %+v", role, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("no session activity event for %s message", role)
		}
	}
}
