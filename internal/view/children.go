package view

import (
	"context"
	"fmt"
	"sort"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// Children returns the STRUCTURAL child handles of ref: the edges the Explorer
// map follows when a node is expanded. It is deliberately separate from Project —
// Project summarises ONE node (header + signals + the elision count), Children
// lists what that node drills into, without paying for a full card render of each
// child. Both surfaces (the map UI and the agent-side expand wrapper) call this,
// so they walk the exact same graph.
//
// The list is capped at categoryTopN (the per-node child cap, _Docs/68 §8.1) so a
// 300-session workspace cannot explode the map. The honest total lives on the
// node's own Project (e.g. ProjectCategory reports Elided) — Children is the raw,
// bounded edge list.
//
// An unknown kind is an error, not an empty slice: a caller asking to expand a
// kind that has no children defined has a bug, and a silent empty list would hide
// it behind a plausible "leaf node".
func (p *Projector) Children(ctx context.Context, ref Ref) ([]Handle, error) {
	if p == nil || p.store == nil {
		return nil, fmt.Errorf("view: projector has no store")
	}
	switch ref.Kind {
	case KindSpace:
		return workspaceChildren(), nil
	case KindCategory:
		members, err := p.categoryMembers(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		return capHandles(members, categoryTopN), nil
	case KindBoard:
		if ref.Sub != "" {
			return nil, nil // a single card is a leaf
		}
		cols, err := p.boardColumnChildren(ctx)
		if err != nil {
			return nil, err
		}
		return capHandles(cols, categoryTopN), nil
	case KindAgent:
		sessions, err := p.agentSessionChildren(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		return capHandles(sessions, categoryTopN), nil
	case KindSession:
		workers, err := p.sessionWorkerChildren(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		// A root session's trajectory is its first structural child: the plan
		// hangs off the session the way its workers do (_Docs/77 R9).
		if h := p.trajectoryHandleFor(ctx, ref.ID); h != nil {
			workers = append([]Handle{*h}, workers...)
		}
		return capHandles(workers, categoryTopN), nil
	case KindTrajectory:
		nodes, err := p.trajectoryChildren(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		return capHandles(nodes, categoryTopN), nil
	case KindBudget, KindTools, KindFlowRun, KindSchedule,
		KindArtifact, KindAutomation, KindSkill, KindInsight, KindLogs:
		// Leaves in the map: their breakdown is rendered inline by Project, so
		// there is nothing structural to expand into.
		return nil, nil
	default:
		return nil, fmt.Errorf("view: children unsupported for kind %q", ref.Kind)
	}
}

// Neighborhood is the complete, one-hop structural neighborhood used by the
// Explorer focus graph. Unlike Children it does not apply categoryTopN: visual
// overflow belongs to the client, and every direct relationship must remain
// available through this contract. No recursive walk is performed, so cycles
// and self-loops cannot cause unbounded traversal.
type Neighborhood struct {
	Focus             Handle   `json:"focus"`
	Parents           []Handle `json:"parents"`
	Children          []Handle `json:"children"`
	HiddenParentCount int      `json:"hiddenParentCount"`
	HiddenChildCount  int      `json:"hiddenChildCount"`
}

// Neighborhood resolves focus plus every direct incoming and outgoing
// structural edge in the current projector/store. Handles are de-duplicated by
// Ref and sorted by Ref.String, making multi-parent, cycle and self-loop output
// stable without dropping a relationship silently.
func (p *Projector) Neighborhood(ctx context.Context, focus Ref) (Neighborhood, error) {
	if err := validateStructuralRef(focus); err != nil {
		return Neighborhood{}, err
	}
	projected, err := p.Project(ctx, focus, LevelCard)
	if err != nil {
		return Neighborhood{}, err
	}

	// One snapshot for the whole walk: the parent scan below asks every node in
	// the workspace for its children, and without the cache each session-shaped
	// answer re-read (and re-copied) the full session list from the store.
	cache := p.newStructuralCache()

	children, err := cache.children(ctx, focus)
	if err != nil {
		return Neighborhood{}, err
	}
	candidates, err := cache.nodes(ctx)
	if err != nil {
		return Neighborhood{}, err
	}

	parents := make([]Handle, 0)
	for _, candidate := range candidates {
		candidateChildren, childErr := cache.children(ctx, candidate.Ref)
		if childErr != nil {
			if candidate.Ref.Kind == KindCategory &&
				(candidate.Ref.ID == CategorySkills || candidate.Ref.ID == CategoryInsights) {
				continue
			}
			return Neighborhood{}, childErr
		}
		for _, child := range candidateChildren {
			if child.Ref == focus {
				parents = append(parents, candidate)
				break
			}
		}
	}

	return Neighborhood{
		Focus:    Handle{Label: projected.Header, Ref: focus, Level: LevelCard},
		Parents:  uniqueSortedHandles(parents),
		Children: uniqueSortedHandles(children),
	}, nil
}

func validateStructuralRef(ref Ref) error {
	if ref.ID == "" {
		return fmt.Errorf("view: ref has no id")
	}
	want := ""
	switch ref.Kind {
	case KindSpace:
		want = WorkspaceRefID
	case KindBoard:
		want = BoardRefID
	case KindBudget:
		want = BudgetRefID
	case KindTools:
		want = ToolsRefID
	case KindLogs:
		want = LogsRefID
	}
	if want != "" && ref.ID != want {
		return fmt.Errorf("view: unknown %s ref %q", ref.Kind, ref.ID)
	}
	return nil
}

func uniqueSortedHandles(handles []Handle) []Handle {
	byRef := make(map[Ref]Handle, len(handles))
	for _, handle := range handles {
		if _, exists := byRef[handle.Ref]; !exists {
			byRef[handle.Ref] = handle
		}
	}
	out := make([]Handle, 0, len(byRef))
	for _, handle := range byRef {
		out = append(out, handle)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.String() < out[j].Ref.String() })
	return out
}

// IsExpandable reports whether a ref has structural children worth expanding —
// the map/agent affordance for "can I drill into this?". It mirrors the kinds
// Children resolves to a non-nil edge set: the workspace root, a category, a
// whole board (a single card is a leaf), an agent, and a session. budget / tools
// / flowrun / schedule render their breakdown inline and have no children.
func IsExpandable(ref Ref) bool {
	switch ref.Kind {
	case KindSpace, KindCategory, KindAgent, KindSession, KindTrajectory:
		return true
	case KindBoard:
		return ref.Sub == ""
	default:
		return false
	}
}

// workspaceChildren is the root's fixed set of eleven category/entity nodes.
// Static and always eleven — a category that happens to be empty still appears,
// so the map's shape does not change with the workspace's contents. The first
// four are the original buckets; artifacts/automations/skills/insights + logs
// are the TSK66 extension; budget/tools round the existing set out.
func workspaceChildren() []Handle {
	return []Handle{
		{Label: "Oturumlar", Ref: Ref{Kind: KindCategory, ID: CategorySessions}, Level: LevelCard},
		{Label: "Akışlar", Ref: Ref{Kind: KindCategory, ID: CategoryFlows}, Level: LevelCard},
		{Label: "Pano", Ref: Ref{Kind: KindBoard, ID: BoardRefID}, Level: LevelCard},
		{Label: "Ajanlar", Ref: Ref{Kind: KindCategory, ID: CategoryAgents}, Level: LevelCard},
		{Label: "Artifacts", Ref: Ref{Kind: KindCategory, ID: CategoryArtifacts}, Level: LevelCard},
		{Label: "Otomasyonlar", Ref: Ref{Kind: KindCategory, ID: CategoryAutomations}, Level: LevelCard},
		{Label: "Skill'ler", Ref: Ref{Kind: KindCategory, ID: CategorySkills}, Level: LevelCard},
		{Label: "İçgörüler", Ref: Ref{Kind: KindCategory, ID: CategoryInsights}, Level: LevelCard},
		{Label: "Günlükler", Ref: Ref{Kind: KindLogs, ID: LogsRefID}, Level: LevelCard},
		{Label: "Bütçe", Ref: Ref{Kind: KindBudget, ID: BudgetRefID}, Level: LevelCard},
		{Label: "Araçlar", Ref: Ref{Kind: KindTools, ID: ToolsRefID}, Level: LevelCard},
	}
}

// categoryMembers computes the FULL (uncapped) member handle list for a category.
// ProjectCategory reads this to count members honestly; Children caps the same
// list. An unknown category id is an error — see ProjectCategory's rationale.
func (p *Projector) categoryMembers(ctx context.Context, id string) ([]Handle, error) {
	return p.newStructuralCache().categoryMembers(ctx, id)
}

// boardColumnChildren is the board's structural children: one category node per
// column that exists on the board. A column drills into its cards.
func (p *Projector) boardColumnChildren(ctx context.Context) ([]Handle, error) {
	return p.newStructuralCache().boardColumnChildren(ctx)
}

// agentSessionChildren is an agent's structural children: the sessions bound to
// it.
func (p *Projector) agentSessionChildren(ctx context.Context, agentID string) ([]Handle, error) {
	return p.newStructuralCache().agentSessionChildren(ctx, agentID)
}

// sessionWorkerChildren is a session's structural children: the worker sessions
// that report up to it (a coordinator drills into its fleet). A plain session
// simply has none.
func (p *Projector) sessionWorkerChildren(ctx context.Context, sessionID string) ([]Handle, error) {
	return p.newStructuralCache().sessionWorkerChildren(ctx, sessionID)
}

// sessionHandleList orders sessions most-recently-active first and renders each
// as a session handle. The input is copied before sorting: the caller's slice may
// be the structural cache's memoised session snapshot, which every other node in
// the same walk still reads.
func sessionHandleList(sessions []db.Session) []Handle {
	sorted := append([]db.Session(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].UpdatedAt > sorted[j].UpdatedAt })
	hs := make([]Handle, 0, len(sorted))
	for _, s := range sorted {
		hs = append(hs, Handle{
			Label: "session:" + s.ID + " " + clip(orDash(s.Title), 40),
			Ref:   Ref{Kind: KindSession, ID: s.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// flowRunHandleList orders runs most-recent first and renders each as a
// flow-run handle.
func flowRunHandleList(runs []db.FlowRun) []Handle {
	sorted := append([]db.FlowRun(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].UpdatedAt > sorted[j].UpdatedAt })
	hs := make([]Handle, 0, len(sorted))
	for _, r := range sorted {
		hs = append(hs, Handle{
			Label: "run:" + r.ID + " " + string(r.Status),
			Ref:   Ref{Kind: KindFlowRun, ID: r.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// agentHandleList renders every agent as an agent handle.
func agentHandleList(agents []db.Agent) []Handle {
	hs := make([]Handle, 0, len(agents))
	for _, a := range agents {
		hs = append(hs, Handle{
			Label: "agent:" + a.ID + " " + clip(agentName(a), 40),
			Ref:   Ref{Kind: KindAgent, ID: a.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// artifactHandleList renders every artifact (newest updated first) as an
// artifact handle.
func artifactHandleList(artifacts []db.Artifact) []Handle {
	sorted := append([]db.Artifact(nil), artifacts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].UpdatedAt > sorted[j].UpdatedAt })
	hs := make([]Handle, 0, len(sorted))
	for _, a := range sorted {
		hs = append(hs, Handle{
			Label: "artifact:" + a.ID + " " + clip(orDash(a.Title), 40),
			Ref:   Ref{Kind: KindArtifact, ID: a.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// automationHandleList renders automations (newest first) as automation handles.
func automationHandleList(automations []db.Automation) []Handle {
	sorted := append([]db.Automation(nil), automations...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt > sorted[j].CreatedAt })
	hs := make([]Handle, 0, len(sorted))
	for _, a := range sorted {
		hs = append(hs, Handle{
			Label: "automation:" + a.ID + " " + clip(orDash(a.Name), 40),
			Ref:   Ref{Kind: KindAutomation, ID: a.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// skillHandleList renders the catalog (display order) as skill handles.
func skillHandleList(skills []skills.Skill) []Handle {
	hs := make([]Handle, 0, len(skills))
	for _, sk := range skills {
		hs = append(hs, Handle{
			Label: "skill:" + sk.Slug + " " + clip(orDash(sk.Name), 40),
			Ref:   Ref{Kind: KindSkill, ID: sk.Slug},
			Level: LevelCard,
		})
	}
	return hs
}

// insightHandleList renders findings (store order: priority-ranked, newest
// evidence first) as insight handles.
func insightHandleList(findings []InsightFinding) []Handle {
	hs := make([]Handle, 0, len(findings))
	for _, f := range findings {
		hs = append(hs, Handle{
			Label: "insight:" + f.ID + " " + clip(f.Title, 40),
			Ref:   Ref{Kind: KindInsight, ID: f.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// columnCardHandles renders the cards of one board column as card handles
// (KindBoard drill-down via Sub).
func columnCardHandles(tasks []db.Task, columnKey string) []Handle {
	var cards []db.Task
	for _, t := range tasks {
		key := t.BoardState
		if key == "" {
			key = "(boş)"
		}
		if key == columnKey {
			cards = append(cards, t)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].UpdatedAt > cards[j].UpdatedAt })
	hs := make([]Handle, 0, len(cards))
	for _, t := range cards {
		hs = append(hs, Handle{
			Label: t.ID + " " + clip(orDash(t.Title), 40),
			Ref:   Ref{Kind: KindBoard, ID: BoardRefID, Sub: t.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// liveSessions drops archived sessions — they are not part of the live surface
// the map walks.
func liveSessions(sessions []db.Session) []db.Session {
	out := make([]db.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.State != "archived" {
			out = append(out, s)
		}
	}
	return out
}

// capHandles bounds a handle list to at most n entries. The dropped remainder is
// reported by the node's own Project (never silently), so this cap is a safety
// bound on the edge list, not a truncation of reported facts.
func capHandles(hs []Handle, n int) []Handle {
	if len(hs) > n {
		return hs[:n]
	}
	return hs
}

// cutColumnPrefix splits a "col:<key>" category id.
func cutColumnPrefix(id string) (key string, ok bool) {
	const p = categoryColumnPrefix
	if len(id) > len(p) && id[:len(p)] == p {
		return id[len(p):], true
	}
	return "", false
}
