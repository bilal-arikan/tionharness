package db

// Memory kinds stored in knowledge_sources.
const (
	MemoryDocument   = "document"   // user-provided fact or note
	MemoryJournal    = "journal"    // auto-logged activity (chat turn, task run)
	MemoryReflection = "reflection" // agent-written summary over its journal
	MemoryCore       = "core"       // agent-editable working memory; single row per agent, always in context
)

// KnowledgeSource is one long-term memory belonging to an agent. Embedding
// holds the cached term vector (see internal/memory); it is opaque to the DB
// layer and not serialized to JSON.
type KnowledgeSource struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Embedding []byte `json:"-"`
	CreatedAt int64  `json:"createdAt"`
}
