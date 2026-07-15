package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// TestRunInsightScanSingleFlight verifies the busy guard: while a scan is in
// flight, a second trigger is rejected with ErrInsightScanBusy rather than
// running a second overlapping pass.
func TestRunInsightScanSingleFlight(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Simulate an in-flight scan by holding the flag, then trigger another.
	if !rt.insightScanActive.CompareAndSwap(false, true) {
		t.Fatal("flag should start clear")
	}
	if !rt.InsightScanActive() {
		t.Fatal("InsightScanActive should report true while held")
	}
	if _, err := rt.RunInsightScan(ctx, insight.ScanScope{}, ""); err != ErrInsightScanBusy {
		t.Fatalf("expected ErrInsightScanBusy, got %v", err)
	}
	rt.insightScanActive.Store(false)
	// Once cleared, a scan runs normally again (empty store → no analysis needed).
	if _, err := rt.RunInsightScan(ctx, insight.ScanScope{LensIDs: []string{"tool-errors"}}, ""); err != nil {
		t.Fatalf("scan after release should succeed: %v", err)
	}
}
