package db

// MutationGen returns a counter that advances on every entity mutation the
// store commits (create/update/delete of any entity map, session header and
// transcript writes). Two equal readings mean nothing changed in between, which
// is what lets a caller reuse a derived view — a sorted list, the Explorer
// graph — instead of rebuilding it from the maps on every request.
//
// The counter only ever grows; it is not persisted and restarts from zero on
// every Open, so it must never be compared across processes.
func (d *DB) MutationGen() uint64 { return d.mutGen.Load() }

// markMutatedLocked advances MutationGen. Called at every site that writes an
// entity map; the caller holds d.mu (write) so the bump is ordered with the
// change it announces.
func (d *DB) markMutatedLocked() { d.mutGen.Add(1) }
