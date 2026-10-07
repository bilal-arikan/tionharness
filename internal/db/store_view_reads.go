package db

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"sync"
)

// viewReadsFile is the singleton workspace document recording when an agent last
// read each Explorer map node (keyed by the view ref string, e.g. "session:SES9",
// "artifact:ART3", "board:board#T12").
const viewReadsFile = "view-reads.json"

// viewReadsCap bounds the ledger. Past it the oldest reads are dropped: the map's
// recency window only ever asks about recent reads, so the tail is worthless.
const viewReadsCap = 5000

// viewReadCoalesceSecs is the repeat-read resolution: the same agent reading the
// same node again within this many seconds writes nothing, so a turn that pages
// through one view does not rewrite the file per call.
const viewReadCoalesceSecs = 60

// ViewRead is one node's last agent read.
type ViewRead struct {
	At        int64  `json:"at"` // unix seconds
	AgentID   string `json:"agentId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

type viewReadLedger struct {
	mu     sync.Mutex
	loaded bool
	reads  map[string]ViewRead
}

// loadViewReadsLocked reads the document once (caller holds viewReads.mu). An
// absent or unreadable file starts an empty ledger: reads are telemetry, a lost
// file costs history, not correctness.
func (d *DB) loadViewReadsLocked() {
	l := &d.viewReads
	if l.loaded {
		return
	}
	l.loaded = true
	l.reads = map[string]ViewRead{}
	var m map[string]ViewRead
	if err := readJSONFile(d.dir(viewReadsFile), &m); err == nil {
		maps.Copy(l.reads, m)
	}
}

// RecordViewRead notes that agentID (in sessionID) read the map node ref now.
func (d *DB) RecordViewRead(ref, agentID, sessionID string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	at := now()
	l := &d.viewReads
	l.mu.Lock()
	defer l.mu.Unlock()
	d.loadViewReadsLocked()
	if prev, ok := l.reads[ref]; ok && prev.AgentID == agentID && at-prev.At < viewReadCoalesceSecs {
		return nil
	}
	l.reads[ref] = ViewRead{At: at, AgentID: agentID, SessionID: sessionID}
	if len(l.reads) > viewReadsCap {
		pruneViewReads(l.reads, viewReadsCap*9/10)
	}
	return atomicWriteJSON(d.dir(viewReadsFile), l.reads)
}

// ViewReads returns a copy of the ledger keyed by ref string.
func (d *DB) ViewReads() map[string]ViewRead {
	l := &d.viewReads
	l.mu.Lock()
	defer l.mu.Unlock()
	d.loadViewReadsLocked()
	return maps.Clone(l.reads)
}

// pruneViewReads keeps the keep newest entries.
func pruneViewReads(reads map[string]ViewRead, keep int) {
	keys := slices.Collect(maps.Keys(reads))
	slices.SortFunc(keys, func(a, b string) int {
		if c := cmp.Compare(reads[b].At, reads[a].At); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	for _, k := range keys[min(keep, len(keys)):] {
		delete(reads, k)
	}
}
