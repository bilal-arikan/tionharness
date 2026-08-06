package view

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/billing"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// decodeState parses a persisted orchestration state snapshot.
func decodeState(data string, st *orchestration.State) error {
	return json.Unmarshal([]byte(data), st)
}

// Store is the narrow slice of the workspace store a projection needs. Keeping
// it an interface (rather than taking *db.DB) is what lets projections be tested
// with hand-built fixtures and keeps this package a leaf.
type Store interface {
	GetFlowRun(ctx context.Context, id string) (db.FlowRun, error)
	GetFlow(ctx context.Context, id string) (db.Flow, error)
	GetSession(ctx context.Context, id string) (db.Session, error)
	GetSessionUsage(ctx context.Context, id string) (db.SessionUsage, error)
	ListMessages(ctx context.Context, sessionID string) ([]db.Message, error)
	ListWaitingSessionAsks(ctx context.Context) ([]db.SessionAsk, error)
	ListTasks(ctx context.Context) ([]db.Task, error)
	ListActiveTasks(ctx context.Context) ([]db.Task, error)
	ListAgents(ctx context.Context) ([]db.Agent, error)
	ListSessions(ctx context.Context, agentID string) ([]db.Session, error)
	ListFlowRuns(ctx context.Context, flowID string) ([]db.FlowRun, error)
	ListSchedules(ctx context.Context) ([]db.Schedule, error)
	GetSchedule(ctx context.Context, id string) (db.Schedule, error)
	WorkspaceTokensToday(ctx context.Context) int64
	UsageForDay(ctx context.Context, day string) ([]db.Usage, error)
}

// BoardRefID / WorkspaceRefID are the ids a board or workspace ref carries. Both
// are singletons within a workspace and have no id of their own; naming them
// keeps Ref uniform (every ref has an id) instead of special-casing an empty one.
const (
	BoardRefID     = "board"
	WorkspaceRefID = "workspace"
	// WorkersRefID labels a coordinator's fleet projection. It is not routable
	// through Projector (see KindWorkers) — the id exists so the View is
	// self-describing like every other one.
	WorkersRefID = "workers"
)

// Projector resolves a Ref against a store and renders the matching projection.
type Projector struct {
	store Store
}

// NewProjector wires a projector to a store.
func NewProjector(s Store) *Projector { return &Projector{store: s} }

// Project renders the view for ref at the requested level and lens.
//
// Unknown kinds are an error, not an empty view: a caller asking for a
// projection that does not exist has a bug, and silently handing back a blank
// summary would hide it behind plausible-looking output.
func (p *Projector) Project(ctx context.Context, ref Ref, level Level, lens Lens) (View, error) {
	if p == nil || p.store == nil {
		return View{}, fmt.Errorf("view: projector has no store")
	}
	if strings.TrimSpace(ref.ID) == "" {
		return View{}, fmt.Errorf("view: ref has no id")
	}
	switch ref.Kind {
	case KindFlowRun:
		in, err := p.loadFlowRun(ctx, ref.ID)
		if err != nil {
			return View{}, err
		}
		in.Sub = ref.Sub
		return ProjectFlowRun(in, level, lens)
	case KindSession:
		in, err := p.loadSession(ctx, ref.ID, level)
		if err != nil {
			return View{}, err
		}
		return ProjectSession(in, level, lens)
	case KindBoard:
		tasks, err := p.store.ListActiveTasks(ctx)
		if err != nil {
			return View{}, fmt.Errorf("view: board: %w", err)
		}
		return ProjectBoard(BoardInput{Tasks: tasks, Sub: ref.Sub}, level, lens)
	case KindSchedule:
		sc, err := p.store.GetSchedule(ctx, ref.ID)
		if err != nil {
			return View{}, fmt.Errorf("view: schedule %s: %w", ref.ID, err)
		}
		return ProjectSchedule(ScheduleInput{Schedule: sc}, level, lens)
	case KindSpace:
		in, err := p.loadWorkspace(ctx)
		if err != nil {
			return View{}, err
		}
		return ProjectWorkspace(in, level, lens)
	default:
		return View{}, fmt.Errorf("view: unsupported kind %q", ref.Kind)
	}
}

// loadWorkspace gathers the roll-up inputs. Every one of these is an in-memory
// store read, so a workspace view costs no disk I/O regardless of how much
// history the workspace has — that is what makes it cheap enough to poll.
//
// A failure in any single source fails the whole projection rather than
// rendering a partial workspace: a dashboard silently missing its flow runs
// would read as "no runs", which is the opposite of the truth.
func (p *Projector) loadWorkspace(ctx context.Context) (WorkspaceInput, error) {
	agents, err := p.store.ListAgents(ctx)
	if err != nil {
		return WorkspaceInput{}, fmt.Errorf("view: workspace agents: %w", err)
	}
	sessions, err := p.store.ListSessions(ctx, "")
	if err != nil {
		return WorkspaceInput{}, fmt.Errorf("view: workspace sessions: %w", err)
	}
	tasks, err := p.store.ListTasks(ctx)
	if err != nil {
		return WorkspaceInput{}, fmt.Errorf("view: workspace tasks: %w", err)
	}
	runs, err := p.store.ListFlowRuns(ctx, "")
	if err != nil {
		return WorkspaceInput{}, fmt.Errorf("view: workspace flow runs: %w", err)
	}
	schedules, err := p.store.ListSchedules(ctx)
	if err != nil {
		return WorkspaceInput{}, fmt.Errorf("view: workspace schedules: %w", err)
	}
	asks, err := p.store.ListWaitingSessionAsks(ctx)
	if err != nil {
		return WorkspaceInput{}, fmt.Errorf("view: workspace asks: %w", err)
	}

	in := WorkspaceInput{
		Agents:      agents,
		Sessions:    sessions,
		Tasks:       tasks,
		FlowRuns:    runs,
		Schedules:   schedules,
		WaitingAsks: asks,
		TokensToday: p.store.WorkspaceTokensToday(ctx),
	}

	// Price today's spend the same way every budget surface does (billing.RollupOf),
	// so the header's dollar figure can never disagree with the Budget screen. A
	// usage read error degrades the cost line to zero rather than failing the whole
	// projection: the token figure and every signal above are still correct and
	// useful without it.
	if rows, err := p.store.UsageForDay(ctx, db.Today()); err == nil {
		in.CostToday, in.CostEstimated = workspaceCost(rows)
	}
	return in, nil
}

// workspaceCost sums the USD cost of a day's usage rows across every agent,
// merging their per-model breakdowns into one rollup. estimated is true when any
// priced slice used an equivalent-API estimate or lacked a real list price — the
// same "this is not a real invoice" flag the Budget screen shows.
func workspaceCost(rows []db.Usage) (cost float64, estimated bool) {
	merged := map[string]db.KindStat{}
	for _, u := range rows {
		for key, st := range u.ByModel {
			m := merged[key]
			m.Calls += st.Calls
			m.InputTokens += st.InputTokens
			m.OutputTokens += st.OutputTokens
			m.CacheReadTokens += st.CacheReadTokens
			m.CacheWriteTokens += st.CacheWriteTokens
			merged[key] = m
		}
	}
	roll := billing.RollupOf(merged)
	return roll.CostUSD, roll.Estimated || !roll.Priced
}

// loadSession gathers the session header, its usage rollup, the transcript TAIL
// and any pending question.
//
// Only the tail is read, and only above the tiny tier: a tiny view is answered
// entirely from the in-memory session header, so pushing one costs no file I/O.
// The number of messages skipped is reported as View.Elided.
func (p *Projector) loadSession(ctx context.Context, id string, level Level) (SessionInput, error) {
	sess, err := p.store.GetSession(ctx, id)
	if err != nil {
		return SessionInput{}, fmt.Errorf("view: session %s: %w", id, err)
	}
	in := SessionInput{Session: sess}

	// A session with no recorded usage is normal (nothing has run yet); a read
	// error is not worth failing the whole view over, so the cost line degrades
	// to zero rather than taking the projection down.
	if usage, err := p.store.GetSessionUsage(ctx, id); err == nil {
		in.Usage = usage
	}
	if level == LevelTiny {
		return in, nil
	}

	msgs, err := p.store.ListMessages(ctx, id)
	if err != nil {
		return SessionInput{}, fmt.Errorf("view: session %s messages: %w", id, err)
	}
	if len(msgs) > sessionTailMessages {
		in.TailFrom = len(msgs) - sessionTailMessages
		msgs = msgs[in.TailFrom:]
	}
	in.Messages = msgs

	if asks, err := p.store.ListWaitingSessionAsks(ctx); err == nil {
		for i := range asks {
			if asks[i].SessionID == id {
				in.WaitingAsk = &asks[i]
				break
			}
		}
	}
	return in, nil
}

// loadFlowRun gathers the run, its flow and the parsed graph/state.
//
// A run whose graph or state JSON will not parse is reported as an error rather
// than rendered as an empty chain — a view that shows "0/0 node" for a corrupted
// run is worse than no view at all, because it reads like a healthy empty run.
func (p *Projector) loadFlowRun(ctx context.Context, id string) (FlowRunInput, error) {
	run, err := p.store.GetFlowRun(ctx, id)
	if err != nil {
		return FlowRunInput{}, fmt.Errorf("view: flow run %s: %w", id, err)
	}

	in := FlowRunInput{Run: run}

	// The flow may legitimately be gone (deleted after the run); the run itself
	// is still projectable, it just loses its name.
	if flow, err := p.store.GetFlow(ctx, run.FlowID); err == nil {
		in.Flow = flow
		g, err := orchestration.ParseGraph(flow.Graph)
		if err != nil {
			return FlowRunInput{}, fmt.Errorf("view: flow run %s graph: %w", id, err)
		}
		in.Graph = g
	}

	if s := strings.TrimSpace(run.State); s != "" {
		if err := decodeState(s, &in.State); err != nil {
			return FlowRunInput{}, fmt.Errorf("view: flow run %s state: %w", id, err)
		}
	}
	return in, nil
}
