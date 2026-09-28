package api

import (
	"errors"
	"github.com/bilal-arikan/tionharness/internal/db"
	"net/http"
	"strconv"
)

func (s *Server) handleMessagePage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit < 1 || limit > 200 {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
		return
	}
	cursors := 0
	for _, key := range []string{"before", "after", "around", "start"} {
		if q.Get(key) != "" {
			cursors++
		}
	}
	if cursors > 1 {
		writeError(w, http.StatusBadRequest, "use only one message cursor")
		return
	}
	page, err := ws(r).DB.ListMessagePage(r.Context(), r.PathValue("id"), limit, q.Get("before"), q.Get("after"), q.Get("around"), q.Get("start"))
	if errors.Is(err, db.ErrMessageNotFound) {
		writeError(w, http.StatusNotFound, "message cursor not found")
		return
	}
	if writeDBError(w, err, "session not found") {
		return
	}
	for i := range page.Items {
		page.Items[i].Steps = trimStepsJSON(page.Items[i].Steps)
	}
	writeJSON(w, http.StatusOK, page)
}
