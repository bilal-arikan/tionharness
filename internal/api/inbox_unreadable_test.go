package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// persistedDebugName returns the value the debug journal actually stores for a
// given event Name. Names outside the sanitizer's allow-list are fingerprinted
// before they hit disk, so the expected value is derived by round-tripping one
// control event through the real write/read path instead of re-deriving the hash
// here (which would pin a private detail of the db package).
func persistedDebugName(t *testing.T, database *db.DB, name string) string {
	t.Helper()
	ctx := context.Background()
	agent, err := database.CreateAgent(ctx, db.Agent{Name: "probe"})
	if err != nil {
		t.Fatalf("probe agent: %v", err)
	}
	probe, err := database.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat", Title: "probe"})
	if err != nil {
		t.Fatalf("probe session: %v", err)
	}
	if err := database.AppendDebugEventGated(probe.ID, db.DebugEvent{
		Type: db.DebugError, Name: name, Err: true,
	}); err != nil {
		t.Fatalf("probe debug event: %v", err)
	}
	evs, err := database.ReadDebugEvents(ctx, probe.ID, db.DebugError, 0)
	if err != nil {
		t.Fatalf("read probe events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("probe journal must hold exactly one event, got %d", len(evs))
	}
	return evs[0].Name
}

func TestRecoverInboxesQuarantinesMalformedSidecar(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionDir, err := wsp.DB.SessionDir(sess.ID)
	if err != nil {
		t.Fatalf("session dir: %v", err)
	}
	sidecar := filepath.Join(sessionDir, "inbox.json")
	malformed := []byte(`{"inflight":`)
	if err := os.WriteFile(sidecar, malformed, 0o600); err != nil {
		t.Fatalf("write malformed sidecar: %v", err)
	}
	wantName := persistedDebugName(t, wsp.DB, "inbox_corrupt")

	s.recoverInboxes()

	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("corrupt sidecar still occupies inbox.json: %v", err)
	}
	matches, err := filepath.Glob(sidecar + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob quarantine files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("quarantine files = %v, want one", matches)
	}
	got, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read quarantine: %v", err)
	}
	if string(got) != string(malformed) {
		t.Fatalf("quarantine bytes = %q, want %q", got, malformed)
	}
	events, err := wsp.DB.ReadDebugEvents(ctx, sess.ID, db.DebugError, 0)
	if err != nil {
		t.Fatalf("read debug events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Name == wantName {
			found = true
		}
	}
	if !found {
		t.Fatalf("recoverInboxes did not record inbox_corrupt, journal: %+v", events)
	}
}

// TestRecoverInboxesReportsUnreadableSidecar covers the IO-failure branch of
// recoverInboxes: a sidecar that cannot be READ (as opposed to one that is simply
// absent) may hold waiting user messages, so boot must not treat it as "nothing
// was queued". The loss has to surface on the session's debug journal, and the
// unreadable file must be left alone (quarantine is for corrupt content, not an
// IO fault).
//
// The read failure is injected by putting a DIRECTORY where inbox.json belongs:
// os.ReadFile then fails on every platform, unlike a chmod-based permission trick
// which is unreliable on Windows.
func TestRecoverInboxesReportsUnreadableSidecar(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()

	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionDir, err := wsp.DB.SessionDir(sess.ID)
	if err != nil {
		t.Fatalf("session dir: %v", err)
	}
	sidecar := filepath.Join(sessionDir, "inbox.json")
	if err := os.Mkdir(sidecar, 0o755); err != nil {
		t.Fatalf("plant unreadable sidecar: %v", err)
	}

	// The injection is only useful if the read really fails — assert it before
	// relying on it, so a platform where a directory reads fine fails loudly here
	// rather than silently turning the test into a no-op.
	if _, _, err := wsp.DB.ReadInbox(sess.ID); err == nil {
		t.Fatal("planting a directory at inbox.json did not make ReadInbox fail")
	}

	wantName := persistedDebugName(t, wsp.DB, "inbox_unreadable")

	s.recoverInboxes()

	evs, err := wsp.DB.ReadDebugEvents(ctx, sess.ID, db.DebugError, 0)
	if err != nil {
		t.Fatalf("read debug events: %v", err)
	}
	found := false
	for _, ev := range evs {
		if ev.Name == wantName {
			found = true
			if !ev.Err {
				t.Fatalf("inbox_unreadable must be flagged as an error: %+v", ev)
			}
		}
	}
	if !found {
		t.Fatalf("recoverInboxes did not record an inbox_unreadable event, journal: %+v", evs)
	}

	// The session must NOT come up with a silently empty queue: the unreadable
	// sidecar is skipped, so no queue is registered for it at all.
	s.inbox.lock()
	ib := s.inbox.at(wsp.ID, sess.ID)
	s.inbox.unlock()
	if ib != nil {
		t.Fatalf("an unreadable sidecar must not produce a queue, got %+v", ib.items)
	}

	// An IO fault leaves the file in place — quarantine is reserved for content
	// that parsed badly.
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("unreadable sidecar must be left untouched: %v", err)
	}
	matches, err := filepath.Glob(sidecar + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob quarantine files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("an IO fault must not quarantine the sidecar, found %v", matches)
	}
}
