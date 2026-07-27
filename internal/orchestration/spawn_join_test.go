package orchestration

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fakeAsync is an AgentRunner (via okRunner) that also implements AsyncFlowRunner
// with scripted spawn/join results, capturing what the engine passed it.
type fakeAsync struct {
	okRunner
	spawnIDs      []string
	joinOut       []string
	joinErr       error
	gotFlowIDs    []string
	gotRunIDs     []string
	gotTimeoutSec int
	gotPartial    bool
}

func (f *fakeAsync) SpawnChildFlows(_ context.Context, flowIDs []string, _ string) ([]string, error) {
	f.gotFlowIDs = flowIDs
	return f.spawnIDs, nil
}

func (f *fakeAsync) JoinChildFlows(_ context.Context, runIDs []string, timeoutSec int, partial bool, onProgress func(done, total int)) ([]string, error) {
	f.gotRunIDs = runIDs
	f.gotTimeoutSec = timeoutSec
	f.gotPartial = partial
	if onProgress != nil {
		onProgress(len(runIDs), len(runIDs))
	}
	if f.joinErr != nil {
		return nil, f.joinErr
	}
	return f.joinOut, nil
}

func spawnJoinGraph() Graph {
	return Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "sp"},
			{ID: "sp", Type: NodeSpawn, SpawnFlows: []string{"FLWa", "FLWb"}, Next: "jn"},
			{ID: "jn", Type: NodeJoin, SpawnRef: "sp", Next: "end"},
			{ID: "end", Type: NodeEnd},
		},
	}
}

// TestSpawnJoin_HappyPath drives spawn → join: the spawn node records the launched
// run ids in State.Spawned, and the join collects + joins their outputs, clearing
// the entry afterwards.
func TestSpawnJoin_HappyPath(t *testing.T) {
	r := &fakeAsync{spawnIDs: []string{"r1", "r2"}, joinOut: []string{"A", "B"}}
	final, err := NewEngine(r).Run(context.Background(), spawnJoinGraph(), "X", NewState(spawnJoinGraph()), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.gotFlowIDs[0] != "FLWa" || r.gotFlowIDs[1] != "FLWb" {
		t.Errorf("spawn got wrong flow ids: %v", r.gotFlowIDs)
	}
	if len(r.gotRunIDs) != 2 || r.gotRunIDs[0] != "r1" {
		t.Errorf("join got wrong run ids: %v", r.gotRunIDs)
	}
	if final.Last != "A\n\nB" {
		t.Errorf("join should combine child outputs, got %q", final.Last)
	}
	if len(final.Spawned) != 0 {
		t.Errorf("join should clear its spawn entry, got %v", final.Spawned)
	}
	if final.Current != "" {
		t.Errorf("expected a finished run, got Current=%q", final.Current)
	}
}

// TestJoin_FailsWhenChildFails verifies a join surfaces a spawned child's failure
// as a run error (the barrier never silently drops a failed branch).
func TestJoin_FailsWhenChildFails(t *testing.T) {
	r := &fakeAsync{spawnIDs: []string{"r1"}, joinErr: fmt.Errorf("spawned child r1 failed: boom")}
	_, err := NewEngine(r).Run(context.Background(), spawnJoinGraph(), "X", NewState(spawnJoinGraph()), nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected the child failure to fail the join, got %v", err)
	}
}

// TestJoin_PassesTimeoutAndPartial verifies the join node forwards its
// JoinTimeoutSec + JoinPartial settings to the runner.
func TestJoin_PassesTimeoutAndPartial(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "sp"},
			{ID: "sp", Type: NodeSpawn, SpawnFlows: []string{"FLWa"}, Next: "jn"},
			{ID: "jn", Type: NodeJoin, SpawnRef: "sp", JoinTimeoutSec: 7, JoinPartial: true, Next: "end"},
			{ID: "end", Type: NodeEnd},
		},
	}
	r := &fakeAsync{spawnIDs: []string{"r1"}, joinOut: []string{"A"}}
	if _, err := NewEngine(r).Run(context.Background(), g, "X", NewState(g), nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.gotTimeoutSec != 7 || !r.gotPartial {
		t.Errorf("join should forward timeout/partial, got timeout=%d partial=%v", r.gotTimeoutSec, r.gotPartial)
	}
}

// TestSpawnJoin_ValidateContract checks the structural rules for spawn/join.
func TestSpawnJoin_ValidateContract(t *testing.T) {
	// spawn with no flows.
	g := Graph{Start: "start", Nodes: []Node{
		{ID: "start", Type: NodeStart, Next: "sp"},
		{ID: "sp", Type: NodeSpawn, Next: "end"},
		{ID: "end", Type: NodeEnd},
	}}
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "spawnFlows") {
		t.Errorf("spawn with no flows should be rejected, got %v", err)
	}
	// join referencing a non-spawn node.
	g2 := Graph{Start: "start", Nodes: []Node{
		{ID: "start", Type: NodeStart, Next: "jn"},
		{ID: "jn", Type: NodeJoin, SpawnRef: "start", Next: "end"},
		{ID: "end", Type: NodeEnd},
	}}
	if err := g2.Validate(); err == nil || !strings.Contains(err.Error(), "not a spawn node") {
		t.Errorf("join referencing a non-spawn node should be rejected, got %v", err)
	}
}
