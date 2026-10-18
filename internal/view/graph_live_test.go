package view

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestGraphLiveLayer: running sessions are live, carry their agent's avatar
// data, and vanish from the layer when the running set no longer names them.
// Session meta is always present.
func TestGraphLiveLayer(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Unix()
	store := &fakeStore{
		agents: []db.Agent{{ID: "AG1", Name: "builder", Avatar: "🔧", Color: "#123456"}},
		sessions: []db.Session{
			{ID: "CHAT", AgentID: "AG1", UpdatedAt: now, Kind: "chat", Tags: []string{"x"}},
			{ID: "W1", AgentID: "AG1", UpdatedAt: now, Kind: "task"},
			{ID: "IDLE", AgentID: "AG1", UpdatedAt: now, Kind: "task"},
			{ID: "GONE", AgentID: "NOPE", UpdatedAt: now},
			{ID: "ARCH", AgentID: "AG1", UpdatedAt: now, State: "archived"},
		},
	}
	p := NewProjector(store).WithSources(Sources{Live: RunningSet{"W1": true, "GONE": true, "ARCH": true}})

	g, err := p.Graph(ctx)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if len(g.Live) != 1 {
		t.Fatalf("live = %+v, want only W1 running", g.Live)
	}
	byID := map[string]GraphLive{}
	for _, l := range g.Live {
		byID[l.Session.ID] = l
	}
	if byID["W1"].State != LiveRunning {
		t.Errorf("states: %+v", byID)
	}
	if a := byID["W1"].Agent; a.ID != "AG1" || a.Emoji != "🔧" || a.Color != "#123456" || a.Name != "builder" {
		t.Errorf("agent data: %+v", a)
	}
	// Orphan (deleted agent) and archived sessions are never live.
	if _, ok := byID["GONE"]; ok {
		t.Error("orphan session must not be live")
	}
	if _, ok := byID["ARCH"]; ok {
		t.Error("archived session must not be live")
	}
	m := g.Meta["session:CHAT"]
	if m.Kind != "chat" || m.AgentID != "AG1" || len(m.Tags) != 1 {
		t.Errorf("meta: %+v", m)
	}
	if !g.Meta["session:ARCH"].Archived {
		t.Error("archived meta flag missing")
	}

	// The layer follows the running set: nothing running, nothing live.
	quiet, err := NewProjector(store).Graph(ctx)
	if err != nil {
		t.Fatalf("graph without live source: %v", err)
	}
	if len(quiet.Live) != 0 || quiet.Live == nil {
		t.Errorf("live without source = %+v, want empty non-nil", quiet.Live)
	}
	if len(quiet.Meta) == 0 {
		t.Error("meta must not depend on the live source")
	}
}
