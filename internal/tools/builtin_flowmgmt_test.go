package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestListFlowRuns_FiltersWaiting verifies list_flow_runs surfaces waiting runs
// (with their await node id) so a peer agent can find one to feed.
func TestListFlowRuns_FiltersWaiting(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	// One finished run and one waiting run for the same flow.
	if _, err := d.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1", Input: "a"}); err != nil {
		t.Fatal(err)
	}
	w, err := d.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1", Input: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.MarkFlowRunWaiting(ctx, w.ID, `{"current":"ask","waitingAt":"ask"}`); err != nil {
		t.Fatal(err)
	}

	tool := NewListFlowRunsTool(d, "agent")
	out, err := tool.Call(ctx, json.RawMessage(`{"status":"waiting"}`))
	if err != nil {
		t.Fatalf("list_flow_runs: %v", err)
	}
	var rows []struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		WaitingAt string `json:"waitingAt"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("bad json: %v (%s)", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly the 1 waiting run, got %d: %s", len(rows), out)
	}
	if rows[0].ID != w.ID || rows[0].Status != db.FlowWaiting || rows[0].WaitingAt != "ask" {
		t.Errorf("waiting row wrong: %+v", rows[0])
	}
}

// TestDeliverFlowInput_UsesResumeBridge verifies deliver_flow_input calls the
// resume bridge with the run id + input and reports the resulting status, and
// errors clearly when unwired.
func TestDeliverFlowInput_UsesResumeBridge(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	var gotRun, gotInput string
	tool := NewDeliverFlowInputTool(d, "agent", func(_ context.Context, runID, input string) (db.FlowRun, error) {
		gotRun, gotInput = runID, input
		return db.FlowRun{ID: runID, Status: db.FlowRunning}, nil
	})
	out, err := tool.Call(ctx, json.RawMessage(`{"runId":"RUN9","input":"sample"}`))
	if err != nil {
		t.Fatalf("deliver_flow_input: %v", err)
	}
	if gotRun != "RUN9" || gotInput != "sample" {
		t.Errorf("bridge got wrong args: run=%q input=%q", gotRun, gotInput)
	}
	if !strings.Contains(out, `"status":"running"`) || !strings.Contains(out, `"action":"resumed"`) {
		t.Errorf("unexpected result: %s", out)
	}

	// Unwired bridge → clear error.
	if _, err := NewDeliverFlowInputTool(d, "agent", nil).Call(ctx, json.RawMessage(`{"runId":"x"}`)); err == nil {
		t.Error("expected an error when the resume bridge is not wired")
	}
}

// TestCreateFlow_RejectsMissingAgent is the save-time regression guard for the
// TSK254 bug: create_flow used to accept a graph referencing an agentId that
// does not exist in the workspace, and the failure only surfaced once the flow
// was run. It must now be rejected up front, naming the offending node.
func TestCreateFlow_RejectsMissingAgent(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	graph := `{"start":"start","nodes":[{"id":"start","type":"start","next":"n1"},{"id":"n1","type":"agent","agentId":"AGT-gone","prompt":"hi"}]}`
	tool := NewCreateFlowTool(d, "agent")
	_, err = tool.Call(ctx, json.RawMessage(`{"name":"broken","graph":`+strconv.Quote(graph)+`}`))
	if err == nil {
		t.Fatal("expected create_flow to reject an agentId that does not exist")
	}
	if !strings.Contains(err.Error(), "AGT-gone") || !strings.Contains(err.Error(), "n1") {
		t.Fatalf("error should name the node and the missing agent, got: %v", err)
	}
}

// TestUpdateFlow_RejectsMissingAgent mirrors TestCreateFlow_RejectsMissingAgent
// for the update path — editing a flow's graph to point at a non-existent agent
// must also be rejected at save time.
func TestUpdateFlow_RejectsMissingAgent(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	f, err := d.CreateFlow(ctx, db.Flow{Name: "draft"})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}

	graph := `{"start":"start","nodes":[{"id":"start","type":"start","next":"n1"},{"id":"n1","type":"agent","agentId":"AGT-gone","prompt":"hi"}]}`
	tool := NewUpdateFlowTool(d, "agent")
	body := `{"id":"` + f.ID + `","graph":` + strconv.Quote(graph) + `}`
	if _, err := tool.Call(ctx, json.RawMessage(body)); err == nil {
		t.Fatal("expected update_flow to reject an agentId that does not exist")
	}
}

// TestCreateFlow_AllowsExistingAgent is the regression guard: a graph whose
// agentId resolves to a real agent must still save cleanly.
func TestCreateFlow_AllowsExistingAgent(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	a, err := d.CreateAgent(ctx, db.Agent{Name: "worker"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	graph := `{"start":"start","nodes":[{"id":"start","type":"start","next":"n1"},{"id":"n1","type":"agent","agentId":"` + a.ID + `","prompt":"hi"}]}`
	tool := NewCreateFlowTool(d, "agent")
	out, err := tool.Call(ctx, json.RawMessage(`{"name":"ok","graph":`+strconv.Quote(graph)+`}`))
	if err != nil {
		t.Fatalf("expected create_flow to accept an existing agentId, got: %v", err)
	}
	if !strings.Contains(out, `"action":"created"`) {
		t.Errorf("unexpected result: %s", out)
	}
}
