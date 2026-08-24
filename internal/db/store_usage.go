package db

import (
	"context"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Usage kinds tag every LLM call by its origin so spend can be attributed
// (chat vs. autonomous schedule vs. compaction vs. titling, …). They are the
// single source of truth for the call taxonomy: the agent package's CallKind is
// defined in terms of these strings. UsageKindOther is the fallback for calls
// recorded without an explicit kind.
const (
	UsageKindChat     = "chat"
	UsageKindTask     = "task"
	UsageKindSchedule = "schedule"
	UsageKindFlow     = "flow"
	UsageKindDelegate = "delegate"
	UsageKindSpawn    = "spawned"
	UsageKindSubagent = "subagent"
	UsageKindTitle    = "title"
	UsageKindSummary  = "summary"
	UsageKindReflect  = "reflect"
	UsageKindCompact  = "compact"
	UsageKindBtw      = "btw"
	UsageKindOther    = "other"
)

// KindStat is the per-kind/per-model slice of an agent's daily consumption.
// Cache counters are tracked separately from InputTokens so cost can apply the
// cheaper cache-read / pricier cache-write tiers.
type KindStat struct {
	Calls              int `json:"calls"`
	InputTokens        int `json:"inputTokens"`
	OutputTokens       int `json:"outputTokens"`
	CacheReadTokens    int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens   int `json:"cacheWriteTokens,omitempty"`
	CacheWrite5mTokens int `json:"cacheWrite5mTokens,omitempty"`
	CacheWrite1hTokens int `json:"cacheWrite1hTokens,omitempty"`
}

// UsageDelta is one call's consumption, recorded via AddUsageKind. Bundling the
// counters keeps the recording signature stable as new token classes are added.
type UsageDelta struct {
	Calls              int
	InputTokens        int
	OutputTokens       int
	CacheReadTokens    int
	CacheWriteTokens   int
	CacheWrite5mTokens int
	CacheWrite1hTokens int
	// ProviderCalls is how many underlying model API round-trips this recorded call
	// represents. 0 means "unknown" → folded as Calls (one round-trip). For claude-cli
	// it is the CLI's internal tool-loop turn count (result num_turns), so the rollup
	// can recover the per-call (single-pass) context from the cumulative token totals.
	ProviderCalls int
}

// DeltaFromUsage builds a one-call UsageDelta from a provider's reported usage.
// It is the single mapping point between providers.Usage and the stored counters,
// so every recorder (RecordUsage, compaction) stays in sync as new token classes
// are added. calls is normally 1.
func DeltaFromUsage(calls int, u providers.Usage) UsageDelta {
	return UsageDelta{
		Calls:              calls,
		InputTokens:        u.InputTokens,
		OutputTokens:       u.OutputTokens,
		CacheReadTokens:    u.CacheReadTokens,
		CacheWriteTokens:   u.CacheWriteTokens,
		CacheWrite5mTokens: u.CacheWrite5mTokens,
		CacheWrite1hTokens: u.CacheWrite1hTokens,
	}
}

// providerCallsOf returns how many underlying API round-trips a delta represents,
// flooring an unknown (0) ProviderCalls to the recorded call count so a recorder
// that doesn't report it (native single calls, compaction) still counts as one
// round-trip rather than zero.
func providerCallsOf(d UsageDelta) int {
	if d.ProviderCalls > 0 {
		return d.ProviderCalls
	}
	return d.Calls
}

// add folds a delta into a KindStat.
func (k *KindStat) add(d UsageDelta) {
	k.Calls += d.Calls
	k.InputTokens += d.InputTokens
	k.OutputTokens += d.OutputTokens
	k.CacheReadTokens += d.CacheReadTokens
	k.CacheWriteTokens += d.CacheWriteTokens
	k.CacheWrite5mTokens += d.CacheWrite5mTokens
	k.CacheWrite1hTokens += d.CacheWrite1hTokens
}

// Usage is a per-day rollup of an agent's LLM consumption. The top-level
// counters are the grand total; ByKind breaks the same totals down by call
// origin (chat/task/compact/…) and ByModel by the provider+model that served
// the call, so the budget screen can attribute both where the spend went and
// what it would cost. ByModel is keyed by "<provider>|<model>" (model may be
// empty, e.g. claude-cli's session default).
type Usage struct {
	AgentID            string `json:"agentId"`
	Day                string `json:"day"`
	Calls              int    `json:"calls"`
	InputTokens        int    `json:"inputTokens"`
	OutputTokens       int    `json:"outputTokens"`
	CacheReadTokens    int    `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens   int    `json:"cacheWriteTokens,omitempty"`
	CacheWrite5mTokens int    `json:"cacheWrite5mTokens,omitempty"`
	CacheWrite1hTokens int    `json:"cacheWrite1hTokens,omitempty"`
	// ProviderCalls is the cumulative number of underlying model API round-trips
	// behind Calls (for claude-cli a single TionHarness turn is several internal calls,
	// reported via result num_turns). Lets a consumer divide the cumulative token
	// totals by it to recover per-call figures.
	ProviderCalls int                 `json:"providerCalls,omitempty"`
	ByKind        map[string]KindStat `json:"byKind,omitempty"`
	ByModel       map[string]KindStat `json:"byModel,omitempty"`
	// CoolingWasteUSD is the day's summed AVOIDABLE prompt-cache overpay: warm
	// prefixes that cooled (TTL expiry / server eviction) before the next turn and
	// had to be re-written. It is NOT added to the token cost (those write tokens are
	// already in ByModel and priced) — it isolates the portion a timely reply would
	// have saved. CoolingWasteEstimated latches true when any contribution is a
	// subscription equivalent-API estimate (e.g. claude-cli). Recorded out-of-band
	// via AddCoolingWaste from the cache-break detector, not through AddUsageKind.
	CoolingWasteUSD       float64 `json:"coolingWasteUsd,omitempty"`
	CoolingWasteEstimated bool    `json:"coolingWasteEstimated,omitempty"`
}

// ModelKey builds the ByModel map key from a provider and model id.
func ModelKey(provider, model string) string { return provider + "|" + model }

// today returns the current calendar day as YYYY-MM-DD (server local time).
func today() string { return time.Now().Format("2006-01-02") }

// Today exposes the server's current calendar day (YYYY-MM-DD) so callers
// outside the package (e.g. the budget API) label "today" consistently with how
// usage is keyed.
func Today() string { return today() }

func usageKey(agentID, day string) string { return agentID + "|" + day }

func usageFile(agentID, day string) string { return agentID + "__" + day + ".json" }

func (d *DB) loadUsage() error {
	rows, err := loadJSONDir[Usage](d.dir(dirUsage))
	if err != nil {
		return err
	}
	for _, u := range rows {
		d.usage[usageKey(u.AgentID, u.Day)] = u
	}
	return nil
}

func (d *DB) persistUsageLocked(u Usage) error {
	d.usage[usageKey(u.AgentID, u.Day)] = u
	return atomicWriteJSON(d.dir(dirUsage, usageFile(u.AgentID, u.Day)), u)
}

// AddUsage increments today's usage counters for an agent (upsert), bucketing
// the call under UsageKindOther with no provider/model. Prefer AddUsageKind so
// spend is attributed by origin and model.
func (d *DB) AddUsage(ctx context.Context, agentID string, calls, inputTokens, outputTokens int) error {
	return d.AddUsageKind(ctx, agentID, UsageKindOther, "", "", UsageDelta{
		Calls: calls, InputTokens: inputTokens, OutputTokens: outputTokens,
	})
}

// AddUsageKind increments today's usage counters for an agent (upsert) plus the
// matching per-origin (ByKind) and per-model (ByModel) sub-counters, so the same
// totals are broken down both by where the call came from and by which
// provider+model served it (for cost, including cache tiers). An empty kind is
// recorded as UsageKindOther; an empty provider skips the model breakdown.
func (d *DB) AddUsageKind(ctx context.Context, agentID, kind, provider, model string, delta UsageDelta) error {
	if kind == "" {
		kind = UsageKindOther
	}
	day := today()
	d.usageMu.Lock()
	defer d.usageMu.Unlock()
	u, ok := d.usage[usageKey(agentID, day)]
	if !ok {
		u = Usage{AgentID: agentID, Day: day}
	}
	u.Calls += delta.Calls
	u.InputTokens += delta.InputTokens
	u.OutputTokens += delta.OutputTokens
	u.CacheReadTokens += delta.CacheReadTokens
	u.CacheWriteTokens += delta.CacheWriteTokens
	u.CacheWrite5mTokens += delta.CacheWrite5mTokens
	u.CacheWrite1hTokens += delta.CacheWrite1hTokens
	u.ProviderCalls += providerCallsOf(delta)
	if u.ByKind == nil {
		u.ByKind = map[string]KindStat{}
	}
	k := u.ByKind[kind]
	k.add(delta)
	u.ByKind[kind] = k
	if provider != "" {
		if u.ByModel == nil {
			u.ByModel = map[string]KindStat{}
		}
		mk := ModelKey(provider, model)
		m := u.ByModel[mk]
		m.add(delta)
		u.ByModel[mk] = m
	}
	return d.persistUsageLocked(u)
}

// AddCoolingWaste accumulates one detected cache-cooling overpay (USD) into an
// agent's usage row for today (upsert), latching the estimated flag. It is the
// out-of-band companion to AddUsageKind for the isolated "cooling waste" figure —
// the token cost is already recorded through the normal usage path; this only
// tracks the derived avoidable-overpay penalty so the budget screen can surface a
// workspace/window rollup. A non-positive amount is a no-op.
func (d *DB) AddCoolingWaste(ctx context.Context, agentID string, usd float64, estimated bool) error {
	if usd <= 0 {
		return nil
	}
	day := today()
	d.usageMu.Lock()
	defer d.usageMu.Unlock()
	u, ok := d.usage[usageKey(agentID, day)]
	if !ok {
		u = Usage{AgentID: agentID, Day: day}
	}
	u.CoolingWasteUSD += usd
	if estimated {
		u.CoolingWasteEstimated = true
	}
	return d.persistUsageLocked(u)
}

// WorkspaceTokensToday returns the workspace-wide total token count for the
// current day: input+output+cacheRead+cacheWrite summed across every agent's
// usage row for today. It is the scope total a workspace-scoped token automation
// watches. Held under the same usage lock as the recorders so a reader never
// observes a torn per-agent row.
func (d *DB) WorkspaceTokensToday(ctx context.Context) int64 {
	day := today()
	d.usageMu.RLock()
	defer d.usageMu.RUnlock()
	var total int64
	for _, u := range d.usage {
		if u.Day == day {
			total += int64(u.InputTokens) + int64(u.OutputTokens) +
				int64(u.CacheReadTokens) + int64(u.CacheWriteTokens)
		}
	}
	return total
}

// GetUsageToday returns an agent's usage for the current day (zero-valued if
// nothing has been recorded yet).
func (d *DB) GetUsageToday(ctx context.Context, agentID string) (Usage, error) {
	day := today()
	d.usageMu.RLock()
	defer d.usageMu.RUnlock()
	if u, ok := d.usage[usageKey(agentID, day)]; ok {
		return u, nil
	}
	return Usage{AgentID: agentID, Day: day}, nil
}

// UsageForDay returns every agent's usage row for the given day (YYYY-MM-DD),
// for the workspace-wide budget screen. Agents with no activity that day are
// simply absent.
func (d *DB) UsageForDay(ctx context.Context, day string) ([]Usage, error) {
	d.usageMu.RLock()
	defer d.usageMu.RUnlock()
	var out []Usage
	for _, u := range d.usage {
		if u.Day == day {
			out = append(out, u)
		}
	}
	return out, nil
}

// UsageHistory returns every usage row on or after sinceDay (YYYY-MM-DD),
// across all agents, for trend charts. Lexical comparison works because the day
// format is zero-padded ISO.
func (d *DB) UsageHistory(ctx context.Context, sinceDay string) ([]Usage, error) {
	d.usageMu.RLock()
	defer d.usageMu.RUnlock()
	var out []Usage
	for _, u := range d.usage {
		if u.Day >= sinceDay {
			out = append(out, u)
		}
	}
	return out, nil
}
