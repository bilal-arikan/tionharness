// Message rows inside a session: append (plain and with the claude-cli reply state), feedback, deletion (single and from-a-point-on), and listing. The JSONL append path lives here too.
package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
)

// SetMessageFeedback sets (or clears, when rating==0 and note=="") a user rating
// on an assistant message, rewriting the session's JSONL file. Returns ErrNotFound
// if the session or message is absent.
func (d *DB) SetMessageFeedback(ctx context.Context, sessionID, messageID string, rating int, note string) error {
	// Rewrites the transcript: same lock, and taken BEFORE d.mu (see
	// transcript_lock.go), so it can never interleave with a concurrent append.
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return err
	}
	release, err := d.pinTranscript(sessionID)
	if err != nil {
		tl.Unlock()
		return err
	}
	defer func() {
		release(true)
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	msgs := d.messages[sessionID]
	idx := -1
	for i := range msgs {
		if msgs[i].ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	msgs = append([]Message(nil), msgs...)
	if rating == 0 && note == "" {
		msgs[idx].Feedback = nil
	} else {
		msgs[idx].Feedback = &MessageFeedback{Rating: rating, Note: note, At: now()}
	}
	return d.persistTranscriptEditLocked(s, msgs)
}

// AddMessage appends a message to a session and bumps the session counter.
func (d *DB) AddMessage(ctx context.Context, m Message) (Message, error) {
	// Respect a caller-supplied ID (used so a streamed reply and its crash sidecar
	// share one identity, making recovery idempotent); otherwise allocate one.
	if m.ID == "" {
		m.ID = newID()
	}
	m.CreatedAt = now()
	if m.ToolCalls == "" {
		m.ToolCalls = "[]"
	}
	if m.Steps == "" {
		m.Steps = "[]"
	}
	// Ensure the participant fields are populated before the line is persisted, so
	// the on-disk transcript is canonical (author/recipient recorded, not derived).
	m.NormalizeParticipants()

	// Serialise on THIS session's transcript lock, not on the global store lock:
	// two appends to the same session are ordered, appends to different sessions
	// run concurrently, and no reader of any unrelated entity waits for a disk
	// write. Held across both the file write and the in-memory commit below, so
	// the order of lines in the file is the order of messages in RAM.
	tl := d.transcriptLock(m.SessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(m.SessionID)
	if err != nil {
		tl.Unlock()
		return m, err
	}

	release, err := d.pinTranscript(m.SessionID)
	if err != nil {
		tl.Unlock()
		return m, err
	}
	unlock := func() {
		release(true)
		tl.Unlock()
	}
	d.mu.RLock()
	s, ok := d.sessions[m.SessionID]
	msgs := d.messages[m.SessionID] // pinned and protected by the transcript lock
	d.mu.RUnlock()
	if !ok {
		unlock()
		return m, ErrNotFound
	}
	target, appended, err := currentAppendTarget(s, msgs, m)
	if err != nil {
		unlock()
		return m, err
	}
	if !appended {
		unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, m.SessionID)
		return m, nil
	}
	dir := d.dir(dirSessions, m.SessionID)
	walPath := filepath.Join(dir, cliReplyWALFile)
	toolDelta := target.ToolCallCount - s.ToolCallCount
	wal := cliReplyWAL{Version: cliReplyWALVersion, TxnID: m.ID, Message: m}
	wal, err = d.reserveActivitySequence(dir, wal, 1, int64(toolDelta))
	if err != nil {
		unlock()
		return m, fmt.Errorf("prepare message recovery: %w", err)
	}

	// Persist BEFORE publishing in memory. The failure this ordering rules out is
	// the one 66d1324d had to repair with a rollback: a message that the UI shows
	// and automations fire for, which never reached the transcript and vanishes on
	// the next restart (the counters are recomputed from the file). Writing first
	// means a failed append leaves nothing to undo — the message was never visible,
	// no activity hook fired, and the caller gets the error.
	//
	// Hot path: append only the new message line (O(1)) instead of rewriting the
	// whole conversation file (which was O(n) per message → O(n²) per session).
	// The header line keeps a stale MessageCount/UpdatedAt on disk; both are
	// recomputed from the message lines on load and refreshed by the next full
	// rewrite (title/summary change).
	if appendErr := d.appendMessageLine(m.SessionID, m); appendErr != nil {
		// AddMessage's contract is "a failed append leaves nothing behind". Unlike
		// AddMessageWithCLIState — whose WAL is the recovery record for a multi-file
		// commit and is meant to outlive a crash — this WAL only covers the single
		// line that just failed to land. Leaving it would make every later operation
		// on this session replay a message the caller was already told did not
		// persist, and a permanently unwritable transcript would keep failing there.
		if removeErr := durableRemove(walPath); removeErr != nil {
			unlock()
			return m, errors.Join(appendErr, fmt.Errorf("retire message recovery: %w", removeErr))
		}
		unlock()
		return m, appendErr
	}
	sig := cliReplyActivitySignal(wal, target)
	if err := persistCLIReplyActivity(dir, sig); err != nil {
		unlock()
		return m, fmt.Errorf("persist message activity: %w", err)
	}
	if err := durableRemove(walPath); err != nil {
		unlock()
		return m, fmt.Errorf("retire message recovery: %w", err)
	}

	d.mu.Lock()
	s, ok = d.sessions[m.SessionID]
	if !ok {
		// Unreachable while the transcript lock is held (DeleteSession takes it
		// too), but a session that disappeared must not be resurrected in memory.
		d.mu.Unlock()
		unlock()
		return m, ErrNotFound
	}
	d.messages[m.SessionID] = append(d.messages[m.SessionID], m)
	s.MessageCount++
	// Sum this message's executed tool calls into the session's lifetime tool
	// counter (backs counter automations with metric "tool"). Only assistant
	// messages carry tool steps; a user/system append contributes 0.
	committedToolDelta := 0
	if m.Role == "assistant" {
		committedToolDelta = toolDelta
		s.ToolCallCount += committedToolDelta
	}
	s.UpdatedAt = m.CreatedAt
	// An agent reply marks the session unread; the UI clears it when opened.
	// A machine-written transcript is exempt: it is hidden from the default
	// sessions view, so an unread badge raised there could never be cleared.
	if m.Role == "assistant" && !IsMachineTranscriptKind(s.Kind) {
		s.Unread = true
	}
	// Keep the participant roster in sync: any agent that authors a message or is
	// addressed by one joins the thread. The human "user" and broadcast ("*") are
	// implicit and never stored in the roster.
	s.Participants = addParticipant(s.Participants, m.AuthorKind, m.AuthorID)
	s.Participants = addParticipant(s.Participants, AuthorAgent, m.RecipientID)
	d.sessions[s.ID] = s
	d.markMutatedLocked()
	d.mu.Unlock()
	unlock()
	d.deliverRecoveredCLIReplyActivity(recovered, m.SessionID)
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		slog.Error("durable message activity delivery deferred", "component", "db", "session", m.SessionID, "event", sig.EventID, "error", err)
	}
	return m, nil
}

// CLIReplyState is session bookkeeping committed with an assistant reply.
// Update flags distinguish an absent update from a deliberate zero value.
type CLIReplyState struct {
	UpdateResume       bool
	RetireResume       bool
	ResumeSessionID    string
	ResumeSentMsgCount int
	// Transcript boundary sent at turn start. Notes appended while the CLI runs
	// must stay beyond the cursor even when they precede its persisted reply.
	ResumeInputMsgCount          int
	UpdateCompactBoundary        bool
	CompactMsgCount              int
	ClearNativeCompactionPending bool
}

// AddMessageWithCLIState persists a reply and its CLI resume/compaction state
// through a session-scoped WAL. Open replays any interrupted transaction, so
// every crash phase converges to exactly one reply and its matching CLI state.
func (d *DB) AddMessageWithCLIState(ctx context.Context, m Message, state CLIReplyState) (Message, error) {
	if m.ID == "" {
		m.ID = newID()
	}
	m.CreatedAt = now()
	if m.ToolCalls == "" {
		m.ToolCalls = "[]"
	}
	if m.Steps == "" {
		m.Steps = "[]"
	}
	m.NormalizeParticipants()

	tl := d.transcriptLock(m.SessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(m.SessionID)
	if err != nil {
		tl.Unlock()
		return m, err
	}
	release, err := d.pinTranscript(m.SessionID)
	if err != nil {
		tl.Unlock()
		return m, err
	}
	unlock := func() {
		release(true)
		tl.Unlock()
	}
	d.mu.RLock()
	s, ok := d.sessions[m.SessionID]
	msgs := d.messages[m.SessionID] // pinned and protected by the transcript lock
	d.mu.RUnlock()
	if !ok {
		unlock()
		return m, ErrNotFound
	}
	if err := validateCLIReplyState(state); err != nil {
		unlock()
		return m, err
	}
	dir := d.dir(dirSessions, m.SessionID)
	walPath := filepath.Join(dir, cliReplyWALFile)
	if _, err := os.Stat(walPath); err == nil {
		unlock()
		return m, errors.New("CLI reply recovery transaction remains pending")
	} else if !os.IsNotExist(err) {
		unlock()
		return m, err
	}
	target, appended, err := currentAppendTarget(s, msgs, m)
	if err != nil {
		unlock()
		return m, err
	}
	targetMsgs := msgs
	if appended {
		targetMsgs = append(append([]Message(nil), msgs...), m)
	}
	if state.UpdateResume && state.ResumeInputMsgCount > 0 && len(msgs) > state.ResumeInputMsgCount {
		// The CLI already knows its answer, but not these concurrent notes. Replay
		// the answer on the next delta rather than skipping an unseen worker note.
		state.ResumeSentMsgCount = state.ResumeInputMsgCount
	}
	applyCLIReplyState(&target, state)
	toolDelta := target.ToolCallCount - s.ToolCallCount
	wal := cliReplyWAL{Version: cliReplyWALVersion, TxnID: m.ID, Message: m, State: state}
	wal, err = d.reserveActivitySequence(dir, wal, 1, int64(toolDelta))
	if err != nil {
		unlock()
		return m, fmt.Errorf("prepare CLI reply recovery: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnPrepared); err != nil {
			unlock()
			return m, err
		}
	}
	if err := writeDurableMessages(dir, targetMsgs); err != nil {
		unlock()
		return m, fmt.Errorf("persist CLI reply transcript: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnMessage); err != nil {
			unlock()
			return m, err
		}
	}
	if err := writeDurableSessionHeader(dir, target); err != nil {
		unlock()
		return m, fmt.Errorf("persist CLI reply state: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnHeader); err != nil {
			unlock()
			return m, err
		}
	}
	sig := cliReplyActivitySignal(wal, target)
	if err := persistCLIReplyActivity(dir, sig); err != nil {
		unlock()
		return m, fmt.Errorf("persist CLI reply activity: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnActivity); err != nil {
			unlock()
			return m, err
		}
	}
	if err := durableRemove(walPath); err != nil {
		unlock()
		return m, fmt.Errorf("retire CLI reply recovery: %w", err)
	}
	if d.cliReplyTxnHook != nil {
		if err := d.cliReplyTxnHook(cliReplyTxnRetired); err != nil {
			unlock()
			return m, err
		}
	}
	d.mu.Lock()
	d.messages[m.SessionID] = targetMsgs
	d.sessions[target.ID] = target
	d.markMutatedLocked()
	d.mu.Unlock()
	unlock()
	d.deliverRecoveredCLIReplyActivity(recovered, m.SessionID)
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		slog.Error("durable CLI reply activity delivery deferred", "component", "db", "session", m.SessionID, "event", sig.EventID, "error", err)
	}
	return m, nil
}

// addParticipant appends an agent id to a session's participant roster when it is
// a real, not-yet-present agent participant. Only AuthorAgent ids join: the human
// "user", the broadcast marker "*", and empty ids are implicit and never stored.
func addParticipant(list []string, kind, id string) []string {
	if kind != AuthorAgent || id == "" || id == UserParticipantID || id == BroadcastRecipientID {
		return list
	}
	if slices.Contains(list, id) {
		return list
	}
	return append(list, id)
}

// appendMessageLine appends a single encoded message line to a session's
// transcript file, creating it on the first message (a session's directory is
// made at creation time, but messages.jsonl only appears once it has one).
//
// The caller must hold the session's transcript lock (or be boot, which is
// single-threaded). It deliberately does NOT require d.mu — that is the whole
// point of the split: the disk write happens outside the global lock.
func (d *DB) appendMessageLine(sessionID string, m Message) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return err
	}
	path := d.dir(dirSessions, sessionID, sessionMsgsFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// DeleteMessage removes a single message from a session by id and rewrites the
// session's JSONL file. Returns ErrNotFound if the session or message is absent.
func (d *DB) DeleteMessage(ctx context.Context, sessionID, messageID string) error {
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return err
	}
	release, err := d.pinTranscript(sessionID)
	if err != nil {
		tl.Unlock()
		return err
	}
	defer func() {
		release(true)
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	msgs := d.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	remaining := append(append([]Message(nil), msgs[:idx]...), msgs[idx+1:]...)
	if msgs[idx].Role == "assistant" {
		s.ToolCallCount = max(0, s.ToolCallCount-countToolSteps(msgs[idx].Steps))
	}
	if idx < s.SummaryMsgCount {
		s.Summary = ""
		s.SummaryMsgCount = 0
		s.CompactionCount = 0
	}
	if s.MessageCount > 0 {
		s.MessageCount--
	}
	return d.persistTranscriptEditLocked(s, remaining)
}

// DeleteMessagesFrom removes the message with the given id and every message
// after it (a conversation "rewind" back to a checkpoint), then rewrites the
// session's JSONL file. Returns the number of messages removed, or ErrNotFound
// if the session or message is absent. File changes made by past turns are NOT
// reverted — this only truncates the transcript.
func (d *DB) DeleteMessagesFrom(ctx context.Context, sessionID, messageID string) (int, error) {
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return 0, err
	}
	release, err := d.pinTranscript(sessionID)
	if err != nil {
		tl.Unlock()
		return 0, err
	}
	defer func() {
		release(true)
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return 0, ErrNotFound
	}
	msgs := d.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, ErrNotFound
	}
	removed := len(msgs) - idx
	// A capacity fence alone still retains removed payloads.
	remaining := append([]Message(nil), msgs[:idx]...)
	s.MessageCount = idx
	// Recompute the lifetime tool counter over the retained messages so a truncate
	// (rewind) rolls it back in step with MessageCount.
	s.ToolCallCount = 0
	for _, m := range msgs[:idx] {
		if m.Role == "assistant" {
			s.ToolCallCount += countToolSteps(m.Steps)
		}
	}
	// If the truncation point falls before the summarized boundary, the rolling
	// summary now describes messages that no longer exist. Reset it so the next
	// turn re-derives context from the (shorter) live transcript instead of a
	// stale summary. Loud on purpose — we do not keep a dangling summary.
	if s.SummaryMsgCount > idx {
		s.Summary = ""
		s.SummaryMsgCount = 0
		// The fold chain that produced that summary is gone with it; the next fold
		// starts a fresh one, so its ordinal must start over too.
		s.CompactionCount = 0
	}
	if err := d.persistTranscriptEditLocked(s, remaining); err != nil {
		return 0, err
	}
	return removed, nil
}

// ListMessages returns messages for a session in chronological order.
func (d *DB) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	defer tl.Unlock()
	release, err := d.pinTranscript(sessionID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer release(false)
	d.mu.RLock()
	defer d.mu.RUnlock()
	msgs := d.messages[sessionID]
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out, nil
}
