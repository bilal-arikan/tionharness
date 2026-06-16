package db

import (
	"context"
	"time"
)

// Usage is a per-day rollup of an agent's LLM consumption.
type Usage struct {
	AgentID      string `json:"agentId"`
	Day          string `json:"day"`
	Calls        int    `json:"calls"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
}

// today returns the current calendar day as YYYY-MM-DD (server local time).
func today() string { return time.Now().Format("2006-01-02") }

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

// AddUsage increments today's usage counters for an agent (upsert).
func (d *DB) AddUsage(ctx context.Context, agentID string, calls, inputTokens, outputTokens int) error {
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

// UpdateBudget sets an agent's daily spend caps (0 = unlimited).
func (d *DB) UpdateBudget(ctx context.Context, agentID string, callLimit, tokenLimit int) error {
	_, err := d.mutateAgentLocked(agentID, func(a *Agent) {
		a.DailyCallLimit = callLimit
		a.DailyTokenLimit = tokenLimit
	})
	return err
}
