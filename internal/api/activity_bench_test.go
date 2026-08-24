package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// BenchmarkWorkspaceRunning measures the exact call the cross-workspace switcher
// poll makes once per workspace per tick. It is the number that justifies this
// whole change: it used to scan the runs and flowRuns maps in full (under d.mu)
// and allocate on every call, so it grew with store size. It must now be flat in
// the store size and report 0 allocs/op.
//
// Run it as:
//
//	go test ./internal/api -run '^$' -bench WorkspaceRunning -benchmem
func BenchmarkWorkspaceRunning(b *testing.B) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(b)

	// A store big enough that a full scan would be obvious in the numbers.
	for i := 0; i < 2000; i++ {
		run, err := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1"})
		if err != nil {
			b.Fatalf("CreateFlowRun: %v", err)
		}
		if err := wsp.DB.FinishFlowRun(ctx, run.ID, db.FlowSuccess, "", ""); err != nil {
			b.Fatalf("FinishFlowRun: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.workspaceRunning(wsp) {
			b.Fatal("idle workspace reported running")
		}
	}
}
