package db

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
)

// Automation fire ledger (_Docs/77 R5).
//
// Every attempt to fire an automation — the ones that ran AND the ones the
// guards skipped — is appended to <store>/automation-fires/<id>.jsonl. The row
// itself only keeps the last outcome (LastFiredAt / LastSessionID / LastError),
// which cannot answer "why did this rule not fire for the last three cards" or
// "how often does it hit its cooldown"; the trajectory view and the curator
// both need that history. Bounded: the file is trimmed to automationFireCap
// records once it grows past automationFireTrimAt, so a chatty rule never
// grows a log without limit.

// Fire outcomes.
const (
	AutomationFireFired   = "fired"
	AutomationFireSkipped = "skipped"
	AutomationFireFailed  = "failed"
)

// Skip reasons written by the engine's guard chain.
const (
	AutomationSkipArchived       = "archived"
	AutomationSkipDisabled       = "disabled"
	AutomationSkipExpired        = "expired"
	AutomationSkipCooldown       = "cooldown"
	AutomationSkipMaxIterations  = "max_iterations"
	AutomationSkipBackstop       = "absolute_backstop"
	AutomationSkipAutonomyPaused = "autonomy_paused"
	AutomationSkipTargetMissing  = "target_missing"
	AutomationSkipEmptyPrompt    = "empty_prompt"
)

// AutomationFireRecord is one ledger line.
type AutomationFireRecord struct {
	At               int64  `json:"at"`
	Outcome          string `json:"outcome"`
	Reason           string `json:"reason,omitempty"`
	TriggerKind      string `json:"triggerKind,omitempty"`
	TriggerSessionID string `json:"triggerSessionId,omitempty"`
	// SessionID is the session (or flow transcript session) the fire produced.
	SessionID string `json:"sessionId,omitempty"`
	Driver    string `json:"driver,omitempty"` // session | flow
	Error     string `json:"error,omitempty"`
	// Iteration is the rule's IterationCount after this attempt.
	Iteration int `json:"iteration,omitempty"`
}

const (
	dirAutomationFires    = "automation-fires"
	automationFireCap     = 500
	automationFireTrimAt  = 600
	automationFireMaxLine = 64 * 1024
)

func (d *DB) automationFiresPath(id string) string {
	return d.dir(dirAutomationFires, id+".jsonl")
}

// firesMu serialises ledger appends/trims across automations (one lock is
// enough: appends are rare and tiny).
var firesMu sync.Mutex

// AppendAutomationFire records one attempt. A missing automation is not an
// error at this layer (the rule may have been deleted mid-dispatch); the
// caller's bookkeeping decides what to do with that.
func (d *DB) AppendAutomationFire(ctx context.Context, id string, rec AutomationFireRecord) error {
	if id == "" {
		return nil
	}
	if rec.At == 0 {
		rec.At = now()
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	firesMu.Lock()
	defer firesMu.Unlock()
	path := d.automationFiresPath(id)
	if err := os.MkdirAll(d.dir(dirAutomationFires), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	return d.trimAutomationFiresLocked(path)
}

// trimAutomationFiresLocked keeps the newest automationFireCap records once
// the file exceeds automationFireTrimAt lines. Caller holds firesMu.
func (d *DB) trimAutomationFiresLocked(path string) error {
	recs, err := readAutomationFires(path)
	if err != nil || len(recs) <= automationFireTrimAt {
		return err
	}
	keep := recs[len(recs)-automationFireCap:]
	var buf []byte
	for _, r := range keep {
		b, merr := json.Marshal(r)
		if merr != nil {
			continue
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	return atomicWriteBytes(path, buf)
}

// readAutomationFires reads the whole ledger, oldest first. A missing file is
// an empty ledger; an unreadable line is skipped rather than failing the read.
func readAutomationFires(path string) ([]AutomationFireRecord, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []AutomationFireRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), automationFireMaxLine)
	for sc.Scan() {
		var r AutomationFireRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			out = append(out, r)
		}
	}
	return out, sc.Err()
}

// ListAutomationFires returns the newest `limit` records (0 = all, capped at
// automationFireCap), newest first.
func (d *DB) ListAutomationFires(ctx context.Context, id string, limit int) ([]AutomationFireRecord, error) {
	firesMu.Lock()
	recs, err := readAutomationFires(d.automationFiresPath(id))
	firesMu.Unlock()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > automationFireCap {
		limit = automationFireCap
	}
	if len(recs) > limit {
		recs = recs[len(recs)-limit:]
	}
	// newest first
	for i, j := 0, len(recs)-1; i < j; i, j = i+1, j-1 {
		recs[i], recs[j] = recs[j], recs[i]
	}
	if recs == nil {
		recs = []AutomationFireRecord{}
	}
	return recs, nil
}

// ClearAutomationFires removes an automation's ledger (on delete).
func (d *DB) ClearAutomationFires(id string) error {
	firesMu.Lock()
	defer firesMu.Unlock()
	return removeFile(d.automationFiresPath(id))
}
