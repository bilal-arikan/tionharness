package db

import "context"

// LegacyThinkingLevelFor returns the reasoning tier an agent row with an EMPTY
// ThinkingLevel actually behaved as, given its provider kind. It is the whole
// migration rule, kept in one place so the boot backfill and CreateAgent cannot
// drift apart:
//
//   - The CLI providers mapped "" onto Claude Code's effortLevel "high"
//     (climcp.EffortLevel), so "high" is what preserves their behaviour.
//   - Every other provider mapped "" onto a thinking budget of 0
//     (agent.thinkingBudgetForLevel), i.e. reasoning off.
//
// An empty provider kind is treated as claude-cli, matching the read-time
// backfill in models.go (a row predating the provider field is a CLI row).
func LegacyThinkingLevelFor(providerKind string) string {
	switch providerKind {
	case "", "claude-cli", "codex-cli":
		return "high"
	default:
		return "off"
	}
}

// BackfillThinkingLevels fills in every agent row that still stores an empty
// ThinkingLevel, using LegacyThinkingLevelFor so no agent changes behaviour.
// Rows that already carry a level — including "off" — are never touched, which
// makes the pass idempotent: the second boot writes nothing. Returns how many
// rows were rewritten.
//
// Deleted rows are migrated too: they stay readable (a session outlives its
// agent) and leaving an invalid value behind would only defer the problem.
func (d *DB) BackfillThinkingLevels(ctx context.Context) (int, error) {
	type pending struct{ id, level string }
	var todo []pending
	d.mu.RLock()
	for id, a := range d.agents {
		if a.ThinkingLevel != "" {
			continue
		}
		todo = append(todo, pending{id: id, level: LegacyThinkingLevelFor(a.Provider)})
	}
	d.mu.RUnlock()

	for _, p := range todo {
		level := p.level
		if _, err := d.mutateAgentLocked(p.id, func(a *Agent) { a.ThinkingLevel = level }); err != nil {
			return 0, err
		}
	}
	return len(todo), nil
}
