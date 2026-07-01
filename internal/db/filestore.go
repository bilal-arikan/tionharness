package db

import "sort"

// This file holds the generic, entity-agnostic building blocks that the
// per-entity stores (task/hook/flow/schedule/mcp/...) delegate to. They keep the
// shared d.mu semantics intact — every entity still lives in its own map on DB
// and shares the single RWMutex — while eliminating the copy-pasted RLock/loop/
// append/sort and map-write/atomic-write scaffolding that used to be repeated in
// each store_*.go file.
//
// These are free functions (not methods on a generic EntityStore type) precisely
// because the maps share one mutex and one on-disk root; a per-store mutex would
// break that invariant. Passing the map explicitly keeps them type-safe.

// dbGet returns a copy of m[id] under the read lock, or ErrNotFound.
func dbGet[T any](d *DB, m map[string]T, id string) (T, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	v, ok := m[id]
	if !ok {
		var zero T
		return zero, ErrNotFound
	}
	return v, nil
}

// dbList returns every value in m under the read lock, stably sorted by less
// (pass nil to leave the order unspecified).
func dbList[T any](d *DB, m map[string]T, less func(a, b T) bool) []T {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]T, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	if less != nil {
		sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	}
	return out
}

// dbFilter returns the values in m matching keep, under the read lock, stably
// sorted by less (pass nil for unspecified order).
func dbFilter[T any](d *DB, m map[string]T, keep func(T) bool, less func(a, b T) bool) []T {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]T, 0, len(m))
	for _, v := range m {
		if keep(v) {
			out = append(out, v)
		}
	}
	if less != nil {
		sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	}
	return out
}

// dbPersistLocked stores v under id in both the in-memory map and on disk
// (<dirName>/<id>.json). The caller must already hold d.mu.
func dbPersistLocked[T any](d *DB, m map[string]T, dirName, id string, v T) error {
	m[id] = v
	return atomicWriteJSON(d.dir(dirName, id+".json"), v)
}

// dbDeleteLocked removes id from the map and its on-disk file. Returns
// ErrNotFound if the entity does not exist. The caller must already hold d.mu.
func dbDeleteLocked[T any](d *DB, m map[string]T, dirName, id string) error {
	if _, ok := m[id]; !ok {
		return ErrNotFound
	}
	delete(m, id)
	return removeFile(d.dir(dirName, id+".json"))
}
