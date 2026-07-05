package db

// Artifact kinds. The frontend picks a renderer per kind (markdown prose, code
// with syntax highlighting, sandboxed HTML, raw text, inline SVG/Mermaid, or a
// media file rendered from SourcePath: image/video/audio, plus a generic file).
const (
	ArtifactMarkdown = "markdown"
	ArtifactCode     = "code"
	ArtifactHTML     = "html"
	ArtifactText     = "text"
	ArtifactSVG      = "svg"
	ArtifactMermaid  = "mermaid"
	// Media/file kinds: the content lives on disk (a workspace-relative upload),
	// referenced by SourcePath; Content holds an optional caption.
	ArtifactImage = "image"
	ArtifactVideo = "video"
	ArtifactAudio = "audio"
	ArtifactFile  = "file"
)

// Artifact is a substantial, self-contained piece of content an agent produced
// (a document, code file, HTML page, diagram) worth saving and viewing apart
// from the chat stream — TionSwarm's take on Claude.ai artifacts. Workspace-scoped.
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

	// Origin records how this artifact entered the workspace: "chat" (a chat
	// attachment), "manual" (dropped into the Artifacts screen), "agent" (a file
	// the agent wrote, auto-captured), "tool" (create_artifact) or "plan" (the
	// session's rolling artifact of approved ExitPlanMode plans). Empty for legacy
	// rows. Surfaced in the UI as "where it came from".
	Origin string `json:"origin,omitempty"`

	// SourcePath is the file path this artifact mirrors when it was captured
	// automatically from a file the agent wrote (empty for manual/tool-created
	// artifacts). It dedups repeated writes of the same file within a session.
	SourcePath string `json:"sourcePath,omitempty"`

	// ContentFile is the workspace-relative path (artifacts/<id><ext>) of the
	// real file holding a text artifact's body. When set, the body lives on disk
	// (not embedded in this JSON) and is read back into Content on load. Empty for
	// media kinds (their bytes live at SourcePath).
	ContentFile string `json:"contentFile,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}
