package memory

import (
	"context"
	"sort"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
)

// minScore is the cosine floor below which a memory is considered irrelevant.
const minScore = 0.04

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

// List returns an agent's memories (optionally filtered by kind), newest first.
func (s *Store) List(ctx context.Context, agentID string, kinds ...string) ([]db.KnowledgeSource, error) {
	return s.db.ListKnowledge(ctx, agentID, kinds...)
}

// Delete removes a memory by id.
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.db.DeleteKnowledge(ctx, id)
}

// Hit is a recalled memory with its relevance score.
type Hit struct {
	Source db.KnowledgeSource
	Score  float64
}

// Recall returns the top-N memories most similar to query, ranked by lexical
// cosine. kinds optionally restricts the search (defaults to all kinds).
func (s *Store) Recall(ctx context.Context, agentID, query string, limit int, kinds ...string) ([]Hit, error) {
	if limit <= 0 {
		limit = 5
	}
	sources, err := s.db.ListKnowledge(ctx, agentID, kinds...)
	if err != nil {
		return nil, err
	}
	qv := buildVector(query)
	if len(qv) == 0 {
		return nil, nil
	}

	hits := make([]Hit, 0, len(sources))
	for _, src := range sources {
		vec := unmarshalVector(src.Embedding)
		if vec == nil {
			vec = buildVector(src.Content) // backfill for rows without a cached vector
		}
		score := cosine(qv, vec)
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
