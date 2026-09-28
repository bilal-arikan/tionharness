package sessionhub

// trimCommitted releases payloads as soon as their replay window expires. An
// active turn may exceed the limit: none of its uncommitted events may be lost.
// Caller holds the hub mutex.
func (st *sessionState) trimCommitted(limit int) {
	excess := len(st.ring) - limit
	drop := 0
	for drop < excess && st.ring[drop].Seq <= st.committed {
		drop++
	}
	if drop == 0 {
		return
	}
	clear(st.ring[:drop])
	tail := st.ring[drop:]
	if cap(st.ring) > 2*max(limit, len(tail)) {
		st.ring = append([]Event(nil), tail...)
		clear(tail)
	} else {
		st.ring = tail
	}
}
