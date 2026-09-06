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
	"time"
)

const (
	cliReplyWALFile         = "cli-reply.wal.json"
	cliReplyWALVersion      = 3
	cliReplyActivityPrefix  = "cli-reply-activity-"
	cliReplyActivitySuffix  = ".outbox.json"
	cliReplyActivityVersion = 1
	cliReplyDegradedFile    = "cli-reply.recovery-degraded.json"
)

var ErrCLIReplyRecoveryDegraded = errors.New("CLI reply recovery degraded")

type cliReplyTxnPhase string

const (
	cliReplyTxnPrepared cliReplyTxnPhase = "prepared"
	cliReplyTxnMessage  cliReplyTxnPhase = "message"
	cliReplyTxnHeader   cliReplyTxnPhase = "header"
	cliReplyTxnActivity cliReplyTxnPhase = "activity"
	cliReplyTxnRetired  cliReplyTxnPhase = "retired"
)

type cliReplyWAL struct {
	Version                  int           `json:"version"`
	TxnID                    string        `json:"txnId"`
	Message                  Message       `json:"message"`
	State                    CLIReplyState `json:"state"`
	WorkspaceMessageTotal    int64         `json:"workspaceMessageTotal,omitempty"`
	WorkspaceToolTotal       int64         `json:"workspaceToolTotal,omitempty"`
	WorkspaceMessagePrevious int64         `json:"workspaceMessagePrevious,omitempty"`
	WorkspaceToolPrevious    int64         `json:"workspaceToolPrevious,omitempty"`
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
	messagePrevious := wal.WorkspaceMessagePrevious
	toolPrevious := wal.WorkspaceToolPrevious
	if wal.Version < 3 {
		messagePrevious = wal.WorkspaceMessageTotal - 1
		toolPrevious = wal.WorkspaceToolTotal - int64(toolDelta)
	}
	return ActivitySignal{
		EventID:                  "cli-reply:" + target.ID + ":" + wal.TxnID,
		SessionID:                target.ID,
		MessageTotal:             target.MessageCount,
		MessageDelta:             1,
		ToolTotal:                target.ToolCallCount,
		ToolDelta:                toolDelta,
		WorkspaceMessageTotal:    wal.WorkspaceMessageTotal,
		WorkspaceToolTotal:       wal.WorkspaceToolTotal,
		WorkspaceMessagePrevious: messagePrevious,
		WorkspaceToolPrevious:    toolPrevious,
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

func (d *DB) recoverCLIReplyTransaction(dir string) error {
	if marker, err := os.ReadFile(filepath.Join(dir, cliReplyDegradedFile)); err == nil {
		return fmt.Errorf("%w: %s", ErrCLIReplyRecoveryDegraded, strings.TrimSpace(string(marker)))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect CLI reply recovery degraded marker: %w", err)
	}
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
		return quarantineCLIReplyWAL(dir, walPath, fmt.Errorf("invalid CLI reply recovery record: %w", err))
	}
	if (wal.Version < 1 || wal.Version > cliReplyWALVersion) || wal.TxnID == "" || wal.Message.ID == "" || wal.TxnID != wal.Message.ID {
		return quarantineCLIReplyWAL(dir, walPath, errors.New("invalid CLI reply recovery record metadata"))
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
		return quarantineCLIReplyWAL(dir, walPath, errors.New("CLI reply recovery session mismatch"))
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
	if err := d.observeRecoveredActivitySequence(wal); err != nil {
		return fmt.Errorf("recover CLI reply activity sequence: %w", err)
	}
	if err := durableRemove(walPath); err != nil {
		return fmt.Errorf("retire CLI reply recovery record: %w", err)
	}
	return nil
}

func quarantineCLIReplyWAL(dir, walPath string, cause error) error {
	marker := struct {
		Version int    `json:"version"`
		Error   string `json:"error"`
		At      int64  `json:"at"`
	}{Version: 1, Error: cause.Error(), At: time.Now().Unix()}
	raw, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("%w: encode marker: %v", ErrCLIReplyRecoveryDegraded, err)
	}
	if err := durableAtomicWriteBytes(filepath.Join(dir, cliReplyDegradedFile), raw, 0o600); err != nil {
		return fmt.Errorf("%w: persist marker: %v", ErrCLIReplyRecoveryDegraded, err)
	}
	quarantine := walPath + ".quarantine"
	if err := os.Rename(walPath, quarantine); err != nil && !os.IsNotExist(err) {
		slog.Error("invalid CLI reply WAL retained", "component", "db", "path", walPath, "error", cause, "quarantine_error", err)
	} else {
		slog.Error("invalid CLI reply WAL quarantined", "component", "db", "path", quarantine, "error", cause)
	}
	return fmt.Errorf("%w: %v", ErrCLIReplyRecoveryDegraded, cause)
}

func (d *DB) recoverCLIReplyBeforeMutationLocked(sessionID string) (bool, error) {
	dir := d.dir(dirSessions, sessionID)
	if _, err := os.Stat(filepath.Join(dir, cliReplyWALFile)); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect CLI reply recovery record: %w", err)
	}
	if err := d.recoverCLIReplyTransaction(dir); err != nil {
		return false, err
	}
	s, msgs, err := readSessionDir(dir)
	if err != nil {
		return false, fmt.Errorf("reload recovered CLI reply transaction: %w", err)
	}
	d.mu.Lock()
	d.sessions[sessionID] = s
	d.markMutatedLocked()
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
	var deliveryErrors []error
	for _, path := range paths {
		d.activityDeliveryMu.Lock()
		if _, busy := d.activityDelivering[path]; busy {
			d.activityDeliveryMu.Unlock()
			continue
		}
		d.activityDelivering[path] = struct{}{}
		d.activityDeliveryMu.Unlock()
		func() {
			defer func() {
				d.activityDeliveryMu.Lock()
				delete(d.activityDelivering, path)
				d.activityDeliveryMu.Unlock()
			}()
			raw, err := os.ReadFile(path)
			if err != nil {
				deliveryErrors = append(deliveryErrors, err)
				return
			}
			var record cliReplyActivity
			if err := json.Unmarshal(raw, &record); err != nil {
				quarantineCLIReplyActivity(path, fmt.Errorf("invalid CLI reply activity record: %w", err))
				return
			}
			if record.Version != cliReplyActivityVersion || record.Signal.EventID == "" || record.Signal.SessionID == "" || cliReplyActivityPath(filepath.Dir(path), record.Signal.EventID) != path {
				quarantineCLIReplyActivity(path, errors.New("invalid CLI reply activity record metadata"))
				return
			}
			if err := invokeActivityHook(fn, record.Signal); err != nil {
				deliveryErrors = append(deliveryErrors, fmt.Errorf("activity hook rejected %s: %w", record.Signal.EventID, err))
				return
			}
			if d.cliReplyActivityHook != nil {
				if err := d.cliReplyActivityHook(record.Signal); err != nil {
					deliveryErrors = append(deliveryErrors, err)
					return
				}
			}
			if err := durableRemove(path); err != nil {
				if !os.IsNotExist(err) {
					deliveryErrors = append(deliveryErrors, fmt.Errorf("retire CLI reply activity: %w", err))
				}
			}
		}()
	}
	return errors.Join(deliveryErrors...)
}

func quarantineCLIReplyActivity(path string, cause error) {
	quarantine := path + ".quarantine"
	if err := os.Rename(path, quarantine); err != nil {
		slog.Error("invalid CLI reply activity retained", "component", "db", "path", path, "error", cause, "quarantine_error", err)
		return
	}
	slog.Error("invalid CLI reply activity quarantined", "component", "db", "path", quarantine, "error", cause)
}
