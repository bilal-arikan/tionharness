package api

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestBuildAgentInstancesOnlyRunning verifies the network's core rule: an agent
// node exists per RUNNING session and nowhere else — an agent with nothing in
// flight contributes no node at all.
func TestBuildAgentInstancesOnlyRunning(t *testing.T) {
	agents := []db.Agent{{ID: "AGT1", Name: "Alice"}, {ID: "AGT2", Name: "Bob"}}
	sessions := []db.Session{
		{ID: "SES1", AgentID: "AGT1", Kind: "chat"},
		{ID: "SES2", AgentID: "AGT2", Kind: "task", SourceID: "TSK9"},
	}

	// Nothing running → no agent nodes.
	nodes, inst := buildAgentInstances(agents, sessions, map[string]string{})
	if len(nodes) != 0 || len(inst) != 0 {
		t.Fatalf("idle workspace must yield no agent nodes, got %d nodes / %d entries", len(nodes), len(inst))
	}

	// Only Bob running → only Bob appears.
	nodes, inst = buildAgentInstances(agents, sessions, map[string]string{"SES2": "running"})
	if len(nodes) != 1 || nodes[0].Label != "Bob" {
		t.Fatalf("expected only Bob's instance, got %+v", nodes)
	}
	if nodes[0].ID != "agent:AGT2#SES2" {
		t.Fatalf("instance id must carry the session: %q", nodes[0].ID)
	}
	if !nodes[0].Running || nodes[0].RunKind != "task" || nodes[0].RunTarget != "task:TSK9" {
		t.Fatalf("task instance must bond to its task, got %+v", nodes[0])
	}
	if got := inst["AGT2"]; len(got) != 1 || got[0] != "agent:AGT2#SES2" {
		t.Fatalf("instance index wrong: %+v", inst)
	}
}

// TestBuildAgentInstancesOneNodePerSession covers the copy rule: the same agent
// driving several sessions appears once per session, each with its own id.
func TestBuildAgentInstancesOneNodePerSession(t *testing.T) {
	agents := []db.Agent{{ID: "AGT1", Name: "Alice"}}
	sessions := []db.Session{
		{ID: "SES1", AgentID: "AGT1", Kind: "chat"},
		{ID: "SES2", AgentID: "AGT1", Kind: "flow", SourceID: "FLW1"},
		{ID: "SES3", AgentID: "AGT1", Kind: "spawned"},
		{ID: "SES4", AgentID: "AGT1", Kind: "chat"}, // not running
		{ID: "SES5", AgentID: "GONE", Kind: "chat"}, // deleted agent
	}
	running := map[string]string{"SES1": "running", "SES2": "running", "SES3": "running", "SES5": "running"}

	nodes, inst := buildAgentInstances(agents, sessions, running)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 live copies of Alice, got %d: %+v", len(nodes), nodes)
	}
	if len(inst["AGT1"]) != 3 {
		t.Fatalf("instance index must list all 3 copies, got %+v", inst["AGT1"])
	}
	if _, ok := inst["GONE"]; ok {
		t.Fatalf("session of a deleted agent must not produce a node")
	}
	seen := map[string]bool{}
	for _, n := range nodes {
		if seen[n.ID] {
			t.Fatalf("duplicate instance id %q", n.ID)
		}
		seen[n.ID] = true
	}
	if nodes[1].RunTarget != "flow:FLW1" {
		t.Fatalf("flow instance must bond to its flow, got %q", nodes[1].RunTarget)
	}
	// Chat/spawn instances have no task/flow to bond to.
	if nodes[0].RunTarget != "" || nodes[2].RunTarget != "" {
		t.Fatalf("chat/spawn instances must carry no run target: %+v", nodes)
	}
	if instanceCount(inst) != 3 {
		t.Fatalf("instanceCount mismatch: %d", instanceCount(inst))
	}
}

func TestBuildGraphLiveScope(t *testing.T) {
	sessions := []db.Session{
		{ID: "parent"},
		{ID: "child", CoordinatorSessionID: "parent"},
		{ID: "idle"},
		{ID: "running-parent"},
		{ID: "running-child", CoordinatorSessionID: "running-parent"},
	}
	scope := buildGraphLiveScope(sessions, map[string]bool{
		"child":          true,
		"running-parent": true,
		"running-child":  true,
	})
	if got := scope["parent"]; got != "awaiting-workers" {
		t.Fatalf("parent scope = %q, want awaiting-workers", got)
	}
	if got := scope["child"]; got != "running" {
		t.Fatalf("child scope = %q, want running", got)
	}
	if got := scope["running-parent"]; got != "running" {
		t.Fatalf("running parent scope = %q, want running", got)
	}
	if _, ok := scope["idle"]; ok {
		t.Fatal("idle session entered live scope")
	}
}

func TestBuildAgentInstancesIncludesAwaitingWorkers(t *testing.T) {
	agents := []db.Agent{{ID: "AGT1", Name: "Coordinator"}}
	sessions := []db.Session{{ID: "SES1", AgentID: "AGT1", Kind: "chat"}}
	nodes, inst := buildAgentInstances(agents, sessions, map[string]string{"SES1": "awaiting-workers"})
	if len(nodes) != 1 || len(inst["AGT1"]) != 1 {
		t.Fatalf("awaiting coordinator missing: nodes=%+v instances=%+v", nodes, inst)
	}
	if nodes[0].Running || nodes[0].LiveScope != "awaiting-workers" {
		t.Fatalf("awaiting coordinator state wrong: %+v", nodes[0])
	}
}

// TestInstanceSub checks the subtitle that tells two copies of one agent apart.
func TestInstanceSub(t *testing.T) {
	if got := instanceSub("task", "Refactor parser"); got != "Görev · Refactor parser" {
		t.Fatalf("got %q", got)
	}
	if got := instanceSub("chat", ""); got != "Sohbet" {
		t.Fatalf("got %q", got)
	}
	// Unknown kinds fall through to the raw kind rather than going blank.
	if got := instanceSub("mystery", ""); got != "mystery" {
		t.Fatalf("got %q", got)
	}
	if got := instanceSub("", ""); got != "Çalışıyor" {
		t.Fatalf("got %q", got)
	}
}
