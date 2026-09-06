package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

// Session activity bouts for the Rota canvas (_Docs/78 §16).
//
//	GET /api/sessions/activity?ids=SES1,SES2&gap=600
//
// The canvas draws a session as one bar from createdAt to updatedAt, which
// hides the shape of an intermittent conversation. This endpoint answers "when
// was this session actually working" by folding the transcript's message
// timestamps into stretches; the canvas then draws one block per stretch.
//
// It is a batch read because the canvas asks for every visible lane at once,
// and it is deliberately NOT part of the session listing: the answer is only
// needed by one screen, and a per-session field would pay for it everywhere.
// Reading is cheap — the timestamps are already in the store's memory, so a
// call costs a scan and no allocation per message.
const (
	// How many sessions one call may ask about. The canvas shows far fewer
	// lanes than this; the cap is here so a hand-written URL cannot walk the
	// whole workspace transcript set in one request.
	activityMaxIDs = 200
	// Longest idle stretch a caller may declare as "still the same bout".
	activityMaxGapSec = 24 * 60 * 60
)

type sessionActivityResp struct {
	GapSec   int64                        `json:"gapSec"`
	Sessions map[string][]trajectory.Span `json:"sessions"`
}

func (s *Server) handleSessionActivity(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()

	ids := splitActivityIDs(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}
	if len(ids) > activityMaxIDs {
		writeError(w, http.StatusBadRequest, "too many ids (max "+strconv.Itoa(activityMaxIDs)+")")
		return
	}
	gap := trajectory.DefaultBoutGapSec
	if raw := r.URL.Query().Get("gap"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 || n > activityMaxGapSec {
			writeError(w, http.StatusBadRequest, "gap must be 1.."+strconv.Itoa(activityMaxGapSec)+" seconds")
			return
		}
		gap = n
	}

	out := make(map[string][]trajectory.Span, len(ids))
	for _, id := range ids {
		var times []int64
		err := wsp.DB.StreamMessages(ctx, id, func(m db.Message) bool {
			times = append(times, m.CreatedAt)
			return true
		})
		if err != nil {
			// An id the workspace does not hold is left out rather than failing
			// the batch: the canvas asks about the lanes it drew, and one that
			// was deleted mid-flight must not blank the whole picture.
			if errors.Is(err, db.ErrNotFound) {
				continue
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		spans := trajectory.Bouts(times, gap)
		if len(spans) == 0 {
			continue
		}
		out[id] = spans
	}
	writeJSON(w, http.StatusOK, sessionActivityResp{GapSec: gap, Sessions: out})
}

// splitActivityIDs parses the comma-separated ids parameter, dropping blanks
// and duplicates while keeping the caller's order.
func splitActivityIDs(raw string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
