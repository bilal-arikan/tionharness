package awareness

import (
	"context"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/notes"
)

// Store is what the producers read. *db.DB satisfies it; tests use a fake.
type Store interface {
	GetSession(ctx context.Context, id string) (db.Session, error)
	ListSessions(ctx context.Context, agentID string) ([]db.Session, error)
	ListMessages(ctx context.Context, sessionID string) ([]db.Message, error)
	ListMessagesTail(ctx context.Context, sessionID string, n int) ([]db.Message, int, error)
	ListWaitingSessionAsks(ctx context.Context) ([]db.SessionAsk, error)
	ListTasks(ctx context.Context) ([]db.Task, error)
	ListAgents(ctx context.Context) ([]db.Agent, error)
	ListFlowRuns(ctx context.Context, flowID string, limit int) ([]db.FlowRun, error)
	ListSchedules(ctx context.Context) ([]db.Schedule, error)
	ListArtifacts(ctx context.Context, sessionID string) ([]db.Artifact, error)
	SessionDir(sessionID string) (string, error)
}

// Settings are the per-workspace knobs (ws-settings.json "awareness"). Zero
// values are filled from DefaultSettings by Normalized, so a file that predates
// a field still behaves.
type Settings struct {
	// Enabled switches the whole layer. Off = no brief, no pulse, no digest;
	// the per-turn suffix still carries the pinned essentials.
	Enabled bool `json:"enabled"`
	// BriefBudgetBytes bounds the session-start brief (frozen into the prefix).
	BriefBudgetBytes int `json:"briefBudgetBytes"`
	// TurnBudgetBytes bounds the whole volatile per-turn suffix.
	TurnBudgetBytes int `json:"turnBudgetBytes"`
	// PulseBudgetBytes bounds the one-line workspace pulse.
	PulseBudgetBytes int `json:"pulseBudgetBytes"`
	// DigestBudgetBytes bounds the rendered end-of-turn digest.
	DigestBudgetBytes int `json:"digestBudgetBytes"`
	// RecentSessions is how many other sessions the brief lists.
	RecentSessions int `json:"recentSessions"`
	// RecentDigests is how many finished-work digests the brief lists.
	RecentDigests int `json:"recentDigests"`
	// NoteCount is how many notes the brief serves inline.
	NoteCount int `json:"noteCount"`
	// StaleCardDays is how long an in-progress card may sit before it is an
	// open loop.
	StaleCardDays int `json:"staleCardDays"`
}

// DefaultSettings are the shipped values. The brief ceiling is set so that a
// brief plus the rest of a typical static prefix stays well inside what a
// hook-style delivery would cap at; the turn ceiling is generous because the
// coordinator situation block is legitimately large.
func DefaultSettings() Settings {
	return Settings{
		Enabled:           true,
		BriefBudgetBytes:  6144,
		TurnBudgetBytes:   16384,
		PulseBudgetBytes:  320,
		DigestBudgetBytes: 2048,
		RecentSessions:    5,
		RecentDigests:     5,
		NoteCount:         6,
		StaleCardDays:     3,
	}
}

// Normalized fills zero numeric fields from the defaults and clamps the rest
// to sane floors. Enabled is left as stored.
func (s Settings) Normalized() Settings {
	d := DefaultSettings()
	fill := func(v *int, def, floor int) {
		if *v == 0 {
			*v = def
		}
		if *v < floor {
			*v = floor
		}
	}
	fill(&s.BriefBudgetBytes, d.BriefBudgetBytes, 1024)
	fill(&s.TurnBudgetBytes, d.TurnBudgetBytes, 2048)
	fill(&s.PulseBudgetBytes, d.PulseBudgetBytes, 80)
	fill(&s.DigestBudgetBytes, d.DigestBudgetBytes, 512)
	fill(&s.RecentSessions, d.RecentSessions, 0)
	fill(&s.RecentDigests, d.RecentDigests, 0)
	fill(&s.NoteCount, d.NoteCount, 0)
	fill(&s.StaleCardDays, d.StaleCardDays, 1)
	return s
}

// Input is everything a producer may read for one composition. The runtime
// fills it; producers never reach past it.
type Input struct {
	Session db.Session
	Agent   db.Agent
	// WorkspaceID / WorkspaceName / DataDir identify the workspace for the
	// identity line and the workspace card header.
	WorkspaceID   string
	WorkspaceName string
	DataDir       string

	Store Store
	Notes *notes.Store
	// Settings is already Normalized.
	Settings Settings
	Now      time.Time

	// Cwd is the session's effective working directory (project root for the
	// notes reach rule); ProgressDir is where the durable todo file lives.
	Cwd            string
	ProgressDir    string
	ProgressResume bool

	// Fresh marks a session's first turn (the brief is being composed for the
	// first time). IsCoordinator selects the coordinator-only sections.
	Fresh         bool
	IsCoordinator bool

	// Running lists the sessions executing right now (runtime liveness); nil =
	// unknown.
	Running map[string]bool

	// Digests reads the recent finished-work index (set by the service).
	Digests DigestReader

	// RankNotes, when set, re-ranks the candidate notes for the brief (the
	// brief-relevance decision authority). It receives the rule-ranked list and
	// returns the ordered subset to serve, at most limit long. nil = rule order.
	RankNotes func(ctx context.Context, candidates []notes.Note, limit int) []notes.Note
	// JudgeUrgent, when set, decides whether a changed pulse deserves the
	// agent's immediate attention (the pulse-urgency authority). ok=false means
	// no verdict; the rule-based flag stands.
	JudgeUrgent func(ctx context.Context, pulse string) (urgent, ok bool)
	// SuggestNote, when set, decides whether a finished session's digest shows
	// something worth a memory note (the wrap-up-memory authority). ok=false
	// means no verdict; SuggestNoteRule stands.
	SuggestNote func(ctx context.Context, d Digest) (suggest, ok bool)
}

// DigestReader lists recent digests for the brief's "recently finished work".
type DigestReader interface {
	RecentDigests(limit int, excludeSession string) []DigestIndexEntry
}

func (in Input) now() time.Time {
	if in.Now.IsZero() {
		return time.Now()
	}
	return in.Now
}
