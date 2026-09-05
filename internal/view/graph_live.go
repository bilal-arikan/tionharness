package view

import (
	"context"
	"sort"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// LiveSource reports which sessions are executing right now. The set comes from
// the runtime (process-wide liveness), which the view package cannot import; the
// api layer hands it in through Sources.Live. Nil means "nothing is live".
type LiveSource interface {
	RunningSessions() map[string]bool
}

// RunningSet is the plain-map LiveSource: a snapshot of running session ids.
type RunningSet map[string]bool

func (r RunningSet) RunningSessions() map[string]bool { return r }

// GraphLiveAgent is the agent driving a live session — enough for the map to
// draw its avatar next to the session node.
type GraphLiveAgent struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji,omitempty"`
	Color string `json:"color,omitempty"`
}

// GraphLive is one live session on the map: running, or a coordinator that is
// idle between turns while one of its direct workers runs (awaiting-workers).
// The Explorer draws these glowing and attaches the agent's avatar to them; the
// entry disappears — and with it the glow and the avatar — once the session
// stops.
type GraphLive struct {
	Session Ref            `json:"session"`
	State   string         `json:"state"` // running | awaiting-workers
	Agent   GraphLiveAgent `json:"agent"`
}

// GraphMeta is the per-session facet data the map's filters need (kind, owning
// agent, tags, archived) — the same facets the Network screen filtered on.
// Keyed by Ref.String in Graph.Meta; only session nodes carry an entry.
type GraphMeta struct {
	Kind     string   `json:"kind,omitempty"`
	AgentID  string   `json:"agentId,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Archived bool     `json:"archived,omitempty"`
}

const (
	LiveRunning         = "running"
	LiveAwaitingWorkers = "awaiting-workers"
)

// graphLive computes the live layer against the walk's session snapshot. The
// awaiting-workers rule mirrors the Network screen's: a coordinator counts as
// live while any of its direct workers is running.
func (p *Projector) graphLive(ctx context.Context, cache *structuralCache) ([]GraphLive, map[string]GraphMeta, error) {
	sessions, err := cache.allSessions(ctx)
	if err != nil {
		return nil, nil, err
	}
	agents, err := cache.allAgents(ctx)
	if err != nil {
		return nil, nil, err
	}
	agentByID := make(map[string]db.Agent, len(agents))
	for _, a := range agents {
		agentByID[a.ID] = a
	}

	meta := make(map[string]GraphMeta, len(sessions))
	for _, s := range sessions {
		meta[Ref{Kind: KindSession, ID: s.ID}.String()] = GraphMeta{
			Kind:     s.Kind,
			AgentID:  s.AgentID,
			Tags:     s.Tags,
			Archived: s.State == "archived",
		}
	}

	var running map[string]bool
	if p.sources.Live != nil {
		running = p.sources.Live.RunningSessions()
	}
	if len(running) == 0 {
		return []GraphLive{}, meta, nil
	}
	scope := make(map[string]string, len(running))
	for _, s := range sessions {
		if running[s.ID] && s.State != "archived" {
			scope[s.ID] = LiveRunning
		}
	}
	for _, s := range sessions {
		if scope[s.ID] == LiveRunning && s.CoordinatorSessionID != "" {
			if _, already := scope[s.CoordinatorSessionID]; !already {
				scope[s.CoordinatorSessionID] = LiveAwaitingWorkers
			}
		}
	}

	live := make([]GraphLive, 0, len(scope))
	for _, s := range sessions {
		state, ok := scope[s.ID]
		if !ok {
			continue
		}
		a, known := agentByID[s.AgentID]
		if !known {
			continue // orphan session pointing at a deleted agent
		}
		live = append(live, GraphLive{
			Session: Ref{Kind: KindSession, ID: s.ID},
			State:   state,
			Agent:   GraphLiveAgent{ID: a.ID, Name: a.Name, Emoji: a.Avatar, Color: a.Color},
		})
	}
	sort.Slice(live, func(i, j int) bool { return live[i].Session.ID < live[j].Session.ID })
	return live, meta, nil
}
