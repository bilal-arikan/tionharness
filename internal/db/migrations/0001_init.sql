-- 0001_init.sql — initial SwarmGo schema.
-- All timestamps are unix epoch seconds (INTEGER).

CREATE TABLE IF NOT EXISTS agents (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    soul          TEXT NOT NULL DEFAULT '',
    identity      TEXT NOT NULL DEFAULT '',
    provider      TEXT NOT NULL DEFAULT 'anthropic',
    model         TEXT NOT NULL DEFAULT '',
    capabilities  TEXT NOT NULL DEFAULT '[]',   -- JSON array
    planning_mode TEXT NOT NULL DEFAULT 'standard', -- standard | strict
    dream_config  TEXT NOT NULL DEFAULT '{}',   -- JSON object
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT REFERENCES agents(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL DEFAULT 'chat', -- chat | chatroom
    title         TEXT NOT NULL DEFAULT '',
    message_count INTEGER NOT NULL DEFAULT 0,
    state         TEXT NOT NULL DEFAULT 'active',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent_id);

CREATE TABLE IF NOT EXISTS session_messages (
    id                TEXT PRIMARY KEY,
    session_id        TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role              TEXT NOT NULL, -- system | user | assistant | tool
    text              TEXT NOT NULL DEFAULT '',
    tool_calls        TEXT NOT NULL DEFAULT '[]',  -- JSON array
    reasoning_content TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON session_messages(session_id, created_at);

CREATE TABLE IF NOT EXISTS tasks (
    id                     TEXT PRIMARY KEY,
    title                  TEXT NOT NULL,
    description            TEXT NOT NULL DEFAULT '',
    owner_agent_id         TEXT REFERENCES agents(id) ON DELETE SET NULL,
    board_state            TEXT NOT NULL DEFAULT 'todo', -- todo | in_progress | review | done | failed
    execution_policy       TEXT NOT NULL DEFAULT '{}',
    execution_policy_state TEXT NOT NULL DEFAULT '{}',
    workspace_path         TEXT NOT NULL DEFAULT '',
    dependencies           TEXT NOT NULL DEFAULT '[]', -- JSON array of task ids
    created_at             INTEGER NOT NULL,
    updated_at             INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS schedules (
    id                   TEXT PRIMARY KEY,
    agent_id             TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    cron_expr            TEXT NOT NULL DEFAULT '',
    next_run_at          INTEGER,
    last_delivery_status TEXT NOT NULL DEFAULT '',
    last_delivery_error  TEXT NOT NULL DEFAULT '',
    enabled              INTEGER NOT NULL DEFAULT 1,
    created_at           INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    id         TEXT PRIMARY KEY,
    task_id    TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    agent_id   TEXT REFERENCES agents(id) ON DELETE SET NULL,
    status     TEXT NOT NULL DEFAULT 'pending', -- pending | running | success | failure
    error      TEXT NOT NULL DEFAULT '',
    usage      TEXT NOT NULL DEFAULT '{}',  -- JSON token usage
    run_state  TEXT NOT NULL DEFAULT '{}',  -- restart-safe state
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS connectors (
    id             TEXT PRIMARY KEY,
    platform       TEXT NOT NULL, -- discord | slack | telegram | email ...
    encrypted_key  TEXT NOT NULL DEFAULT '',
    routing_policy TEXT NOT NULL DEFAULT '{}',
    health         TEXT NOT NULL DEFAULT 'unknown',
    created_at     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS knowledge_sources (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT REFERENCES agents(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL DEFAULT 'document', -- document | journal | reflection
    content    TEXT NOT NULL DEFAULT '',
    embedding  BLOB,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_knowledge_agent ON knowledge_sources(agent_id);

CREATE TABLE IF NOT EXISTS skills (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    tags    TEXT NOT NULL DEFAULT '[]',
    is_live INTEGER NOT NULL DEFAULT 0,
    scope   TEXT NOT NULL DEFAULT 'shared', -- shared | scoped
    body    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS mcp_servers (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    transport  TEXT NOT NULL DEFAULT 'stdio', -- stdio | sse | http
    env_config TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS provider_configs (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT REFERENCES agents(id) ON DELETE CASCADE,
    model         TEXT NOT NULL DEFAULT '',
    endpoint      TEXT NOT NULL DEFAULT '',
    encrypted_key TEXT NOT NULL DEFAULT ''
);
