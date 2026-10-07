package view

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// GraphTimes is one map node's recency facets (unix seconds, 0 = unknown): when
// the entity was created, last edited, and last read by an agent (with the
// reader). The Explorer's time window keeps a node whose NEWEST of the three
// falls inside the window, plus the nodes on its path to the root.
type GraphTimes struct {
	Created int64  `json:"created,omitempty"`
	Updated int64  `json:"updated,omitempty"`
	Read    int64  `json:"read,omitempty"`
	ReadBy  string `json:"readBy,omitempty"` // agent id of the last reader
}

// trajectoryLister is the optional store capability that lists Rota index rows.
// Optional so projection-only fakes need not grow it: without it Rota nodes
// simply carry no times.
type trajectoryLister interface {
	ListTrajectories(ctx context.Context, f db.TrajectoryFilter) []db.TrajectoryIndexEntry
}

// GraphTimes computes created/updated stamps for every timed entity the map
// draws, keyed by Ref.String, and lays the agent read ledger over them. Like
// GraphLive it is per-request (stamps move on every edit), never cached with
// the structure. Optional sources that are unavailable contribute nothing.
func (p *Projector) GraphTimes(ctx context.Context, reads map[string]db.ViewRead) (map[string]GraphTimes, error) {
	if p == nil || p.store == nil {
		return nil, nil
	}
	cache := p.newStructuralCache()
	out := map[string]GraphTimes{}
	put := func(ref Ref, created, updated int64) {
		out[ref.String()] = GraphTimes{Created: created, Updated: updated}
	}

	sessions, err := cache.allSessions(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		put(Ref{Kind: KindSession, ID: s.ID}, s.CreatedAt, s.UpdatedAt)
	}
	agents, err := cache.allAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		put(Ref{Kind: KindAgent, ID: a.ID}, a.CreatedAt, a.UpdatedAt)
	}
	tasks, err := cache.activeTasks(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		put(Ref{Kind: KindBoard, ID: BoardRefID, Sub: t.ID}, t.CreatedAt, t.UpdatedAt)
	}
	if list, err := cache.allArtifacts(ctx); err == nil {
		for _, a := range list {
			put(Ref{Kind: KindArtifact, ID: a.ID}, a.CreatedAt, a.UpdatedAt)
		}
	}
	if list, err := cache.allAutomations(ctx); err == nil {
		for _, a := range list {
			put(Ref{Kind: KindAutomation, ID: a.ID}, a.CreatedAt, a.UpdatedAt)
		}
	}
	if list, err := p.store.ListSchedules(ctx); err == nil {
		for _, s := range list {
			put(Ref{Kind: KindSchedule, ID: s.ID}, s.CreatedAt, s.UpdatedAt)
		}
	}
	if list, err := cache.allSkills(); err == nil {
		for _, s := range list {
			put(Ref{Kind: KindSkill, ID: s.Slug}, 0, s.ModifiedAt)
		}
	}
	if list, err := cache.allFindings(); err == nil {
		for _, f := range list {
			put(Ref{Kind: KindInsight, ID: f.ID}, 0, f.LastSeen)
		}
	}
	if list, err := cache.allNotes(); err == nil {
		for _, n := range list {
			put(Ref{Kind: KindNote, ID: n.ID}, n.Created, n.Updated)
		}
	}
	if lister, ok := p.store.(trajectoryLister); ok {
		for _, t := range lister.ListTrajectories(ctx, db.TrajectoryFilter{}) {
			put(Ref{Kind: KindTrajectory, ID: t.ID}, t.CreatedAt, t.UpdatedAt)
		}
	}

	// Reads may name nodes that carry no stamps of their own (a category, the
	// budget): they still count, a read is activity.
	for key, r := range reads {
		t := out[key]
		t.Read, t.ReadBy = r.At, r.AgentID
		out[key] = t
	}
	return out, nil
}
