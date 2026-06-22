package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// minScore is the cosine floor below which a memory is considered irrelevant.
const minScore = 0.04

// recallKinds are the memory kinds eligible for similarity recall. The core
// kinds (core_persona/core_human) are deliberately excluded: they are the
// agent-editable working-memory blocks already injected into every prompt
// verbatim, so surfacing them again via recall would be redundant. This same
// allowlist is the set of user-facing memory kinds shown in the Memory screen's
// list and knowledge-graph views — core blocks have their own dedicated card.
var recallKinds = []string{db.MemoryDocument, db.MemoryJournal, db.MemoryReflection}

// DisplayKinds returns the memory kinds shown in the Memory screen (everything
// except the always-in-context core blocks, which render in their own card).
// Returns a fresh copy so callers can pass it as a variadic filter safely.
func DisplayKinds() []string {
	return append([]string(nil), recallKinds...)
}

// Store is a thin, stateless wrapper over the DB that adds vector recall on top
// of knowledge_sources CRUD. Construct one per request; it holds no state.
type Store struct {
	db *db.DB
}

// New constructs a memory store bound to a workspace database.
func New(database *db.DB) *Store { return &Store{db: database} }

// Remember stores a memory of the given kind, caching its term vector.
func (s *Store) Remember(ctx context.Context, agentID, kind, content string) (db.KnowledgeSource, error) {
	content = strings.TrimSpace(content)
	vec := buildVector(content)
	return s.db.CreateKnowledge(ctx, db.KnowledgeSource{
		AgentID:   agentID,
		Kind:      kind,
		Content:   content,
		Embedding: marshalVector(vec),
	})
}

// Default core block labels (MemGPT persona/human). "persona" is the agent's
// self-model; "human" is its model of the user. They are the seeded blocks every
// agent has unless it defines its own set (see db.DefaultCoreBlocks).
const (
	CorePersona = "persona"
	CoreHuman   = "human"
)

// ErrUnknownCoreBlock is returned when a write targets a label the agent has not
// defined. Blocks must be defined (DefineCoreBlock or a default) before writing,
// so block definitions and content never drift apart.
var ErrUnknownCoreBlock = errors.New("unknown core block label")

// CoreBlockFullError reports a write that would exceed a block's character limit.
// It carries the numbers so the tool layer can tell the agent exactly how much to
// shed.
type CoreBlockFullError struct {
	Label string
	Count int
	Limit int
}

func (e *CoreBlockFullError) Error() string {
	return fmt.Sprintf("core block %q is full: %d/%d chars — replace or condense it before adding more", e.Label, e.Count, e.Limit)
}

// BlockView is one core block's definition plus its current content, the shape
// used for prompt injection and the core API.
type BlockView struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Content     string `json:"content"`
	CharLimit   int    `json:"charLimit"`
	ReadOnly    bool   `json:"readOnly"`
	Order       int    `json:"order"`
}

// CoreBlocks returns the agent's defined core blocks, or the default persona/human
// pair when it has defined none. The returned slice is a copy safe to mutate.
func (s *Store) CoreBlocks(ctx context.Context, agentID string) ([]db.CoreBlock, error) {
	a, err := s.db.GetAgent(ctx, agentID)
	if err != nil {
		// A missing agent has no custom blocks — fall back to the defaults rather
		// than failing, so core memory works for any agent id the caller holds.
		if errors.Is(err, db.ErrNotFound) {
			return append([]db.CoreBlock(nil), db.DefaultCoreBlocks...), nil
		}
		return nil, err
	}
	defs := a.CoreBlocks
	if len(defs) == 0 {
		defs = db.DefaultCoreBlocks
	}
	return append([]db.CoreBlock(nil), defs...), nil
}

// blockDef finds an agent's block definition by label (case-insensitive).
func (s *Store) blockDef(ctx context.Context, agentID, label string) (db.CoreBlock, bool, error) {
	defs, err := s.CoreBlocks(ctx, agentID)
	if err != nil {
		return db.CoreBlock{}, false, err
	}
	label = strings.ToLower(strings.TrimSpace(label))
	for _, d := range defs {
		if strings.ToLower(d.Label) == label {
			return d, true, nil
		}
	}
	return db.CoreBlock{}, false, nil
}

// blockLimit returns a block's effective character limit (its own, or the
// package default when unset).
func blockLimit(d db.CoreBlock) int {
	if d.CharLimit > 0 {
		return d.CharLimit
	}
	return db.DefaultCoreCharLimit
}

// WriteCore replaces the agent's core block content for the given label, caching
// its term vector. Upsert: the block's row is updated in place (or created on
// first write), so there is exactly one row per label. Returns ErrUnknownCoreBlock
// for an undefined label and *CoreBlockFullError when content exceeds the limit.
// The read-only flag is NOT enforced here — that is a tool-layer policy (humans
// edit read-only blocks through the API); the store stays mechanism, not policy.
func (s *Store) WriteCore(ctx context.Context, agentID, label, content string) error {
	def, ok, err := s.blockDef(ctx, agentID, label)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnknownCoreBlock
	}
	content = strings.TrimSpace(content)
	if n := utf8.RuneCountInString(content); n > blockLimit(def) {
		return &CoreBlockFullError{Label: def.Label, Count: n, Limit: blockLimit(def)}
	}
	vec := buildVector(content)
	_, err = s.db.UpsertKnowledgeByKind(ctx, agentID, db.CoreKind(def.Label), content, marshalVector(vec))
	return err
}

// ReadCore returns the agent's core block content for the given label, or "" if
// the block has no content yet. It does not require the label to be defined.
func (s *Store) ReadCore(ctx context.Context, agentID, label string) (string, error) {
	sources, err := s.db.ListKnowledge(ctx, agentID, db.CoreKind(label))
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "", nil
	}
	return sources[0].Content, nil
}

// AppendCore appends a line to a block's content (read-modify-write). A blank
// existing block yields just the line, so the first append reads cleanly. The
// combined content is subject to the same limit as WriteCore.
func (s *Store) AppendCore(ctx context.Context, agentID, label, line string) error {
	line = strings.TrimSpace(line)
	cur, err := s.ReadCore(ctx, agentID, label)
	if err != nil {
		return err
	}
	next := line
	if strings.TrimSpace(cur) != "" {
		next = strings.TrimRight(cur, "\n") + "\n" + line
	}
	return s.WriteCore(ctx, agentID, label, next)
}

// ReadCoreBlocks returns every defined block with its current content, ordered by
// Order then label — the shape used for prompt injection and the core API.
func (s *Store) ReadCoreBlocks(ctx context.Context, agentID string) ([]BlockView, error) {
	defs, err := s.CoreBlocks(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]BlockView, 0, len(defs))
	for _, d := range defs {
		content, err := s.ReadCore(ctx, agentID, d.Label)
		if err != nil {
			return nil, err
		}
		out = append(out, BlockView{
			Label:       d.Label,
			Description: d.Description,
			Content:     content,
			CharLimit:   blockLimit(d),
			ReadOnly:    d.ReadOnly,
			Order:       d.Order,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

// DefineCoreBlock creates or updates a block definition on the agent, seeding the
// default persona/human pair first when the agent had defined none (so adding a
// custom block never silently drops the defaults). A new block is appended after
// the existing ones; an existing label (case-insensitive) is updated in place.
func (s *Store) DefineCoreBlock(ctx context.Context, agentID string, block db.CoreBlock) error {
	defs, err := s.CoreBlocks(ctx, agentID) // returns defaults (copy) when empty
	if err != nil {
		return err
	}
	block.Label = strings.ToLower(strings.TrimSpace(block.Label))
	if block.Label == "" {
		return errors.New("core block label is required")
	}
	if block.CharLimit < 0 {
		block.CharLimit = 0
	}
	for i := range defs {
		if strings.ToLower(defs[i].Label) == block.Label {
			block.Order = defs[i].Order // preserve position on update
			defs[i] = block
			_, err = s.db.SetAgentCoreBlocks(ctx, agentID, defs)
			return err
		}
	}
	block.Order = len(defs)
	defs = append(defs, block)
	_, err = s.db.SetAgentCoreBlocks(ctx, agentID, defs)
	return err
}

// DeleteCoreBlock removes a block definition and its content. Deleting down to an
// empty set means the agent falls back to the default persona/human pair again.
func (s *Store) DeleteCoreBlock(ctx context.Context, agentID, label string) error {
	defs, err := s.CoreBlocks(ctx, agentID)
	if err != nil {
		return err
	}
	label = strings.ToLower(strings.TrimSpace(label))
	kept := make([]db.CoreBlock, 0, len(defs))
	for _, d := range defs {
		if strings.ToLower(d.Label) != label {
			kept = append(kept, d)
		}
	}
	if len(kept) == len(defs) {
		return nil // nothing to delete
	}
	if _, err := s.db.SetAgentCoreBlocks(ctx, agentID, kept); err != nil {
		return err
	}
	// Drop the block's content rows (usually one) so no orphan lingers.
	rows, err := s.db.ListKnowledge(ctx, agentID, db.CoreKind(label))
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := s.db.DeleteKnowledge(ctx, r.ID); err != nil && err != db.ErrNotFound {
			return err
		}
	}
	return nil
}

// List returns an agent's memories (optionally filtered by kind), newest first.
func (s *Store) List(ctx context.Context, agentID string, kinds ...string) ([]db.KnowledgeSource, error) {
	return s.db.ListKnowledge(ctx, agentID, kinds...)
}

// Delete removes a memory by id.
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.db.DeleteKnowledge(ctx, id)
}

// PruneKind keeps only the newest `keep` memories of the given kind for an agent,
// deleting the rest. It is the ring-buffer that bounds unbounded kinds (journal):
// document/reflection are durable and should not be pruned. Returns the number
// deleted. A keep <= 0 is treated as "keep nothing of this kind".
func (s *Store) PruneKind(ctx context.Context, agentID, kind string, keep int) (int, error) {
	if keep < 0 {
		keep = 0
	}
	sources, err := s.db.ListKnowledge(ctx, agentID, kind) // newest first
	if err != nil {
		return 0, err
	}
	if len(sources) <= keep {
		return 0, nil
	}
	deleted := 0
	for _, src := range sources[keep:] {
		if err := s.db.DeleteKnowledge(ctx, src.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// DeleteIDs removes the given memories by id, ignoring any that are already gone.
func (s *Store) DeleteIDs(ctx context.Context, ids ...string) error {
	for _, id := range ids {
		if err := s.db.DeleteKnowledge(ctx, id); err != nil && err != db.ErrNotFound {
			return err
		}
	}
	return nil
}

// Hit is a recalled memory with its relevance score.
type Hit struct {
	Source db.KnowledgeSource
	Score  float64
}

// Recall returns the top-N memories most similar to query, ranked by lexical
// cosine. kinds optionally restricts the search; when omitted it defaults to the
// recallable kinds (everything except the always-injected "core" block).
func (s *Store) Recall(ctx context.Context, agentID, query string, limit int, kinds ...string) ([]Hit, error) {
	if limit <= 0 {
		limit = 5
	}
	if len(kinds) == 0 {
		kinds = recallKinds
	}
	sources, err := s.db.ListKnowledge(ctx, agentID, kinds...)
	if err != nil {
		return nil, err
	}
	qv := buildVector(query)
	if len(qv) == 0 {
		return nil, nil
	}
	qnorm := norm(qv) // constant across candidates; compute once

	hits := make([]Hit, 0, len(sources))
	for _, src := range sources {
		vec := unmarshalVector(src.Embedding)
		if vec == nil {
			vec = buildVector(src.Content) // backfill for rows without a cached vector
		}
		score := cosineNorm(qv, vec, qnorm)
		if score >= minScore {
			hits = append(hits, Hit{Source: src, Score: score})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// ContextBlock recalls relevant memories and formats them as a system-prompt
// section. Returns "" when nothing relevant is found, so callers can append it
// unconditionally.
func (s *Store) ContextBlock(ctx context.Context, agentID, query string, limit int) string {
	hits, err := s.Recall(ctx, agentID, query, limit)
	if err != nil || len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Relevant memory\n")
	b.WriteString("The following are your own recalled memories; use them if helpful.\n")
	for _, h := range hits {
		b.WriteString("- (")
		b.WriteString(h.Source.Kind)
		b.WriteString(") ")
		b.WriteString(oneLine(h.Source.Content))
		b.WriteString("\n")
	}
	return b.String()
}

// oneLine collapses whitespace and caps a memory's length for prompt injection.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 280
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max]) + "…"
	}
	return s
}
