-- 0002_heartbeat.sql — autonomous heartbeat config for agents.

ALTER TABLE agents ADD COLUMN heartbeat_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN heartbeat_interval_sec INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN heartbeat_prompt TEXT NOT NULL DEFAULT '';
