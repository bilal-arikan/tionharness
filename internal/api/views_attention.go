package api

// The Explorer map's attention layer (_Docs/94 §11, _Docs/68 §7.2): which nodes
// a human should look at right now, and the status strip's counters. Both are
// derived from the awareness open-loop scan — the same facts the brief and the
// pulse give an agent — so the map and the agent never disagree about what is
// stuck, waiting or failed. Computed per request: every input is a store read
// or a small persisted index, which keeps it correct after a restart without a
// second index to maintain.

import (
	"context"
	"net/http"
	"time"

	"github.com/bilal-arikan/tionharness/internal/awareness"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/view"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// graphAttention builds the attention map and the status counters. pendingAsks
// are the sessions parked on an interactive prompt (interactionStore): the
// durable asks the open-loop scan sees are the headless path, so both feed the
// same "waiting for you" marker.
func graphAttention(ctx context.Context, wsp *workspace.Workspace, running map[string]bool, pendingAsks map[string]string, now time.Time) (map[string]view.GraphAttention, *view.GraphStatus) {
	settings := awareness.DefaultSettings()
	var digests []awareness.DigestIndexEntry
	var noteStore *notes.Store
	if rt := wsp.Runtime; rt != nil {
		if svc := rt.Awareness(); svc != nil {
			settings = svc.Settings()
			digests = svc.AllDigests(0)
		}
		noteStore = rt.Notes()
	}
	loops := awareness.CollectOpenLoops(ctx, wsp.DB, settings, now, "")

	attention := map[string]view.GraphAttention{}
	mark := func(ref view.Ref, level, reason string, at int64) {
		key := ref.String()
		cur := attention[key]
		cur.Reasons = append(cur.Reasons, reason)
		if attentionRank(level) > attentionRank(cur.Level) {
			cur.Level = level
		}
		if at > cur.At {
			cur.At = at
		}
		attention[key] = cur
	}
	sessionRef := func(id string) view.Ref { return view.Ref{Kind: view.KindSession, ID: id} }
	waiting := map[string]bool{}
	for _, id := range loops.WaitingAsks {
		waiting[id] = true
	}
	for id := range pendingAsks {
		waiting[id] = true
	}
	for id := range waiting {
		mark(sessionRef(id), view.AttentionWarn, view.ReasonWaitingAsk, 0)
	}
	for _, id := range loops.Stuck {
		mark(sessionRef(id), view.AttentionDanger, view.ReasonStuck, 0)
	}
	for _, id := range loops.Blocked {
		mark(sessionRef(id), view.AttentionWarn, view.ReasonBlocked, 0)
	}
	for _, t := range loops.StaleCards {
		mark(view.Ref{Kind: view.KindBoard, ID: view.BoardRefID, Sub: t.ID}, view.AttentionWarn, view.ReasonStaleCard, t.UpdatedAt)
	}
	for _, t := range loops.FailedCards {
		mark(view.Ref{Kind: view.KindBoard, ID: view.BoardRefID, Sub: t.ID}, view.AttentionDanger, view.ReasonFailedCard, t.UpdatedAt)
	}
	// Tool errors come from the session's last digest: a closed session whose
	// tail piled up errors is worth a look even though nothing is "open".
	var lastDigest int64
	for _, d := range digests {
		if d.At > lastDigest {
			lastDigest = d.At
		}
		if d.Errors > 0 && !running[d.SessionID] {
			mark(sessionRef(d.SessionID), view.AttentionNotice, view.ReasonToolErrors, d.At)
		}
	}

	status := &view.GraphStatus{
		At:           now.Unix(),
		Running:      len(running),
		Waiting:      len(waiting),
		Stuck:        len(loops.Stuck) + len(loops.Blocked),
		FailedCards:  len(loops.FailedCards),
		FailedRuns:   len(loops.FailedRuns),
		Stale:        len(loops.StaleCards),
		LastDigestAt: lastDigest,
	}
	if noteStore != nil {
		y, m, d := now.Date()
		midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
		for _, n := range noteStore.List(notes.Filter{Since: midnight}) {
			if n.Created >= midnight {
				status.NotesToday++
			}
		}
	}
	return attention, status
}

// attentionRank orders the levels so a node with several reasons keeps the
// strongest one.
func attentionRank(level string) int {
	switch level {
	case view.AttentionDanger:
		return 3
	case view.AttentionWarn:
		return 2
	case view.AttentionNotice:
		return 1
	}
	return 0
}

// handleGetViewGraphLive serves the map's volatile layers without the
// structural walk:
//
//	GET /api/views/graph/live
//
// The Explorer re-pulls this on state events (a turn started, a digest landed,
// a session got stuck) and keeps its node/edge set as it is, so the physics
// never re-settles for a badge change. Structural events still re-pull the
// whole map.
func (s *Server) handleGetViewGraphLive(w http.ResponseWriter, r *http.Request) {
	p := s.viewProjector(r)
	live, meta, err := p.GraphLive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	running := s.liveSessions(ws(r)).RunningSet()
	attention, status := graphAttention(r.Context(), ws(r), running, s.interactions.pendingSessions(ws(r).ID), time.Now())
	if live == nil {
		live = []view.GraphLive{}
	}
	if meta == nil {
		meta = map[string]view.GraphMeta{}
	}
	times, err := p.GraphTimes(r.Context(), ws(r).DB.ViewReads())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if times == nil {
		times = map[string]view.GraphTimes{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"live":      live,
		"meta":      meta,
		"attention": attention,
		"times":     times,
		"status":    status,
	})
}
