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
	"strings"
)

const (
	cliReplyWALFile         = "cli-reply.wal.json"
	cliReplyWALVersion      = 1
	cliReplyActivityPrefix  = "cli-reply-activity-"
	cliReplyActivitySuffix  = ".outbox.json"
	cliReplyActivityVersion = 1
)

type cliReplyTxnPhase string

const (
	cliReplyTxnPrepared cliReplyTxnPhase = "prepared"
	cliReplyTxnMessage  cliReplyTxnPhase = "message"
	cliReplyTxnHeader   cliReplyTxnPhase = "header"
	cliReplyTxnActivity cliReplyTxnPhase = "activity"
	cliReplyTxnRetired  cliReplyTxnPhase = "retired"
)

type cliReplyWAL struct {
	Version int           `json:"version"`
	TxnID   string        `json:"txnId"`
	Message Message       `json:"message"`
	State   CLIReplyState `json:"state"`
}

type cliReplyActivity struct {
	Version int            `json:"version"`
	Signal  ActivitySignal `json:"signal"`
}

func cliReplyActivitySignal(wal cliReplyWAL, target Session) ActivitySignal {
	toolDelta := 0
	if wal.Message.Role == "assistant" {
		toolDelta = countToolSteps(wal.Message.Steps)
	}
	return ActivitySignal{
		EventID:      "cli-reply:" + wal.TxnID,
		SessionID:    target.ID,
		MessageTotal: target.MessageCount,
		MessageDelta: 1,
		ToolTotal:    target.ToolCallCount,
		ToolDelta:    toolDelta,
	}
}

func cliReplyActivityPath(dir, eventID string) string {
	sum := sha256.Sum256([]byte(eventID))
	return filepath.Join(dir, cliReplyActivityPrefix+hex.EncodeToString(sum[:16])+cliReplyActivitySuffix)
}

func persistCLIReplyActivity(dir string, signal ActivitySignal) error {
	if signal.EventID == "" || signal.SessionID == "" {
		return errors.New("invalid CLI reply activity metadata")
	}
	record := cliReplyActivity{Version: cliReplyActivityVersion, Signal: signal}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := cliReplyActivityPath(dir, signal.EventID)
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, data) {
			return errors.New("CLI reply activity identity collision")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return durableAtomicWriteBytes(path, data, 0o600)
}

func validateCLIReplyState(state CLIReplyState) error {
	if state.ClearNativeCompactionPending && (!state.UpdateResume || state.ResumeSessionID == "") && !state.RetireResume {
		return errors.New("clear CLI native compaction recovery requires new resume state or explicit retirement")
	}
	return nil
}

func applyCLIReplyState(s *Session, state CLIReplyState) {
	if state.UpdateResume || state.RetireResume {
		s.CLISessionID = state.ResumeSessionID
		s.CLISentMsgCount = state.ResumeSentMsgCount
		if state.RetireResume {
			s.CLICompactMsgCount = 0
		}
	}
	if state.UpdateCompactBoundary && state.CompactMsgCount > s.CLICompactMsgCount {
		s.CLICompactMsgCount = state.CompactMsgCount
	}
	if state.ClearNativeCompactionPending {
		s.CLINativeCompactionPending = false
	}
}

func encodeSessionHeader(s Session) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeMessages(msgs []Message) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func prepareCLIReplyTarget(s Session, msgs []Message, m Message, state CLIReplyState) (Session, []Message, bool, error) {
	if err := validateCLIReplyState(state); err != nil {
		return Session{}, nil, false, err
	}
	found := false
	for _, existing := range msgs {
		if existing.ID != m.ID {
			continue
		}
		if !reflect.DeepEqual(existing, m) {
			return Session{}, nil, false, errors.New("CLI reply recovery message id collision")
		}
		found = true
		break
	}
	if !found {
		msgs = append(append([]Message(nil), msgs...), m)
	}
	s = reconcileHeader(s, msgs)
	if m.Role == "assistant" && !IsMachineTranscriptKind(s.Kind) {
		s.Unread = true
	}
	applyCLIReplyState(&s, state)
	return s, msgs, !found, nil
}

func writeDurableSessionHeader(dir string, s Session) error {
	data, err := encodeSessionHeader(s)
	if err != nil {
		return err
	}
	return durableAtomicWriteBytes(filepath.Join(dir, sessionHeaderFile), data, 0o644)
}

func writeDurableMessages(dir string, msgs []Message) error {
	data, err := encodeMessages(msgs)
	if err != nil {
		return err
	}
	return durableAtomicWriteBytes(filepath.Join(dir, sessionMsgsFile), data, 0o644)
}

func recoverCLIReplyTransaction(dir string) error {
	walPath := filepath.Join(dir, cliReplyWALFile)
	raw, err := os.ReadFile(walPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read CLI reply recovery record: %w", err)
	}
	var wal cliReplyWAL
	if err := json.Unmarshal(raw, &wal); err != nil {
		return fmt.Errorf("invalid CLI reply recovery record: %w", err)
	}
	if wal.Version != cliReplyWALVersion || wal.TxnID == "" || wal.Message.ID == "" || wal.TxnID != wal.Message.ID {
		return errors.New("invalid CLI reply recovery record metadata")
	}
	headerRaw, err := os.ReadFile(filepath.Join(dir, sessionHeaderFile))
	if err != nil {
		return fmt.Errorf("recover CLI reply header: %w", err)
	}
	var s Session
	if err := json.Unmarshal(headerRaw, &s); err != nil {
		return fmt.Errorf("recover CLI reply header: %w", err)
	}
	if s.ID == "" || wal.Message.SessionID != s.ID {
		return errors.New("CLI reply recovery session mismatch")
	}
	msgs, err := readMessagesFile(filepath.Join(dir, sessionMsgsFile))
	if err != nil {
		return fmt.Errorf("recover CLI reply transcript: %w", err)
	}
	target, targetMsgs, appended, err := prepareCLIReplyTarget(s, msgs, wal.Message, wal.State)
	if err != nil {
		return fmt.Errorf("recover CLI reply transaction: %w", err)
	}
	if appended {
		if err := writeDurableMessages(dir, targetMsgs); err != nil {
			return fmt.Errorf("recover CLI reply transcript: %w", err)
		}
	}
	if err := writeDurableSessionHeader(dir, target); err != nil {
		return fmt.Errorf("recover CLI reply state: %w", err)
	}
	if err := persistCLIReplyActivity(dir, cliReplyActivitySignal(wal, target)); err != nil {
		return fmt.Errorf("recover CLI reply activity: %w", err)
	}
	if err := durableRemove(walPath); err != nil {
		return fmt.Errorf("retire CLI reply recovery record: %w", err)
	}
	return nil
}

func (d *DB) recoverCLIReplyBeforeMutationLocked(sessionID string) (bool, error) {
	dir := d.dir(dirSessions, sessionID)
	if _, err := os.Stat(filepath.Join(dir, cliReplyWALFile)); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect CLI reply recovery record: %w", err)
	}
	if err := recoverCLIReplyTransaction(dir); err != nil {
		return false, err
	}
	s, msgs, err := readSessionDir(dir)
	if err != nil {
		return false, fmt.Errorf("reload recovered CLI reply transaction: %w", err)
	}
	d.mu.Lock()
	d.sessions[sessionID] = s
	d.messages[sessionID] = msgs
	d.mu.Unlock()
	return true, nil
}

func (d *DB) deliverRecoveredCLIReplyActivity(recovered bool, sessionID string) {
	if !recovered {
		return
	}
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		slog.Error("recovered CLI reply activity delivery deferred", "component", "db", "session", sessionID, "error", err)
	}
}

func (d *DB) deliverPendingCLIReplyActivities() error {
	d.activityDeliveryMu.Lock()
	defer d.activityDeliveryMu.Unlock()

	d.activityHookMu.RLock()
	fn := d.activityHook
	d.activityHookMu.RUnlock()
	if fn == nil {
		return nil
	}
	root := d.dir(dirSessions)
	sessions, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var paths []string
	for _, session := range sessions {
		if !session.IsDir() {
			continue
		}
		dir := filepath.Join(root, session.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), cliReplyActivityPrefix) && strings.HasSuffix(entry.Name(), cliReplyActivitySuffix) {
				paths = append(paths, filepath.Join(dir, entry.Name()))
			}
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var record cliReplyActivity
		if err := json.Unmarshal(raw, &record); err != nil {
			return fmt.Errorf("invalid CLI reply activity record: %w", err)
		}
		if record.Version != cliReplyActivityVersion || record.Signal.EventID == "" || record.Signal.SessionID == "" || cliReplyActivityPath(filepath.Dir(path), record.Signal.EventID) != path {
			return errors.New("invalid CLI reply activity record metadata")
		}
		if err := fn(record.Signal); err != nil {
			return fmt.Errorf("activity hook rejected %s: %w", record.Signal.EventID, err)
		}
		if d.cliReplyActivityHook != nil {
			if err := d.cliReplyActivityHook(record.Signal); err != nil {
				return err
			}
		}
		if err := durableRemove(path); err != nil {
			return fmt.Errorf("retire CLI reply activity: %w", err)
		}
	}
	return nil
}
