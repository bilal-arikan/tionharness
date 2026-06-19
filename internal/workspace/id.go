package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// wsIDPrefix is the human-readable workspace id prefix ("WS1", "WS2", ...).
const wsIDPrefix = "WS"

// wsCounterFile holds the persisted workspace id sequence at the manager root.
func (m *Manager) wsCounterPath() string {
	return filepath.Join(m.rootDir, "ws-counter.json")
}

// parseWSID returns the numeric part of a "WS<n>" id, or ok=false for any other
// shape (e.g. a legacy UUID). Used to keep the counter ahead of live ids.
func parseWSID(id string) (int64, bool) {
	if !strings.HasPrefix(id, wsIDPrefix) {
		return 0, false
	}
	n, err := strconv.ParseInt(id[len(wsIDPrefix):], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// loadWSCounter reads the persisted counter (0 if missing/unreadable: callers
// also fold in the max live id, so a fresh start is safe).
func (m *Manager) loadWSCounter() int64 {
	data, err := os.ReadFile(m.wsCounterPath())
	if err != nil {
		return 0
	}
	var v struct {
		N int64 `json:"n"`
	}
	if json.Unmarshal(data, &v) != nil {
		return 0
	}
	return v.N
}

// persistWSCounter writes the counter atomically (tmp + rename).
func (m *Manager) persistWSCounter(n int64) {
	data, err := json.Marshal(struct {
		N int64 `json:"n"`
	}{n})
	if err != nil {
		return
	}
	tmp := m.wsCounterPath() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, m.wsCounterPath())
}

// nextWorkspaceID allocates the next monotonic, never-reused workspace id. It
// takes mu itself (Create does not hold it) and persists the counter before
// returning, so a deletion never frees a number and a restart never reissues one.
func (m *Manager) nextWorkspaceID() string {
	m.mu.Lock()
	m.wsCounter++
	n := m.wsCounter
	m.mu.Unlock()
	m.persistWSCounter(n)
	return wsIDPrefix + strconv.FormatInt(n, 10)
}
