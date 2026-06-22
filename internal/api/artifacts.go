package api

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/tools"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
)

// artifactDeliverableGuidance is the always-on instruction (kept in the static
// prompt prefix) that makes "produce a file/document" requests surface as
// artifacts by default — the user expects deliverables to open in the Artifacts
// screen, not be buried in chat or written only via an ad-hoc script.
const artifactDeliverableGuidance = "# Deliverables → Artifacts\n" +
	"When asked to produce a file, document, dataset, report, spreadsheet, diagram or code module, write it out with your file tool (write_file / Write) — files are captured as artifacts automatically. " +
	"If you can't write a file but have create_artifact, call it with the full content. " +
	"Don't deliver substantial output only as inline chat text or via an ad-hoc shell command (that bypasses artifact capture)."

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
		b.WriteString("\n")
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
		Origin:    "tool",
	})
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	return toArtifactRef(a), nil
}

func (s artifactSink) UpdateArtifact(ctx context.Context, id, content string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactContent(ctx, id, content)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	return toArtifactRef(a), nil
}

func toArtifactRef(a db.Artifact) tools.ArtifactRef {
	return tools.ArtifactRef{ID: a.ID, Title: a.Title, Kind: a.Kind}
}

// attachmentArtifactKind maps a chat attachment's coarse kind to an artifact
// kind so the right renderer is used in the Artifacts screen.
func attachmentArtifactKind(k string) string {
	switch k {
	case "image":
		return db.ArtifactImage
	case "video":
		return db.ArtifactVideo
	case "audio":
		return db.ArtifactAudio
	case "code":
		return db.ArtifactCode
	case "text":
		return db.ArtifactText
	default: // pdf | office | archive | file | unknown
		return db.ArtifactFile
	}
}

// captureAttachmentArtifacts records each chat attachment as an artifact so every
// file added to a session lands in the Artifacts screen (origin "chat"), tagged
// with its origin session. Deduped by relPath; best-effort (never breaks a turn).
func (s *Server) captureAttachmentArtifacts(ctx context.Context, database *db.DB, sessionID, agentID string, atts []db.Attachment) {
	for _, a := range atts {
		if a.RelPath == "" {
			continue
		}
		kind := attachmentArtifactKind(a.Kind)
		if _, err := database.UpsertAttachmentArtifact(ctx, sessionID, agentID, a.RelPath, a.Name, kind); err != nil {
			s.logger.Warn("attachment artifact capture failed", "name", a.Name, "error", err)
		}
	}
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

// handleGetArtifact returns one artifact.
func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	a, err := ws(r).DB.GetArtifact(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "artifact not found") {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type createArtifactReq struct {
	SessionID  string `json:"sessionId"`
	AgentID    string `json:"agentId"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Language   string `json:"language"`
	Content    string `json:"content"`
	SourcePath string `json:"sourcePath"` // workspace-relative path for media/file kinds
	Origin     string `json:"origin"`     // chat | manual | agent | tool
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
	origin := req.Origin
	if origin == "" {
		origin = "manual"
	}
	a, err := ws(r).DB.CreateArtifact(r.Context(), db.Artifact{
		SessionID:  req.SessionID,
		AgentID:    req.AgentID,
		Title:      req.Title,
		Kind:       req.Kind,
		Language:   req.Language,
		Content:    req.Content,
		SourcePath: req.SourcePath,
		Origin:     origin,
	})
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type updateArtifactReq struct {
	// Content (when present) overwrites the body in place.
	Content *string `json:"content"`
	// Metadata edits.
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
}

// handleUpdateArtifact overwrites content and/or edits metadata in place.
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
		if _, err := database.UpdateArtifactContent(ctx, id, *req.Content); writeDBError(w, err, "artifact not found") {
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

// artifactDiskPath returns the absolute on-disk path an artifact maps to: the
// source file it mirrors (media/file kinds, resolved under the workspace sandbox
// when relative), otherwise the artifact's own store JSON.
func artifactDiskPath(wsp *workspace.Workspace, a db.Artifact) string {
	// Prefer the externalised content file (text kinds), then the mirrored source
	// file (media), then the artifact's own store JSON.
	rel := a.ContentFile
	if rel == "" {
		rel = a.SourcePath
	}
	if rel != "" {
		if filepath.IsAbs(rel) {
			return rel
		}
		return filepath.Join(wsp.DataDir, "workspace", filepath.FromSlash(rel))
	}
	return filepath.Join(wsp.DataDir, "store", "artifacts", a.ID+".json")
}

// handleArtifactPath returns the artifact's on-disk path and its containing
// folder, without side effects (used by the copy-path button).
//
// GET /api/artifacts/{id}/path
func (s *Server) handleArtifactPath(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	a, err := wsp.DB.GetArtifact(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "artifact not found") {
		return
	}
	path := artifactDiskPath(wsp, a)
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "dir": filepath.Dir(path)})
}

// handleRevealArtifact opens the folder containing the artifact's file in the OS
// file manager (Windows: Explorer) and returns the file path.
//
// POST /api/artifacts/{id}/reveal
func (s *Server) handleRevealArtifact(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	a, err := wsp.DB.GetArtifact(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "artifact not found") {
		return
	}
	path := artifactDiskPath(wsp, a)
	dir := filepath.Dir(path)
	if err := exec.CommandContext(r.Context(), "explorer.exe", dir).Start(); err != nil {
		// explorer.exe returns a non-zero exit code even on success; only a
		// failure to *start* the process is a real error.
		s.logger.Warn("reveal artifact folder failed", "id", a.ID, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "dir": dir})
}
