package api

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// recordHandler collects the messages passed to a slog.Logger.
type recordHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordHandler) WithGroup(string) slog.Handler { return h }

// warnCount returns how many warn-level records mention substr.
func (h *recordHandler) warnCount(substr string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, substr) {
			n++
		}
	}
	return n
}

// TestAutonomousInteractionWarnsWithoutSessionID: a session-less autonomous turn
// silently loses every session-scoped sink (artifacts, notify, focus_view,
// update_session, todo persistence, schedule_wake) while shell and spawn keep
// running against the workspace default dir. The degradation must be announced
// once at setup instead of being inferred from a missing artifact later.
func TestAutonomousInteractionWarnsWithoutSessionID(t *testing.T) {
	s, rt := newAutonomousTestServer(t)
	h := &recordHandler{}
	s.logger = slog.New(h)

	ag := db.Agent{ID: "AG1", Provider: "claude-cli", PermissionMode: "auto"}
	_, done := s.autonomousInteraction(rt)(context.Background(), ag, "")
	defer done()

	if got := h.warnCount("no session id"); got != 1 {
		t.Fatalf("warn count for the session-less setup = %d, want exactly 1", got)
	}
}

// TestAutonomousInteractionQuietWithSessionID keeps the warning honest: a normal
// turn must not emit it, or it becomes noise operators learn to ignore.
func TestAutonomousInteractionQuietWithSessionID(t *testing.T) {
	s, rt := newAutonomousTestServer(t)
	h := &recordHandler{}
	s.logger = slog.New(h)

	ag := db.Agent{ID: "AG1", Provider: "claude-cli", PermissionMode: "auto"}
	_, done := s.autonomousInteraction(rt)(context.Background(), ag, "SES1")
	defer done()

	if got := h.warnCount("no session id"); got != 0 {
		t.Fatalf("warn count for a normal setup = %d, want 0", got)
	}
}
