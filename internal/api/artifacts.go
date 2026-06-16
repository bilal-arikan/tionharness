package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/tools"
)

// artifactsContextBlock builds a system-prompt section listing the artifacts a
// session already has, so the agent can revise them with update_artifact (by id)
// instead of creating duplicates. Returns "" when the session has none. Kept in
// the dynamic (uncached) part of the prompt since it changes as artifacts grow.
func artifactsContextBlock(ctx context.Context, database *db.DB, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	arts, err := database.ListArtifacts(ctx, sessionID)
	if err != nil || len(arts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Artifacts in this session\n")
	b.WriteString("You have already created these artifacts. To revise one, call update_artifact with its id and the FULL new content — do not create a duplicate. Create a new artifact only for genuinely new content.\n")
	const max = 30
	for i, a := range arts {
		if i >= max {
			fmt.Fprintf(&b, "- … and %d more\n", len(arts)-max)
			break
		}
		fmt.Fprintf(&b, "- id=%s · %q · kind=%s", a.ID, a.Title, a.Kind)
		if a.Language != "" {
			b.WriteString("/" + a.Language)
		}
		fmt.Fprintf(&b, " · v%d\n", a.Version)
	}
	return strings.TrimSpace(b.String())
}

// artifactSink adapts a workspace DB into a tools.ArtifactSink for one chat
// turn, stamping every artifact with its origin session and creating agent.
type artifactSink struct {
	db        *db.DB
	sessionID string
	agentID   string
}

// newArtifactSink builds a sink bound to the given session/agent.
func newArtifactSink(database *db.DB, sessionID, agentID string) artifactSink {
	return artifactSink{db: database, sessionID: sessionID, agentID: agentID}
}

func (s artifactSink) CreateArtifact(ctx context.Context, title, kind, language, content string) (tools.ArtifactRef, error) {
	a, err := s.db.CreateArtifact(ctx, db.Artifact{
		SessionID: s.sessionID,
		AgentID:   s.agentID,
		Title:     title,
		Kind:      kind,
		Language:  language,
		Content:   content,
	})
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	return toArtifactRef(a), nil
}

func (s artifactSink) UpdateArtifact(ctx context.Context, id, content, note string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactContent(ctx, id, content, note)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	return toArtifactRef(a), nil
}

func toArtifactRef(a db.Artifact) tools.ArtifactRef {
	return tools.ArtifactRef{ID: a.ID, Title: a.Title, Kind: a.Kind, Version: a.Version}
}

// ---- HTTP handlers ----

// handleListArtifacts lists artifacts in the workspace, optionally filtered to a
// session via ?sessionId=. Newest-updated first.
func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	list, err := ws(r).DB.ListArtifacts(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleGetArtifact returns one artifact with its full revision history.
func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	a, err := ws(r).DB.GetArtifact(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "artifact not found") {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type createArtifactReq struct {
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Language  string `json:"language"`
	Content   string `json:"content"`
}

// handleCreateArtifact creates an artifact manually (from the UI).
func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	var req createArtifactReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	a, err := ws(r).DB.CreateArtifact(r.Context(), db.Artifact{
		SessionID: req.SessionID,
		AgentID:   req.AgentID,
		Title:     req.Title,
		Kind:      req.Kind,
		Language:  req.Language,
		Content:   req.Content,
	})
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type updateArtifactReq struct {
	// Content (when present) creates a new revision; Note is its change summary.
	Content *string `json:"content"`
	Note    string  `json:"note"`
	// Metadata edits (don't create a revision).
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
}

// handleUpdateArtifact revises content (archiving the prior version) and/or
// edits metadata. Content and metadata may be updated in the same call.
func (s *Server) handleUpdateArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateArtifactReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	database := ws(r).DB
	ctx := r.Context()
	if req.Title != "" || req.Kind != "" || req.Language != "" {
		if _, err := database.UpdateArtifactMeta(ctx, id, req.Title, req.Kind, req.Language); writeDBError(w, err, "artifact not found") {
			return
		}
	}
	if req.Content != nil {
		if _, err := database.UpdateArtifactContent(ctx, id, *req.Content, req.Note); writeDBError(w, err, "artifact not found") {
			return
		}
	}
	a, err := database.GetArtifact(ctx, id)
	if writeDBError(w, err, "artifact not found") {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleDeleteArtifact removes an artifact.
func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	if err := ws(r).DB.DeleteArtifact(r.Context(), r.PathValue("id")); writeDBError(w, err, "artifact not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
