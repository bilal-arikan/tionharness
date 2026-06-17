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

// Artifact is a substantial, self-contained piece of content an agent produced
// (a document, code file, HTML page, diagram) worth saving and viewing apart
// from the chat stream — SwarmGo's take on Claude.ai artifacts. Workspace-scoped.
// SessionID/AgentID record where it originated so the chat UI can link back.
//
// Artifacts are NOT versioned: an update overwrites the content in place.
type Artifact struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"` // origin session (optional)
	AgentID   string `json:"agentId"`   // creator (optional)
	Title     string `json:"title"`
	Kind      string `json:"kind"`     // markdown|code|html|text|svg|mermaid
	Language  string `json:"language"` // for code kind (e.g. "go", "python")
	Content   string `json:"content"`

	// SourcePath is the file path this artifact mirrors when it was captured
	// automatically from a file the agent wrote (empty for manual/tool-created
	// artifacts). It dedups repeated writes of the same file within a session.
	SourcePath string `json:"sourcePath,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}
