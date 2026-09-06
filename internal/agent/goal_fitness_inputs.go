package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/billing"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/goals"
)

// FitnessInputs reads every telemetry source goals.Evaluate consumes: sessions
// with their snapshot stamp, trajectories, priced session usage, board cards,
// automation fires, human asks and the snapshots in play. Costs are priced
// here (billing) so the goals package stays a leaf. Shared by the fitness API
// and the evolver.
func (r *Runtime) FitnessInputs(ctx context.Context, now, since int64) goals.FitnessInputs {
	in := goals.FitnessInputs{Now: now, Since: since, Usage: map[string]goals.UsageRow{}, Snapshots: map[string]goals.ConfigSnapshot{}}
	in.CurrentHash = r.CurrentSnapshotHash()
	sessions, _ := r.db.ListSessions(ctx, "")
	hashes := map[string]bool{}
	for _, s := range sessions {
		in.Sessions = append(in.Sessions, goals.SessionRow{
			ID: s.ID, RootID: s.RootCoordinator(), AgentID: s.AgentID, SnapshotHash: s.SnapshotHash,
			RunState: s.RunState, Kind: s.Kind, Tags: s.Tags, CreatedAt: s.CreatedAt, StuckTurns: s.StuckTurns,
		})
		if s.SnapshotHash != "" {
			hashes[s.SnapshotHash] = true
		}
	}
	if in.CurrentHash != "" {
		hashes[in.CurrentHash] = true
	}
	for h := range hashes {
		var snap goals.ConfigSnapshot
		if err := r.db.GetSnapshot(ctx, h, &snap); err == nil {
			in.Snapshots[h] = snap
		}
	}
	in.Trajectories = r.db.ListTrajectories(ctx, db.TrajectoryFilter{})
	for id, u := range r.db.ListSessionUsage(ctx) {
		roll := billing.RollupOf(u.ByModel)
		in.Usage[id] = goals.UsageRow{
			Tokens: u.TotalTokens(), InputTokens: int64(u.InputTokens), CacheRead: int64(u.CacheReadTokens),
			CacheWrite: int64(u.CacheWriteTokens), CostUSD: roll.CostUSD, Priced: roll.Priced,
		}
	}
	if tasks, err := r.db.ListTasks(ctx); err == nil {
		for _, t := range tasks {
			in.Tasks = append(in.Tasks, goals.TaskRow{ID: t.ID, BoardState: t.BoardState, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, SessionIDs: t.SessionIDs})
		}
	}
	if autos, err := r.db.ListAutomations(ctx); err == nil {
		for _, a := range autos {
			recs, err := r.db.ListAutomationFires(ctx, a.ID, 0)
			if err != nil {
				continue
			}
			for _, f := range recs {
				in.Fires = append(in.Fires, goals.FireRow{AutomationID: a.ID, At: f.At, SessionID: f.SessionID, Failed: f.Error != "" || f.Outcome == "error" || f.Outcome == "failed"})
			}
		}
	}
	for _, a := range r.db.ListSessionAsks(ctx) {
		in.Asks = append(in.Asks, goals.AskRow{SessionID: a.SessionID})
	}
	return in
}
