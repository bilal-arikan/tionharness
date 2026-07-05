package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// textArtifactKinds lists the renderers whose body is inline text Content.
var textArtifactKinds = map[string]bool{
	"markdown": true, "code": true, "html": true,
	"text": true, "svg": true, "mermaid": true,
}

// mediaArtifactKinds reference a file on disk via sourcePath instead of carrying
// inline content: the viewer renders the file (image/video/audio) or links it
// (file). Content, when present, is an optional caption. This lets an agent turn
// a screenshot/PDF/binary it produced into an artifact WITHOUT base64-embedding
// the bytes into the model context (which would blow the token budget).
var mediaArtifactKinds = map[string]bool{
	"image": true, "video": true, "audio": true, "file": true,
}

// createArtifactInput is the ask shape for create_artifact.
type createArtifactInput struct {
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Language   string `json:"language"`
	Content    string `json:"content"`
	SourcePath string `json:"sourcePath"`
}

// updateArtifactInput is the ask shape for update_artifact.
type updateArtifactInput struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// CreateArtifactTool lets the agent save a substantial, self-contained piece of
// content (a document, code file, HTML page, diagram) as an artifact the user
// can open in a dedicated screen — rather than burying it in the chat stream.
// The result is a JSON ref ({id,title,kind,action}) the chat UI special-cases
// into a clickable artifact card.
type CreateArtifactTool struct{}

// NewCreateArtifactTool constructs the create_artifact tool.
func NewCreateArtifactTool() CreateArtifactTool { return CreateArtifactTool{} }

func (CreateArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "create_artifact",
		Description: "Save a substantial, self-contained piece of content as a versioned artifact the " +
			"user can open in a dedicated viewer — documents, code, HTML, SVG or Mermaid diagrams worth " +
			"keeping or revisiting, NOT short conversational replies. For an image/PDF/binary file already " +
			"on disk (e.g. a screenshot), use kind=image|video|audio|file with sourcePath — do NOT " +
			"base64-embed the bytes. Returns the artifact id; revise it later with update_artifact.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": { "type": "string", "description": "Short descriptive title." },
    "kind": { "type": "string", "enum": ["markdown", "code", "html", "text", "svg", "mermaid", "image", "video", "audio", "file"], "description": "How the content should be rendered. Text kinds (markdown/code/html/text/svg/mermaid) use content; media kinds (image/video/audio/file) use sourcePath." },
    "language": { "type": "string", "description": "Programming language for kind=code (e.g. \"go\", \"python\", \"typescript\")." },
    "content": { "type": "string", "description": "The full artifact body for text kinds. For media kinds it is an optional caption." },
    "sourcePath": { "type": "string", "description": "For media kinds (image/video/audio/file): the path to the file on disk (absolute, e.g. \"C:\\Users\\me\\shot.png\", or workspace-relative). Files outside the workspace are copied in so the artifact owns a stable copy." }
  },
  "required": ["title", "kind"],
  "additionalProperties": false
}`),
	}
}

func (CreateArtifactTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[createArtifactInput]("create_artifact", input)
	if err != nil {
		return "", err
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind == "" {
		in.Kind = "text"
	}
	isText := textArtifactKinds[in.Kind]
	isMedia := mediaArtifactKinds[in.Kind]
	if !isText && !isMedia {
		return "", fmt.Errorf("invalid kind %q (want markdown|code|html|text|svg|mermaid|image|video|audio|file)", in.Kind)
	}
	in.SourcePath = strings.TrimSpace(in.SourcePath)
	if isText && strings.TrimSpace(in.Content) == "" {
		return "", fmt.Errorf("content is required for kind %q", in.Kind)
	}
	if isMedia && in.SourcePath == "" {
		return "", fmt.Errorf("sourcePath is required for kind %q (the path to the file on disk)", in.Kind)
	}
	sink := artifactsFrom(ctx)
	if sink == nil {
		return "", fmt.Errorf("artifacts are not available in this context (only in interactive chat)")
	}
	ref, err := sink.CreateArtifact(ctx, CreateArtifactSpec{
		Title:      in.Title,
		Kind:       in.Kind,
		Language:   in.Language,
		Content:    in.Content,
		SourcePath: in.SourcePath,
	})
	if err != nil {
		return "", fmt.Errorf("save artifact: %w", err)
	}
	return encodeArtifactResult(ref, "create")
}

// UpdateArtifactTool overwrites an existing artifact's content in place.
type UpdateArtifactTool struct{}

// NewUpdateArtifactTool constructs the update_artifact tool.
func NewUpdateArtifactTool() UpdateArtifactTool { return UpdateArtifactTool{} }

func (UpdateArtifactTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "update_artifact",
		Description: "Replace the content of an existing artifact (created with create_artifact). " +
			"Pass the FULL new content, not a diff.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "The artifact id returned by create_artifact." },
    "content": { "type": "string", "description": "The full new content (replaces the old)." }
  },
  "required": ["id", "content"],
  "additionalProperties": false
}`),
	}
}

func (UpdateArtifactTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[updateArtifactInput]("update_artifact", input)
	if err != nil {
		return "", err
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
	ref, err := sink.UpdateArtifact(ctx, in.ID, in.Content)
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
