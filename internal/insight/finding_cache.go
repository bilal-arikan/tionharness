package insight

import (
	"os"
	"sync"
	"time"
)

// findingsFileCache remembers the last parse of each findings.jsonl, keyed by
// path and validated against the file's size + modification time. Every API
// request opened the store by re-reading and re-decoding the whole file, and
// the Insight screen lists findings on every open, filter change and scan
// poll; between two writes the file is byte-identical, so the decode is only
// repeated when a stat says the file changed. Writers refresh the entry after
// their atomic rename, so a process's own writes never even pay the re-read.
type findingsFileCache struct {
	mu      sync.Mutex
	entries map[string]findingsCacheEntry
}

type findingsCacheEntry struct {
	modTime time.Time
	size    int64
	items   []Finding
}

var findingsCache = &findingsFileCache{entries: map[string]findingsCacheEntry{}}

// load returns the findings at path, from the cache when the file is unchanged
// since the last read. A missing file is an empty store (and drops any stale
// entry, which is how Reset propagates). Callers get their own copy.
func (c *findingsFileCache) load(path string) ([]Finding, error) {
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		c.forget(path)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	e, ok := c.entries[path]
	c.mu.Unlock()
	if ok && e.size == st.Size() && e.modTime.Equal(st.ModTime()) {
		return cloneFindings(e.items), nil
	}
	items, err := readFindings(path)
	if err != nil {
		return nil, err
	}
	c.remember(path, st, items)
	return cloneFindings(items), nil
}

// remember stores items as the parse of path at the given stat.
func (c *findingsFileCache) remember(path string, st os.FileInfo, items []Finding) {
	c.mu.Lock()
	c.entries[path] = findingsCacheEntry{modTime: st.ModTime(), size: st.Size(), items: cloneFindings(items)}
	c.mu.Unlock()
}

// rememberWritten is called by writeFindings after the rename so the writer's
// in-memory slice becomes the cached parse without a re-read.
func (c *findingsFileCache) rememberWritten(path string, items []Finding) {
	st, err := os.Stat(path)
	if err != nil {
		c.forget(path)
		return
	}
	c.remember(path, st, items)
}

func (c *findingsFileCache) forget(path string) {
	c.mu.Lock()
	delete(c.entries, path)
	c.mu.Unlock()
}

// cloneFindings deep-copies the slice-valued and pointer-valued fields so a
// store mutating its copy (Upsert merges evidence ids, status changes stamp an
// AppliedEntity) can never write through into the shared cached items.
func cloneFindings(items []Finding) []Finding {
	if items == nil {
		return nil
	}
	out := make([]Finding, len(items))
	for i, f := range items {
		if f.EvidenceSessionIDs != nil {
			f.EvidenceSessionIDs = append([]string(nil), f.EvidenceSessionIDs...)
		}
		if f.AppliedEntity != nil {
			v := *f.AppliedEntity
			f.AppliedEntity = &v
		}
		if f.Proposal != nil {
			v := *f.Proposal
			f.Proposal = &v
		}
		if f.Evolution != nil {
			v := *f.Evolution
			f.Evolution = &v
		}
		out[i] = f
	}
	return out
}
