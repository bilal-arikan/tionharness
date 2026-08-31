package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestParallelLoadPreservesInputOrder: results must line up with the input even
// though the work completes out of order — every caller indexes the result by
// position or feeds it straight into an id-keyed map.
func TestParallelLoadPreservesInputOrder(t *testing.T) {
	in := make([]int, 200)
	for i := range in {
		in[i] = i
	}
	out, err := parallelLoad(in, func(i int) (string, error) {
		return fmt.Sprintf("v%d", i), nil
	})
	if err != nil {
		t.Fatalf("parallelLoad: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if want := fmt.Sprintf("v%d", i); out[i] != want {
			t.Fatalf("out[%d] = %q, want %q", i, out[i], want)
		}
	}
}

// TestParallelLoadReturnsFirstErrorByInputOrder: boot treats a corrupt entity
// file as fatal, so WHICH file gets blamed must not depend on scheduling. With
// two failing items the earlier one always wins.
func TestParallelLoadReturnsFirstErrorByInputOrder(t *testing.T) {
	errLate := errors.New("late")
	errEarly := errors.New("early")

	for attempt := 0; attempt < 50; attempt++ {
		in := make([]int, 100)
		for i := range in {
			in[i] = i
		}
		_, err := parallelLoad(in, func(i int) (int, error) {
			switch i {
			case 10:
				return 0, errEarly
			case 90:
				return 0, errLate
			}
			return i, nil
		})
		if !errors.Is(err, errEarly) {
			t.Fatalf("attempt %d: err = %v, want the earlier-indexed error", attempt, err)
		}
	}
}

// TestParallelLoadRunsEveryItem: the pool must not drop work when the item count
// is not a multiple of the worker count, and must handle the degenerate sizes.
func TestParallelLoadRunsEveryItem(t *testing.T) {
	for _, n := range []int{0, 1, 2, 15, 16, 17, 500} {
		var calls atomic.Int64
		in := make([]int, n)
		out, err := parallelLoad(in, func(int) (int, error) {
			calls.Add(1)
			return 1, nil
		})
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if got := int(calls.Load()); got != n {
			t.Fatalf("n=%d: fn ran %d times", n, got)
		}
		if len(out) != n {
			t.Fatalf("n=%d: len(out) = %d", n, len(out))
		}
		if out == nil {
			t.Fatalf("n=%d: out is nil; callers rely on a non-nil empty slice", n)
		}
	}
}

func TestLoadWorkersBounds(t *testing.T) {
	if got := loadWorkers(0); got != 1 {
		t.Fatalf("loadWorkers(0) = %d, want 1", got)
	}
	if got := loadWorkers(1); got != 1 {
		t.Fatalf("loadWorkers(1) = %d, want 1", got)
	}
	if got := loadWorkers(3); got > 3 {
		t.Fatalf("loadWorkers(3) = %d, must not exceed the item count", got)
	}
	if got := loadWorkers(10_000); got > maxLoadWorkers {
		t.Fatalf("loadWorkers(10000) = %d, must not exceed maxLoadWorkers=%d", got, maxLoadWorkers)
	}
}

// Per-file corruption is no longer routed through parallelLoad's error path:
// loadJSONDir skips and logs the bad file so one entity cannot take the whole
// store down. That behaviour is covered by TestLoadJSONDirSkipsCorruptFile in
// store_append_rollback_test.go.

// TestConcurrentBootLoadsEverySession is the end-to-end guard on the concurrent
// loaders: a store with many sessions must come back byte-identical after a
// reopen, with no session lost, duplicated or cross-wired.
func TestConcurrentBootLoadsEverySession(t *testing.T) {
	ctx := t.Context()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	const sessions = 60
	want := map[string][]string{} // session id -> message texts
	for i := 0; i < sessions; i++ {
		s, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: fmt.Sprintf("S%d", i)})
		if err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
		for j := 0; j <= i%5; j++ {
			text := fmt.Sprintf("s%d-m%d", i, j)
			if _, err := d.AddMessage(ctx, Message{SessionID: s.ID, Role: "user", Text: text}); err != nil {
				t.Fatalf("add message: %v", err)
			}
			want[s.ID] = append(want[s.ID], text)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.ListSessions(ctx, "")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(got) != sessions {
		t.Fatalf("reopened %d sessions, want %d", len(got), sessions)
	}
	for id, texts := range want {
		msgs, err := d2.ListMessages(ctx, id)
		if err != nil {
			t.Fatalf("list messages %s: %v", id, err)
		}
		if len(msgs) != len(texts) {
			t.Fatalf("session %s: %d messages, want %d", id, len(msgs), len(texts))
		}
		for i, text := range texts {
			if msgs[i].Text != text {
				t.Fatalf("session %s message %d = %q, want %q", id, i, msgs[i].Text, text)
			}
			if msgs[i].SessionID != id {
				t.Fatalf("session %s message %d carries sessionId %q — transcripts got cross-wired",
					id, i, msgs[i].SessionID)
			}
		}
	}
}
