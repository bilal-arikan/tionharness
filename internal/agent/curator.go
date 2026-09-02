package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// Curator (Rota F3, brief §7.4–7.5): the weekly, idle-triggered, LLM-free
// pass that keeps the automation surface from growing monotonically. It never
// deletes — it archives — and only touches what an agent or a recipe created;
// anything user-made becomes a suggestion. Pinned entities are exempt from
// every automatic action.
//
// Rules (deterministic, all from stored state):
//   - automation exhausted (iteration cap reached) or expired → archive
//     (agent-made) / suggest (user-made);
//   - one-shot schedule already fired, or expired schedule → archive / suggest;
//   - hook that never fired in curatorHookQuietDays → suggest;
//   - recipe watcher unfired in every one of ≥ curatorMinRuns summarized runs
//     → suggest "unbind from the recipe";
//   - declared phase never reached in every one of ≥ curatorMinRuns runs →
//     suggest "make optional / drop".

const (
	curatorInterval      = 7 * 24 * time.Hour
	curatorCheckEvery    = time.Hour
	curatorHookQuietDays = 30
	curatorMinRuns       = 3
)

// Curator reasons.
const (
	CuratorReasonExhausted      = "exhausted"
	CuratorReasonExpired        = "expired"
	CuratorReasonOneShotDone    = "one_shot_done"
	CuratorReasonNeverFired     = "never_fired"
	CuratorReasonUnfiredWatcher = "unfired_watcher"
	CuratorReasonGhostPhase     = "ghost_phase"
)

// agentMade reports whether an entity was created by an agent / automation /
// seed — the provenance the curator may act on by itself.
func agentMade(createdBy string) bool {
	cb := strings.TrimSpace(createdBy)
	return cb != "" && cb != "user"
}

// RunCurator performs one pass. apply=false only proposes (every action
// becomes a suggestion); the report is saved either way.
func (r *Runtime) RunCurator(ctx context.Context, trigger string, apply bool) (db.CuratorReport, error) {
	now := time.Now().Unix()
	rep := db.CuratorReport{At: now, Trigger: trigger, Idle: r.workspaceIdle(ctx), Actions: []db.CuratorAction{}}
	add := func(a db.CuratorAction) {
		if a.Applied {
			rep.Archived++
		} else {
			rep.Suggestions++
		}
		rep.Actions = append(rep.Actions, a)
	}
	archiveOrSuggest := func(entity, id, name, createdBy, reason, detail string, doArchive func() error) {
		a := db.CuratorAction{Kind: db.CuratorActionSuggest, Entity: entity, ID: id, Name: name, Reason: reason, Detail: detail}
		if apply && agentMade(createdBy) {
			if err := doArchive(); err != nil {
				r.logger.Warn("curator: archive failed", "entity", entity, "id", id, "error", err)
			} else {
				a.Kind, a.Applied = db.CuratorActionArchive, true
			}
		}
		add(a)
	}

	if autos, err := r.db.ListAutomations(ctx); err == nil {
		for _, a := range autos {
			if a.Archived || a.Pinned {
				continue
			}
			switch {
			case a.MaxIterations > 0 && a.IterationCount >= a.MaxIterations:
				archiveOrSuggest(db.CuratorEntityAutomation, a.ID, automationLabel(a), a.CreatedBy, CuratorReasonExhausted,
					fmt.Sprintf("iterasyon tavanına ulaştı (%d/%d)", a.IterationCount, a.MaxIterations),
					func() error { return r.db.SetAutomationArchived(ctx, a.ID, true) })
			case a.ExpiresAt > 0 && now >= a.ExpiresAt:
				archiveOrSuggest(db.CuratorEntityAutomation, a.ID, automationLabel(a), a.CreatedBy, CuratorReasonExpired,
					"son tarihi geçti", func() error { return r.db.SetAutomationArchived(ctx, a.ID, true) })
			}
		}
	}
	if scs, err := r.db.ListSchedules(ctx); err == nil {
		for _, sc := range scs {
			if sc.Archived || sc.Pinned {
				continue
			}
			name := sc.Name
			if name == "" {
				name = sc.ID
			}
			switch {
			case sc.OneShot && sc.LastRunAt > 0 && !sc.Enabled:
				archiveOrSuggest(db.CuratorEntitySchedule, sc.ID, name, sc.CreatedBy, CuratorReasonOneShotDone,
					"tek seferlik zamanlama çalıştı", func() error { return r.db.SetScheduleArchived(ctx, sc.ID, true) })
			case sc.ExpiresAt > 0 && now >= sc.ExpiresAt:
				archiveOrSuggest(db.CuratorEntitySchedule, sc.ID, name, sc.CreatedBy, CuratorReasonExpired,
					"son tarihi geçti", func() error { return r.db.SetScheduleArchived(ctx, sc.ID, true) })
			}
		}
	}
	if hooks, err := r.db.ListHooks(ctx); err == nil {
		quiet := now - curatorHookQuietDays*24*3600
		for _, h := range hooks {
			if h.Archived || h.Pinned || h.FireCount > 0 || h.CreatedAt == 0 || h.CreatedAt > quiet {
				continue
			}
			name := strings.TrimSpace(h.Event + " " + h.Matcher)
			if strings.TrimSpace(h.Matcher) == "" {
				name = h.Event + " *"
			}
			add(db.CuratorAction{Kind: db.CuratorActionSuggest, Entity: db.CuratorEntityHook, ID: h.ID, Name: name,
				Reason: CuratorReasonNeverFired, Detail: fmt.Sprintf("%d gündür hiç tetiklenmedi", curatorHookQuietDays)})
		}
	}
	// Recipes: from the trajectory index summaries.
	for _, st := range RecipeStatsFromIndex(r.db.ListTrajectories(ctx, db.TrajectoryFilter{})) {
		if st.TemplateRef == "" || st.Summarized < curatorMinRuns {
			continue
		}
		evidence := fmt.Sprintf("%d özetlenmiş koşu, son %s", st.Summarized, st.LatestID)
		for _, w := range sortedKeys(st.UnfiredWatchers) {
			if st.UnfiredWatchers[w] == st.Summarized {
				add(db.CuratorAction{Kind: db.CuratorActionSuggest, Entity: db.CuratorEntityRecipe, ID: st.Slug, Name: st.TemplateRef,
					Reason: CuratorReasonUnfiredWatcher, Detail: fmt.Sprintf("izleyici %q hiçbir koşuda ateşlenmedi — reçeteden çöz", w), Evidence: evidence})
			}
		}
		for _, p := range sortedKeys(st.GhostPhases) {
			if st.GhostPhases[p] == st.Summarized {
				add(db.CuratorAction{Kind: db.CuratorActionSuggest, Entity: db.CuratorEntityRecipe, ID: st.Slug, Name: st.TemplateRef,
					Reason: CuratorReasonGhostPhase, Detail: fmt.Sprintf("faz %q hiçbir koşuda başlamadı — optional yap ya da kaldır", p), Evidence: evidence})
			}
		}
	}
	if err := r.db.SaveCuratorReport(ctx, rep); err != nil {
		return rep, err
	}
	if rep.Archived > 0 || rep.Suggestions > 0 {
		r.publish(events.Event{
			Type: events.TypeAutomation, Level: "info",
			Title:  fmt.Sprintf("🧹 Küratör geçti — %d arşivlendi, %d öneri", rep.Archived, rep.Suggestions),
			Body:   "Ayrıntı: Otomasyon ▸ Küratör",
			Target: map[string]string{"view": "schedules"},
		})
	}
	r.logger.Info("curator pass", "trigger", trigger, "apply", apply, "archived", rep.Archived, "suggestions", rep.Suggestions, "idle", rep.Idle)
	return rep, nil
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// workspaceIdle reports whether no session is running or waiting right now.
func (r *Runtime) workspaceIdle(ctx context.Context) bool {
	defer func() { _ = recover() }() // a bare test runtime has no liveness sources
	return len(r.Liveness(ctx).Entries) == 0
}

// StartCuratorSweeper runs the weekly pass: every hour it checks whether the
// last report is older than curatorInterval and the workspace is idle, then
// runs with apply=true. Manual runs (RunCurator via the API) reset the clock.
func (r *Runtime) StartCuratorSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(curatorCheckEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.curatorTick(ctx, time.Now())
			}
		}
	}()
}

// curatorTick is one sweeper check, split out so it is testable with a clock.
func (r *Runtime) curatorTick(ctx context.Context, now time.Time) bool {
	last, ok, err := r.db.GetCuratorReport(ctx)
	if err != nil {
		return false
	}
	if ok && now.Unix()-last.At < int64(curatorInterval.Seconds()) {
		return false
	}
	if !r.workspaceIdle(ctx) {
		return false
	}
	if _, err := r.RunCurator(ctx, "weekly", true); err != nil {
		r.logger.Warn("curator: weekly pass failed", "error", err)
		return false
	}
	return true
}
