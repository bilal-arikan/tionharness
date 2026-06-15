package db

import (
	"context"
	"database/sql"
	"errors"
)

const knowledgeColumns = `id, agent_id, kind, content, embedding, created_at`

func scanKnowledge(s interface{ Scan(...any) error }, k *KnowledgeSource) error {
	return s.Scan(&k.ID, &k.AgentID, &k.Kind, &k.Content, &k.Embedding, &k.CreatedAt)
}

// CreateKnowledge inserts a memory row and returns it.
func (d *DB) CreateKnowledge(ctx context.Context, k KnowledgeSource) (KnowledgeSource, error) {
	k.ID = newID()
	k.CreatedAt = now()
	if k.Kind == "" {
		k.Kind = MemoryDocument
	}
	_, err := d.ExecContext(ctx, `INSERT INTO knowledge_sources
		(id, agent_id, kind, content, embedding, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		k.ID, k.AgentID, k.Kind, k.Content, k.Embedding, k.CreatedAt)
	return k, err
}

// GetKnowledge loads a memory by id.
func (d *DB) GetKnowledge(ctx context.Context, id string) (KnowledgeSource, error) {
	var k KnowledgeSource
	err := scanKnowledge(d.QueryRowContext(ctx, `SELECT `+knowledgeColumns+` FROM knowledge_sources WHERE id = ?`, id), &k)
	if errors.Is(err, sql.ErrNoRows) {
		return k, ErrNotFound
	}
	return k, err
}

// ListKnowledge returns an agent's memories, newest first. If kinds is
// non-empty, only those kinds are returned.
func (d *DB) ListKnowledge(ctx context.Context, agentID string, kinds ...string) ([]KnowledgeSource, error) {
	query := `SELECT ` + knowledgeColumns + ` FROM knowledge_sources WHERE agent_id = ?`
	args := []any{agentID}
	if len(kinds) > 0 {
		query += ` AND kind IN (` + placeholders(len(kinds)) + `)`
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	query += ` ORDER BY created_at DESC`

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KnowledgeSource
	for rows.Next() {
		var k KnowledgeSource
		if err := scanKnowledge(rows, &k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteKnowledge removes a memory.
func (d *DB) DeleteKnowledge(ctx context.Context, id string) error {
	res, err := d.ExecContext(ctx, `DELETE FROM knowledge_sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// placeholders returns "?, ?, ..." with n entries for IN clauses.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*3)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',', ' ')
		}
		b = append(b, '?')
	}
	return string(b)
}
