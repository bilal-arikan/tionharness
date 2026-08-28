package view

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/billing"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/skills"
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
	// ListMessagesTail is deliberately the only transcript reader here: a
	// projection never renders more than the tail (sessionTailMessages), so
	// asking for the whole transcript would copy megabytes to throw them away.
	// The second result is the tail's start index, which is exactly TailFrom.
	ListMessagesTail(ctx context.Context, sessionID string, n int) ([]db.Message, int, error)
	ListWaitingSessionAsks(ctx context.Context) ([]db.SessionAsk, error)
	ListTasks(ctx context.Context) ([]db.Task, error)
	ListActiveTasks(ctx context.Context) ([]db.Task, error)
	ListAgents(ctx context.Context) ([]db.Agent, error)
	ListSessions(ctx context.Context, agentID string) ([]db.Session, error)
	ListFlowRuns(ctx context.Context, flowID string) ([]db.FlowRun, error)
	ListSchedules(ctx context.Context) ([]db.Schedule, error)
	GetSchedule(ctx context.Context, id string) (db.Schedule, error)
	// UsageForDay backs the budget view only — no other projection reports spend.
	UsageForDay(ctx context.Context, day string) ([]db.Usage, error)
	GetAgent(ctx context.Context, id string) (db.Agent, error)
	GetUsageToday(ctx context.Context, agentID string) (db.Usage, error)
	ListMCPServers(ctx context.Context) ([]db.MCPServer, error)
	GetWorkspaceToolConfig(ctx context.Context) (db.WorkspaceToolConfig, error)
	// Artifacts + automations are store-backed Explorer leaves (TSK66).
	GetArtifact(ctx context.Context, id string) (db.Artifact, error)
	ListArtifacts(ctx context.Context, sessionID string) ([]db.Artifact, error)
	GetAutomation(ctx context.Context, id string) (db.Automation, error)
	ListAutomations(ctx context.Context) ([]db.Automation, error)
}

// BoardRefID / WorkspaceRefID are the ids a board or workspace ref carries. Both
// are singletons within a workspace and have no id of their own; naming them
// keeps Ref uniform (every ref has an id) instead of special-casing an empty one.
const (
	BoardRefID     = "board"
	WorkspaceRefID = "workspace"
	// BudgetRefID / ToolsRefID are the singleton ids the budget and tools
	// projections carry. Like the board, both are workspace singletons with no id
	// of their own; naming them keeps every Ref uniform.
	BudgetRefID = "budget"
	ToolsRefID  = "tools"
	// LogsRefID is the singleton id the logs projection carries. The log stream
	// is process-global (one ring buffer for every workspace), so the ref names a
	// singleton like budget/tools rather than a workspace-scoped entity.
	LogsRefID = "logs"
	// WorkersRefID labels a coordinator's fleet projection. It is not routable
	// through Projector (see KindWorkers) — the id exists so the View is
	// self-describing like every other one.
	WorkersRefID = "workers"
)

// Sources carries the optional non-store inputs a few Explorer projections need:
// the workspace skill catalog (held by the agent runtime), the insight findings
// sidecar and the process log ring buffer. Everything store-backed goes through
// Store; these are the three inputs that live outside it. A nil source degrades
// its projection to an explicit "(yok)" line — a map node must never fail just
// because the caller did not wire an optional source.
type Sources struct {
	Skills   SkillsSource
	Findings FindingsSource
	Logs     LogsSource
}

// SkillsSource enumerates and resolves the workspace skill catalog.
type SkillsSource interface {
	List() []skills.Skill
	Get(slug string) (skills.Skill, bool)
}

// FindingsSource lists insight findings. The finding shape is view-local
// (InsightFinding) because the insight package already imports view; a reverse
// edge would cycle. The api layer adapts insight.Findings to this shape.
type FindingsSource interface {
	ListFindings() []InsightFinding
}

// LogsSource reads the process-wide log ring buffer.
type LogsSource interface {
	Entries(limit int) []logbuf.Entry
}

// InsightFinding is the projection's view of one insight finding — the minimal
// field set the map renders (id, severity, status, evidence, recurrence).
// Deliberately view-local: see FindingsSource.
type InsightFinding struct {
	ID                 string
	LensID             string
	Channel            string
	Title              string
	RootCause          string
	ProposedFix        string
	Severity           string
	Status             string
	Occurrences        int
	EvidenceSessionIDs []string
	Regressed          bool
	LastSeen           int64 // unix seconds
}

// Projector resolves a Ref against a store and renders the matching projection.
type Projector struct {
	store        Store
	sources      Sources
	wsName       string // workspace display name, set by WithName
	defaultAgent string // workspace default agent id, set by WithDefaultAgent
}

// NewProjector wires a projector to a store.
func NewProjector(s Store) *Projector { return &Projector{store: s} }

// WithSources attaches the optional non-store inputs (skills catalog, insight
// findings, log buffer) and returns the projector for chaining. Callers that
// only have a store (the agent expand/get_view tools) may omit it — the
// affected projections then report the source as unavailable rather than
// failing the whole map.
func (p *Projector) WithSources(src Sources) *Projector {
	if p == nil {
		return nil
	}
	p.sources = src
	return p
}

// WithName sets the workspace display name, so the workspace header can show
// "WORKSPACE "TionHarnessRepo"" instead of just "WORKSPACE". Callers that only
// have a store (the agent tool paths) omit it — the header falls back to the
// generic label.
func (p *Projector) WithName(name string) *Projector {
	if p == nil {
		return nil
	}
	p.wsName = name
	return p
}

// WithDefaultAgent sets the workspace's default agent id (the agent pre-selected
// for new sessions), which lives in the workspace settings rather than on the
// agent record. Callers that do not have it omit it — the agent projection then
// leaves the "varsayılan" marker off instead of guessing.
func (p *Projector) WithDefaultAgent(id string) *Projector {
	if p == nil {
		return nil
	}
	p.defaultAgent = id
	return p
}

// Project renders the view for ref at the requested level.
//
// Unknown kinds are an error, not an empty view: a caller asking for a
// projection that does not exist has a bug, and silently handing back a blank
// summary would hide it behind plausible-looking output.
func (p *Projector) Project(ctx context.Context, ref Ref, level Level) (View, error) {
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
		return ProjectFlowRun(in, level)
	case KindSession:
		in, err := p.loadSession(ctx, ref.ID, level)
		if err != nil {
			return View{}, err
		}
		return ProjectSession(in, level)
	case KindBoard:
		tasks, err := p.store.ListActiveTasks(ctx)
		if err != nil {
			return View{}, fmt.Errorf("view: board: %w", err)
		}
		return ProjectBoard(BoardInput{Tasks: tasks, Sub: ref.Sub}, level)
	case KindSchedule:
		sc, err := p.store.GetSchedule(ctx, ref.ID)
		if err != nil {
			return View{}, fmt.Errorf("view: schedule %s: %w", ref.ID, err)
		}
		return ProjectSchedule(ScheduleInput{Schedule: sc}, level)
	case KindSpace:
		in, err := p.loadWorkspace(ctx)
		if err != nil {
			return View{}, err
		}
		return ProjectWorkspace(in, level)
	case KindAgent:
		in, err := p.loadAgent(ctx, ref.ID)
		if err != nil {
			return View{}, err
		}
		return ProjectAgent(in, level)
	case KindBudget:
		in, err := p.loadBudget(ctx)
		if err != nil {
			return View{}, err
		}
		return ProjectBudget(in, level)
	case KindTools:
		in, err := p.loadTools(ctx)
		if err != nil {
			return View{}, err
		}
		return ProjectTools(in, level)
	case KindCategory:
		members, err := p.categoryMembers(ctx, ref.ID)
		if err != nil {
			return View{}, err
		}
		return ProjectCategory(CategoryInput{ID: ref.ID, Members: members}, level)
	case KindArtifact:
		in, err := p.loadArtifact(ctx, ref.ID)
		if err != nil {
			return View{}, err
		}
		return ProjectArtifact(in, level)
	case KindAutomation:
		in, err := p.loadAutomation(ctx, ref.ID)
		if err != nil {
			return View{}, err
		}
		return ProjectAutomation(in, level)
	case KindSkill:
		sk, err := p.loadSkill(ref.ID)
		if err != nil {
			return View{}, err
		}
		return ProjectSkill(SkillInput{Skill: sk}, level)
	case KindInsight:
		f, err := p.loadInsight(ref.ID)
		if err != nil {
			return View{}, err
		}
		return ProjectInsight(InsightInput{Finding: f}, level)
	case KindLogs:
		return ProjectLogs(LogsInput{Entries: p.logEntries()}, level)
	default:
		return View{}, fmt.Errorf("view: unsupported kind %q", ref.Kind)
	}
}

// Workspace renders the workspace roll-up AND returns its counters from the
// same load, for callers (the dashboard) that need both. Going through Project
// and then counting the store again would give two tallies of the same facts,
// taken at two different instants — exactly the drift this layer exists to stop.
func (p *Projector) Workspace(ctx context.Context, level Level) (View, WorkspaceCounts, error) {
	if p == nil || p.store == nil {
		return View{}, WorkspaceCounts{}, fmt.Errorf("view: projector has no store")
	}
	in, err := p.loadWorkspace(ctx)
	if err != nil {
		return View{}, WorkspaceCounts{}, err
	}
	// Pin the clock so the text and the counters describe the same instant.
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	v, err := ProjectWorkspace(in, level)
	if err != nil {
		return View{}, WorkspaceCounts{}, err
	}
	return v, CountWorkspace(in, in.Now), nil
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
		Name:        p.wsName,
		Agents:      agents,
		Sessions:    sessions,
		Tasks:       tasks,
		FlowRuns:    runs,
		Schedules:   schedules,
		WaitingAsks: asks,
	}
	return in, nil
}

// loadSession gathers the session header, the transcript TAIL and any pending
// question.
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

	if level == LevelTiny {
		return in, nil
	}

	msgs, tailFrom, err := p.store.ListMessagesTail(ctx, id, sessionTailMessages)
	if err != nil {
		return SessionInput{}, fmt.Errorf("view: session %s messages: %w", id, err)
	}
	in.TailFrom = tailFrom
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

// loadAgent gathers one agent, the sessions bound to it, and its usage row for
// today. All three are in-memory reads. A missing agent is an error (a stale id
// must not render as a blank agent); a missing usage row is normal (nothing has
// run today) and leaves Usage zero-valued rather than failing. The row itself is
// never rendered — spend belongs to the budget view — it only feeds the call
// count in the projection's Source fingerprint.
func (p *Projector) loadAgent(ctx context.Context, id string) (AgentInput, error) {
	agent, err := p.store.GetAgent(ctx, id)
	if err != nil {
		return AgentInput{}, fmt.Errorf("view: agent %s: %w", id, err)
	}
	sessions, err := p.store.ListSessions(ctx, id)
	if err != nil {
		return AgentInput{}, fmt.Errorf("view: agent %s sessions: %w", id, err)
	}
	in := AgentInput{
		Agent:     agent,
		Sessions:  sessions,
		IsDefault: p.defaultAgent != "" && p.defaultAgent == id,
	}
	if usage, err := p.store.GetUsageToday(ctx, id); err == nil {
		in.Usage = usage
	}
	return in, nil
}

// loadBudget prices today's spend exactly once, the same way every budget surface
// does: merge every agent's per-model breakdown and hand it to billing.RollupOf.
// The projection then only renders the rollup — it re-prices nothing.
//
// A usage read error fails the projection rather than rendering a zero budget: a
// budget silently reading "$0.00" would be indistinguishable from a genuinely
// idle day, which is a different fact.
func (p *Projector) loadBudget(ctx context.Context) (BudgetInput, error) {
	day := db.Today()
	rows, err := p.store.UsageForDay(ctx, day)
	if err != nil {
		return BudgetInput{}, fmt.Errorf("view: budget usage: %w", err)
	}
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
	return BudgetInput{Rollup: billing.RollupOf(merged), Day: day}, nil
}

// loadTools gathers the MCP server pool and the workspace tool-activation config.
func (p *Projector) loadTools(ctx context.Context) (ToolsInput, error) {
	servers, err := p.store.ListMCPServers(ctx)
	if err != nil {
		return ToolsInput{}, fmt.Errorf("view: tools mcp servers: %w", err)
	}
	cfg, err := p.store.GetWorkspaceToolConfig(ctx)
	if err != nil {
		return ToolsInput{}, fmt.Errorf("view: tools config: %w", err)
	}
	return ToolsInput{MCPServers: servers, ToolConfig: cfg}, nil
}

// loadArtifact reads one artifact for its metadata projection. A missing id or
// entity is an error, never a blank artifact — a blank card would read like a
// real (but empty) artifact.
func (p *Projector) loadArtifact(ctx context.Context, id string) (ArtifactInput, error) {
	if id == "" {
		return ArtifactInput{}, fmt.Errorf("view: artifact: empty id")
	}
	a, err := p.store.GetArtifact(ctx, id)
	if err != nil {
		return ArtifactInput{}, fmt.Errorf("view: artifact %s: %w", id, err)
	}
	return ArtifactInput{Artifact: a}, nil
}

// loadAutomation reads one automation rule for its projection.
func (p *Projector) loadAutomation(ctx context.Context, id string) (AutomationInput, error) {
	if id == "" {
		return AutomationInput{}, fmt.Errorf("view: automation: empty id")
	}
	a, err := p.store.GetAutomation(ctx, id)
	if err != nil {
		return AutomationInput{}, fmt.Errorf("view: automation %s: %w", id, err)
	}
	return AutomationInput{Automation: a}, nil
}

// loadSkill resolves one skill from the optional catalog. A missing source is
// reported distinctly from an absent slug — "catalog unavailable" is a wiring
// gap, "not found" is a stale ref; neither may render as a blank skill.
func (p *Projector) loadSkill(slug string) (skills.Skill, error) {
	if p.sources.Skills == nil {
		return skills.Skill{}, fmt.Errorf("view: skill %s: skill catalog unavailable", slug)
	}
	sk, ok := p.sources.Skills.Get(slug)
	if !ok {
		return skills.Skill{}, fmt.Errorf("view: skill %s not found", slug)
	}
	return sk, nil
}

// loadInsight resolves one finding from the optional findings source.
func (p *Projector) loadInsight(id string) (InsightFinding, error) {
	if p.sources.Findings == nil {
		return InsightFinding{}, fmt.Errorf("view: insight %s: findings store unavailable", id)
	}
	for _, f := range p.sources.Findings.ListFindings() {
		if f.ID == id {
			return f, nil
		}
	}
	return InsightFinding{}, fmt.Errorf("view: insight %s not found", id)
}

// logEntries reads the optional log buffer. The projection caps the tail itself,
// so the full retained stream is passed in (the buffer holds at most ~2000 rows).
func (p *Projector) logEntries() []logbuf.Entry {
	if p.sources.Logs == nil {
		return nil
	}
	return p.sources.Logs.Entries(0)
}
