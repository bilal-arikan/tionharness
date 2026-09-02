// Failure lessons (self-healing, hata→ders döngüsü) — read/prune endpoints.
// Lessons are written only by the lesson reflector (internal/agent/lessons.go);
// the API exposes the workspace-wide store read-only plus a delete for pruning
// a stale or wrong lesson from the Settings UI.
package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func (s *Server) handleListLessons(w http.ResponseWriter, r *http.Request) {
	// ?limit=N caps the newest-first journal. The store has always accepted a
	// limit; the handler hardcoded 0 ("no limit"), so an external consumer had
	// no way to bound a file that only ever grows. 0 stays the default so the
	// existing UI keeps its full list.
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer, got "+strconv.Quote(raw))
			return
		}
		limit = n
	}
	lessons, err := ws(r).DB.ListLessons(limit)
	if writeDBError(w, err, "") {
		return
	}
	if lessons == nil {
		lessons = []db.Lesson{}
	}
	writeJSON(w, http.StatusOK, lessons)
}

func (s *Server) handleDeleteLesson(w http.ResponseWriter, r *http.Request) {
	if writeDBError(w, ws(r).DB.DeleteLesson(r.PathValue("id")), "lesson") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})
}
