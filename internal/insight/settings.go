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
	// The default is DefaultMaxSessions; an explicit 0 removes the cap.
	MaxSessions int `json:"maxSessions"`
	// MaxAnalyzed caps how many analyzer (LLM) calls one scan makes — the real cost
	// driver. 0 = no cap. Pairs beyond the cap are retried on the next scan.
	MaxAnalyzed int `json:"maxAnalyzed,omitempty"`
	// ScanSinceDays limits a scan to sessions active within the last N days (0 =
	// all history). Keeps a scan focused on recent sessions instead of surfacing
	// findings from long-old ones whose issues may be stale or already fixed.
	ScanSinceDays int `json:"scanSinceDays,omitempty"`
	// AutoScanCron is a standard 5-field cron expression (minute hour dom month dow)
	// that drives automatic scans. Empty disables automatic scanning (manual only).
	AutoScanCron string `json:"autoScanCron,omitempty"`
	// AutoScanAgentID selects the agent whose provider+model runs scans (both the
	// manual "Tara" button and the cron, unless a scan passes an explicit agent).
	// An explicitly selected agent runs on ITS OWN model; empty = the workspace's
	// first agent with the cheap title-model override.
	AutoScanAgentID string `json:"autoScanAgentId,omitempty"`
	// AutoVerifyDays: an APPLIED finding not seen for this many days (and not
	// regressed) is auto-marked VERIFIED during maintenance. 0 = default (14).
	AutoVerifyDays int `json:"autoVerifyDays,omitempty"`
	// PruneDays: a DISMISSED/VERIFIED finding untouched for this many days is
	// deleted during maintenance. 0 = default (45).
	PruneDays int `json:"pruneDays,omitempty"`
	// MaxRunSessions caps how many read-only scan sessions stay live: after a run,
	// all but the newest N are ARCHIVED (never deleted — the transcript stays
	// readable). 0 = default (DefaultMaxRunSessions); negative = keep all.
	MaxRunSessions int `json:"maxRunSessions,omitempty"`
}

// DefaultMaxRunSessions bounds the live scan-session backlog when the setting is
// unset. The run log keeps its own (larger) cap; sessions are heavier, so the
// hourly cron gets a tighter one.
const DefaultMaxRunSessions = 200

// DefaultMaxSessions bounds a scan unless the workspace explicitly overrides it.
const DefaultMaxSessions = 20

// RunSessionRetention resolves MaxRunSessions: 0 → the default, negative → 0,
// which the caller reads as "keep everything".
func (s Settings) RunSessionRetention() int {
	if s.MaxRunSessions == 0 {
		return DefaultMaxRunSessions
	}
	if s.MaxRunSessions < 0 {
		return 0
	}
	return s.MaxRunSessions
}

var settingsRelPath = filepath.Join("insight", "settings.json")

// LoadSettings reads the insight settings under a store root (db.Root()). Missing
// settings and legacy files without maxSessions receive DefaultMaxSessions.
func LoadSettings(root string) (Settings, error) {
	s := Settings{MaxSessions: DefaultMaxSessions}
	b, err := os.ReadFile(filepath.Join(root, settingsRelPath))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return Settings{}, err
	}
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
