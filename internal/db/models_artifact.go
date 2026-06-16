package db

// Artifact kinds. The frontend picks a renderer per kind (markdown prose, code
// with syntax highlighting, sandboxed HTML, raw text, inline SVG/Mermaid).
const (
	ArtifactMarkdown = "markdown"
	ArtifactCode     = "code"
	ArtifactHTML     = "html"
	ArtifactText     = "text"
	ArtifactSVG      = "svg"
	ArtifactMermaid  = "mermaid"
)

// ArtifactRevision is one historical version of an artifact's content. Revisions
// are kept inline on the artifact (oldest first) so the full edit history is
// available without a separate store, matching the file-backed design.
type ArtifactRevision struct {
	Version   int    `json:"version"`
	Content   string `json:"content"`
	Note      string `json:"note"` // optional change summary
	CreatedAt int64  `json:"createdAt"`
}

// Artifact is a substantial, self-contained piece of content an agent produced
// (a document, code file, HTML page, diagram) worth saving, versioning and
// viewing apart from the chat stream — SwarmGo's take on Claude.ai artifacts.
// Workspace-scoped. SessionID/AgentID record where it originated so the chat UI
// can link back to it.
type Artifact struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"` // origin session (optional)
	AgentID   string `json:"agentId"`   // creator (optional)
	Title     string `json:"title"`
	Kind      string `json:"kind"`     // markdown|code|html|text|svg|mermaid
	Language  string `json:"language"` // for code kind (e.g. "go", "python")
	Content   string `json:"content"`  // current content
	Version   int    `json:"version"`  // current version number (starts at 1)

	// Revisions holds the prior versions (oldest first); the current content is
	// always Content/Version, not duplicated here.
	Revisions []ArtifactRevision `json:"revisions"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}
