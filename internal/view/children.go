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
// lens filters the children: LensErrors returns only the troubled ones. Finer
// lens semantics (stale/recent slicing) are a later refinement; today only the
// errors lens narrows the set, every other lens passes the members through.
//
// The list is capped at categoryTopN (the per-node child cap, _Docs/68 §8.1) so a
// 300-session workspace cannot explode the map. The honest total lives on the
// node's own Project (e.g. ProjectCategory reports Elided) — Children is the raw,
// bounded edge list.
//
// An unknown kind is an error, not an empty slice: a caller asking to expand a
// kind that has no children defined has a bug, and a silent empty list would hide
// it behind a plausible "leaf node".
func (p *Projector) Children(ctx context.Context, ref Ref, lens Lens) ([]Handle, error) {
	if p == nil || p.store == nil {
		return nil, fmt.Errorf("view: projector has no store")
	}
	switch ref.Kind {
	case KindSpace:
		return workspaceChildren(), nil
	case KindCategory:
		members, err := p.categoryMembers(ctx, ref.ID, lens)
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
		sessions, err := p.agentSessionChildren(ctx, ref.ID, lens)
		if err != nil {
			return nil, err
		}
		return capHandles(sessions, categoryTopN), nil
	case KindSession:
		workers, err := p.sessionWorkerChildren(ctx, ref.ID, lens)
		if err != nil {
			return nil, err
		}
		return capHandles(workers, categoryTopN), nil
	case KindBudget, KindTools, KindFlowRun, KindSchedule,
		KindArtifact, KindAutomation, KindSkill, KindInsight, KindLogs:
		// Leaves in the map: their breakdown is rendered inline by Project, so
		// there is nothing structural to expand into.
		return nil, nil
	default:
		return nil, fmt.Errorf("view: children unsupported for kind %q", ref.Kind)
	}
}

// IsExpandable reports whether a ref has structural children worth expanding —
// the map/agent affordance for "can I drill into this?". It mirrors the kinds
// Children resolves to a non-nil edge set: the workspace root, a category, a
// whole board (a single card is a leaf), an agent, and a session. budget / tools
// / flowrun / schedule render their breakdown inline and have no children.
func IsExpandable(ref Ref) bool {
	switch ref.Kind {
	case KindSpace, KindCategory, KindAgent, KindSession:
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
func (p *Projector) categoryMembers(ctx context.Context, id string, lens Lens) ([]Handle, error) {
	switch id {
	case CategorySessions:
		sessions, err := p.store.ListSessions(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("view: category sessions: %w", err)
		}
		return sessionHandleList(liveSessions(sessions), lens), nil
	case CategoryFlows:
		runs, err := p.store.ListFlowRuns(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("view: category flows: %w", err)
		}
		return flowRunHandleList(runs, lens), nil
	case CategoryAgents:
		agents, err := p.store.ListAgents(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category agents: %w", err)
		}
		return agentHandleList(agents), nil
	case CategoryArtifacts:
		artifacts, err := p.store.ListArtifacts(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("view: category artifacts: %w", err)
		}
		return artifactHandleList(artifacts), nil
	case CategoryAutomations:
		automations, err := p.store.ListAutomations(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category automations: %w", err)
		}
		return automationHandleList(automations, lens), nil
	case CategorySkills:
		if p.sources.Skills == nil {
			return nil, fmt.Errorf("view: category skills: skill catalog unavailable")
		}
		return skillHandleList(p.sources.Skills.List()), nil
	case CategoryInsights:
		if p.sources.Findings == nil {
			return nil, fmt.Errorf("view: category insights: findings store unavailable")
		}
		return insightHandleList(p.sources.Findings.ListFindings(), lens), nil
	}
	if key, found := cutColumnPrefix(id); found {
		tasks, err := p.store.ListActiveTasks(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category %s: %w", id, err)
		}
		return columnCardHandles(tasks, key, lens), nil
	}
	return nil, fmt.Errorf("view: unknown category %q", id)
}

// boardColumnChildren is the board's structural children: one category node per
// column that exists on the board. A column drills into its cards.
func (p *Projector) boardColumnChildren(ctx context.Context) ([]Handle, error) {
	tasks, err := p.store.ListActiveTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("view: board columns: %w", err)
	}
	cols := boardColumns(tasks)
	hs := make([]Handle, 0, len(cols))
	for _, c := range cols {
		hs = append(hs, Handle{
			Label: fmt.Sprintf("%s (%d kart)", c.Key, len(c.Tasks)),
			Ref:   Ref{Kind: KindCategory, ID: categoryColumnPrefix + c.Key},
			Level: LevelCard,
		})
	}
	return hs, nil
}

// agentSessionChildren is an agent's structural children: the sessions bound to
// it, lens-filtered.
func (p *Projector) agentSessionChildren(ctx context.Context, agentID string, lens Lens) ([]Handle, error) {
	if agentID == "" {
		return nil, fmt.Errorf("view: agent children: empty id")
	}
	sessions, err := p.store.ListSessions(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("view: agent %s sessions: %w", agentID, err)
	}
	return sessionHandleList(liveSessions(sessions), lens), nil
}

// sessionWorkerChildren is a session's structural children: the worker sessions
// that report up to it (a coordinator drills into its fleet). A plain session
// simply has none.
func (p *Projector) sessionWorkerChildren(ctx context.Context, sessionID string, lens Lens) ([]Handle, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("view: session children: empty id")
	}
	all, err := p.store.ListSessions(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("view: session %s workers: %w", sessionID, err)
	}
	var workers []db.Session
	for _, s := range all {
		if s.State != "archived" && s.CoordinatorSessionID == sessionID {
			workers = append(workers, s)
		}
	}
	return sessionHandleList(workers, lens), nil
}

// sessionHandleList orders sessions most-recently-active first, applies the lens
// filter, and renders each as a session handle.
func sessionHandleList(sessions []db.Session, lens Lens) []Handle {
	filtered := make([]db.Session, 0, len(sessions))
	for _, s := range sessions {
		if sessionMatchesLens(s, lens) {
			filtered = append(filtered, s)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].UpdatedAt > filtered[j].UpdatedAt })
	hs := make([]Handle, 0, len(filtered))
	for _, s := range filtered {
		hs = append(hs, Handle{
			Label: "session:" + s.ID + " " + clip(orDash(s.Title), 40),
			Ref:   Ref{Kind: KindSession, ID: s.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// flowRunHandleList orders runs most-recent first, applies the lens filter, and
// renders each as a flow-run handle.
func flowRunHandleList(runs []db.FlowRun, lens Lens) []Handle {
	filtered := make([]db.FlowRun, 0, len(runs))
	for _, r := range runs {
		if flowRunMatchesLens(r, lens) {
			filtered = append(filtered, r)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].UpdatedAt > filtered[j].UpdatedAt })
	hs := make([]Handle, 0, len(filtered))
	for _, r := range filtered {
		hs = append(hs, Handle{
			Label: "run:" + r.ID + " " + string(r.Status),
			Ref:   Ref{Kind: KindFlowRun, ID: r.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// agentHandleList renders every agent as an agent handle. The errors lens does
// not narrow agents — an agent is not itself an error state — so all agents pass.
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
// artifact handle. The errors lens does not narrow artifacts — an artifact is
// not itself an error state — so all pass.
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
// The errors lens keeps only rules whose last fire failed.
func automationHandleList(automations []db.Automation, lens Lens) []Handle {
	filtered := make([]db.Automation, 0, len(automations))
	for _, a := range automations {
		if automationMatchesLens(a, lens) {
			filtered = append(filtered, a)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	hs := make([]Handle, 0, len(filtered))
	for _, a := range filtered {
		hs = append(hs, Handle{
			Label: "automation:" + a.ID + " " + clip(orDash(a.Name), 40),
			Ref:   Ref{Kind: KindAutomation, ID: a.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// skillHandleList renders the catalog (display order) as skill handles. The
// errors lens does not narrow skills — a skill is not an error state.
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
// evidence first) as insight handles. The errors lens keeps only findings that
// still need attention (fresh or regressed).
func insightHandleList(findings []InsightFinding, lens Lens) []Handle {
	filtered := make([]InsightFinding, 0, len(findings))
	for _, f := range findings {
		if insightMatchesLens(f, lens) {
			filtered = append(filtered, f)
		}
	}
	hs := make([]Handle, 0, len(filtered))
	for _, f := range filtered {
		hs = append(hs, Handle{
			Label: "insight:" + f.ID + " " + clip(f.Title, 40),
			Ref:   Ref{Kind: KindInsight, ID: f.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// columnCardHandles renders the cards of one board column as card handles
// (KindBoard drill-down via Sub), applying the lens filter.
func columnCardHandles(tasks []db.Task, columnKey string, lens Lens) []Handle {
	var cards []db.Task
	for _, t := range tasks {
		key := t.BoardState
		if key == "" {
			key = "(boş)"
		}
		if key == columnKey && taskMatchesLens(t, lens) {
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

// sessionMatchesLens decides whether a session belongs in a lens-filtered child
// list. Only the errors lens narrows the set (to stuck sessions); every other
// lens passes the session through.
func sessionMatchesLens(s db.Session, lens Lens) bool {
	if lens == LensErrors {
		return s.StuckTurns > 0
	}
	return true
}

// flowRunMatchesLens narrows a run to failures under the errors lens.
func flowRunMatchesLens(r db.FlowRun, lens Lens) bool {
	if lens == LensErrors {
		return r.Status == db.FlowFailure
	}
	return true
}

// taskMatchesLens narrows a card to failed cards under the errors lens.
func taskMatchesLens(t db.Task, lens Lens) bool {
	if lens == LensErrors {
		return t.BoardState == db.BoardFailed
	}
	return true
}

// automationMatchesLens narrows automations to rules whose last fire failed
// under the errors lens — a rule that never fired has no error, and a disabled
// rule is a deliberate pause, not a problem.
func automationMatchesLens(a db.Automation, lens Lens) bool {
	if lens == LensErrors {
		return a.LastError != ""
	}
	return true
}

// insightMatchesLens narrows findings to ones needing attention under the
// errors lens: a fresh (unreviewed) finding or a regressed (recurring after
// being closed) one. A triaged/accepted/applied finding is under review and
// not an open error.
func insightMatchesLens(f InsightFinding, lens Lens) bool {
	if lens == LensErrors {
		return f.Status == insightStatusNew || f.Regressed
	}
	return true
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
