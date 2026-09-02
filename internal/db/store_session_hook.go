package db

// Session change ops delivered to the session hook (see SetSessionHook).
const (
	// SessionOpCreate: a session row was created (any creation path).
	SessionOpCreate = "create"
	// SessionOpState: Session.State changed (active ↔ archived).
	SessionOpState = "state"
	// SessionOpRunState: Session.RunState was written (a background turn ended).
	SessionOpRunState = "runstate"
	// SessionOpOrigin: Session.Origin gained its run id (SetSessionOriginRun).
	SessionOpOrigin = "origin"
	// SessionOpDelete: the session and its transcript were removed.
	SessionOpDelete = "delete"
)

// SessionChangeEvent describes one session lifecycle change. Session is the row
// AFTER the change (for delete, the row as it was just before removal). Prev*
// carry the field's previous value for the state/runstate ops so an observer can
// tell a real transition from a rewrite of the same value.
type SessionChangeEvent struct {
	SessionID    string
	Op           string
	Session      Session
	PrevState    string
	PrevRunState string
}

// SessionChangeFn observes session lifecycle changes. The store calls it after
// releasing its locks; implementations must return promptly (dispatch async) and
// may re-enter the DB.
type SessionChangeFn func(ev SessionChangeEvent)

// SetSessionHook registers (or clears, with nil) the session-change observer.
// Same contract as SetBoardHook: wired once at workspace boot, best-effort,
// never on the store's critical path. It backs the workspace event log and the
// trajectory projection (see _Docs/77 R1/R3); nothing in the store itself
// depends on a hook being present.
func (d *DB) SetSessionHook(fn SessionChangeFn) {
	d.sessionHookMu.Lock()
	d.sessionHook = fn
	d.sessionHookMu.Unlock()
}

// fireSessionHook dispatches a session-change event to the registered observer
// (if any). Called after the store lock — and the transcript lock — are released.
func (d *DB) fireSessionHook(ev SessionChangeEvent) {
	d.sessionHookMu.RLock()
	fn := d.sessionHook
	d.sessionHookMu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}
