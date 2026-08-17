package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// seedTranscript opens a fresh store and appends n user messages "m0".."m<n-1>",
// returning the store, the session and the message ids in order.
func seedTranscript(t *testing.T, n int) (*DB, Session, []string) {
	t.Helper()
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		m, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "m"})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		ids = append(ids, m.ID)
	}
	return d, sess, ids
}

// TestListMessagesTail covers the window arithmetic, including the start index
// that view.loadSession reports as "messages not looked at".
func TestListMessagesTail(t *testing.T) {
	ctx := context.Background()
	d, sess, ids := seedTranscript(t, 10)

	cases := []struct {
		name     string
		n        int
		wantLen  int
		wantFrom int
		wantHead string // id expected at position 0
	}{
		{"tail smaller than transcript", 3, 3, 7, ids[7]},
		{"tail equals transcript", 10, 10, 0, ids[0]},
		{"tail larger than transcript", 50, 10, 0, ids[0]},
		{"zero means everything", 0, 10, 0, ids[0]},
		{"negative means everything", -5, 10, 0, ids[0]},
		{"single", 1, 1, 9, ids[9]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs, from, err := d.ListMessagesTail(ctx, sess.ID, tc.n)
			if err != nil {
				t.Fatalf("tail: %v", err)
			}
			if len(msgs) != tc.wantLen || from != tc.wantFrom {
				t.Fatalf("got len=%d from=%d, want len=%d from=%d", len(msgs), from, tc.wantLen, tc.wantFrom)
			}
			if msgs[0].ID != tc.wantHead {
				t.Fatalf("head id = %s, want %s", msgs[0].ID, tc.wantHead)
			}
			// The tail must always end at the newest message.
			if msgs[len(msgs)-1].ID != ids[len(ids)-1] {
				t.Fatalf("tail does not end at the newest message")
			}
		})
	}
}

// TestListMessagesTailEmptyAndUnknown pins the two edge answers apart: an empty
// session is a valid empty tail, an unknown session is an error. Collapsing them
// is how a wrong session id renders as an empty chat.
func TestListMessagesTailEmptyAndUnknown(t *testing.T) {
	ctx := context.Background()
	d, sess, _ := seedTranscript(t, 0)

	msgs, from, err := d.ListMessagesTail(ctx, sess.ID, 5)
	if err != nil || len(msgs) != 0 || from != 0 {
		t.Fatalf("empty session: got %d msgs from=%d err=%v, want 0/0/nil", len(msgs), from, err)
	}
	if _, _, err := d.ListMessagesTail(ctx, "SESnope", 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session err = %v, want ErrNotFound", err)
	}
}

// TestListMessagesTailIsACopy guards the invariant every reader here relies on:
// callers get their own memory, so mutating a result cannot corrupt the store.
func TestListMessagesTailIsACopy(t *testing.T) {
	ctx := context.Background()
	d, sess, _ := seedTranscript(t, 4)

	msgs, _, err := d.ListMessagesTail(ctx, sess.ID, 2)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	msgs[0].Text = "mutated"

	again, _, err := d.ListMessagesTail(ctx, sess.ID, 2)
	if err != nil {
		t.Fatalf("tail again: %v", err)
	}
	if again[0].Text != "m" {
		t.Fatalf("store text = %q, want %q — the returned slice aliased the store", again[0].Text, "m")
	}
}

func TestLastMessage(t *testing.T) {
	ctx := context.Background()
	d, sess, ids := seedTranscript(t, 3)

	m, ok, err := d.LastMessage(ctx, sess.ID)
	if err != nil || !ok {
		t.Fatalf("last: ok=%v err=%v", ok, err)
	}
	if m.ID != ids[2] {
		t.Fatalf("last id = %s, want %s", m.ID, ids[2])
	}

	if _, _, err := d.LastMessage(ctx, "SESnope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session err = %v, want ErrNotFound", err)
	}
}

func TestLastMessageEmptySession(t *testing.T) {
	ctx := context.Background()
	d, sess, _ := seedTranscript(t, 0)

	m, ok, err := d.LastMessage(ctx, sess.ID)
	if err != nil {
		t.Fatalf("last: %v", err)
	}
	if ok {
		t.Fatalf("ok = true for an empty session (got %+v)", m)
	}
}

// TestFindMessage checks the two failure modes stay distinguishable: unknown
// session vs. unknown message inside a known session.
func TestFindMessage(t *testing.T) {
	ctx := context.Background()
	d, sess, ids := seedTranscript(t, 5)

	m, err := d.FindMessage(ctx, sess.ID, ids[2])
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if m.ID != ids[2] {
		t.Fatalf("found %s, want %s", m.ID, ids[2])
	}

	if _, err := d.FindMessage(ctx, sess.ID, "no-such-message"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("missing message err = %v, want ErrMessageNotFound", err)
	}
	if _, err := d.FindMessage(ctx, "SESnope", ids[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session err = %v, want ErrNotFound", err)
	}
}

func TestStreamMessages(t *testing.T) {
	ctx := context.Background()
	d, sess, ids := seedTranscript(t, 6)

	var seen []string
	if err := d.StreamMessages(ctx, sess.ID, func(m Message) bool {
		seen = append(seen, m.ID)
		return true
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(seen) != len(ids) {
		t.Fatalf("streamed %d, want %d", len(seen), len(ids))
	}
	for i := range ids {
		if seen[i] != ids[i] {
			t.Fatalf("order broke at %d: %s != %s", i, seen[i], ids[i])
		}
	}

	// Early stop must not walk the rest.
	count := 0
	if err := d.StreamMessages(ctx, sess.ID, func(Message) bool {
		count++
		return count < 2
	}); err != nil {
		t.Fatalf("stream early stop: %v", err)
	}
	if count != 2 {
		t.Fatalf("callback ran %d times after stopping at 2", count)
	}

	if err := d.StreamMessages(ctx, "SESnope", func(Message) bool { return true }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session err = %v, want ErrNotFound", err)
	}
}

// TestStatsCountsResidentMessages pins the measurement baseline: every session's
// transcript is resident after Open, and MessageBytes scales with content.
func TestStatsCountsResidentMessages(t *testing.T) {
	ctx := context.Background()
	d, sess, _ := seedTranscript(t, 4)

	st := d.Stats()
	if st.Sessions != 1 || st.LoadedSessions != 1 {
		t.Fatalf("sessions=%d loaded=%d, want 1/1", st.Sessions, st.LoadedSessions)
	}
	if st.Messages != 4 {
		t.Fatalf("messages = %d, want 4", st.Messages)
	}
	before := st.MessageBytes
	if before <= 0 {
		t.Fatalf("messageBytes = %d, want > 0", before)
	}

	// A message with a large trace must move the number, or the metric is useless.
	big := make([]byte, 0, 40_000)
	for len(big) < 40_000 {
		big = append(big, 'x')
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: string(big)}); err != nil {
		t.Fatalf("append big: %v", err)
	}
	after := d.Stats().MessageBytes
	if after-before < 40_000 {
		t.Fatalf("messageBytes grew by %d for a 40KB message, want >= 40000", after-before)
	}

	// A session that has never been written to retains nothing and must not be
	// counted as loaded — it would overstate the footprint lazy loading removes.
	fresh, err := d.CreateSession(ctx, Session{AgentID: "AGT1", Title: "fresh"})
	if err != nil {
		t.Fatalf("create fresh: %v", err)
	}
	st2 := d.Stats()
	if st2.Sessions != 2 {
		t.Fatalf("sessions = %d after creating %s, want 2", st2.Sessions, fresh.ID)
	}
	if st2.LoadedSessions != 1 {
		t.Fatalf("loadedSessions = %d, want 1 (the empty session retains nothing)", st2.LoadedSessions)
	}
}
