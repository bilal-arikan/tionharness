// Startup load of the session store from disk: per-session directories, the legacy single-file layout migration, JSONL decoding and header reconciliation.
package db

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
)

// loadSessions reads every sessions/<id>/ directory: the header from
// session.json and the transcript from messages.jsonl, migrating a legacy
// combined session.jsonl on the way.
func (d *DB) loadSessions() error {
	entries, err := os.ReadDir(d.dir(dirSessions))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(d.dir(dirSessions), e.Name()))
		}
	}
	// The dominant boot cost: two cold file opens per session (session.json +
	// messages.jsonl), each taxed ~15 ms by the AV filter driver on Windows. Read
	// them concurrently (see loadpar.go) and populate the maps serially after.
	type loadedSession struct {
		s    Session
		msgs []Message
		skip bool // absent or headerless directory — not an error
	}
	loaded, err := parallelLoad(dirs, func(dir string) (loadedSession, error) {
		if err := d.recoverCLIReplyTransaction(dir); err != nil {
			if !errors.Is(err, ErrCLIReplyRecoveryDegraded) {
				return loadedSession{}, err
			}
			slog.Error("session CLI reply recovery degraded", "component", "db", "session_dir", dir, "error", err)
		}
		s, msgs, err := readSessionDir(dir)
		if err != nil {
			// A directory that vanished between ReadDir and the open is skipped, as
			// before; any other read/parse failure stays fatal.
			if os.IsNotExist(err) {
				return loadedSession{skip: true}, nil
			}
			return loadedSession{}, err
		}
		if s.ID == "" { // empty/headerless session — nothing usable
			return loadedSession{skip: true}, nil
		}
		return loadedSession{s: s, msgs: msgs}, nil
	})
	if err != nil {
		return err
	}
	for _, l := range loaded {
		if l.skip {
			continue
		}
		d.sessions[l.s.ID] = l.s
		d.markMutatedLocked()
		d.messages[l.s.ID] = l.msgs
	}
	return nil
}

// readSessionDir loads one session from the split layout, falling back to the
// legacy combined file (and converting it) when no header file is present.
func readSessionDir(dir string) (Session, []Message, error) {
	b, err := os.ReadFile(filepath.Join(dir, sessionHeaderFile))
	if os.IsNotExist(err) {
		return migrateLegacySession(dir)
	}
	if err != nil {
		return Session{}, nil, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, nil, err // header corruption is fatal
	}
	msgs, err := readMessagesFile(filepath.Join(dir, sessionMsgsFile))
	if err != nil {
		return Session{}, nil, err
	}
	return reconcileHeader(s, msgs), msgs, nil
}

// migrateLegacySession converts a combined session.jsonl (header on line 1,
// messages after) into the split layout. Write order is what makes it
// crash-safe: the transcript lands first, the header second — and the header is
// the marker the loader keys on — so a crash before that point simply leaves the
// legacy file in place for the next boot to redo. Dropping the legacy file last
// is best-effort; a leftover copy is inert once session.json exists.
func migrateLegacySession(dir string) (Session, []Message, error) {
	path := filepath.Join(dir, legacySessionFile)
	lines, err := readJSONLines(path)
	if err != nil {
		return Session{}, nil, err // includes IsNotExist → caller skips the dir
	}
	if len(lines) == 0 {
		return Session{}, nil, nil
	}
	var s Session
	if err := json.Unmarshal(lines[0], &s); err != nil {
		return Session{}, nil, err // header corruption is fatal
	}
	msgs, err := decodeMessages(lines[1:])
	if err != nil {
		return Session{}, nil, err
	}
	s = reconcileHeader(s, msgs)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return Session{}, nil, err
		}
	}
	if err := atomicWriteBytes(filepath.Join(dir, sessionMsgsFile), buf.Bytes()); err != nil {
		return Session{}, nil, err
	}
	buf.Reset()
	if err := enc.Encode(s); err != nil {
		return Session{}, nil, err
	}
	if err := atomicWriteBytes(filepath.Join(dir, sessionHeaderFile), buf.Bytes()); err != nil {
		return Session{}, nil, err
	}
	_ = os.Remove(path)
	return s, msgs, nil
}

// readMessagesFile reads a transcript file. A session with no messages yet has
// no file at all, which is not an error.
func readMessagesFile(path string) ([]Message, error) {
	lines, err := readJSONLines(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeMessages(lines)
}

// readJSONLines returns the file's non-empty lines, copied out of the scanner's
// reused buffer.
func readJSONLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large message lines
	var lines [][]byte
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		b := make([]byte, len(line))
		copy(b, line)
		lines = append(lines, b)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func decodeMessages(lines [][]byte) ([]Message, error) {
	msgs := make([]Message, 0, len(lines))
	for i, line := range lines {
		var m Message
		if err := json.Unmarshal(line, &m); err != nil {
			// A torn trailing line (crash mid-append) is tolerated by dropping it;
			// corruption on any earlier line is real and fatal.
			if i == len(lines)-1 {
				break
			}
			return nil, err
		}
		// Back-fill the participant fields for messages stored before the model
		// (idempotent once set), so consumers never see empty AuthorKind on legacy
		// transcripts. No disk rewrite — this is an in-memory projection.
		m.NormalizeParticipants()
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// reconcileHeader makes in-memory state authoritative over the stored header.
// The append hot path deliberately leaves the header's counters stale (it never
// rewrites it), so they are recomputed from the actual messages. The participant
// roster is rebuilt the same way — an agent that joined via the append path
// never reached the header — so it self-heals across a restart.
func reconcileHeader(s Session, msgs []Message) Session {
	// Lineage backfill for headers written before schema version 4: derive the
	// origin IN MEMORY so every loaded session answers Lineage() the same way a
	// new one does. The header is not rewritten for this — it lands on disk only
	// when some later mutation persists the row anyway.
	if s.Origin == nil {
		o := deriveOrigin(s)
		o.At = s.CreatedAt
		s.Origin = &o
	}
	s.MessageCount = len(msgs)
	s.ToolCallCount = 0
	for _, m := range msgs {
		s.Participants = addParticipant(s.Participants, m.AuthorKind, m.AuthorID)
		s.Participants = addParticipant(s.Participants, AuthorAgent, m.RecipientID)
		if m.Role == "assistant" {
			s.ToolCallCount += countToolSteps(m.Steps)
		}
	}
	if n := len(msgs); n > 0 && msgs[n-1].CreatedAt > s.UpdatedAt {
		s.UpdatedAt = msgs[n-1].CreatedAt
	}
	return s
}
