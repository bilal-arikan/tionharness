package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/awareness"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// Workspace memory + awareness endpoints (_Docs/94): the Notes screen reads and
// edits notes (including the archived / superseded / private ones the agents
// never see), lists session digests and shows what a session was told.

// registerNoteRoutes registers the notes + awareness routes.
func (s *Server) registerNoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notes", s.handleListNotes)
	mux.HandleFunc("GET /api/notes/stats", s.handleNoteStats)
	mux.HandleFunc("GET /api/notes/search", s.handleSearchNotes)
	mux.HandleFunc("POST /api/notes", s.handleCreateNote)
	mux.HandleFunc("GET /api/notes/{id}", s.handleGetNote)
	mux.HandleFunc("PUT /api/notes/{id}", s.handleUpdateNote)
	mux.HandleFunc("DELETE /api/notes/{id}", s.handleDeleteNote)
	mux.HandleFunc("GET /api/notes/{id}/expand", s.handleExpandNote)
	mux.HandleFunc("POST /api/notes/{id}/archive", s.handleArchiveNote)
	mux.HandleFunc("POST /api/notes/{id}/correct", s.handleCorrectNote)
	mux.HandleFunc("GET /api/awareness/digests", s.handleListDigests)
	mux.HandleFunc("GET /api/awareness/settings", s.handleGetAwarenessSettings)
	mux.HandleFunc("GET /api/sessions/{id}/digest", s.handleSessionDigest)
	mux.HandleFunc("GET /api/sessions/{id}/awareness", s.handleSessionAwareness)
}

func notesStore(w http.ResponseWriter, wsp *workspace.Workspace) (*notes.Store, bool) {
	st := wsp.Runtime.Notes()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "the notes store is unavailable in this workspace")
		return nil, false
	}
	return st, true
}

func noteFilterFromQuery(r *http.Request) notes.Filter {
	q := r.URL.Query()
	f := notes.Filter{
		Scope:           notes.Scope(strings.TrimSpace(q.Get("scope"))),
		Source:          strings.TrimSpace(q.Get("source")),
		Tag:             strings.TrimSpace(q.Get("tag")),
		SourceSession:   strings.TrimSpace(q.Get("session")),
		SourceAgent:     strings.TrimSpace(q.Get("agent")),
		IncludeArchived: q.Get("archived") == "true" || q.Get("all") == "true",
		IncludeRetired:  q.Get("retired") == "true" || q.Get("all") == "true",
		IncludePrivate:  true, // the human sees their own notes
	}
	if k := strings.TrimSpace(q.Get("kind")); k != "" {
		for _, part := range strings.Split(k, ",") {
			if part = strings.TrimSpace(part); part != "" {
				f.Kinds = append(f.Kinds, notes.Kind(part))
			}
		}
	}
	if since, err := strconv.ParseInt(q.Get("since"), 10, 64); err == nil && since > 0 {
		f.Since = since
	}
	return f
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	st, ok := notesStore(w, ws(r))
	if !ok {
		return
	}
	out := st.List(noteFilterFromQuery(r))
	if limit, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []notes.Note{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleNoteStats(w http.ResponseWriter, r *http.Request) {
	st, ok := notesStore(w, ws(r))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, st.Stats())
}

func (s *Server) handleSearchNotes(w http.ResponseWriter, r *http.Request) {
	st, ok := notesStore(w, ws(r))
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits := st.Search(r.URL.Query().Get("q"), noteFilterFromQuery(r), limit)
	if hits == nil {
		hits = []notes.Hit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	st, ok := notesStore(w, ws(r))
	if !ok {
		return
	}
	n, found := st.Resolve(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleExpandNote(w http.ResponseWriter, r *http.Request) {
	st, ok := notesStore(w, ws(r))
	if !ok {
		return
	}
	ex, err := st.Expand(r.PathValue("id"))
	if err != nil {
		writeNoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

// noteWriteDTO is the user-authored note shape (the Notes screen form).
type noteWriteDTO struct {
	Kind         notes.Kind       `json:"kind"`
	Title        string           `json:"title"`
	Body         string           `json:"body"`
	Scope        notes.Scope      `json:"scope"`
	Agents       []string         `json:"agents"`
	Projects     []string         `json:"projects"`
	Confidence   notes.Confidence `json:"confidence"`
	Verification string           `json:"verification"`
	Tags         []string         `json:"tags"`
	// Private is a pointer so a partial update that omits it keeps the stored value.
	Private *bool `json:"private"`
}

func (d noteWriteDTO) note() notes.Note {
	n := notes.Note{
		Kind: d.Kind, Title: d.Title, Body: d.Body, Scope: d.Scope, Agents: d.Agents, Projects: d.Projects,
		Confidence: d.Confidence, Verification: d.Verification, Tags: d.Tags,
	}
	if d.Private != nil {
		n.Private = *d.Private
	}
	return n
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	st, ok := notesStore(w, wsp)
	if !ok {
		return
	}
	in, ok := bindJSON[noteWriteDTO](w, r)
	if !ok {
		return
	}
	n := in.note()
	if n.Scope == "" {
		n.Scope = notes.ScopeWorkspace
	}
	if n.Confidence == "" {
		n.Confidence = notes.ConfidenceInferred
	}
	n.Source = notes.SourceUser
	out, err := st.Put(n)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	publishNotesChange(wsp, out, "Not eklendi")
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	st, ok := notesStore(w, wsp)
	if !ok {
		return
	}
	cur, found := st.Get(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	in, ok := bindJSON[noteWriteDTO](w, r)
	if !ok {
		return
	}
	// A user edit rewrites the note in place (it is theirs); provenance,
	// signature and the correction chain are kept.
	n := cur
	if in.Kind != "" {
		n.Kind = in.Kind
	}
	if in.Title != "" {
		n.Title = in.Title
	}
	if in.Body != "" {
		n.Body = in.Body
	}
	if in.Scope != "" {
		n.Scope = in.Scope
	}
	if in.Agents != nil {
		n.Agents = in.Agents
	}
	if in.Projects != nil {
		n.Projects = in.Projects
	}
	if in.Confidence != "" {
		n.Confidence = in.Confidence
	}
	if in.Verification != "" {
		n.Verification = in.Verification
	}
	if in.Tags != nil {
		n.Tags = in.Tags
	}
	if in.Private != nil {
		n.Private = *in.Private
	}
	out, err := st.Put(n)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	publishNotesChange(wsp, out, "Not güncellendi")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	st, ok := notesStore(w, wsp)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := st.Delete(id); err != nil {
		writeNoteError(w, err)
		return
	}
	publishNotesChange(wsp, notes.Note{ID: id}, "Not silindi")
	writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})
}

func (s *Server) handleArchiveNote(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	st, ok := notesStore(w, wsp)
	if !ok {
		return
	}
	in, ok := bindJSON[struct {
		Archived bool `json:"archived"`
	}](w, r)
	if !ok {
		return
	}
	out, err := st.SetArchived(r.PathValue("id"), in.Archived)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	publishNotesChange(wsp, out, "Not arşiv durumu değişti")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCorrectNote(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	st, ok := notesStore(w, wsp)
	if !ok {
		return
	}
	old, found := st.Get(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	in, ok := bindJSON[noteWriteDTO](w, r)
	if !ok {
		return
	}
	n := old
	if in.Title != "" {
		n.Title = in.Title
	}
	n.Body = in.Body
	if in.Confidence != "" {
		n.Confidence = in.Confidence
	}
	if in.Verification != "" {
		n.Verification = in.Verification
	}
	if in.Scope != "" {
		n.Scope = in.Scope
	}
	if in.Agents != nil {
		n.Agents = in.Agents
	}
	if in.Projects != nil {
		n.Projects = in.Projects
	}
	if in.Tags != nil {
		n.Tags = in.Tags
	}
	n.Source = notes.SourceCorrection
	n.Occurrences = 0
	out, err := st.Correct(old.ID, n)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	publishNotesChange(wsp, out, "Not düzeltildi")
	writeJSON(w, http.StatusCreated, out)
}

func writeNoteError(w http.ResponseWriter, err error) {
	var ve *notes.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, notes.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case strings.Contains(err.Error(), "archive it instead") || strings.Contains(err.Error(), "already superseded"):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func publishNotesChange(wsp *workspace.Workspace, n notes.Note, title string) {
	wsp.Runtime.Emit(events.Event{
		Type:   events.TypeNotes,
		Level:  "info",
		Title:  title,
		Body:   n.Title,
		Target: map[string]string{"view": "notes", "noteId": n.ID},
	})
}

// ---- awareness ---------------------------------------------------------------

func awarenessService(w http.ResponseWriter, wsp *workspace.Workspace) (*awareness.Service, bool) {
	svc := wsp.Runtime.Awareness()
	if svc == nil {
		writeError(w, http.StatusServiceUnavailable, "the awareness layer is unavailable in this workspace")
		return nil, false
	}
	return svc, true
}

func (s *Server) handleListDigests(w http.ResponseWriter, r *http.Request) {
	svc, ok := awarenessService(w, ws(r))
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out := svc.AllDigests(limit)
	if out == nil {
		out = []awareness.DigestIndexEntry{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetAwarenessSettings(w http.ResponseWriter, r *http.Request) {
	svc, ok := awarenessService(w, ws(r))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, svc.Settings())
}

func (s *Server) handleSessionDigest(w http.ResponseWriter, r *http.Request) {
	svc, ok := awarenessService(w, ws(r))
	if !ok {
		return
	}
	d, found := svc.LoadDigest(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "no digest recorded for this session yet")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleSessionAwareness(w http.ResponseWriter, r *http.Request) {
	svc, ok := awarenessService(w, ws(r))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, svc.Seen(r.PathValue("id")))
}
