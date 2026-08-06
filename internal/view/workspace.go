package view

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// WorkspaceInput is everything the workspace projection reads. Each slice is the
// same data the per-entity projections use; this one rolls them up rather than
// re-deriving anything.
type WorkspaceInput struct {
	Agents      []db.Agent
	Sessions    []db.Session
	Tasks       []db.Task
	FlowRuns    []db.FlowRun
	Schedules   []db.Schedule
	WaitingAsks []db.SessionAsk
	// TokensToday is the workspace-wide token spend for the current day.
	TokensToday int64
	// CostToday is the workspace-wide USD cost for the current day, priced by
	// billing.RollupOf. CostEstimated is set when any of that spend is priced via
	// an equivalent-API estimate (subscription providers like claude-cli) rather
	// than a real list price — the header then prefixes "~".
	CostToday     float64
	CostEstimated bool
	// Now is the clock used for age computations. Zero means time.Now().
	Now time.Time
}

const (
	// wsActiveWindow is how recently a session must have moved to count as
	// "active". Long enough to survive a coffee break, short enough that a
	// workspace nobody touched today does not look busy.
	wsActiveWindow = 24 * time.Hour
	// wsSignalNames is how many ids a single signal line spells out.
	wsSignalNames = 5
)

// ProjectWorkspace renders the whole workspace: how much is running, how much is
// stuck, and what is asking for attention.
//
// This is the roll-up the dashboard shows and the projection a future supervisor
// agent would read. It re-uses the same L0/L1 discipline as the per-entity
// views: every number is counted in Go, nothing is narrated by a model.
func ProjectWorkspace(in WorkspaceInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindSpace, ID: WorkspaceRefID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%d/%d/%d", len(in.Agents), len(in.Sessions), len(in.Tasks), len(in.FlowRuns)),
	}

	st := workspaceStats(in, now)
	// Cost rides next to the token figure so the reader sees spend in money, not
	// only volume. It is omitted (not shown as "$0.00") when there is no priced
	// spend today: a bare $0.00 next to a non-zero token count would read as "free"
	// when it actually means "this provider has no price", which is a different fact.
	cost := ""
	if in.CostToday > 0 {
		cost = " · " + usd(in.CostToday, in.CostEstimated) + " bugün"
	}
	v.Header = fmt.Sprintf("WORKSPACE · %d ajan · %d oturum (%d aktif) · %d kart · %d koşu · %s tok bugün%s · asOf %s",
		len(in.Agents), len(in.Sessions), st.ActiveSessions, len(in.Tasks),
		len(in.FlowRuns), compactCount(in.TokensToday), cost, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	if len(in.Tasks) > 0 && lens != LensErrors {
		l.add("pano: %s", boardHistogram(boardColumns(in.Tasks)))
	}
	if lens != LensErrors {
		l.add("koşular: %d çalışıyor · %d bekliyor · %d başarısız (son 24s: %d)",
			st.RunsRunning, st.RunsWaiting, st.RunsFailed, st.RunsRecent)
	}

	sigs := workspaceSignals(in, st, now, lens)
	for _, s := range sigs {
		l.add("%s", s)
	}
	if len(sigs) == 0 && lens != LensHealth {
		// A lens that found nothing must say so; a blank body reads like a broken
		// projection rather than a clean bill of health.
		l.add("(bu mercekte dikkat çeken bir şey yok)")
	}

	if level == LevelFull {
		if detail := workspaceDetail(in, st, now); detail != "" {
			l.add("--")
			l.add("%s", detail)
		}
	}
	v.Body = l.String()
	v.Handles = workspaceHandles(st)
	v.finalize()
	return v, nil
}

// wsStats are the counts every part of the projection shares, computed once.
type wsStats struct {
	ActiveSessions  int
	StuckSessions   []db.Session
	CoordinatorRuns int
	RunsRunning     int
	RunsWaiting     int
	RunsFailed      int
	RunsRecent      int
	FailedRuns      []db.FlowRun
	BrokenSchedules []db.Schedule
	StaleCards      []db.Task
	FailedCards     []db.Task
}

// workspaceStats does the L0 counting pass.
func workspaceStats(in WorkspaceInput, now time.Time) wsStats {
	var st wsStats
	activeCutoff := now.Add(-wsActiveWindow).Unix()

	for _, s := range in.Sessions {
		if s.State == "archived" {
			continue
		}
		if s.UpdatedAt >= activeCutoff {
			st.ActiveSessions++
		}
		if s.StuckTurns > 0 {
			st.StuckSessions = append(st.StuckSessions, s)
		}
		if s.IsCoordinator() {
			st.CoordinatorRuns++
		}
	}

	recentCutoff := now.Add(-24 * time.Hour).Unix()
	for _, r := range in.FlowRuns {
		switch r.Status {
		case db.FlowRunning:
			st.RunsRunning++
		case db.FlowWaiting:
			st.RunsWaiting++
		case db.FlowFailure:
			st.RunsFailed++
			st.FailedRuns = append(st.FailedRuns, r)
		}
		if r.CreatedAt >= recentCutoff {
			st.RunsRecent++
		}
	}

	for _, sc := range in.Schedules {
		// Only an ENABLED schedule that last failed is a problem: a disabled one
		// is a deliberate pause, and reporting it as broken trains the reader to
		// ignore the line.
		if sc.Enabled && sc.LastDeliveryStatus == "error" {
			st.BrokenSchedules = append(st.BrokenSchedules, sc)
		}
	}

	st.StaleCards = staleCards(boardColumns(in.Tasks), now)
	st.FailedCards = cardsInColumn(boardColumns(in.Tasks), db.BoardFailed)

	// Newest trouble first, so a truncated signal line names what changed last.
	sort.Slice(st.StuckSessions, func(i, j int) bool {
		return st.StuckSessions[i].UpdatedAt > st.StuckSessions[j].UpdatedAt
	})
	sort.Slice(st.FailedRuns, func(i, j int) bool {
		return st.FailedRuns[i].UpdatedAt > st.FailedRuns[j].UpdatedAt
	})
	return st
}

// workspaceSignals is the L1 layer: what a human (or a supervisor agent) should
// look at first.
func workspaceSignals(in WorkspaceInput, st wsStats, now time.Time, lens Lens) []string {
	var out []string

	if n := len(st.StuckSessions); n > 0 {
		out = append(out, fmt.Sprintf("⚠ %d oturum takılmış (StuckTurns>0): %s",
			n, sessionNames(st.StuckSessions, wsSignalNames)))
	}
	if n := len(in.WaitingAsks); n > 0 {
		if oldest := oldestAsk(in.WaitingAsks); oldest != nil {
			out = append(out, fmt.Sprintf("⏸ %d oturum cevap bekliyor — en eskisi %s'dir: session:%s",
				n, age(tsSec(oldest.CreatedAt), now), oldest.SessionID))
		}
	}
	if n := len(st.FailedRuns); n > 0 {
		out = append(out, fmt.Sprintf("✗ %d başarısız akış koşusu: %s",
			n, runNames(st.FailedRuns, wsSignalNames)))
	}
	if n := len(st.BrokenSchedules); n > 0 {
		out = append(out, fmt.Sprintf("⏰ %d zamanlama son çalışmada hata verdi", n))
	}
	if n := len(st.FailedCards); n > 0 {
		out = append(out, fmt.Sprintf("✗ %d başarısız kart: %s", n, namesOf(st.FailedCards, wsSignalNames)))
	}
	if lens == LensErrors {
		return out
	}

	if n := len(st.StaleCards); n > 0 {
		out = append(out, fmt.Sprintf("⚠ %d kart >%dg çalışan sütunda hareketsiz: %s",
			n, boardStaleDays, namesOf(st.StaleCards, wsSignalNames)))
	}
	if lens == LensStale {
		return out
	}

	if st.RunsWaiting > 0 {
		out = append(out, fmt.Sprintf("⏸ %d akış koşusu girdi bekliyor (await-input)", st.RunsWaiting))
	}
	if st.CoordinatorRuns > 0 {
		out = append(out, fmt.Sprintf("⇵ %d koordinatör oturumu", st.CoordinatorRuns))
	}
	return out
}

// workspaceDetail is the LevelFull tail: the specific things the signal lines
// only counted.
func workspaceDetail(in WorkspaceInput, st wsStats, now time.Time) string {
	var l lines
	for _, s := range st.StuckSessions {
		l.add("stuck   session:%-10s %-40s %s önce", s.ID, clip(s.Title, 40), age(tsSec(s.UpdatedAt), now))
	}
	for _, r := range st.FailedRuns {
		l.add("failed  run:%-14s %s", r.ID, clip(r.Error, 90))
	}
	for _, sc := range st.BrokenSchedules {
		l.add("sched   %-14s %s", sc.ID, clip(sc.LastDeliveryError, 90))
	}
	return l.String()
}

// workspaceHandles points at the sub-projections worth opening next.
func workspaceHandles(st wsStats) []Handle {
	var hs []Handle
	if len(st.StaleCards) > 0 || len(st.FailedCards) > 0 {
		hs = append(hs, Handle{
			Label: "pano detayı",
			Ref:   Ref{Kind: KindBoard, ID: BoardRefID},
			Level: LevelCard,
		})
	}
	if len(st.FailedRuns) > 0 {
		hs = append(hs, Handle{
			Label: "başarısız koşu: " + st.FailedRuns[0].ID,
			Ref:   Ref{Kind: KindFlowRun, ID: st.FailedRuns[0].ID},
			Level: LevelCard,
		})
	}
	if len(st.StuckSessions) > 0 {
		hs = append(hs, Handle{
			Label: "takılmış oturum: " + st.StuckSessions[0].ID,
			Ref:   Ref{Kind: KindSession, ID: st.StuckSessions[0].ID},
			Level: LevelCard,
		})
	}
	return hs
}

// oldestAsk returns the longest-pending question, or nil when there are none.
// The oldest one is what matters: it bounds how long the workspace has been
// waiting on a human.
func oldestAsk(asks []db.SessionAsk) *db.SessionAsk {
	var oldest *db.SessionAsk
	for i := range asks {
		if oldest == nil || asks[i].CreatedAt < oldest.CreatedAt {
			oldest = &asks[i]
		}
	}
	return oldest
}

// sessionNames lists up to max session ids, reporting the remainder as a count.
func sessionNames(ss []db.Session, max int) string {
	ids := make([]string, 0, max)
	for i, s := range ss {
		if i >= max {
			return strings.Join(ids, ", ") + fmt.Sprintf(" +%d", len(ss)-max)
		}
		ids = append(ids, s.ID)
	}
	return strings.Join(ids, ", ")
}

// runNames lists up to max flow-run ids, reporting the remainder as a count.
func runNames(rs []db.FlowRun, max int) string {
	ids := make([]string, 0, max)
	for i, r := range rs {
		if i >= max {
			return strings.Join(ids, ", ") + fmt.Sprintf(" +%d", len(rs)-max)
		}
		ids = append(ids, r.ID)
	}
	return strings.Join(ids, ", ")
}
