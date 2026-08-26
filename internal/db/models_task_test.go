package db

import "testing"

func TestDefaultBoardColumns(t *testing.T) {
	cols := DefaultBoardColumns()
	wantKeys := []string{
		BoardPBI,
		BoardTodo,
		BoardInProgress,
		BoardReview,
		BoardDone,
		BoardFailed,
		BoardCancelled,
	}

	if len(cols) != len(wantKeys) {
		t.Fatalf("len(DefaultBoardColumns()) = %d, want %d", len(cols), len(wantKeys))
	}

	seen := make(map[string]struct{}, len(cols))
	for i, col := range cols {
		if col.Key != wantKeys[i] {
			t.Errorf("column %d key = %q, want %q", i, col.Key, wantKeys[i])
		}
		if col.Key == "" {
			t.Errorf("column %d has empty key", i)
		}
		if _, exists := seen[col.Key]; exists {
			t.Errorf("column %d has duplicate key %q", i, col.Key)
		}
		seen[col.Key] = struct{}{}
		if col.Label == "" {
			t.Errorf("column %d with key %q has empty label", i, col.Key)
		}
	}
}
