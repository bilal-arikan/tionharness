package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// artifactKinds lists the renderers the UI understands.
var artifactKinds = map[string]bool{
	"markdown": true, "code": true, "html": true,
	"text": true, "svg": true, "mermaid": true,
}

// createArtifactInput is the ask shape for create_artifact.
type createArtifactInput struct {
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

// updateArtifactInput is the ask shape for update_artifact.
type updateArtifactInput struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Note    string `json:"note"`
}

// CreateArtifactTool lets the agent save a substantial, self-contained piece of
// content (a document, code file, HTML page, diagram) as a versioned artifact
// the user can open in a dedicated screen — rather than burying it in the chat
// stream. The result is a JSON ref ({id,title,kind,version,action}) the chat UI
// special-cases into a clickable artifact card.
type CreateArtifactTool struct{}

// NewCreateArtifactTool constructs the create_artifact tool.
func NewCreateArtifactTool() CreateArtifactTool { return CreateArtifactTool{} }

func (CreateArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "create_artifact",
		Description: "Save a substantial, self-contained piece of content as a versioned " +
			"artifact the user can open in a dedicated viewer (like a canvas). Use this for " +
			"documents, code files, HTML pages, SVG or Mermaid diagrams that the user will " +
			"want to keep, copy or revisit — NOT for short conversational replies. Returns the " +
			"artifact id; reference it with update_artifact to revise the same artifact later.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": { "type": "string", "description": "Short descriptive title." },
    "kind": { "type": "string", "enum": ["markdown", "code", "html", "text", "svg", "mermaid"], "description": "How the content should be rendered." },
    "language": { "type": "string", "description": "Programming language for kind=code (e.g. \"go\", \"python\", \"typescript\")." },
    "content": { "type": "string", "description": "The full artifact content." }
  },
  "required": ["title", "kind", "content"],
  "additionalProperties": false
}`),
	}
}

func (CreateArtifactTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in createArtifactInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid create_artifact input: %w", err)
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.Content) == "" {
		return "", fmt.Errorf("content is required")
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind == "" {
		in.Kind = "text"
	}
	if !artifactKinds[in.Kind] {
		return "", fmt.Errorf("invalid kind %q (want markdown|code|html|text|svg|mermaid)", in.Kind)
	}
	sink := artifactsFrom(ctx)
	if sink == nil {
		return "", fmt.Errorf("artifacts are not available in this context (only in interactive chat)")
	}
	ref, err := sink.CreateArtifact(ctx, in.Title, in.Kind, in.Language, in.Content)
	if err != nil {
		return "", fmt.Errorf("save artifact: %w", err)
	}
	return encodeArtifactResult(ref, "create")
}

// UpdateArtifactTool revises an existing artifact, archiving the prior version.
type UpdateArtifactTool struct{}

// NewUpdateArtifactTool constructs the update_artifact tool.
func NewUpdateArtifactTool() UpdateArtifactTool { return UpdateArtifactTool{} }

func (UpdateArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "update_artifact",
		Description: "Replace the content of an existing artifact (created with create_artifact). " +
			"The previous version is archived automatically and the version number bumped. " +
			"Pass the FULL new content, not a diff.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "The artifact id returned by create_artifact." },
    "content": { "type": "string", "description": "The full new content (replaces the old)." },
    "note": { "type": "string", "description": "Optional one-line summary of what changed." }
  },
  "required": ["id", "content"],
  "additionalProperties": false
}`),
	}
}

func (UpdateArtifactTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in updateArtifactInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid update_artifact input: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if strings.TrimSpace(in.Content) == "" {
		return "", fmt.Errorf("content is required")
	}
	sink := artifactsFrom(ctx)
	if sink == nil {
		return "", fmt.Errorf("artifacts are not available in this context (only in interactive chat)")
	}
	ref, err := sink.UpdateArtifact(ctx, in.ID, in.Content, strings.TrimSpace(in.Note))
	if err != nil {
		return "", fmt.Errorf("update artifact: %w", err)
	}
	return encodeArtifactResult(ref, "update")
}

// encodeArtifactResult returns a compact JSON result the chat UI parses to draw
// the artifact card. The "action" tags whether it was created or revised.
func encodeArtifactResult(ref ArtifactRef, action string) (string, error) {
	b, err := json.Marshal(struct {
		ArtifactRef
		Action string `json:"action"`
	}{ArtifactRef: ref, Action: action})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
