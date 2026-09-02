package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Sidecar is a typed JSON file that lives NEXT TO an entity rather than inside
// the entity store proper — a per-session queue, a crash-recovery snapshot, a
// trajectory. Every sidecar in this package shares three needs that used to be
// re-implemented per file (inbox.json, inflight.json, prompt_epoch.json,
// progress/): an atomic tmp→rename write, an "absent is not an error" read, and
// a loud-but-safe answer to a file that no longer parses. Sidecar is that shared
// implementation; the trajectory store is its first client and the older files
// migrate to it opportunistically.
//
// Locking is the CALLER's: a Sidecar takes no store lock. Whoever owns the
// entity serialises its own writers (a per-key mutex, like transcriptLock) and
// must never hold d.mu while reading or writing a sidecar — the same order rule
// as the transcript lock (sidecar lock FIRST, d.mu SECOND).
//
// Corruption policy: a file that exists but does not decode is moved aside as
// <file>.corrupt-<unix> (never deleted — it may be the only copy of what it
// held), a DebugError event is recorded against the owning session when there is
// one, and Load returns a *SidecarCorruptError. The caller decides whether that
// is fatal for its operation; the next Load sees "absent". This is the inbox
// quarantine rule (_Docs/58) generalised.
type Sidecar[T any] struct {
	d         *DB
	path      string
	name      string // file name, used in log/event labels
	sessionID string // owning session for debug-journal attribution ("" = none)
}

// newSidecar binds a typed sidecar to an absolute path. sessionID attributes
// corruption events to a session's debug journal and may be empty.
func newSidecar[T any](d *DB, sessionID, path string) Sidecar[T] {
	return Sidecar[T]{d: d, path: path, name: filepath.Base(path), sessionID: sessionID}
}

// Path returns the sidecar's absolute file path.
func (s Sidecar[T]) Path() string { return s.path }

// SidecarCorruptError reports a sidecar that existed but did not decode. By the
// time it is returned the file has already been moved to Quarantined (empty when
// even that failed — see Err for the decode failure and the log for the rename
// failure).
type SidecarCorruptError struct {
	Path        string
	Quarantined string
	Err         error
}

func (e *SidecarCorruptError) Error() string {
	if e.Quarantined == "" {
		return fmt.Sprintf("sidecar %s is corrupt (could not be quarantined): %v", e.Path, e.Err)
	}
	return fmt.Sprintf("sidecar %s is corrupt; moved to %s: %v", e.Path, e.Quarantined, e.Err)
}

func (e *SidecarCorruptError) Unwrap() error { return e.Err }

// IsSidecarCorrupt reports whether err (or anything it wraps) is a
// *SidecarCorruptError.
func IsSidecarCorrupt(err error) bool {
	var ce *SidecarCorruptError
	return errors.As(err, &ce)
}

// Load reads the sidecar. ok=false with a nil error means the file is absent
// (the normal "nothing stored yet" case). A file that exists but does not decode
// is quarantined and reported as *SidecarCorruptError with ok=false; any other
// read failure is returned as-is.
func (s Sidecar[T]) Load() (T, bool, error) {
	var zero T
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return zero, false, s.quarantine(err)
	}
	return v, true, nil
}

// Save atomically writes v (tmp file + rename), creating parent directories.
func (s Sidecar[T]) Save(v T) error {
	return atomicWriteJSON(s.path, v)
}

// Clear removes the sidecar. A missing file is not an error.
func (s Sidecar[T]) Clear() error {
	return removeFile(s.path)
}

// Exists reports whether the file is present (decodable or not).
func (s Sidecar[T]) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// quarantine moves an undecodable sidecar aside, records the event and builds
// the error Load hands back.
func (s Sidecar[T]) quarantine(cause error) error {
	dest := s.path + ".corrupt-" + strconv.FormatInt(time.Now().Unix(), 10)
	cerr := &SidecarCorruptError{Path: s.path, Quarantined: dest, Err: cause}
	if err := os.Rename(s.path, dest); err != nil {
		cerr.Quarantined = ""
		slog.Error("corrupt sidecar could not be quarantined", "component", "db",
			"file", s.name, "path", s.path, "session", s.sessionID, "renameError", err, "parseError", cause)
	} else {
		slog.Error("corrupt sidecar quarantined", "component", "db",
			"file", s.name, "path", s.path, "quarantine", dest, "session", s.sessionID, "parseError", cause)
	}
	if s.d != nil && s.sessionID != "" {
		if aerr := s.d.AppendDebugEventGated(s.sessionID, DebugEvent{
			Type:   DebugError,
			Name:   "sidecar_corrupt",
			Detail: s.name + " was unreadable and was quarantined",
			Error:  cause.Error(),
			Err:    true,
		}); aerr != nil {
			slog.Error("record corrupt-sidecar debug event failed", "component", "db",
				"file", s.name, "session", s.sessionID, "error", aerr)
		}
	}
	return cerr
}
