package db

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

const debugCLICompactionDedupe = "_cli_compaction_dedupe"

func cliSessionFingerprint(id string) string {
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("tionharness:cli-session:v1:" + id))
	return hex.EncodeToString(sum[:12])
}

// AppendCLICompactionEvent is the provider-to-journal adapter. The provider
// event contract excludes raw process output and prompt/tool payloads.
func (d *DB) AppendCLICompactionEvent(sessionID, agentID string, ev providers.CLICompactionEvent) error {
	if d == nil || !d.DebugJournalEnabled() {
		return nil
	}
	return d.appendCLICompactionEvent(sessionID, agentID, ev, d.DebugJournalCap())
}

func (d *DB) appendCLICompactionEvent(sessionID, agentID string, ev providers.CLICompactionEvent, cap int) error {
	if d == nil || sessionID == "" {
		return nil
	}
	var retryable *bool
	if ev.Phase == providers.CLICompactionError {
		retryable = new(bool)
		*retryable = ev.Retryable
	}
	safeError := ""
	switch ev.Phase {
	case providers.CLICompactionError:
		safeError = "claude CLI native compaction failed"
	case providers.CLICompactionCancelled:
		safeError = "claude CLI native compaction cancelled"
	}
	lifecycle := DebugEvent{
		Type: DebugCLICompaction, AgentID: agentID,
		Phase: string(ev.Phase), Provider: ev.Provider,
		AttemptID: ev.AttemptID, Attempt: ev.Attempt, Signal: ev.Signal,
		DurationMs:               ev.DurationMs,
		CLISessionFingerprintIn:  cliSessionFingerprint(ev.CLISessionIDIn),
		CLISessionFingerprintOut: cliSessionFingerprint(ev.CLISessionIDOut),
		Retryable:                retryable, ErrorKind: ev.ErrorKind, Error: safeError, ExitCode: ev.ExitCode,
	}
	if ev.Phase != providers.CLICompactionSuccess {
		return d.AppendDebugEvent(sessionID, lifecycle, cap)
	}
	if ev.AttemptID == "" {
		return errors.New("CLI compaction success requires attempt id")
	}
	dedupeSum := sha256.Sum256([]byte(ev.Provider + "\x00" + ev.AttemptID + "\x00" + string(ev.Phase)))
	dedupeKey := hex.EncodeToString(dedupeSum[:])

	d.debugMu.Lock()
	defer d.debugMu.Unlock()
	records, err := readRawDebugRecords(d.debugPath(sessionID))
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.checkpoint && record.name == dedupeKey {
			return nil
		}
	}
	// Replace this provider's prior checkpoint. Compactions are serialized by
	// the provider parser/process contract, so callbacks from an older attempt
	// cannot arrive after a newer attempt has begun.
	kept := records[:0]
	for _, record := range records {
		if !record.checkpoint || record.provider != ev.Provider {
			kept = append(kept, record)
		}
	}
	records = kept
	nowMs := time.Now().UnixMilli()
	batch := []DebugEvent{
		{Type: debugCLICompactionDedupe, Provider: ev.Provider, Name: dedupeKey},
		lifecycle,
		{
			Type: DebugCompaction, AgentID: agentID, Name: "cli-native",
			Detail: "Claude CLI native compaction completed",
		},
	}
	if countVisibleDebugRecords(records) == 0 {
		if build, ok := currentDebugBuildEvent(); ok {
			batch = append([]DebugEvent{build}, batch...)
		}
	}
	for i := range batch {
		batch[i].Time = nowMs
		batch[i].SessionID = sessionID
		batch[i] = sanitizeDebugEvent(batch[i])
	}
	for _, event := range batch {
		raw, err := rawDebugRecordFromEvent(event)
		if err != nil {
			return err
		}
		records = append(records, raw)
	}
	return d.replaceRawDebugRecordsLocked(sessionID, records, cap+debugSuccessBatchHeadroom)
}

type rawDebugRecord struct {
	data       []byte
	checkpoint bool
	provider   string
	name       string
}

func readRawDebugRecords(path string) ([]rawDebugRecord, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []rawDebugRecord
	for len(data) > 0 {
		end := bytes.IndexByte(data, '\n')
		if end < 0 {
			end = len(data)
		} else {
			end++
		}
		line := append([]byte(nil), data[:end]...)
		records = append(records, rawDebugRecordFromLine(line))
		data = data[end:]
	}
	return records, nil
}

func rawDebugRecordFromLine(line []byte) rawDebugRecord {
	record := rawDebugRecord{data: line}
	var meta struct {
		Type     string `json:"type"`
		Provider string `json:"provider"`
		Name     string `json:"name"`
	}
	if json.Unmarshal(bytes.TrimSpace(line), &meta) == nil && meta.Type == debugCLICompactionDedupe {
		record.checkpoint = true
		record.provider = meta.Provider
		record.name = meta.Name
	}
	return record
}

func rawDebugRecordFromEvent(event DebugEvent) (rawDebugRecord, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(event); err != nil {
		return rawDebugRecord{}, err
	}
	return rawDebugRecordFromLine(buf.Bytes()), nil
}

func countVisibleDebugRecords(records []rawDebugRecord) int {
	n := 0
	for _, record := range records {
		if !record.checkpoint {
			n++
		}
	}
	return n
}

func trimRawDebugRecords(records []rawDebugRecord, visibleKeep int) ([]rawDebugRecord, int) {
	if visibleKeep < 0 {
		visibleKeep = 0
	}
	latestCheckpoint := map[string]int{}
	visible := 0
	for i, record := range records {
		if record.checkpoint {
			latestCheckpoint[record.provider] = i
		} else {
			visible++
		}
	}
	dropVisible := visible - visibleKeep
	if dropVisible < 0 {
		dropVisible = 0
	}
	out := make([]rawDebugRecord, 0, len(records)-dropVisible)
	seenVisible := 0
	keptVisible := 0
	for i, record := range records {
		if record.checkpoint {
			if latestCheckpoint[record.provider] == i {
				out = append(out, record)
			}
			continue
		}
		if seenVisible < dropVisible {
			seenVisible++
			continue
		}
		out = append(out, record)
		keptVisible++
	}
	return out, keptVisible
}

func (d *DB) replaceRawDebugRecordsLocked(sessionID string, records []rawDebugRecord, visibleKeep int) error {
	records, visible := trimRawDebugRecords(records, visibleKeep)
	var data bytes.Buffer
	for _, record := range records {
		if data.Len() > 0 && data.Bytes()[data.Len()-1] != '\n' {
			data.WriteByte('\n')
		}
		data.Write(record.data)
	}
	path := d.debugPath(sessionID)
	var err error
	if d.debugAtomicWrite != nil {
		err = d.debugAtomicWrite(path, data.Bytes())
	} else {
		err = durableAtomicWriteBytes(path, data.Bytes(), 0o600)
	}
	if err != nil {
		return err
	}
	d.debugCount[sessionID] = visible
	return nil
}
