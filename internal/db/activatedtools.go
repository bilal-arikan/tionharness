package db

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// activatedToolsFile is the per-session sidecar holding the set of on-demand
// (extended-tier) tool names the model has activated on the claude-cli gateway
// path. It lives next to session.jsonl, like inbox.json and prompt_epoch.json,
// and is written atomically on every change.
//
// The set used to live only in the api layer's RAM, keyed by the per-session
// Bearer token: a restart — or a claude-cli subprocess reconnecting under a new
// token — reset it, so tools the model had already activated silently fell off
// its tool list and calling one failed with
// "No such tool available: mcp__tionharness_extended__<name>". Persisting it per
// SESSION (not per token) keeps the activated surface across both.
const activatedToolsFile = "activated-tools.json"

// activatedToolsDoc is the sidecar payload. A named field (rather than a bare
// array) leaves room for future per-tool metadata without a format break.
type activatedToolsDoc struct {
	Tools []string `json:"tools"`
}

// activatedToolsPath returns the sidecar path for a session.
func (d *DB) activatedToolsPath(sessionID string) string {
	return d.dir(dirSessions, sessionID, activatedToolsFile)
}

// WriteActivatedTools atomically persists a session's activated tool names. The
// list is sorted and de-duplicated so the file is stable across writes. An empty
// list clears the sidecar (nothing activated leaves no file).
func (d *DB) WriteActivatedTools(sessionID string, names []string) error {
	if sessionID == "" {
		return nil
	}
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		return d.ClearActivatedTools(sessionID)
	}
	sort.Strings(out)
	return atomicWriteJSON(d.activatedToolsPath(sessionID), activatedToolsDoc{Tools: out})
}

// ReadActivatedTools loads a session's activated tool names, returning ok=false
// when the sidecar is absent. A corrupt sidecar is an ERROR, not an empty set:
// silently returning "nothing activated" is indistinguishable from a genuine
// reset and would hide exactly the bug this file exists to prevent.
func (d *DB) ReadActivatedTools(sessionID string) ([]string, bool, error) {
	if sessionID == "" {
		return nil, false, nil
	}
	path := d.activatedToolsPath(sessionID)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var doc activatedToolsDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.Tools, true, nil
}

// ClearActivatedTools removes a session's activation sidecar. A missing file is
// not an error.
func (d *DB) ClearActivatedTools(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	err := os.Remove(d.activatedToolsPath(sessionID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
