// Package progress persists an agent's working checklist (the todo_write list)
// to a durable, human-readable JSON file so it survives across sessions — the
// SwarmGo equivalent of Claude Code's claude-progress.txt + feature_list.json
// convention for long-running agents. A fresh session reads it back at start to
// "get up to speed on what was recently worked on".
//
// The file lives at <dir>/.swarmgo/progress.json, where <dir> is the session's
// working directory (the project) when set — so it is git-committable and tied to
// the project, not the ephemeral session. It is intentionally free of any
// internal/db dependency so both internal/agent and internal/api can use it.
package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Version is the on-disk schema version.
const Version = 1

// maxLog bounds the rolling progress log so the file never grows without limit.
const maxLog = 50

// subDir + fileName form the on-disk location under a project directory.
const (
	subDir   = ".swarmgo"
	fileName = "progress.json"
)

// TodoItem mirrors the todo_write checklist entry. Status is one of
// pending|in_progress|completed; "completed" is the equivalent of Anthropic's
// feature_list `passes: true`. Category and Steps are the optional richer
// feature_list fields (a grouping label and verification sub-steps); both are
// omitted from the file when empty.
type TodoItem struct {
	Content  string   `json:"content"`
	Status   string   `json:"status"`
	Category string   `json:"category,omitempty"`
	Steps    []string `json:"steps,omitempty"`
}

// LogEntry is one line of the rolling progress journal (the claude-progress.txt
// analogue): a short note stamped with the session that produced it.
type LogEntry struct {
	TS        int64  `json:"ts"`
	SessionID string `json:"sessionId,omitempty"`
	Note      string `json:"note"`
}

// Record is the full persisted progress document for a project directory.
type Record struct {
	Version   int        `json:"version"`
	UpdatedAt int64      `json:"updatedAt"`
	SessionID string     `json:"sessionId,omitempty"`
	AgentID   string     `json:"agentId,omitempty"`
	Todos     []TodoItem `json:"todos"`
	Log       []LogEntry `json:"log,omitempty"`
}

// File returns the absolute path of the progress file for a project directory.
func File(dir string) string { return filepath.Join(dir, subDir, fileName) }

// Load reads the progress record for dir. The boolean is false (with a nil
// error) when no progress file exists yet — the normal "nothing to resume" case.
func Load(dir string) (Record, bool, error) {
	if dir == "" {
		return Record{}, false, nil
	}
	b, err := os.ReadFile(File(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}

// Save writes the record to dir atomically (tmp file + rename), creating the
// .swarmgo directory as needed. The log is trimmed to the most recent maxLog
// entries. Version is stamped automatically.
func Save(dir string, r Record) error {
	if dir == "" {
		return nil
	}
	r.Version = Version
	if len(r.Log) > maxLog {
		r.Log = r.Log[len(r.Log)-maxLog:]
	}
	target := File(dir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
