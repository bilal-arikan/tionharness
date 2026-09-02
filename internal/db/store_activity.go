package db

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

// ActivitySignal describes one message append's effect on a session's monotonic
// activity counters. It is delivered to the activity hook (see SetActivityHook)
// so counter-triggered automations can detect an interval crossing statelessly:
// the previous total is (NewTotal - Delta), mirroring the token path's use of a
// per-call delta. One signal carries BOTH metrics; a user message moves only the
// message counter (ToolDelta == 0), an assistant turn may move both.
type ActivitySignal struct {
	EventID                  string // stable id for at-least-once durable delivery dedupe
	SessionID                string
	MessageTotal             int   // session lifetime message count AFTER this append
	MessageDelta             int   // messages this append added (always 1)
	ToolTotal                int   // session lifetime tool-call count AFTER this append
	ToolDelta                int   // tool calls this append's message carried (0 for non-tool msgs)
	WorkspaceMessageTotal    int64 // workspace total captured at this append
	WorkspaceToolTotal       int64 // workspace total captured at this append
	WorkspaceMessagePrevious int64 // workspace total immediately before this append
	WorkspaceToolPrevious    int64 // workspace total immediately before this append
}

// ActivityFn observes a message-append activity signal.
type ActivityFn func(sig ActivitySignal) error

// SetActivityHook registers (or clears, with nil) the activity observer. Wired
// once at workspace boot by the manager to the AutomationEngine.
func (d *DB) SetActivityHook(fn ActivityFn) error {
	d.activityHookMu.Lock()
	d.activityHook = fn
	d.activityHookMu.Unlock()
	if fn == nil {
		return nil
	}
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		return fmt.Errorf("deliver pending CLI reply activity: %w", err)
	}
	return nil
}

// fireActivityHook dispatches an activity signal to the registered observer (if
// any). Called after the store lock is released.
func (d *DB) fireActivityHook(sig ActivitySignal) {
	d.activityHookMu.RLock()
	fn := d.activityHook
	d.activityHookMu.RUnlock()
	if fn != nil {
		if err := invokeActivityHook(fn, sig); err != nil {
			slog.Error("activity hook failed", "component", "db", "session", sig.SessionID, "error", err)
		}
	}
}

func invokeActivityHook(fn ActivityFn, sig ActivitySignal) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("activity hook panic: %v", recovered)
		}
	}()
	return fn(sig)
}

const (
	activityInboxDir     = "activity-inbox"
	activityInboxVersion = 1
	activityPending      = "pending"
	activityCompleted    = "completed"
)

type activityInboxRecord struct {
	Version int            `json:"version"`
	Status  string         `json:"status"`
	Signal  ActivitySignal `json:"signal"`
}

func activityInboxPath(root, eventID string) string {
	sum := sha256.Sum256([]byte(eventID))
	return filepath.Join(root, activityInboxDir, hex.EncodeToString(sum[:16])+".json")
}

// AcceptActivitySignal durably accepts a source outbox event. Repeated accepts
// validate the complete payload and return without changing its processing state.
func (d *DB) AcceptActivitySignal(sig ActivitySignal) (bool, error) {
	if sig.EventID == "" || sig.SessionID == "" {
		return false, errors.New("invalid activity inbox metadata")
	}
	path := activityInboxPath(d.root, sig.EventID)
	if raw, err := os.ReadFile(path); err == nil {
		var existing activityInboxRecord
		if err := json.Unmarshal(raw, &existing); err != nil {
			return false, fmt.Errorf("invalid activity inbox record: %w", err)
		}
		if existing.Version != activityInboxVersion || existing.Signal.EventID != sig.EventID || !reflect.DeepEqual(existing.Signal, sig) {
			return false, errors.New("activity inbox identity collision")
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	record := activityInboxRecord{Version: activityInboxVersion, Status: activityPending, Signal: sig}
	raw, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	if err := durableAtomicWriteBytes(path, raw, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// PendingActivitySignals returns durably accepted events not yet processed.
func (d *DB) PendingActivitySignals() ([]ActivitySignal, error) {
	dir := filepath.Join(d.root, activityInboxDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []ActivitySignal
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			slog.Error("activity inbox record unreadable", "component", "db", "path", path, "error", err)
			continue
		}
		var record activityInboxRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			quarantineActivityInbox(path, fmt.Errorf("invalid activity inbox record: %w", err))
			continue
		}
		if record.Version != activityInboxVersion || record.Signal.EventID == "" || activityInboxPath(d.root, record.Signal.EventID) != filepath.Join(dir, entry.Name()) || (record.Status != activityPending && record.Status != activityCompleted) {
			quarantineActivityInbox(path, errors.New("invalid activity inbox record metadata"))
			continue
		}
		if record.Status == activityPending {
			out = append(out, record.Signal)
		}
	}
	return out, nil
}

func quarantineActivityInbox(path string, cause error) {
	quarantine := path + ".quarantine"
	if err := os.Rename(path, quarantine); err != nil {
		slog.Error("invalid activity inbox retained", "component", "db", "path", path, "error", cause, "quarantine_error", err)
		return
	}
	slog.Error("invalid activity inbox quarantined", "component", "db", "path", quarantine, "error", cause)
}

// CompleteActivitySignal records durable consumer completion. Completed receipts
// are retained so a replayed source outbox cannot repeat the side effect.
func (d *DB) CompleteActivitySignal(sig ActivitySignal) error {
	path := activityInboxPath(d.root, sig.EventID)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record activityInboxRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return err
	}
	if record.Version != activityInboxVersion || !reflect.DeepEqual(record.Signal, sig) {
		return errors.New("activity inbox completion mismatch")
	}
	if record.Status == activityCompleted {
		return nil
	}
	record.Status = activityCompleted
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if bytes.Equal(raw, encoded) {
		return nil
	}
	return durableAtomicWriteBytes(path, encoded, 0o600)
}

// WorkspaceCounterTotal returns the workspace-wide cumulative total of the given
// counter metric — the sum of every session's MessageCount (metric "message" or
// "") or ToolCallCount (metric "tool"). It backs a workspace-scoped counter
// automation, resolved lazily by the engine only when such a rule exists (the
// analogue of WorkspaceTokensToday). Cumulative, not daily-reset: crossing the
// next interval fires on every CounterInterval of new activity. An unknown metric
// returns 0.
func (d *DB) WorkspaceCounterTotal(metric string) int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var total int64
	for _, s := range d.sessions {
		switch metric {
		case CounterMetricTool:
			total += int64(s.ToolCallCount)
		case "", CounterMetricMessage:
			total += int64(s.MessageCount)
		}
	}
	return total
}

// countToolSteps returns how many tool calls a persisted assistant message's
// Steps JSON carries — the steps whose kind is "tool" (agent.StepTool). It is a
// tolerant scan: an empty/"[]"/malformed Steps value counts as zero rather than
// erroring, because a bad transcript line must not break message persistence.
// The db layer cannot import agent.TurnStep (that would be an import cycle), so
// it decodes only the one field it needs.
func countToolSteps(stepsJSON string) int {
	if stepsJSON == "" || stepsJSON == "[]" {
		return 0
	}
	var steps []struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return 0
	}
	n := 0
	for _, s := range steps {
		if s.Kind == "tool" {
			n++
		}
	}
	return n
}
