-- 0003_tasks_schedules.sql — Phase 5 additions for the task board and scheduler.
-- Base tables (tasks, schedules, runs) already exist from 0001_init.sql; this
-- migration adds the columns Phase 5 needs and the indexes that keep board and
-- scheduler queries cheap.

-- Tasks: the prompt is the instruction fed to the owner agent when the task runs.
ALTER TABLE tasks ADD COLUMN prompt          TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN last_run_id     TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN last_run_status TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN last_run_at     INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_tasks_board ON tasks(board_state, updated_at);

-- Schedules: a schedule either runs a linked task or delivers a standalone
-- prompt to its agent. task_id is nullable (standalone prompt schedules).
ALTER TABLE schedules ADD COLUMN task_id     TEXT    REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE schedules ADD COLUMN prompt      TEXT    NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN last_run_at INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON schedules(enabled);

-- Runs: store the agent's textual output and what triggered the run.
ALTER TABLE runs ADD COLUMN output  TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN trigger TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id, created_at);
