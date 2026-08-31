package view

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// countingStore counts list reads and models the real store's cost: db.DB hands
// out a fresh, normalised, sorted COPY on every ListSessions call, so a fake that
// returns its own backing slice would hide exactly the waste under test.
type countingStore struct {
	*fakeStore
	sessionReads int
	taskReads    int
}

func (s *countingStore) ListSessions(_ context.Context, agentID string) ([]db.Session, error) {
	s.sessionReads++
	out := make([]db.Session, 0, len(s.fakeStore.sessions))
	for _, session := range s.fakeStore.sessions {
		if agentID == "" || session.AgentID == agentID {
			out = append(out, session)
		}
	}
	// Byte-for-byte the comparator db.DB.ListSessions uses: pinned first, then
	// most-recently-updated, then a descending-ID tie-break. Any drift here makes
	// TestAgentSessionChildrenMatchesStoreFilter compare the cache against an
	// order production never produces.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *countingStore) ListActiveTasks(_ context.Context) ([]db.Task, error) {
	s.taskReads++
	return append([]db.Task(nil), s.fakeStore.tasks...), nil
}

func neighborhoodFixture(sessionCount int) *countingStore {
	sessions := []db.Session{
		{ID: "A", AgentID: "AG1", UpdatedAt: 100, CoordinatorSessionID: "B"},
		{ID: "B", AgentID: "AG1", UpdatedAt: 100, CoordinatorSessionID: "A"},
	}
	for i := 0; i < sessionCount; i++ {
		sessions = append(sessions, db.Session{
			ID:                   fmt.Sprintf("S%d", i),
			AgentID:              fmt.Sprintf("AG%d", i%4),
			UpdatedAt:            int64(100 - i),
			CoordinatorSessionID: "A",
		})
	}
	// AG2 carries the ordering edge cases the equivalence test needs: two sessions
	// sharing an UpdatedAt (so the ID tie-break actually runs) and a pinned session
	// with the oldest timestamp (so the pin has to float it past everything else).
	sessions = append(sessions,
		db.Session{ID: "T1", AgentID: "AG2", UpdatedAt: 50, CoordinatorSessionID: "A"},
		db.Session{ID: "T2", AgentID: "AG2", UpdatedAt: 50, CoordinatorSessionID: "A"},
		db.Session{ID: "P1", AgentID: "AG2", UpdatedAt: 1, Pinned: true, CoordinatorSessionID: "A"},
	)
	return &countingStore{fakeStore: &fakeStore{
		agents: []db.Agent{
			{ID: "AG0", Name: "zero"}, {ID: "AG1", Name: "builder"},
			{ID: "AG2", Name: "two"}, {ID: "AG3", Name: "three"},
		},
		sessions: sessions,
		tasks:    []db.Task{{ID: "TSK1", BoardState: "todo", UpdatedAt: 10}},
	}}
}

// TestNeighborhoodReadsEachListOnce pins the fix: the parent scan visits every
// node in the workspace, and each session-shaped node used to re-read the whole
// session list. One walk must now read each store list exactly once.
func TestNeighborhoodReadsEachListOnce(t *testing.T) {
	store := neighborhoodFixture(40)
	p := NewProjector(store)

	if _, err := p.Neighborhood(context.Background(), Ref{Kind: KindSession, ID: "A"}); err != nil {
		t.Fatalf("neighborhood: %v", err)
	}
	if store.sessionReads != 1 {
		t.Errorf("session list read %d times, want 1", store.sessionReads)
	}
	if store.taskReads != 1 {
		t.Errorf("task list read %d times, want 1", store.taskReads)
	}
}

// TestAgentSessionChildrenMatchesStoreFilter guards the one behavioural change
// the cache makes: an agent's sessions are now filtered from the full snapshot
// instead of being filtered by the store. Both must produce the same handles in
// the same order.
func TestAgentSessionChildrenMatchesStoreFilter(t *testing.T) {
	store := neighborhoodFixture(20)
	p := NewProjector(store)
	ctx := context.Background()

	got, err := p.agentSessionChildren(ctx, "AG2")
	if err != nil {
		t.Fatalf("agent children: %v", err)
	}
	filtered, err := store.ListSessions(ctx, "AG2")
	if err != nil {
		t.Fatalf("store filter: %v", err)
	}
	// These two guards keep the FIXTURE honest — they assert that countingStore's
	// comparator still reaches its pin and tie-break branches, so the fake keeps
	// mirroring db.DB.ListSessions. Only the tie-break carries into the equivalence
	// below: both sides feed sessionHandleList, which re-sorts on UpdatedAt alone,
	// so two sessions sharing an UpdatedAt must arrive in the same relative order on
	// both paths. The pin does NOT survive that re-sort — this test says nothing
	// about pinned sessions being ordered first in the projected handles.
	if filtered[0].ID != "P1" {
		t.Fatalf("fixture no longer exercises the pin branch: first session = %q", filtered[0].ID)
	}
	tie := -1
	for i := 0; i+1 < len(filtered); i++ {
		if filtered[i].ID == "T2" && filtered[i+1].ID == "T1" {
			tie = i
			break
		}
	}
	if tie < 0 {
		t.Fatalf("fixture no longer exercises the equal-UpdatedAt tie-break: %+v", filtered)
	}

	want := sessionHandleList(liveSessions(filtered))
	if len(got) != len(want) {
		t.Fatalf("handle count=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("handle %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func BenchmarkNeighborhood(b *testing.B) {
	for _, size := range []int{50, 200} {
		b.Run(fmt.Sprintf("sessions=%d", size), func(b *testing.B) {
			p := NewProjector(neighborhoodFixture(size))
			ctx := context.Background()
			focus := Ref{Kind: KindSession, ID: "A"}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := p.Neighborhood(ctx, focus); err != nil {
					b.Fatalf("neighborhood: %v", err)
				}
			}
		})
	}
}
