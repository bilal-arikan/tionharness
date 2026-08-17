package db

// Store footprint measurement.
//
// The whole store is loaded into RAM at Open (see db.load), and the transcripts
// dominate that: every session's messages.jsonl is parsed into []Message and
// kept for the process's lifetime, whether or not anything ever reads it. This
// file exposes that footprint so the lazy-loading work (Madde 5) has a
// before/after number instead of an opinion.
//
// LoadedSessions is deliberately part of the shape TODAY, when it is always
// equal to Sessions: it is the metric that will actually move once transcripts
// load on demand, and having it in place now means the same endpoint answers
// "how much did this help?" without changing its contract.

// StoreStats is a snapshot of one workspace store's in-memory footprint.
type StoreStats struct {
	// Sessions is how many sessions the store knows about.
	Sessions int `json:"sessions"`
	// LoadedSessions is how many of them currently hold a materialised
	// transcript in memory. Equal to Sessions while loading is eager.
	LoadedSessions int `json:"loadedSessions"`
	// Messages is the total number of messages resident in memory.
	Messages int `json:"messages"`
	// MessageBytes approximates the heap those messages retain. It counts the
	// backing arrays of the variable-length fields plus a fixed per-message
	// overhead — a comparison aid, not allocator accounting.
	MessageBytes int64 `json:"messageBytes"`

	Agents      int `json:"agents"`
	Tasks       int `json:"tasks"`
	Artifacts   int `json:"artifacts"`
	Flows       int `json:"flows"`
	Automations int `json:"automations"`

	// LoadPhaseMs is how many milliseconds each boot phase of db.Open took,
	// keyed by phase name. Captured once at Open and never updated, so it is a
	// property of this process's startup, not of the current state.
	LoadPhaseMs map[string]int64 `json:"loadPhaseMs,omitempty"`
}

// messageFixedBytes is the per-message cost that does not scale with content:
// the Message struct itself (~20 fields, mostly string headers) plus the slice
// element slot and map/GC bookkeeping. Approximate by design.
const messageFixedBytes = 384

// Stats returns a snapshot of the store's in-memory footprint. It walks every
// resident message, so it is a diagnostic call — not something to put on a hot
// path or a polling UI.
func (d *DB) Stats() StoreStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	st := StoreStats{
		Sessions:    len(d.sessions),
		Agents:      len(d.agents),
		Tasks:       len(d.tasks),
		Artifacts:   len(d.artifacts),
		Flows:       len(d.flows),
		Automations: len(d.automations),
	}
	if len(d.loadPhases) > 0 {
		st.LoadPhaseMs = make(map[string]int64, len(d.loadPhases))
		for name, dur := range d.loadPhases {
			st.LoadPhaseMs[name] = dur.Milliseconds()
		}
	}
	for _, msgs := range d.messages {
		if msgs == nil {
			// A session created but never written to holds a nil slice and retains
			// nothing; counting it as "loaded" would overstate the footprint the
			// lazy work is meant to remove.
			continue
		}
		st.LoadedSessions++
		st.Messages += len(msgs)
		for i := range msgs {
			st.MessageBytes += approxMessageBytes(&msgs[i])
		}
	}
	return st
}

// approxMessageBytes estimates one message's retained heap: every string's
// backing array plus the fixed struct overhead. Steps and ToolCalls are the
// fields that actually matter — a tool-heavy turn carries kilobytes of trace
// JSON there while Text is a sentence.
func approxMessageBytes(m *Message) int64 {
	n := int64(messageFixedBytes)
	n += int64(len(m.ID) + len(m.SessionID) + len(m.Role) + len(m.AgentID))
	n += int64(len(m.AuthorKind) + len(m.AuthorID) + len(m.RecipientID))
	n += int64(len(m.Text) + len(m.ToolCalls) + len(m.ReasoningContent) + len(m.Steps))
	n += int64(len(m.Model) + len(m.StopReason) + len(m.Origin))
	for i := range m.Attachments {
		a := &m.Attachments[i]
		n += int64(len(a.ID)+len(a.Name)+len(a.Mime)+len(a.Kind)+len(a.RelPath)+len(a.TextContent)) + 96
	}
	if m.Usage != nil {
		n += 64
	}
	if m.Feedback != nil {
		n += int64(len(m.Feedback.Note)) + 48
	}
	return n
}
