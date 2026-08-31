package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

const (
	maxArtifactImageSourceBytes int64 = 20_971_520
	maxArtifactImageDimension         = 8_192
	maxArtifactImagePixels            = 40_000_000
)

var errArtifactImageTooLarge = errors.New("artifact image exceeds byte limit")

var (
	errArtifactImageHeaderInvalid      = errors.New("artifact image header is invalid")
	errArtifactImageDecodeFailed       = errors.New("artifact image decode failed")
	errArtifactImageDimensionExceeded  = errors.New("artifact image dimension exceeds limit")
	errArtifactImagePixelLimitExceeded = errors.New("artifact image pixel count exceeds limit")
)

const (
	sourceArtifactNotFound  = "SOURCE_ARTIFACT_NOT_FOUND — Kaynak görsel artifact bulunamadı."
	sourceArtifactNotImage  = "SOURCE_ARTIFACT_NOT_IMAGE — Yalnız image artifact düzenlenebilir."
	sourceArtifactForbidden = "SOURCE_ARTIFACT_FORBIDDEN — Bu kaynak görsele erişim izniniz yok."
)

// artifactDeliverableGuidance is the always-on instruction (kept in the static
// prompt prefix) telling the agent that artifacts are created DELIBERATELY: there
// is no automatic capture path — writing a file never registers an artifact by
// itself, so ordinary edits to project source files stay out of the Artifacts
// screen. Only the short rule of thumb lives here to keep the cached prefix small;
// the full rules (binary files, inline media, galleries) live in the
// `tionharness-deliverables` skill so they cost attention/tokens only when a
// deliverable is actually in play.
const artifactDeliverableGuidance = "# Deliverables → Artifacts\n" +
	"Writing a file does NOT create an artifact. When you produce a deliverable the user should keep (a document/dataset/report/standalone code file), register it DELIBERATELY by calling create_artifact (or the artifacts API) — do not assume a plain file write will surface it. Ordinary edits to project source files stay out of the Artifacts screen. " +
	"Content meant to be SEEN (a diagram, an image/video, a gallery) goes INLINE in your reply (markdown ![alt](path), a ```mermaid block). " +
	"For the full rules (binary files via sourcePath, inline media, galleries, updating by id), load the `tionharness-deliverables` skill before producing the deliverable."

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
	// emit publishes a workspace-scoped "artifact" change event so open windows
	// badge the Artifacts view / workspace label and (per prefs) raise a toast.
	// Optional (nil-safe) — the generic change-notification signal for agent-made
	// artifacts, the main "external notification" case.
	emit func(events.Event)
}

// newArtifactSink builds a sink bound to the given session/agent. emit may be nil
// (no change notification is published then).
func newArtifactSink(database *db.DB, sessionID, agentID string, emit func(events.Event)) artifactSink {
	return artifactSink{db: database, sessionID: sessionID, agentID: agentID, emit: emit}
}

// notifyArtifact publishes the generic artifact change event (best-effort).
func (s artifactSink) notifyArtifact(a db.Artifact, verb string) {
	if s.emit == nil {
		return
	}
	s.emit(events.Event{
		Type:  "artifact",
		Level: "info",
		Title: "Artifact " + verb + ": " + a.Title,
		Body:  a.Kind,
		Target: map[string]string{
			"view":       "artifacts",
			"artifactId": a.ID,
			"sessionId":  s.sessionID,
		},
	})
}

func (s artifactSink) CreateArtifact(ctx context.Context, spec tools.CreateArtifactSpec) (tools.ArtifactRef, error) {
	row := db.Artifact{
		SessionID: s.sessionID,
		AgentID:   s.agentID,
		Title:     spec.Title,
		Kind:      spec.Kind,
		Language:  spec.Language,
		Content:   spec.Content,
		Origin:    "tool",
	}
	// Media/file kinds carry a file path, not inline bytes: resolve it to a
	// workspace-relative path (copying the file in if it lives outside) so the
	// viewer can stream it and the artifact owns a stable copy.
	if spec.SourcePath != "" {
		rel, err := s.db.ImportMediaSource(s.sessionID, spec.SourcePath)
		if err != nil {
			return tools.ArtifactRef{}, err
		}
		row.SourcePath = rel
	}
	a, err := s.db.CreateArtifact(ctx, row)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	s.notifyArtifact(a, "oluşturuldu")
	return toArtifactRef(a), nil
}

func (s artifactSink) UpdateArtifact(ctx context.Context, id, content string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactContent(ctx, id, content)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	s.notifyArtifact(a, "güncellendi")
	return toArtifactRef(a), nil
}

// AppendPlanArtifact records an approved plan (ExitPlanMode) in this session's
// single rolling plan artifact. It is a TionHarness-specific capability used by the
// plan-approval bridge (callExitPlan), kept off the generic tools.ArtifactSink
// interface and reached there via a duck-typed assertion. Best-effort.
func (s artifactSink) AppendPlanArtifact(ctx context.Context, planMarkdown string) (tools.ArtifactRef, error) {
	a, err := s.db.AppendPlanArtifact(ctx, s.sessionID, s.agentID, planMarkdown)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	s.notifyArtifact(a, "güncellendi")
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

// handleListArtifacts lists artifacts in the workspace, optionally filtered via
// query params: sessionId (origin session), kind, origin, q (title substring,
// case-insensitive), archived (bool). With any of limit/offset/sort present the
// response is the standard {items,total,offset,limit,hasMore} envelope (same
// keys as the list_artifacts tool); without them it stays the legacy full
// unwrapped list so existing UI clients keep working.
func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sessionID := q.Get("sessionId")
	list, err := ws(r).DB.ListArtifacts(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}

	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := strings.TrimSpace(q.Get("kind"))
	origin := strings.TrimSpace(q.Get("origin"))
	search := strings.ToLower(strings.TrimSpace(q.Get("q")))
	archived, archivedGiven, err := boolQuery(q, "archived")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	matches := make([]db.Artifact, 0, len(list))
	for _, a := range list {
		if kind != "" && a.Kind != kind {
			continue
		}
		if origin != "" && a.Origin != origin {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(a.Title), search) {
			continue
		}
		if archivedGiven && a.Archived != *archived {
			continue
		}
		matches = append(matches, a)
	}

	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	less, err := tools.SortByField(matches, field, asc,
		func(a db.Artifact) int64 { return a.UpdatedAt },
		func(a db.Artifact) int64 { return a.CreatedAt },
		func(a db.Artifact) string { return a.Title },
		func(a db.Artifact) string { return a.ID },
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sort.SliceStable(matches, less)
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
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
	SessionID             string `json:"sessionId"`
	AgentID               string `json:"agentId"`
	Title                 string `json:"title"`
	Kind                  string `json:"kind"`
	Language              string `json:"language"`
	Content               string `json:"content"`
	SourcePath            string `json:"sourcePath"` // workspace-relative path for media/file kinds
	Origin                string `json:"origin"`     // chat | manual | agent | tool
	DerivedFromArtifactID string `json:"derivedFromArtifactId"`
}

func workspaceArtifactPath(wsp *workspace.Workspace, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	root := filepath.Clean(wsp.SandboxRoot())
	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
		root = resolvedRoot
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(abs)); parentErr == nil {
		abs = filepath.Join(resolvedParent, filepath.Base(abs))
	}
	within, err := filepath.Rel(root, abs)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}

func artifactImageMIME(header []byte) (string, string, bool) {
	if len(header) >= 8 && string(header[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png", ".png", true
	}
	if len(header) >= 3 && header[0] == 0xff && header[1] == 0xd8 && header[2] == 0xff {
		return "image/jpeg", ".jpg", true
	}
	if len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		return "image/webp", ".webp", true
	}
	return "", "", false
}

func validateArtifactImage(data []byte) (string, string, error) {
	mime, ext, ok := artifactImageMIME(data)
	if !ok {
		return "", "", errArtifactImageHeaderInvalid
	}
	var width, height int
	if mime == "image/webp" {
		width, height, ok = decodeWebPConfig(data)
		if !ok {
			return "", "", errArtifactImageDecodeFailed
		}
	} else {
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || "image/"+format != mime || config.Width <= 0 || config.Height <= 0 {
			return "", "", errArtifactImageDecodeFailed
		}
		width, height = config.Width, config.Height
	}
	if width > maxArtifactImageDimension || height > maxArtifactImageDimension {
		return "", "", errArtifactImageDimensionExceeded
	}
	if int64(width)*int64(height) > maxArtifactImagePixels {
		return "", "", errArtifactImagePixelLimitExceeded
	}
	if mime != "image/webp" {
		decoded, format, err := image.Decode(bytes.NewReader(data))
		if err != nil || "image/"+format != mime || decoded.Bounds().Dx() != width || decoded.Bounds().Dy() != height {
			return "", "", errArtifactImageDecodeFailed
		}
	}
	return mime, ext, nil
}

func decodeWebPConfig(data []byte) (int, int, bool) {
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || int64(binary.LittleEndian.Uint32(data[4:8]))+8 != int64(len(data)) {
		return 0, 0, false
	}
	var width, height int
	foundImage := false
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return 0, 0, false
		}
		kind := string(data[offset : offset+4])
		size := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start := offset + 8
		end64 := int64(start) + size
		if end64 > int64(len(data)) {
			return 0, 0, false
		}
		chunk := data[start:int(end64)]
		var chunkWidth, chunkHeight int
		switch kind {
		case "VP8 ":
			if len(chunk) < 10 || !bytes.Equal(chunk[3:6], []byte{0x9d, 0x01, 0x2a}) {
				return 0, 0, false
			}
			chunkWidth = int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff)
			chunkHeight = int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff)
			foundImage = true
		case "VP8L":
			if len(chunk) < 5 || chunk[0] != 0x2f {
				return 0, 0, false
			}
			bits := binary.LittleEndian.Uint32(chunk[1:5])
			chunkWidth = int(bits&0x3fff) + 1
			chunkHeight = int((bits>>14)&0x3fff) + 1
			foundImage = true
		case "VP8X":
			if len(chunk) != 10 {
				return 0, 0, false
			}
			chunkWidth = int(chunk[4]) | int(chunk[5])<<8 | int(chunk[6])<<16
			chunkHeight = int(chunk[7]) | int(chunk[8])<<8 | int(chunk[9])<<16
			chunkWidth++
			chunkHeight++
		}
		if chunkWidth > 0 && chunkHeight > 0 {
			if width != 0 && (width != chunkWidth || height != chunkHeight) {
				return 0, 0, false
			}
			width, height = chunkWidth, chunkHeight
		}
		offset = int(end64) + int(size&1)
		if offset > len(data) {
			return 0, 0, false
		}
	}
	return width, height, foundImage && width > 0 && height > 0
}

func writeArtifactImageValidationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errArtifactImageHeaderInvalid):
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_HEADER_INVALID — Görsel başlığı geçersiz veya dosya türüyle uyuşmuyor.")
	case errors.Is(err, errArtifactImageDimensionExceeded):
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_DIMENSION_EXCEEDED — Görsel boyutu 8192 px sınırını aşıyor.")
	case errors.Is(err, errArtifactImagePixelLimitExceeded):
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_PIXEL_LIMIT_EXCEEDED — Görsel 40 megapiksel sınırını aşıyor.")
	default:
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_DECODE_FAILED — Görsel açılamadı; dosya bozuk olabilir.")
	}
}

func readArtifactImageFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	if info.Size() > maxArtifactImageSourceBytes {
		return nil, errArtifactImageTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(f, maxArtifactImageSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxArtifactImageSourceBytes {
		return nil, errArtifactImageTooLarge
	}
	return data, nil
}

// handleArtifactSource serves an image only after atomically resolving its ID
// against the active workspace store and validating its confined source file.
func (s *Server) handleArtifactSource(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	a, err := wsp.DB.GetArtifact(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, sourceArtifactNotFound)
		return
	}
	if a.Kind != db.ArtifactImage || a.SourcePath == "" {
		writeError(w, http.StatusUnprocessableEntity, sourceArtifactNotImage)
		return
	}
	path, ok := workspaceArtifactPath(wsp, a.SourcePath)
	if !ok {
		writeError(w, http.StatusForbidden, sourceArtifactForbidden)
		return
	}
	data, err := readArtifactImageFile(path)
	if err != nil {
		if errors.Is(err, errArtifactImageTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE_BYTES — Görsel 20 MB sınırını aşıyor.")
		} else if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, sourceArtifactNotFound)
		} else {
			writeError(w, http.StatusForbidden, sourceArtifactForbidden)
		}
		return
	}
	mime, _, validationErr := validateArtifactImage(data)
	if validationErr != nil {
		writeArtifactImageValidationError(w, validationErr)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleCreateArtifact creates an artifact manually (from the UI).
func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[createArtifactReq](w, r)
	if !ok {
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
	wsp := ws(r)
	row := db.Artifact{
		SessionID:             req.SessionID,
		AgentID:               req.AgentID,
		Title:                 req.Title,
		Kind:                  req.Kind,
		Language:              req.Language,
		Content:               req.Content,
		SourcePath:            req.SourcePath,
		Origin:                origin,
		DerivedFromArtifactID: req.DerivedFromArtifactID,
	}
	var a db.Artifact
	var err error
	if req.DerivedFromArtifactID == "" {
		a, err = wsp.DB.CreateArtifact(r.Context(), row)
	} else {
		stagedPath, safe := workspaceArtifactPath(wsp, req.SourcePath)
		if !safe {
			writeError(w, http.StatusForbidden, sourceArtifactForbidden)
			return
		}
		stagedOwned := false
		defer func() {
			if !stagedOwned {
				_ = os.Remove(stagedPath)
			}
		}()
		data, readErr := readArtifactImageFile(stagedPath)
		if readErr != nil {
			if errors.Is(readErr, errArtifactImageTooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE_BYTES — Görsel 20 MB sınırını aşıyor.")
			} else if errors.Is(readErr, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, sourceArtifactNotFound)
			} else {
				writeError(w, http.StatusForbidden, sourceArtifactForbidden)
			}
			return
		}
		_, ext, validationErr := validateArtifactImage(data)
		if validationErr != nil {
			writeArtifactImageValidationError(w, validationErr)
			return
		}
		a, err = wsp.DB.CreateDerivedArtifact(r.Context(), row, func(derived *db.Artifact, parent db.Artifact) (string, func() error, error) {
			parentPath, parentSafe := workspaceArtifactPath(wsp, parent.SourcePath)
			if !parentSafe {
				return "", nil, os.ErrPermission
			}
			parentFile, parentErr := os.Open(parentPath)
			if parentErr != nil {
				return "", nil, parentErr
			}
			if closeErr := parentFile.Close(); closeErr != nil {
				return "", nil, closeErr
			}
			derived.SessionID = parent.SessionID
			rel := filepath.ToSlash(filepath.Join("artifacts", parent.SessionID, derived.ID+ext))
			target, safe := workspaceArtifactPath(wsp, rel)
			if !safe {
				return "", nil, errors.New("unsafe derived artifact target")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", nil, err
			}
			if err := os.Rename(stagedPath, target); err != nil {
				return "", nil, err
			}
			stagedOwned = true
			return rel, func() error { return os.Remove(target) }, nil
		})
	}
	if errors.Is(err, db.ErrArtifactParentNotFound) || errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, sourceArtifactNotFound)
		return
	}
	if errors.Is(err, db.ErrArtifactParentNotImage) {
		writeError(w, http.StatusUnprocessableEntity, sourceArtifactNotImage)
		return
	}
	if errors.Is(err, os.ErrPermission) {
		writeError(w, http.StatusForbidden, sourceArtifactForbidden)
		return
	}
	if writeDBError(w, err, "") {
		return
	}
	publishEntityChange(ws(r), "artifact", "Artifact oluşturuldu: "+a.Title, a.Kind,
		map[string]string{"view": "artifacts", "artifactId": a.ID, "sessionId": a.SessionID})
	writeJSON(w, http.StatusOK, a)
}

type updateArtifactReq struct {
	// Content (when present) overwrites the body in place.
	Content *string `json:"content"`
	// SourcePath (when present) repoints the artifact at a different file in
	// place; the previously referenced file is dropped.
	SourcePath *string `json:"sourcePath"`
	// Metadata edits.
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
}

// removeArtifactSourceFile drops a source file an artifact no longer references.
// Best-effort — a missing file is fine and a failure must not fail the request —
// but never silent: anything else is logged.
func (s *Server) removeArtifactSourceFile(wsp *workspace.Workspace, rel string) {
	abs, ok := workspaceArtifactPath(wsp, rel)
	if !ok {
		if s.logger != nil {
			s.logger.Warn("artifact source cleanup skipped: path outside workspace", "path", rel)
		}
		return
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) && s.logger != nil {
		s.logger.Warn("artifact source cleanup failed", "path", rel, "err", err)
	}
}

// handleUpdateArtifact overwrites content and/or edits metadata in place.
func (s *Server) handleUpdateArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[updateArtifactReq](w, r)
	if !ok {
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
	if req.SourcePath != nil {
		if *req.SourcePath == "" {
			writeError(w, http.StatusBadRequest, "sourcePath is required")
			return
		}
		_, old, err := database.UpdateArtifactSource(ctx, id, *req.SourcePath)
		if writeDBError(w, err, "artifact not found") {
			return
		}
		if old != "" && old != *req.SourcePath {
			s.removeArtifactSourceFile(ws(r), old)
		}
	}
	a, err := database.GetArtifact(ctx, id)
	if writeDBError(w, err, "artifact not found") {
		return
	}
	publishEntityChange(ws(r), "artifact", "Artifact güncellendi: "+a.Title, a.Kind,
		map[string]string{"view": "artifacts", "artifactId": a.ID, "sessionId": a.SessionID})
	writeJSON(w, http.StatusOK, a)
}

// handleSetArtifactGroup assigns an artifact's `group` (its Artifacts-UI
// organisation bucket) without touching any other field, then returns the
// updated artifact. An empty group ungroups it. This is the per-artifact
// endpoint the Artifacts screen's bulk "set group" action calls for each
// selected artifact.
//
// PUT /api/artifacts/{id}/group
func (s *Server) handleSetArtifactGroup(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[struct {
		Group string `json:"group"`
	}](w, r)
	if !ok {
		return
	}
	a, err := ws(r).DB.SetArtifactGroup(r.Context(), r.PathValue("id"), req.Group)
	if writeDBError(w, err, "artifact not found") {
		return
	}
	publishEntityChange(ws(r), "artifact", "Artifact güncellendi: "+a.Title, a.Kind,
		map[string]string{"view": "artifacts", "artifactId": a.ID, "sessionId": a.SessionID})
	writeJSON(w, http.StatusOK, a)
}

// handleSetArtifactArchived flips an artifact's `archived` flag (a soft,
// reversible hide) without touching any other field, then returns the updated
// artifact. This is the endpoint the Artifacts screen's archive / un-archive
// action calls.
//
// PUT /api/artifacts/{id}/archive
func (s *Server) handleSetArtifactArchived(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[struct {
		Archived bool `json:"archived"`
	}](w, r)
	if !ok {
		return
	}
	a, err := ws(r).DB.SetArtifactArchived(r.Context(), r.PathValue("id"), req.Archived)
	if writeDBError(w, err, "artifact not found") {
		return
	}
	verb := "arşivden çıkarıldı"
	if req.Archived {
		verb = "arşivlendi"
	}
	publishEntityChange(ws(r), "artifact", "Artifact "+verb+": "+a.Title, a.Kind,
		map[string]string{"view": "artifacts", "artifactId": a.ID, "sessionId": a.SessionID})
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
		return filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(rel))
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
