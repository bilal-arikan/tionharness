package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestInflightRecorder verifies the non-stream chat path's crash sidecar
// (streaming-path parity): persistable steps and text deltas accumulate and land
// in the session's inflight sidecar, transient step kinds are excluded, and
// interruptedTrace appends the trailing error step without losing the kept ones.
func TestInflightRecorder(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	sess, err := database.CreateSession(context.Background(), db.Session{AgentID: "AGT1", Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := &inflightRecorder{db: database, sessionID: sess.ID, agentID: "AGT1", replyID: "MSG_reply", startedAt: 42}
	rec.onStep(agent.TurnStep{Kind: agent.StepTool, Tool: "Write", Output: "ok"})
	rec.onStep(agent.TurnStep{Kind: agent.StepDelta, Text: "partial "})
	rec.onStep(agent.TurnStep{Kind: agent.StepDelta, Text: "answer"})
	// Rewind the write throttle so the next step flushes a fresh snapshot (the
	// deltas above landed within the 600ms window of the first write).
	rec.lastSnap = rec.lastSnap.Add(-time.Second)
	rec.onStep(agent.TurnStep{Kind: agent.StepToolDelta, Text: "TRANSIENT"}) // live-UI only

	got, ok, err := database.ReadInflight(sess.ID)
	if err != nil || !ok {
		t.Fatalf("sidecar must exist after steps: ok=%v err=%v", ok, err)
	}
	if got.MessageID != "MSG_reply" || got.SessionID != sess.ID || got.AgentID != "AGT1" || got.StartedAt != 42 {
		t.Errorf("sidecar identity mismatch: %+v", got)
	}
	if got.Text != "partial answer" {
		t.Errorf("deltas must accumulate into Text, got %q", got.Text)
	}
	if !strings.Contains(got.Steps, "Write") {
		t.Errorf("kept tool step missing from snapshot steps: %s", got.Steps)
	}
	if strings.Contains(got.Steps, "TRANSIENT") {
		t.Errorf("transient step kinds must not be persisted: %s", got.Steps)
	}

	trace := rec.interruptedTrace("boom", "provider_error")
	if len(trace) != 2 || trace[len(trace)-1].Kind != agent.StepError || trace[len(trace)-1].Reason != "provider_error" {
		t.Errorf("interruptedTrace must be kept steps + trailing error step, got %+v", trace)
	}
}
