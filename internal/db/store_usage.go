package db

import (
	"context"
	"database/sql"
	"errors"
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

// AddUsage increments today's usage counters for an agent (upsert).
func (d *DB) AddUsage(ctx context.Context, agentID string, calls, inputTokens, outputTokens int) error {
	_, err := d.ExecContext(ctx, `INSERT INTO agent_usage
		(agent_id, day, calls, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, day) DO UPDATE SET
			calls = calls + excluded.calls,
			input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens`,
		agentID, today(), calls, inputTokens, outputTokens)
	return err
}

// GetUsageToday returns an agent's usage for the current day (zero-valued if
// nothing has been recorded yet).
func (d *DB) GetUsageToday(ctx context.Context, agentID string) (Usage, error) {
	u := Usage{AgentID: agentID, Day: today()}
	err := d.QueryRowContext(ctx, `SELECT calls, input_tokens, output_tokens
		FROM agent_usage WHERE agent_id = ? AND day = ?`, agentID, u.Day).
		Scan(&u.Calls, &u.InputTokens, &u.OutputTokens)
	if errors.Is(err, sql.ErrNoRows) {
		// No row yet → zero usage is the correct answer, not an error.
		return u, nil
	}
	return u, err
}

// UpdateBudget sets an agent's daily spend caps (0 = unlimited).
func (d *DB) UpdateBudget(ctx context.Context, agentID string, callLimit, tokenLimit int) error {
	res, err := d.ExecContext(ctx, `UPDATE agents
		SET daily_call_limit = ?, daily_token_limit = ?, updated_at = ?
		WHERE id = ?`, callLimit, tokenLimit, now(), agentID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}
