package db

import "strings"

// Memory kinds stored in knowledge_sources. These three are the user-facing
// "display" kinds surfaced in the Memory screen and eligible for similarity
// recall; core blocks (below) are deliberately excluded from both.
const (
	MemoryDocument   = "document"   // user-provided fact or note
	MemoryJournal    = "journal"    // auto-logged activity (chat turn, task run)
	MemoryReflection = "reflection" // agent-written summary over its journal
)

// CoreKindPrefix namespaces MemGPT-style core working-memory blocks within
// knowledge_sources. Each named block's text is one row keyed by
// kind = "core:" + label, edited in place by the agent (core_memory_replace/
// append) and injected into every prompt. Core kinds never appear in recall or
// the Memory screen list (both are allowlists of the display kinds above), so a
// block is shown only in its dedicated card.
const CoreKindPrefix = "core:"

// CoreKind returns the knowledge-source kind for a core block label.
func CoreKind(label string) string {
	return CoreKindPrefix + strings.ToLower(strings.TrimSpace(label))
}

// IsCoreKind reports whether a knowledge kind is a core working-memory block.
func IsCoreKind(kind string) bool { return strings.HasPrefix(kind, CoreKindPrefix) }

// DefaultCoreCharLimit caps a block's size when its definition sets no explicit
// limit, mirroring Letta's per-block character ceiling. Writes past the limit are
// rejected so the agent must condense rather than grow context unbounded.
const DefaultCoreCharLimit = 2000

// CoreBlock defines one named core-memory block on an agent. The definition
// (label/limit/description/read-only) lives on the agent record; the block's text
// lives in knowledge_sources under CoreKind(Label). An agent that has defined no
// blocks falls back to DefaultCoreBlocks, so existing agents keep persona/human
// with no migration.
type CoreBlock struct {
	Label       string `json:"label"`                 // slug: persona, human, project…
	Description string `json:"description,omitempty"` // guidance shown to the agent + UI
	CharLimit   int    `json:"charLimit,omitempty"`   // 0 = DefaultCoreCharLimit
	ReadOnly    bool   `json:"readOnly,omitempty"`    // agent cannot edit (human/UI only)
	Order       int    `json:"order"`                 // prompt + UI ordering
}

// DefaultCoreBlocks are the seeded blocks for an agent that has defined none —
// the MemGPT persona/human pair, preserving prior behaviour.
var DefaultCoreBlocks = []CoreBlock{
	{Label: "persona", Description: "Who you are: your identity, role, behaviour and tone.", CharLimit: DefaultCoreCharLimit, Order: 0},
	{Label: "human", Description: "What you know about the user: their name, preferences and durable context.", CharLimit: DefaultCoreCharLimit, Order: 1},
}

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
