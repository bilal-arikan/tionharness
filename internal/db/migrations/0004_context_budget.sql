-- 0004_context_budget.sql — Phase 6.5 hardening: conversation compaction state
-- and autonomous-loop spend guardrails.

-- Sessions carry a rolling summary of older turns so long chats stay within the
-- model context window. summary_msg_count = how many leading messages of the
-- session are already represented by `summary`.
ALTER TABLE sessions ADD COLUMN summary           TEXT    NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN summary_msg_count INTEGER NOT NULL DEFAULT 0;

-- Per-agent daily spend caps for AUTONOMOUS calls (heartbeat, schedules). 0 = no
-- limit. Manual user actions (chat, run-now) are never blocked.
ALTER TABLE agents ADD COLUMN daily_call_limit  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN daily_token_limit INTEGER NOT NULL DEFAULT 0;

-- Rolling daily usage counters, keyed by agent + calendar day (YYYY-MM-DD).
CREATE TABLE IF NOT EXISTS agent_usage (
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    day           TEXT NOT NULL,
    calls         INTEGER NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, day)
);
