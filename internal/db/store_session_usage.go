package db

import (
	"context"
)

// SessionUsage is a per-session lifetime rollup of LLM consumption — the same
// shape as the per-agent/day Usage rollup, but keyed by session id and never
// reset by day, so the budget screen can attribute spend to a single
// conversation across its whole life. AgentID records the
// session's owning agent at the time spend was recorded (sessions are
// single-agent). ByKind breaks the total down by call origin and ByModel by the
// provider+model that served the call (for cost, including cache tiers).
type SessionUsage struct {
	SessionID        string `json:"sessionId"`
	AgentID          string `json:"agentId"`
	Calls            int    `json:"calls"`
	InputTokens      int    `json:"inputTokens"`
	OutputTokens     int    `json:"outputTokens"`
	CacheReadTokens  int    `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int    `json:"cacheWriteTokens,omitempty"`
	// ProviderCalls is the cumulative number of underlying model API round-trips
	// behind Calls over this session's life (for claude-cli one TionSwarm turn is
	// several internal calls — result num_turns). The CLI-overhead preview divides
	// the cumulative token totals by it to recover the per-call (single-pass) context
	// when the per-turn debug journal is unavailable.
	ProviderCalls int                 `json:"providerCalls,omitempty"`
	ByKind        map[string]KindStat `json:"byKind,omitempty"`
	ByModel       map[string]KindStat `json:"byModel,omitempty"`
}

func sessionUsageFile(sessionID string) string { return sessionID + ".json" }

func (d *DB) loadSessionUsage() error {
	rows, err := loadJSONDir[SessionUsage](d.dir(dirSessionUsage))
	if err != nil {
		return err
	}
	for _, u := range rows {
		d.sessionUsage[u.SessionID] = u
	}
	return nil
}

func (d *DB) persistSessionUsageLocked(u SessionUsage) error {
	d.sessionUsage[u.SessionID] = u
	return atomicWriteJSON(d.dir(dirSessionUsage, sessionUsageFile(u.SessionID)), u)
}

// AddSessionUsageKind folds one call's consumption into a session's lifetime
// rollup (upsert), plus the matching per-origin (ByKind) and per-model (ByModel)
// sub-counters — the session-scoped analog of AddUsageKind. A blank sessionID is
// a no-op (the call simply isn't attributed to a session). An empty kind is
// recorded as UsageKindOther; an empty provider skips the model breakdown.
func (d *DB) AddSessionUsageKind(ctx context.Context, sessionID, agentID, kind, provider, model string, delta UsageDelta) error {
	if sessionID == "" {
		return nil
	}
	if kind == "" {
		kind = UsageKindOther
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.sessionUsage[sessionID]
	if !ok {
		u = SessionUsage{SessionID: sessionID, AgentID: agentID}
	}
	if agentID != "" {
		u.AgentID = agentID
	}
	u.Calls += delta.Calls
	u.InputTokens += delta.InputTokens
	u.OutputTokens += delta.OutputTokens
	u.CacheReadTokens += delta.CacheReadTokens
	u.CacheWriteTokens += delta.CacheWriteTokens
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
	return d.persistSessionUsageLocked(u)
}

// GetSessionUsage returns a session's lifetime usage rollup (zero-valued if
// nothing has been recorded for it yet).
func (d *DB) GetSessionUsage(ctx context.Context, sessionID string) (SessionUsage, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if u, ok := d.sessionUsage[sessionID]; ok {
		return u, nil
	}
	return SessionUsage{SessionID: sessionID}, nil
}
