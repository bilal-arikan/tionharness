package db

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const activitySequenceFile = "activity-sequence.json"

type activitySequence struct {
	Version      int   `json:"version"`
	MessageTotal int64 `json:"messageTotal"`
	ToolTotal    int64 `json:"toolTotal"`
}

func (d *DB) loadActivitySequence() error {
	raw, err := os.ReadFile(filepath.Join(d.root, activitySequenceFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state activitySequence
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Version != 1 || state.MessageTotal < 0 || state.ToolTotal < 0 {
		return errors.New("invalid activity sequence")
	}
	d.workspaceMessageTotal = state.MessageTotal
	d.workspaceToolTotal = state.ToolTotal
	return nil
}

func (d *DB) persistActivitySequenceLocked(messageTotal, toolTotal int64) error {
	state := activitySequence{Version: 1, MessageTotal: messageTotal, ToolTotal: toolTotal}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := durableAtomicWriteBytes(filepath.Join(d.root, activitySequenceFile), raw, 0o600); err != nil {
		return err
	}
	d.workspaceMessageTotal = messageTotal
	d.workspaceToolTotal = toolTotal
	return nil
}

func (d *DB) reconcileActivitySequence() error {
	d.activitySequenceMu.Lock()
	defer d.activitySequenceMu.Unlock()
	var messages, tools int64
	for _, session := range d.sessions {
		messages += int64(session.MessageCount)
		tools += int64(session.ToolCallCount)
	}
	if messages < d.workspaceMessageTotal {
		messages = d.workspaceMessageTotal
	}
	if tools < d.workspaceToolTotal {
		tools = d.workspaceToolTotal
	}
	return d.persistActivitySequenceLocked(messages, tools)
}

func (d *DB) reserveActivitySequence(dir string, wal cliReplyWAL, messageDelta, toolDelta int64) (cliReplyWAL, error) {
	d.activitySequenceMu.Lock()
	defer d.activitySequenceMu.Unlock()
	wal.WorkspaceMessagePrevious = d.workspaceMessageTotal
	wal.WorkspaceToolPrevious = d.workspaceToolTotal
	wal.WorkspaceMessageTotal = d.workspaceMessageTotal + messageDelta
	wal.WorkspaceToolTotal = d.workspaceToolTotal + toolDelta
	raw, err := json.Marshal(wal)
	if err != nil {
		return cliReplyWAL{}, err
	}
	if err := durableAtomicWriteBytes(filepath.Join(dir, cliReplyWALFile), raw, 0o600); err != nil {
		return cliReplyWAL{}, err
	}
	if err := d.persistActivitySequenceLocked(wal.WorkspaceMessageTotal, wal.WorkspaceToolTotal); err != nil {
		return cliReplyWAL{}, err
	}
	return wal, nil
}

func (d *DB) observeRecoveredActivitySequence(wal cliReplyWAL) error {
	d.activitySequenceMu.Lock()
	defer d.activitySequenceMu.Unlock()
	messageTotal := d.workspaceMessageTotal
	toolTotal := d.workspaceToolTotal
	if wal.WorkspaceMessageTotal > messageTotal {
		messageTotal = wal.WorkspaceMessageTotal
	}
	if wal.WorkspaceToolTotal > toolTotal {
		toolTotal = wal.WorkspaceToolTotal
	}
	if messageTotal == d.workspaceMessageTotal && toolTotal == d.workspaceToolTotal {
		return nil
	}
	return d.persistActivitySequenceLocked(messageTotal, toolTotal)
}
