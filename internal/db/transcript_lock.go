package db

import "sync"

// ---- per-session transcript lock ----
//
// Every write to a session's messages.jsonl is serialised by a mutex dedicated to
// that ONE session, instead of by the global store lock d.mu.
//
// Why: AddMessage used to hold d.mu (the write lock guarding every in-memory map
// in the store) across a synchronous file write. That made the hottest write path
// in the app — one append per streamed delta batch, for every live session — a
// global serialisation point, and it blocked unrelated readers (sidebar, activity
// polls, other agents' sessions) for the duration of a disk write. NTFS plus an
// antivirus filter driver makes that write far from free.
//
// LOCK ORDER, mandatory: transcript lock FIRST, d.mu SECOND. Nothing may acquire a
// transcript lock while holding d.mu — that inversion deadlocks against every
// transcript writer. In practice the transcript lock is only ever taken at the top
// of an exported method, before d.mu is touched at all.
//
// Boot is exempt: load() runs single-threaded before the DB is published, so
// recoverInflight and the layout migrations write transcripts directly.

// transcriptLock returns the mutex serialising writes to one session's transcript
// file. The mutex outlives any single call, so two goroutines appending to the
// same session get the same lock and therefore a defined file order.
func (d *DB) transcriptLock(sessionID string) *sync.Mutex {
	d.transcriptMusMu.Lock()
	defer d.transcriptMusMu.Unlock()
	if d.transcriptMus == nil {
		d.transcriptMus = map[string]*sync.Mutex{}
	}
	mu, ok := d.transcriptMus[sessionID]
	if !ok {
		mu = &sync.Mutex{}
		d.transcriptMus[sessionID] = mu
	}
	return mu
}

// dropTranscriptLock forgets a deleted session's mutex so the map does not grow
// without bound. A goroutine that already holds the old mutex keeps holding a
// valid one; it will simply find the session gone and return ErrNotFound. Session
// ids are never reused, so no later session can be handed a different mutex for
// the same id while an old holder is still running.
func (d *DB) dropTranscriptLock(sessionID string) {
	d.transcriptMusMu.Lock()
	defer d.transcriptMusMu.Unlock()
	delete(d.transcriptMus, sessionID)
}
