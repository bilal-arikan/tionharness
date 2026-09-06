package db

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
)

// Configuration snapshots (_Docs/83 §4.2). The runtime computes a
// content-addressed picture of every optimizable surface; this store keeps
// each distinct picture once (evolution/snapshots/<hash>.json) plus an index
// of when it was first and last in force. Sessions carry the hash they were
// created under (Session.SnapshotHash), so nothing else has to be written on
// the hot path.

// SnapshotIndexEntry is one row of evolution/snapshots/index.json.
type SnapshotIndexEntry struct {
	Hash      string `json:"hash"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	// Prev is the hash in force right before this one was first seen ("" for
	// the first snapshot) — the edge of the history graph.
	Prev string `json:"prev,omitempty"`
	// Seq orders snapshots by first appearance even within one second.
	Seq int64 `json:"seq"`
}

type snapshotIndex struct {
	Current string                        `json:"current"`
	Entries map[string]SnapshotIndexEntry `json:"entries"`
}

const (
	dirEvolution      = "evolution"
	dirSnapshots      = "snapshots"
	snapshotIndexFile = "index.json"
)

var snapshotMu sync.Mutex

// SnapshotProvider returns the hash of the configuration currently in force.
type SnapshotProvider func() string

// SetSnapshotProvider registers the runtime's provider. Sessions created
// afterwards are stamped with its result.
func (d *DB) SetSnapshotProvider(p SnapshotProvider) {
	d.snapshotProv.Store(p)
}

// stampSnapshot fills Session.SnapshotHash from the provider when the caller
// left it empty. It MUST run before d.mu is taken: the runtime's provider reads
// agents, automations and tool config under the read lock, so calling it from
// inside a write-locked section would deadlock.
func (d *DB) stampSnapshot(s Session) Session {
	if s.SnapshotHash == "" {
		s.SnapshotHash = d.currentSnapshotHash()
	}
	return s
}

func (d *DB) currentSnapshotHash() string {
	if v := d.snapshotProv.Load(); v != nil {
		if p, ok := v.(SnapshotProvider); ok && p != nil {
			return p()
		}
	}
	return ""
}

func (d *DB) snapshotPath(hash string) string {
	return d.dir(dirEvolution, dirSnapshots, hash+".json")
}

func (d *DB) loadSnapshotIndexLocked() (snapshotIndex, error) {
	var idx snapshotIndex
	err := readJSONFile(d.dir(dirEvolution, dirSnapshots, snapshotIndexFile), &idx)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return snapshotIndex{}, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]SnapshotIndexEntry{}
	}
	return idx, nil
}

// SaveSnapshot records that the snapshot with the given hash (content v,
// any JSON-serialisable value) is now in force. The content is written only
// the first time the hash is seen; the index is updated every time.
func (d *DB) SaveSnapshot(ctx context.Context, hash string, v any, at int64) error {
	if hash == "" {
		return errors.New("snapshot: empty hash")
	}
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	idx, err := d.loadSnapshotIndexLocked()
	if err != nil {
		return err
	}
	e, seen := idx.Entries[hash]
	if !seen {
		if err := atomicWriteJSON(d.snapshotPath(hash), v); err != nil {
			return err
		}
		e = SnapshotIndexEntry{Hash: hash, FirstSeen: at, Prev: idx.Current, Seq: int64(len(idx.Entries)) + 1}
	}
	e.LastSeen = at
	idx.Entries[hash] = e
	idx.Current = hash
	return atomicWriteJSON(d.dir(dirEvolution, dirSnapshots, snapshotIndexFile), idx)
}

// GetSnapshot decodes one stored snapshot into v.
func (d *DB) GetSnapshot(ctx context.Context, hash string, v any) error {
	if hash == "" {
		return ErrNotFound
	}
	err := readJSONFile(d.snapshotPath(hash), v)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// ListSnapshots returns the index rows, oldest first, plus the current hash.
func (d *DB) ListSnapshots(ctx context.Context) ([]SnapshotIndexEntry, string, error) {
	snapshotMu.Lock()
	idx, err := d.loadSnapshotIndexLocked()
	snapshotMu.Unlock()
	if err != nil {
		return nil, "", err
	}
	out := make([]SnapshotIndexEntry, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		if out[i].FirstSeen != out[j].FirstSeen {
			return out[i].FirstSeen < out[j].FirstSeen
		}
		return out[i].Hash < out[j].Hash
	})
	return out, idx.Current, nil
}

// CountSessionsBySnapshot tallies live sessions per snapshot hash ("" = unstamped).
func (d *DB) CountSessionsBySnapshot(ctx context.Context) map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := map[string]int{}
	for _, s := range d.sessions {
		out[s.SnapshotHash]++
	}
	return out
}
