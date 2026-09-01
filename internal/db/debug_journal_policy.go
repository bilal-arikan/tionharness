package db

// debugJournalPolicy holds the user's debug-journal setting (settings.json:
// debugJournalEnabled / debugJournalCap) next to the store, so emit points that
// live below the agent runtime — the conversation manager, the API server's
// durability paths — obey the same gate the runtime's emitDebug does instead of
// writing unconditionally with a hard-coded cap.
//
// The zero value is "enabled with the default cap": a DB nobody configured keeps
// the historical behaviour, and only an explicit SetDebugJournal(false, …) turns
// the stream off.
type debugJournalPolicy struct {
	disabled bool
	cap      int
}

// SetDebugJournal records the resolved debug-journal setting for the store-level
// emit points. cap <= 0 selects DefaultDebugJournalCap. The API server calls this
// wherever it pushes the same values onto the runtime tunables, so both funnels
// read one source of truth.
func (d *DB) SetDebugJournal(enabled bool, cap int) {
	if d == nil {
		return
	}
	d.debugMu.Lock()
	d.debugPolicy = debugJournalPolicy{disabled: !enabled, cap: cap}
	d.debugMu.Unlock()
}

// DebugJournalEnabled reports whether debug events may be appended.
func (d *DB) DebugJournalEnabled() bool {
	if d == nil {
		return false
	}
	d.debugMu.Lock()
	defer d.debugMu.Unlock()
	return !d.debugPolicy.disabled
}

// DebugJournalCap returns the configured per-session event cap (the built-in
// default when unset).
func (d *DB) DebugJournalCap() int {
	if d == nil {
		return DefaultDebugJournalCap
	}
	d.debugMu.Lock()
	defer d.debugMu.Unlock()
	if d.debugPolicy.cap <= 0 {
		return DefaultDebugJournalCap
	}
	return d.debugPolicy.cap
}

// AppendDebugEventGated is AppendDebugEvent behind the user's setting: it writes
// nothing when the journal is disabled and otherwise applies the configured cap.
// Every emit point outside the agent runtime should call this rather than
// AppendDebugEvent, which takes the cap as an argument and always writes.
func (d *DB) AppendDebugEventGated(sessionID string, ev DebugEvent) error {
	if d == nil || !d.DebugJournalEnabled() {
		return nil
	}
	return d.AppendDebugEvent(sessionID, ev, d.DebugJournalCap())
}
