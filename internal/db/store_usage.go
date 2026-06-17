package db

import (
	"context"
	"time"
)

// Usage kinds tag every LLM call by its origin so spend can be attributed
// (chat vs. autonomous heartbeat vs. compaction vs. titling, …). They are the
// single source of truth for the call taxonomy: the agent package's CallKind is
// defined in terms of these strings. UsageKindOther is the fallback for calls
// recorded without an explicit kind.
const (
	UsageKindChat      = "chat"
	UsageKindTask      = "task"
	UsageKindSchedule  = "schedule"
	UsageKindFlow      = "flow"
	UsageKindHeartbeat = "heartbeat"
	UsageKindDelegate  = "delegate"
	UsageKindTitle     = "title"
	UsageKindSummary   = "summary"
	UsageKindReflect   = "reflect"
	UsageKindCompact   = "compact"
	UsageKindOther     = "other"
)

// KindStat is the per-kind slice of an agent's daily consumption.
type KindStat struct {
	Calls        int `json:"calls"`
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// Usage is a per-day rollup of an agent's LLM consumption. The top-level
// counters are the grand total; ByKind breaks the same totals down by call
// origin (chat/task/compact/…) and ByModel by the provider+model that served
// the call, so the budget screen can attribute both where the spend went and
// what it would cost. ByModel is keyed by "<provider>|<model>" (model may be
// empty, e.g. claude-cli's session default).
type Usage struct {
	AgentID      string              `json:"agentId"`
	Day          string              `json:"day"`
	Calls        int                 `json:"calls"`
	InputTokens  int                 `json:"inputTokens"`
	OutputTokens int                 `json:"outputTokens"`
	ByKind       map[string]KindStat `json:"byKind,omitempty"`
	ByModel      map[string]KindStat `json:"byModel,omitempty"`
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
	return d.AddUsageKind(ctx, agentID, UsageKindOther, "", "", calls, inputTokens, outputTokens)
}

// AddUsageKind increments today's usage counters for an agent (upsert) plus the
// matching per-origin (ByKind) and per-model (ByModel) sub-counters, so the same
// totals are broken down both by where the call came from and by which
// provider+model served it (for cost). An empty kind is recorded as
// UsageKindOther; an empty provider skips the model breakdown.
func (d *DB) AddUsageKind(ctx context.Context, agentID, kind, provider, model string, calls, inputTokens, outputTokens int) error {
	if kind == "" {
		kind = UsageKindOther
	}
	day := today()
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.usage[usageKey(agentID, day)]
	if !ok {
		u = Usage{AgentID: agentID, Day: day}
	}
	u.Calls += calls
	u.InputTokens += inputTokens
	u.OutputTokens += outputTokens
	if u.ByKind == nil {
		u.ByKind = map[string]KindStat{}
	}
	k := u.ByKind[kind]
	k.Calls += calls
	k.InputTokens += inputTokens
	k.OutputTokens += outputTokens
	u.ByKind[kind] = k
	if provider != "" {
		if u.ByModel == nil {
			u.ByModel = map[string]KindStat{}
		}
		mk := ModelKey(provider, model)
		m := u.ByModel[mk]
		m.Calls += calls
		m.InputTokens += inputTokens
		m.OutputTokens += outputTokens
		u.ByModel[mk] = m
	}
	return d.persistUsageLocked(u)
}

// GetUsageToday returns an agent's usage for the current day (zero-valued if
// nothing has been recorded yet).
func (d *DB) GetUsageToday(ctx context.Context, agentID string) (Usage, error) {
	day := today()
	d.mu.RLock()
	defer d.mu.RUnlock()
	if u, ok := d.usage[usageKey(agentID, day)]; ok {
		return u, nil
	}
	return Usage{AgentID: agentID, Day: day}, nil
}

// UsageForDay returns every agent's usage row for the given day (YYYY-MM-DD),
// for the workspace-wide budget screen. Agents with no activity that day are
// simply absent.
func (d *DB) UsageForDay(ctx context.Context, day string) ([]Usage, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
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
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []Usage
	for _, u := range d.usage {
		if u.Day >= sinceDay {
			out = append(out, u)
		}
	}
	return out, nil
}

// UpdateBudget sets an agent's daily spend caps (0 = unlimited).
func (d *DB) UpdateBudget(ctx context.Context, agentID string, callLimit, tokenLimit int) error {
	_, err := d.mutateAgentLocked(agentID, func(a *Agent) {
		a.DailyCallLimit = callLimit
		a.DailyTokenLimit = tokenLimit
	})
	return err
}
