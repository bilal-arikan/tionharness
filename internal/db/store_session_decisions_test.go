package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func decisionStore(t *testing.T) (*DB, string) {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	session, err := d.CreateSession(context.Background(), Session{Title: "Decision test"})
	if err != nil {
		t.Fatal(err)
	}
	return d, session.ID
}

func TestSessionDecisionsEmptyAndReopen(t *testing.T) {
	d, id := decisionStore(t)
	ctx := context.Background()
	state, err := d.ReadSessionDecisions(ctx, id)
	if err != nil || state.SessionID != id {
		t.Fatalf("empty state: %+v, %v", state, err)
	}
	encoded, err := json.Marshal(state)
	if err != nil || strings.Contains(string(encoded), "null") {
		t.Fatalf("empty arrays must serialize as arrays: %s, %v", encoded, err)
	}
	if err := d.AppendSessionDecision(ctx, id, SessionDecision{Authority: "startup", Status: "applied"}); err != nil {
		t.Fatal(err)
	}
	if err := d.UpdateSessionDecisions(ctx, id, func(state *SessionDecisions) error {
		state.Memories = []DecisionMemory{{Key: "message-1", Text: "Keep this constraint", Pinned: true}}
		state.SelectedSkills = []string{"research"}
		state.SelectedTools = []string{"web_search"}
		state.Route = &DecisionRoute{Provider: "test", Model: "small", Pinned: true}
		state.CompactCount = 2
		state.Feedback = []DecisionFeedback{{DecisionID: state.Entries[0].ID, Rating: "helpful", At: 123}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(d.Root())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err = reopened.ReadSessionDecisions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || state.Entries[0].ID == "" || state.Entries[0].At < 1_000_000_000_000 || state.Entries[0].Items == nil {
		t.Fatalf("invalid generated event: %+v", state.Entries)
	}
	if state.CompactCount != 2 || len(state.Memories) != 1 || !state.Memories[0].Pinned || !state.Route.Pinned || len(state.Feedback) != 1 || state.SelectedSkills[0] != "research" || state.SelectedTools[0] != "web_search" {
		t.Fatalf("state did not survive reopening: %+v", state)
	}
}

func TestSessionDecisionsRejectUnknownPaths(t *testing.T) {
	d, _ := decisionStore(t)
	for _, id := range []string{"", "missing", "..", "../escape", "..\\escape", "/absolute", "C:\\escape"} {
		t.Run(id, func(t *testing.T) {
			if _, err := d.ReadSessionDecisions(context.Background(), id); !errors.Is(err, ErrNotFound) {
				t.Fatalf("read %q: %v", id, err)
			}
			called := false
			err := d.UpdateSessionDecisions(context.Background(), id, func(*SessionDecisions) error { called = true; return nil })
			if !errors.Is(err, ErrNotFound) || called {
				t.Fatalf("unknown session mutation: err=%v, called=%v", err, called)
			}
		})
	}
}

func TestSessionDecisionsRejectedMutationIsAtomic(t *testing.T) {
	d, id := decisionStore(t)
	ctx := context.Background()
	if err := d.AppendSessionDecision(ctx, id, SessionDecision{ID: "initial", Status: "observed"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d.Root(), dirSessions, id, sessionDecisionsFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*SessionDecisions) error{
		"callback":       func(state *SessionDecisions) error { state.CompactCount = 12; return errors.New("rejected") },
		"identity":       func(state *SessionDecisions) error { state.SessionID = "other"; return nil },
		"negative count": func(state *SessionDecisions) error { state.CompactCount = -1; return nil },
		"duplicate entry": func(state *SessionDecisions) error {
			state.Entries = append(state.Entries, state.Entries[0])
			return nil
		},
		"bad strength": func(state *SessionDecisions) error {
			state.Entries[0].Items = []DecisionItem{{Strength: math.NaN()}}
			return nil
		},
		"bad memory": func(state *SessionDecisions) error { state.Memories = []DecisionMemory{{Key: ""}}; return nil },
		"bad feedback": func(state *SessionDecisions) error {
			state.Feedback = []DecisionFeedback{{DecisionID: "initial", Rating: "unknown"}}
			return nil
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := d.UpdateSessionDecisions(ctx, id, mutate); err == nil {
				t.Fatal("invalid mutation succeeded")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatalf("rejected mutation changed durable state: %v", err)
			}
		})
	}
	ctxCanceled, cancel := context.WithCancel(ctx)
	err = d.UpdateSessionDecisions(ctxCanceled, id, func(state *SessionDecisions) error {
		state.CompactCount = 42
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled mutation: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("canceled mutation changed durable state")
	}
	if err := d.UpdateSessionDecisions(ctx, id, nil); err == nil {
		t.Fatal("nil callback succeeded")
	}
}

func TestSessionDecisionsConcurrentUpdatesDoNotLoseEntries(t *testing.T) {
	d, id := decisionStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := d.AppendSessionDecision(ctx, id, SessionDecision{Status: "applied"}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := d.UpdateSessionDecisions(ctx, id, func(state *SessionDecisions) error { state.CompactCount++; return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	state, err := d.ReadSessionDecisions(ctx, id)
	if err != nil || len(state.Entries) != 50 || state.CompactCount != 50 {
		t.Fatalf("lost a concurrent update: entries=%d count=%d err=%v", len(state.Entries), state.CompactCount, err)
	}
}

func TestSessionDecisionsBoundsPreserveProtectedMemory(t *testing.T) {
	d, id := decisionStore(t)
	err := d.UpdateSessionDecisions(context.Background(), id, func(state *SessionDecisions) error {
		for i := 0; i < 140; i++ {
			state.Entries = append(state.Entries, SessionDecision{ID: fmt.Sprintf("decision-%d", i), Error: strings.Repeat("ş", 150), Items: []DecisionItem{{Label: strings.Repeat("ş", 150)}}})
			state.Feedback = append(state.Feedback, DecisionFeedback{DecisionID: fmt.Sprintf("decision-%d", i), Rating: "helpful"})
		}
		for i := 0; i < 40; i++ {
			state.Memories = append(state.Memories, DecisionMemory{Key: fmt.Sprintf("memory-%d", i), Label: strings.Repeat("x", 300), Text: strings.Repeat("ş", 6000), Pinned: i == 0, Mandatory: i == 1})
			state.SelectedSkills = append(state.SelectedSkills, fmt.Sprintf("skill-%d", i))
			state.SelectedTools = append(state.SelectedTools, fmt.Sprintf("tool-%d", i))
		}
		state.SelectedTools = append(state.SelectedTools, "tool-0", "")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := d.ReadSessionDecisions(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 128 || state.Entries[0].ID != "decision-12" || len(state.Feedback) != 128 || len(state.Memories) != 32 || len(state.SelectedSkills) != 16 || len(state.SelectedTools) != 32 {
		t.Fatalf("collection bounds failed: entries=%d feedback=%d memories=%d skills=%d tools=%d", len(state.Entries), len(state.Feedback), len(state.Memories), len(state.SelectedSkills), len(state.SelectedTools))
	}
	if state.Memories[0].Key != "memory-0" || state.Memories[1].Key != "memory-1" || state.Memories[0].Text == "" || state.Memories[1].Text == "" {
		t.Fatal("protected memory was pruned or deprived of its text budget")
	}
	total := 0
	for _, memory := range state.Memories {
		total += len(memory.Text)
		if len(memory.Text) > 8192 || len(memory.Label) > 160 || !utf8.ValidString(memory.Text) {
			t.Fatalf("memory string bounds failed: %+v", memory)
		}
	}
	if total > 32768 || len(state.Entries[0].Error) > 200 || len(state.Entries[0].Items[0].Label) > 160 {
		t.Fatalf("aggregate/string bounds failed: %d", total)
	}
}

func TestSessionDecisionsCorruptOrMismatchedSidecarIsNotOverwritten(t *testing.T) {
	d, id := decisionStore(t)
	path := filepath.Join(d.Root(), dirSessions, id, sessionDecisionsFile)
	for _, content := range []string{"{broken", `{"sessionId":"different"}`} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.AppendSessionDecision(context.Background(), id, SessionDecision{Status: "fallback"}); err == nil {
			t.Fatal("corrupt or mismatched sidecar was accepted")
		}
		got, _ := os.ReadFile(path)
		if string(got) != content {
			t.Fatal("corrupt sidecar was overwritten")
		}
	}
}
