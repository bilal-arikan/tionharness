package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Artifact management tools complement create_artifact / update_artifact (which
// run through the per-turn sink) with deletion and listing that work directly
// against the workspace store. No provenance gate: any artifact (user- or
// agent-created) may be removed. Artifact.AgentID is still stamped for provenance/display.

type artifactMgmtDeps struct {
	db      *db.DB
	actorID string
}

// ReadArtifactTool returns an artifact's full content by id. It saves the agent
// from guessing the on-disk path (artifacts live under workspace/artifacts/<...>/,
// not next to the file the agent wrote) — a common failure when an agent tries to
// re-display an artifact it created earlier.
type ReadArtifactTool struct{ d artifactMgmtDeps }

// NewReadArtifactTool constructs read_artifact.
func NewReadArtifactTool(database *db.DB, actorID string) ReadArtifactTool {
	return ReadArtifactTool{d: artifactMgmtDeps{db: database, actorID: actorID}}
}

func (ReadArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "read_artifact",
		Description: "Return the full content of an artifact by id (see list_artifacts). Use this to re-display or revise an artifact you created earlier — do NOT guess its file path on disk.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The artifact id (see list_artifacts)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t ReadArtifactTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	art, err := t.d.db.GetArtifact(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no artifact with id %q (use list_artifacts)", in.ID)
	}
	out := struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Kind     string `json:"kind"`
		Language string `json:"language,omitempty"`
		Content  string `json:"content"`
	}{art.ID, art.Title, art.Kind, art.Language, art.Content}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// DeleteArtifactTool removes an artifact (user- or agent-created).
type DeleteArtifactTool struct{ d artifactMgmtDeps }

// NewDeleteArtifactTool constructs delete_artifact.
func NewDeleteArtifactTool(database *db.DB, actorID string) DeleteArtifactTool {
	return DeleteArtifactTool{d: artifactMgmtDeps{db: database, actorID: actorID}}
}

func (DeleteArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_artifact",
		Description: "Delete an artifact (user- or agent-created). Pass the artifact id (see list_artifacts).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The artifact id (see list_artifacts)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteArtifactTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.db.GetArtifact(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no artifact with id %q (use list_artifacts)", in.ID)
	}
	if err := t.d.db.DeleteArtifact(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete artifact: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}

// ListArtifactsTool lists artifacts in the workspace with provenance.
type ListArtifactsTool struct{ d artifactMgmtDeps }

// NewListArtifactsTool constructs list_artifacts.
func NewListArtifactsTool(database *db.DB, actorID string) ListArtifactsTool {
	return ListArtifactsTool{d: artifactMgmtDeps{db: database, actorID: actorID}}
}

func (ListArtifactsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_artifacts",
		Description: "List the artifacts in this workspace (id, title, kind, optional contentFile path, and whether each " +
			"was created by an agent — provenance only; you can delete any of them). Results are PAGINATED: " +
			"pass limit (default 20, max 100) and offset to page; the reply reports total and hasMore, and you " +
			"reach the next page with offset += limit. Filters: sessionId (only artifacts from that session), " +
			"kind (markdown|code|html|text|svg|mermaid|image|video|audio|file), origin (chat|manual|agent|tool|plan). Sort: updated_desc " +
			"(default), updated_asc, created_desc, created_asc, name_asc, name_desc (name sorts by title). " +
			"Use read_artifact to get content by id, update_artifact to edit, delete_artifact to remove.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "sessionId": { "type": "string", "description": "Only artifacts created in this session." },
    "kind": { "type": "string", "description": "Only artifacts of this kind (markdown|code|html|text|svg|mermaid|image|video|audio|file)." },
    "origin": { "type": "string", "description": "Only artifacts with this origin (chat|manual|agent|tool|plan)." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc; name sorts by title)." },
    "limit": { "type": "integer", "description": "Max artifacts per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching artifacts to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListArtifactsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		SessionID string `json:"sessionId"`
		Kind      string `json:"kind"`
		Origin    string `json:"origin"`
		Sort      string `json:"sort"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	artifacts, err := t.d.db.ListArtifacts(ctx, strings.TrimSpace(in.SessionID)) // "" = all sessions in workspace
	if err != nil {
		return "", err
	}
	kind := strings.TrimSpace(in.Kind)
	origin := strings.TrimSpace(in.Origin)
	matches := make([]db.Artifact, 0, len(artifacts))
	for _, a := range artifacts {
		if kind != "" && a.Kind != kind {
			continue
		}
		if origin != "" && a.Origin != origin {
			continue
		}
		matches = append(matches, a)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	less, err := SortByField(matches, field, asc,
		func(a db.Artifact) int64 { return a.UpdatedAt },
		func(a db.Artifact) int64 { return a.CreatedAt },
		func(a db.Artifact) string { return a.Title },
		func(a db.Artifact) string { return a.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Kind           string `json:"kind"`
		ContentFile    string `json:"contentFile,omitempty"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(page))
	for _, a := range page {
		out = append(out, row{
			ID:             a.ID,
			Title:          a.Title,
			Kind:           a.Kind,
			ContentFile:    a.ContentFile,
			CreatedByAgent: a.AgentID != "",
		})
	}
	return pageResult(out, total, offset, limit)
}
