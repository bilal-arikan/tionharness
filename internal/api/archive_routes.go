package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/bilal-arikan/tionharness/internal/archive"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// Archive endpoints shared by skills, artifacts, automations and agents.
// They mirror the kanban card's pair (POST /api/tasks/{id}/archive and
// /unarchive): archiving is a reversible hide — the entity keeps its
// configuration and history, leaves the default lists and the Map, and can be
// restored. One helper registers both routes for every entity so they answer
// identically:
//
//	POST {base}/archive     optional body {"archived": bool} (default true)
//	POST {base}/unarchive   no body
//
// Both reply {"id": <id>, "archived": <bool>}. A missing entity is 404, a
// refused archive (e.g. a system agent) is 409.

// archiveSetter flips one entity's archive flag. It must return db.ErrNotFound
// for an unknown id.
type archiveSetter func(r *http.Request, id string, archived bool) error

// archiveRoute describes one entity's archive pair.
type archiveRoute struct {
	// base is the entity path pattern ending in its id wildcard, e.g.
	// "/api/skills/{slug}"; param names that wildcard.
	base, param string
	// kind labels log lines and the not-found message ("skill").
	kind string
	// event is the workspace event type whose refresh signal reloads the
	// entity's screen ("" = none); view is the screen a toast links to.
	event, view string
	set         archiveSetter
}

// registerArchiveRoutes wires the archive/unarchive pair for one entity.
func (s *Server) registerArchiveRoutes(mux *http.ServeMux, rt archiveRoute) {
	handle := func(w http.ResponseWriter, r *http.Request, archived bool) {
		id := r.PathValue(rt.param)
		if err := rt.set(r, id, archived); writeArchiveError(w, err, rt.kind+" not found") {
			return
		}
		s.logger.Info(rt.kind+" archive toggled", "id", id, "archived", archived)
		if rt.event != "" {
			verb := "arşivden çıkarıldı"
			if archived {
				verb = "arşivlendi"
			}
			publishEntityChange(ws(r), rt.event, rt.kind+" "+verb+": "+id, "",
				map[string]string{"view": rt.view})
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "archived": archived})
	}
	mux.HandleFunc("POST "+rt.base+"/archive", func(w http.ResponseWriter, r *http.Request) {
		archived, err := optionalArchivedBody(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		handle(w, r, archived)
	})
	mux.HandleFunc("POST "+rt.base+"/unarchive", func(w http.ResponseWriter, r *http.Request) {
		handle(w, r, false)
	})
}

// optionalArchivedBody reads the archive endpoint's optional {"archived": bool}
// body. An empty body (or one without the field) means archive — the endpoint's
// name — so a bare POST archives; the body form is kept so existing callers
// that send {"archived": false} keep restoring.
func optionalArchivedBody(r *http.Request) (bool, error) {
	defer r.Body.Close()
	var req struct {
		Archived *bool `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		return false, err
	}
	if req.Archived == nil {
		return true, nil
	}
	return *req.Archived, nil
}

// writeArchiveError is writeDBError plus the archive-specific refusals that
// are the caller's mistake, not a server failure (409 Conflict).
func writeArchiveError(w http.ResponseWriter, err error, notFoundMsg string) bool {
	if errors.Is(err, db.ErrSystemAgentArchive) {
		writeError(w, http.StatusConflict, err.Error())
		return true
	}
	return writeDBError(w, err, notFoundMsg)
}

// archiveFilterQuery reads the shared `archived` list parameter (see
// archive.ParseFilter), answering 400 on a bad value. def is the entity's
// historical default so existing clients keep their view.
func archiveFilterQuery(w http.ResponseWriter, q url.Values, def archive.Filter) (archive.Filter, bool) {
	f, err := archive.ParseFilter(q.Get("archived"), def)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return def, false
	}
	return f, true
}
