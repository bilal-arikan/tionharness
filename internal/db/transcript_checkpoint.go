package db

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// This disposable checkpoint accelerates startup without changing canonical
// session files. Older binaries ignore it; edits, append recovery, removal or
// corruption invalidate it and rebuild counters from the original transcript.
type fileStamp struct {
	Size     int64 `json:"size"`
	Modified int64 `json:"modified"`
}

func stampFile(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileStamp{}, nil
	}
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{Size: info.Size(), Modified: info.ModTime().UnixNano()}, nil
}

type transcriptCheckpoint struct {
	Stamp          fileStamp `json:"stamp"`
	Header         Session   `json:"header"`
	HasAttachments bool      `json:"hasAttachments,omitempty"`
}

type transcriptCheckpoints struct {
	Version int                             `json:"version"`
	Entries map[string]transcriptCheckpoint `json:"entries"`
}

func (d *DB) readTranscriptCheckpoints() map[string]transcriptCheckpoint {
	var cache transcriptCheckpoints
	if readJSONFile(d.dir("transcripts-cache.json"), &cache) == nil && cache.Version == 1 {
		return cache.Entries
	}
	return nil
}

func readSessionCheckpoint(dir string, cached transcriptCheckpoint) (Session, transcriptCheckpoint, error) {
	raw, err := os.ReadFile(filepath.Join(dir, sessionHeaderFile))
	var s Session
	var msgs []Message
	if os.IsNotExist(err) {
		s, msgs, err = migrateLegacySession(dir)
	} else if err == nil {
		err = json.Unmarshal(raw, &s)
	}
	if err != nil {
		return s, transcriptCheckpoint{}, err
	}
	stamp, err := stampFile(filepath.Join(dir, sessionMsgsFile))
	if err != nil {
		return s, transcriptCheckpoint{}, err
	}
	if cached.Header.ID == s.ID && s.ID != "" && cached.Stamp == stamp {
		// Only transcript-derived fields come from the checkpoint. Title, state,
		// summary, CLI state, and other mutable metadata stay header-authoritative.
		s.MessageCount = cached.Header.MessageCount
		s.ToolCallCount = cached.Header.ToolCallCount
		for _, p := range cached.Header.Participants {
			s.Participants = addParticipant(s.Participants, AuthorAgent, p)
		}
		s.UpdatedAt = max(s.UpdatedAt, cached.Header.UpdatedAt)
		if s.Origin == nil {
			o := deriveOrigin(s)
			o.At = s.CreatedAt
			s.Origin = &o
		}
		return s, cached, nil
	}
	if msgs == nil {
		msgs, err = readMessagesFile(filepath.Join(dir, sessionMsgsFile))
		if err != nil {
			return s, transcriptCheckpoint{}, err
		}
	}
	s = reconcileHeader(s, msgs)
	cache := transcriptCheckpoint{Stamp: stamp, Header: Session{ID: s.ID, MessageCount: s.MessageCount, ToolCallCount: s.ToolCallCount, Participants: s.Participants, UpdatedAt: s.UpdatedAt}}
	for _, m := range msgs {
		cache.HasAttachments = cache.HasAttachments || len(m.Attachments) > 0
	}
	return s, cache, nil
}
