package conversation

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func TestPendingAfterSummary(t *testing.T) {
	history := []db.Message{{ID: "m1"}, {ID: "m2"}, {ID: "m3"}}

	if got := PendingAfterSummary(history, 0); len(got) != 3 {
		t.Fatalf("no summary yet: want full history, got %d", len(got))
	}
	got := PendingAfterSummary(history, 2)
	if len(got) != 1 || got[0].ID != "m3" {
		t.Fatalf("want only the turns after the compaction boundary, got %+v", got)
	}
	if got := PendingAfterSummary(history, 9); len(got) != 0 {
		t.Fatalf("boundary past the history end must clamp to empty, got %d", len(got))
	}
}
