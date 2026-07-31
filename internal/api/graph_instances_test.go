package api

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
	nodes, inst := buildAgentInstances(agents, sessions, map[string]bool{})
	if len(nodes) != 0 || len(inst) != 0 {
		t.Fatalf("idle workspace must yield no agent nodes, got %d nodes / %d entries", len(nodes), len(inst))
	}

	// Only Bob running → only Bob appears.
	nodes, inst = buildAgentInstances(agents, sessions, map[string]bool{"SES2": true})
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
	running := map[string]bool{"SES1": true, "SES2": true, "SES3": true, "SES5": true}

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
