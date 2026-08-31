package view

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// structuralCache is a per-request snapshot of the store reads the structural
// walk needs.
//
// Neighborhood enumerates every node in the workspace and then asks each one for
// its children. The session-shaped answers (an agent's sessions, a coordinator's
// worker sessions) each re-read the WHOLE session list from the store, and the
// store returns a fresh copy — normalised and sorted — on every call. With N
// sessions and A agents the walk therefore paid for roughly 2*(N+A) full copies
// of the session list per focus request. The cache reads each list at most once
// and filters that one snapshot instead.
//
// Sharing a snapshot also makes a single walk internally consistent: parents and
// children are resolved against one store state rather than against N slightly
// different ones.
//
// A cache is cheap and single-use. Entry points that read one list once
// (Children, ProjectCategory) build a throwaway cache, so they behave exactly as
// they did when they called the store directly.
type structuralCache struct {
	p *Projector

	sessions    cachedList[db.Session]
	tasks       cachedList[db.Task]
	agents      cachedList[db.Agent]
	runs        cachedList[db.FlowRun]
	artifacts   cachedList[db.Artifact]
	automations cachedList[db.Automation]
	skills      cachedList[skills.Skill]
	findings    cachedList[InsightFinding]
}

// cachedList memoises one list read, including its error, so a failing source
// (an unavailable skill catalog, say) reports the same failure on every lookup
// instead of being retried once per visited node.
type cachedList[T any] struct {
	items  []T
	err    error
	loaded bool
}

func (c *cachedList[T]) get(load func() ([]T, error)) ([]T, error) {
	if !c.loaded {
		c.items, c.err = load()
		c.loaded = true
	}
	return c.items, c.err
}

func (p *Projector) newStructuralCache() *structuralCache {
	return &structuralCache{p: p}
}

func (c *structuralCache) allSessions(ctx context.Context) ([]db.Session, error) {
	return c.sessions.get(func() ([]db.Session, error) { return c.p.store.ListSessions(ctx, "") })
}

// agentSessions filters the snapshot instead of asking the store to filter. The
// store returns its sessions in one total order and filtering preserves a
// subsequence of it, so the result is ordered identically to ListSessions(agentID).
func (c *structuralCache) agentSessions(ctx context.Context, agentID string) ([]db.Session, error) {
	all, err := c.allSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]db.Session, 0, len(all))
	for _, s := range all {
		if s.AgentID == agentID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (c *structuralCache) activeTasks(ctx context.Context) ([]db.Task, error) {
	return c.tasks.get(func() ([]db.Task, error) { return c.p.store.ListActiveTasks(ctx) })
}

func (c *structuralCache) allAgents(ctx context.Context) ([]db.Agent, error) {
	return c.agents.get(func() ([]db.Agent, error) { return c.p.store.ListAgents(ctx) })
}

func (c *structuralCache) allFlowRuns(ctx context.Context) ([]db.FlowRun, error) {
	return c.runs.get(func() ([]db.FlowRun, error) { return c.p.store.ListFlowRuns(ctx, "") })
}

func (c *structuralCache) allArtifacts(ctx context.Context) ([]db.Artifact, error) {
	return c.artifacts.get(func() ([]db.Artifact, error) { return c.p.store.ListArtifacts(ctx, "") })
}

func (c *structuralCache) allAutomations(ctx context.Context) ([]db.Automation, error) {
	return c.automations.get(func() ([]db.Automation, error) { return c.p.store.ListAutomations(ctx) })
}

func (c *structuralCache) allSkills() ([]skills.Skill, error) {
	return c.skills.get(func() ([]skills.Skill, error) {
		if c.p.sources.Skills == nil {
			return nil, fmt.Errorf("view: category skills: skill catalog unavailable")
		}
		return c.p.sources.Skills.List(), nil
	})
}

func (c *structuralCache) allFindings() ([]InsightFinding, error) {
	return c.findings.get(func() ([]InsightFinding, error) {
		if c.p.sources.Findings == nil {
			return nil, fmt.Errorf("view: category insights: findings store unavailable")
		}
		return c.p.sources.Findings.ListFindings(), nil
	})
}

// children is Children against the snapshot, without the legacy per-node
// presentation cap.
func (c *structuralCache) children(ctx context.Context, ref Ref) ([]Handle, error) {
	switch ref.Kind {
	case KindSpace:
		return workspaceChildren(), nil
	case KindCategory:
		return c.categoryMembers(ctx, ref.ID)
	case KindBoard:
		if ref.Sub != "" {
			return nil, nil
		}
		return c.boardColumnChildren(ctx)
	case KindAgent:
		return c.agentSessionChildren(ctx, ref.ID)
	case KindSession:
		return c.sessionWorkerChildren(ctx, ref.ID)
	case KindBudget, KindTools, KindFlowRun, KindSchedule,
		KindArtifact, KindAutomation, KindSkill, KindInsight, KindLogs:
		return nil, nil
	default:
		return nil, fmt.Errorf("view: children unsupported for kind %q", ref.Kind)
	}
}

// categoryMembers computes the FULL (uncapped) member handle list for a category.
func (c *structuralCache) categoryMembers(ctx context.Context, id string) ([]Handle, error) {
	switch id {
	case CategorySessions:
		sessions, err := c.allSessions(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category sessions: %w", err)
		}
		return sessionHandleList(liveSessions(sessions)), nil
	case CategoryFlows:
		runs, err := c.allFlowRuns(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category flows: %w", err)
		}
		return flowRunHandleList(runs), nil
	case CategoryAgents:
		agents, err := c.allAgents(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category agents: %w", err)
		}
		return agentHandleList(agents), nil
	case CategoryArtifacts:
		artifacts, err := c.allArtifacts(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category artifacts: %w", err)
		}
		return artifactHandleList(artifacts), nil
	case CategoryAutomations:
		automations, err := c.allAutomations(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category automations: %w", err)
		}
		return automationHandleList(automations), nil
	case CategorySkills:
		catalog, err := c.allSkills()
		if err != nil {
			return nil, err
		}
		return skillHandleList(catalog), nil
	case CategoryInsights:
		findings, err := c.allFindings()
		if err != nil {
			return nil, err
		}
		return insightHandleList(findings), nil
	}
	if key, found := cutColumnPrefix(id); found {
		tasks, err := c.activeTasks(ctx)
		if err != nil {
			return nil, fmt.Errorf("view: category %s: %w", id, err)
		}
		return columnCardHandles(tasks, key), nil
	}
	return nil, fmt.Errorf("view: unknown category %q", id)
}

// boardColumnChildren is the board's structural children: one category node per
// column that exists on the board. A column drills into its cards.
func (c *structuralCache) boardColumnChildren(ctx context.Context) ([]Handle, error) {
	tasks, err := c.activeTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("view: board columns: %w", err)
	}
	cols := boardColumns(tasks)
	hs := make([]Handle, 0, len(cols))
	for _, col := range cols {
		hs = append(hs, Handle{
			Label: fmt.Sprintf("%s (%d kart)", col.Key, len(col.Tasks)),
			Ref:   Ref{Kind: KindCategory, ID: categoryColumnPrefix + col.Key},
			Level: LevelCard,
		})
	}
	return hs, nil
}

// agentSessionChildren is an agent's structural children: the sessions bound to
// it.
func (c *structuralCache) agentSessionChildren(ctx context.Context, agentID string) ([]Handle, error) {
	if agentID == "" {
		return nil, fmt.Errorf("view: agent children: empty id")
	}
	sessions, err := c.agentSessions(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("view: agent %s sessions: %w", agentID, err)
	}
	return sessionHandleList(liveSessions(sessions)), nil
}

// sessionWorkerChildren is a session's structural children: the worker sessions
// that report up to it (a coordinator drills into its fleet). A plain session
// simply has none.
func (c *structuralCache) sessionWorkerChildren(ctx context.Context, sessionID string) ([]Handle, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("view: session children: empty id")
	}
	all, err := c.allSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("view: session %s workers: %w", sessionID, err)
	}
	var workers []db.Session
	for _, s := range all {
		if s.State != "archived" && s.CoordinatorSessionID == sessionID {
			workers = append(workers, s)
		}
	}
	return sessionHandleList(workers), nil
}

// nodes enumerates every possible parent node once, against the snapshot.
func (c *structuralCache) nodes(ctx context.Context) ([]Handle, error) {
	root := Handle{Label: "workspace", Ref: Ref{Kind: KindSpace, ID: WorkspaceRefID}, Level: LevelCard}
	nodes := []Handle{root}
	rootChildren := workspaceChildren()
	nodes = append(nodes, rootChildren...)

	for _, node := range rootChildren {
		children, err := c.children(ctx, node.Ref)
		if err != nil {
			// Skills and insights are optional projector sources. Their absence
			// must not prevent resolving an unrelated focus node.
			if node.Ref.Kind == KindCategory &&
				(node.Ref.ID == CategorySkills || node.Ref.ID == CategoryInsights) {
				continue
			}
			return nil, err
		}
		nodes = append(nodes, children...)
	}

	// Sessions can also parent worker sessions, and agents parent their sessions.
	// Both kinds are already present through root categories; expanding them here
	// is enough to discover every incoming structural edge without recursion.
	base := uniqueSortedHandles(nodes)
	for _, node := range base {
		if node.Ref.Kind != KindAgent && node.Ref.Kind != KindSession {
			continue
		}
		children, err := c.children(ctx, node.Ref)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, children...)
	}
	return uniqueSortedHandles(nodes), nil
}
