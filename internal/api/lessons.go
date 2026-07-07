// Failure lessons (self-healing, hata→ders döngüsü) — read/prune endpoints.
// Lessons are written only by the lesson reflector (internal/agent/lessons.go);
// the API exposes the workspace-wide store read-only plus a delete for pruning
// a stale or wrong lesson from the Settings UI.
package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func (s *Server) handleListLessons(w http.ResponseWriter, r *http.Request) {
	lessons, err := ws(r).DB.ListLessons(0)
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
