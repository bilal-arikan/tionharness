package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Artifact management tools complement create_artifact / update_artifact (which
// run through the per-turn sink) with deletion and listing that work directly
// against the workspace store. Provenance is enforced for deletion: only
// agent-created artifacts (AgentID != "") may be removed by an agent.

type artifactMgmtDeps struct {
	db      *db.DB
	actorID string
}

// DeleteArtifactTool removes an agent-created artifact.
type DeleteArtifactTool struct{ d artifactMgmtDeps }

// NewDeleteArtifactTool constructs delete_artifact.
func NewDeleteArtifactTool(database *db.DB, actorID string) DeleteArtifactTool {
	return DeleteArtifactTool{d: artifactMgmtDeps{db: database, actorID: actorID}}
}

func (DeleteArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_artifact",
		Description: "Delete an agent-created artifact (not one the user made manually). Pass the artifact id (see list_artifacts).",
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
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	art, err := t.d.db.GetArtifact(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no artifact with id %q (use list_artifacts)", in.ID)
	}
	if art.AgentID == "" {
		return "", fmt.Errorf("artifact %q was created by the user and cannot be deleted by an agent", art.Title)
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
		Name:        "list_artifacts",
		Description: "List the artifacts in this workspace (id, title, kind, and whether each was created by an agent and is therefore deletable by you). Use update_artifact to edit content, delete_artifact to remove.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListArtifactsTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	artifacts, err := t.d.db.ListArtifacts(ctx, "") // "" = all sessions in workspace
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Kind           string `json:"kind"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(artifacts))
	for _, a := range artifacts {
		out = append(out, row{
			ID:             a.ID,
			Title:          a.Title,
			Kind:           a.Kind,
			CreatedByAgent: a.AgentID != "",
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
