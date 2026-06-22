package db

// Memory kinds stored in knowledge_sources.
const (
	MemoryDocument   = "document"   // user-provided fact or note
	MemoryJournal    = "journal"    // auto-logged activity (chat turn, task run)
	MemoryReflection = "reflection" // agent-written summary over its journal
	// Core working memory is the MemGPT-style block injected into every prompt and
	// edited in place by the agent. It is split into two labelled sections, one row
	// each per agent: "persona" (the agent's self-model) and "human" (its model of
	// the user). Both are excluded from similarity recall.
	MemoryCorePersona = "core_persona" // agent's self-description; single row per agent
	MemoryCoreHuman   = "core_human"   // agent's model of the user; single row per agent
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
