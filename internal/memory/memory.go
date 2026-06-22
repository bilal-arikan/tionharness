package memory

import (
	"context"
	"sort"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
)

// minScore is the cosine floor below which a memory is considered irrelevant.
const minScore = 0.04

// recallKinds are the memory kinds eligible for similarity recall. The "core"
// kind is deliberately excluded: it is the agent-editable working-memory block
// that is already injected into every prompt verbatim, so surfacing it again via
// recall would be redundant.
var recallKinds = []string{db.MemoryDocument, db.MemoryJournal, db.MemoryReflection}

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

// Core-memory sections (MemGPT persona/human split). "persona" is the agent's
// self-model; "human" is its model of the user. Each is a single, in-place
// editable row per agent, injected into every prompt.
const (
	CorePersona = "persona"
	CoreHuman   = "human"
)

// coreKind maps a section name to its storage kind. An empty/unknown section
// defaults to persona so a bare core_memory_* call still has a home.
func coreKind(section string) string {
	if strings.ToLower(strings.TrimSpace(section)) == CoreHuman {
		return db.MemoryCoreHuman
	}
	return db.MemoryCorePersona
}

// WriteCore replaces the agent's core block for the given section (persona|human)
// with content, caching its term vector. Upsert: the section's row is updated in
// place (or created on first write), so there is exactly one row per section.
func (s *Store) WriteCore(ctx context.Context, agentID, section, content string) error {
	content = strings.TrimSpace(content)
	vec := buildVector(content)
	_, err := s.db.UpsertKnowledgeByKind(ctx, agentID, coreKind(section), content, marshalVector(vec))
	return err
}

// ReadCore returns the agent's core block for the given section, or "" if none.
func (s *Store) ReadCore(ctx context.Context, agentID, section string) (string, error) {
	sources, err := s.db.ListKnowledge(ctx, agentID, coreKind(section))
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "", nil
	}
	return sources[0].Content, nil
}

// AppendCore appends a line to a section's core block (read-modify-write). A
// blank existing block yields just the line, so the first append reads cleanly.
func (s *Store) AppendCore(ctx context.Context, agentID, section, line string) error {
	line = strings.TrimSpace(line)
	cur, err := s.ReadCore(ctx, agentID, section)
	if err != nil {
		return err
	}
	next := line
	if strings.TrimSpace(cur) != "" {
		next = strings.TrimRight(cur, "\n") + "\n" + line
	}
	return s.WriteCore(ctx, agentID, section, next)
}

// ReadCoreSections returns both core sections (persona, human) for an agent —
// the shape used for prompt injection and the core API. Either may be "".
func (s *Store) ReadCoreSections(ctx context.Context, agentID string) (persona, human string, err error) {
	if persona, err = s.ReadCore(ctx, agentID, CorePersona); err != nil {
		return "", "", err
	}
	if human, err = s.ReadCore(ctx, agentID, CoreHuman); err != nil {
		return "", "", err
	}
	return persona, human, nil
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
