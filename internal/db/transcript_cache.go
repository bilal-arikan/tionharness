package db

// Transcripts are cached independently of session headers. Disk remains the
// source of truth; eviction never writes or removes history. A pinned operation
// may temporarily exceed the budget, then releases the excess on completion.
const defaultTranscriptCacheBytes int64 = 16 << 20

type transcriptCacheEntry struct {
	pins    int
	used    uint64
	size    int64
	count   int
	summary TranscriptSummary
}

// pinTranscript requires the session transcript mutex, but never d.mu. File I/O
// is outside the global lock, so a cold history cannot stall unrelated sessions.
func (d *DB) pinTranscript(id string) (func(bool), error) {
	d.mu.Lock()
	if _, ok := d.sessions[id]; !ok {
		d.mu.Unlock()
		return nil, ErrNotFound
	}
	_, loaded := d.messages[id]
	if !loaded {
		d.mu.Unlock()
		msgs, err := readMessagesFile(d.dir(dirSessions, id, sessionMsgsFile))
		if err != nil {
			return nil, err
		}
		d.mu.Lock()
		d.messages[id] = msgs
	}
	if d.transcriptCache == nil {
		d.transcriptCache = make(map[string]*transcriptCacheEntry)
	}
	entry := d.transcriptCache[id]
	if entry == nil {
		entry = &transcriptCacheEntry{size: transcriptBytes(d.messages[id])}
		entry.summary.appendMessages(d.messages[id])
		entry.count = len(d.messages[id])
		d.transcriptCache[id] = entry
	}
	d.transcriptClock++
	entry.used = d.transcriptClock
	entry.pins++
	d.mu.Unlock()
	return func(changed bool) {
		d.mu.Lock()
		defer d.mu.Unlock()
		entry.pins--
		if changed {
			entry.size = transcriptBytes(d.messages[id])
			msgs := d.messages[id]
			if len(msgs) <= entry.count {
				entry.summary = TranscriptSummary{}
				entry.count = 0
			}
			entry.summary.appendMessages(msgs[entry.count:])
			entry.count = len(msgs)
		}
		d.evictTranscriptsLocked()
	}, nil
}

func transcriptBytes(msgs []Message) int64 {
	var size int64
	for i := range msgs {
		size += approxMessageBytes(&msgs[i])
	}
	return size
}

func (d *DB) evictTranscriptsLocked() {
	limit := d.transcriptCacheLimit
	if limit <= 0 {
		limit = defaultTranscriptCacheBytes
	}
	var size int64
	for _, entry := range d.transcriptCache {
		size += entry.size
	}
	for size > limit {
		var oldest string
		var candidate *transcriptCacheEntry
		for id, entry := range d.transcriptCache {
			if entry.pins == 0 && (candidate == nil || entry.used < candidate.used) {
				oldest, candidate = id, entry
			}
		}
		if candidate == nil {
			return
		}
		delete(d.messages, oldest)
		delete(d.transcriptCache, oldest)
		size -= candidate.size
	}
}
