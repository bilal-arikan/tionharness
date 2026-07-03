package tools

import (
	"crypto/sha256"
	"errors"
	"os"
	"sync"
	"time"
)

// ReadRecord captures what a Read tool observed for a file, so a later Edit/Write
// can detect that the file changed underneath the agent — edited by the user, a
// linter, another tool, or a concurrent turn — since it was last read. This mirrors
// Claude Code's readFileState freshness guard (FileEditTool/FileWriteTool).
type ReadRecord struct {
	ModTime time.Time // file mtime at read time (informational; the check uses Sum)
	Size    int64     // byte length observed at read time
	Sum     [32]byte  // sha256 of the FULL on-disk content at read time
	Partial bool      // the read was truncated (file exceeded the read cap)
}

// ReadTracker is a session-scoped, concurrency-safe map from absolute file path to
// the last ReadRecord observed for it. A nil *ReadTracker is a valid no-op value:
// every method tolerates it, so freshness enforcement is simply skipped when no
// tracker is wired (tests, catalog/preview builds, or when the guard is disabled).
type ReadTracker struct {
	mu sync.Mutex
	m  map[string]ReadRecord
}

// NewReadTracker constructs an empty tracker.
func NewReadTracker() *ReadTracker { return &ReadTracker{m: map[string]ReadRecord{}} }

// Record stores (or replaces) the freshness baseline for abs.
func (t *ReadTracker) Record(abs string, rec ReadRecord) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.m[abs] = rec
	t.mu.Unlock()
}

// Get returns the recorded baseline for abs and whether one exists.
func (t *ReadTracker) Get(abs string) (ReadRecord, bool) {
	if t == nil {
		return ReadRecord{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.m[abs]
	return rec, ok
}

// contentSum is the fingerprint compared to detect an out-of-band modification.
func contentSum(data []byte) [32]byte { return sha256.Sum256(data) }

// Freshness-guard error messages. Wording mirrors Claude Code so an agent trained
// on that behaviour reacts correctly (Read again, then retry the write).
const (
	errFileNotRead = "file has not been read yet — Read it first before editing it (the freshness guard needs a baseline to detect concurrent changes)"
	errFileStale   = "file has been modified since it was last read (by the user, a linter, or another tool) — Read it again before editing it"
)

// checkFreshness enforces the read-before-write / not-modified-since-read guard for
// a file that ALREADY EXISTS on disk, given its current on-disk content. A nil
// tracker disables the guard (returns nil), so callers without a wired tracker keep
// their historical unchecked behaviour. It returns a non-nil error — surfaced to the
// model — when the file was never read, or its content changed since the recorded
// read. The comparison is content-hash based (not mtime), which avoids false
// positives from spurious mtime bumps (cloud sync, antivirus) while still catching
// every real edit, even one that left the mtime untouched.
func checkFreshness(tracker *ReadTracker, abs string, current []byte) error {
	if tracker == nil {
		return nil
	}
	rec, ok := tracker.Get(abs)
	if !ok {
		return errors.New(errFileNotRead)
	}
	if contentSum(current) != rec.Sum {
		return errors.New(errFileStale)
	}
	return nil
}

// recordWritten refreshes the baseline after a tool authored new content, so an
// immediately-following Write/Edit sees THIS content as the last-read state (the
// agent just wrote it — it need not Read it back first). Best-effort mtime via stat.
func recordWritten(tracker *ReadTracker, abs string, content []byte) {
	if tracker == nil {
		return
	}
	var mod time.Time
	if info, err := os.Stat(abs); err == nil {
		mod = info.ModTime()
	}
	tracker.Record(abs, ReadRecord{
		ModTime: mod,
		Size:    int64(len(content)),
		Sum:     contentSum(content),
	})
}
