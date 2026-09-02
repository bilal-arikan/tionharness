package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
)

const (
	automationActivityDispatchDir     = "automation-activity-dispatches"
	automationActivityDispatchVersion = 1
	automationDispatchPending         = "pending"
	automationDispatchCompleted       = "completed"
)

type AutomationActivityDispatchOutcome struct {
	SessionID string `json:"sessionId,omitempty"`
	Driver    string `json:"driver,omitempty"`
	Error     string `json:"error,omitempty"`
	Skipped   bool   `json:"skipped,omitempty"`
}

type automationActivityDispatchRecord struct {
	Version      int                               `json:"version"`
	Status       string                            `json:"status"`
	EventID      string                            `json:"eventId"`
	AutomationID string                            `json:"automationId"`
	Outcome      AutomationActivityDispatchOutcome `json:"outcome,omitempty"`
}

func automationActivityDispatchKey(eventID, automationID string) string {
	sum := sha256.Sum256([]byte(eventID + "\x00" + automationID))
	return hex.EncodeToString(sum[:16])
}

func (d *DB) automationActivityDispatchPath(eventID, automationID string) string {
	return filepath.Join(d.root, automationActivityDispatchDir, automationActivityDispatchKey(eventID, automationID)+".json")
}

// BeginAutomationActivityDispatch durably claims one activity/automation pair.
// A completed replay returns its existing outcome without launching again.
func (d *DB) BeginAutomationActivityDispatch(ctx context.Context, eventID, automationID string) (AutomationActivityDispatchOutcome, bool, error) {
	if eventID == "" || automationID == "" {
		return AutomationActivityDispatchOutcome{}, false, errors.New("invalid automation activity dispatch identity")
	}
	path := d.automationActivityDispatchPath(eventID, automationID)
	if raw, err := os.ReadFile(path); err == nil {
		var record automationActivityDispatchRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return AutomationActivityDispatchOutcome{}, false, err
		}
		if record.Version != automationActivityDispatchVersion || record.EventID != eventID || record.AutomationID != automationID {
			return AutomationActivityDispatchOutcome{}, false, errors.New("automation activity dispatch identity collision")
		}
		if record.Status == automationDispatchCompleted {
			return record.Outcome, true, nil
		}
		if record.Status != automationDispatchPending {
			return AutomationActivityDispatchOutcome{}, false, errors.New("invalid automation activity dispatch status")
		}
	} else if !os.IsNotExist(err) {
		return AutomationActivityDispatchOutcome{}, false, err
	} else {
		record := automationActivityDispatchRecord{
			Version: automationActivityDispatchVersion, Status: automationDispatchPending,
			EventID: eventID, AutomationID: automationID,
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return AutomationActivityDispatchOutcome{}, false, err
		}
		if err := durableAtomicWriteBytes(path, raw, 0o600); err != nil {
			return AutomationActivityDispatchOutcome{}, false, err
		}
	}

	d.mu.RLock()
	a, ok := d.automations[automationID]
	var receipt AutomationActivityDispatchOutcome
	if ok && a.ActivityDispatchReceipts != nil {
		receipt, ok = a.ActivityDispatchReceipts[eventID]
	} else {
		ok = false
	}
	d.mu.RUnlock()
	if !ok {
		return AutomationActivityDispatchOutcome{}, false, nil
	}
	if err := d.persistAutomationActivityDispatchRecord(eventID, automationID, receipt); err != nil {
		return AutomationActivityDispatchOutcome{}, false, err
	}
	return receipt, true, nil
}

func (d *DB) persistAutomationActivityDispatchRecord(eventID, automationID string, outcome AutomationActivityDispatchOutcome) error {
	record := automationActivityDispatchRecord{
		Version: automationActivityDispatchVersion, Status: automationDispatchCompleted,
		EventID: eventID, AutomationID: automationID, Outcome: outcome,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return durableAtomicWriteBytes(d.automationActivityDispatchPath(eventID, automationID), raw, 0o600)
}

// CompleteAutomationActivityDispatch atomically couples automation fire
// bookkeeping with its durable EventID receipt. Replays never increment twice.
func (d *DB) CompleteAutomationActivityDispatch(ctx context.Context, eventID, automationID string, outcome AutomationActivityDispatchOutcome) error {
	d.mu.Lock()
	a, ok := d.automations[automationID]
	if !ok {
		d.mu.Unlock()
		return ErrNotFound
	}
	if existing, exists := a.ActivityDispatchReceipts[eventID]; exists {
		d.mu.Unlock()
		if !reflect.DeepEqual(existing, outcome) {
			return errors.New("automation activity dispatch outcome collision")
		}
		return d.persistAutomationActivityDispatchRecord(eventID, automationID, existing)
	}
	receipts := make(map[string]AutomationActivityDispatchOutcome, len(a.ActivityDispatchReceipts)+1)
	for key, value := range a.ActivityDispatchReceipts {
		receipts[key] = value
	}
	receipts[eventID] = outcome
	a.ActivityDispatchReceipts = receipts
	if !outcome.Skipped {
		a.IterationCount++
		a.LastFiredAt = now()
		a.LastSessionID = outcome.SessionID
		a.LastError = outcome.Error
	}
	if err := d.persistAutomationLocked(a); err != nil {
		d.mu.Unlock()
		return err
	}
	d.mu.Unlock()
	return d.persistAutomationActivityDispatchRecord(eventID, automationID, outcome)
}
