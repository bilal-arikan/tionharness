package insight

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the workspace-scoped insight configuration, stored as a small JSON
// file at <store>/insight/settings.json (file-based, like the rest of the store).
type Settings struct {
	// AppFixRepoPath is the git repo whose _Docs/INSIGHT-BACKLOG.md receives
	// app-fix findings (chosen from the UI). Empty = no repo backlog sink; findings
	// still land in-app.
	AppFixRepoPath string `json:"appFixRepoPath,omitempty"`
	// MaxSessions caps how many sessions one scan run considers (budget guardrail).
	// 0 = no cap.
	MaxSessions int `json:"maxSessions,omitempty"`
	// MaxAnalyzed caps how many analyzer (LLM) calls one scan makes — the real cost
	// driver. 0 = no cap. Pairs beyond the cap are retried on the next scan.
	MaxAnalyzed int `json:"maxAnalyzed,omitempty"`
	// AutoScanCron is a standard 5-field cron expression (minute hour dom month dow)
	// that drives automatic scans. Empty disables automatic scanning (manual only).
	AutoScanCron string `json:"autoScanCron,omitempty"`
	// AutoScanAgentID selects the agent whose provider/model runs automatic scans.
	// Empty = the workspace's default (newest) agent, same as a manual scan.
	AutoScanAgentID string `json:"autoScanAgentId,omitempty"`
}

var settingsRelPath = filepath.Join("insight", "settings.json")

// LoadSettings reads the insight settings under a store root (db.Root()). A
// missing file yields zero-value defaults (no error).
func LoadSettings(root string) (Settings, error) {
	b, err := os.ReadFile(filepath.Join(root, settingsRelPath))
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveSettings writes the insight settings atomically (tmp+rename).
func SaveSettings(root string, s Settings) error {
	path := filepath.Join(root, settingsRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
