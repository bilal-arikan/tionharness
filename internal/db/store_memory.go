package db

import (
	"context"
	"sort"
)

// knowledgeDisk is the on-disk shape of a KnowledgeSource. The in-memory model
// hides Embedding from JSON (json:"-"), but the store must persist the cached
// term vector, so we serialize it explicitly here (Go encodes []byte as base64).
type knowledgeDisk struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Embedding []byte `json:"embedding"`
	CreatedAt int64  `json:"createdAt"`
}

func toKnowledgeDisk(k KnowledgeSource) knowledgeDisk {
	return knowledgeDisk{k.ID, k.AgentID, k.Kind, k.Content, k.Embedding, k.CreatedAt}
}

func (kd knowledgeDisk) toModel() KnowledgeSource {
	return KnowledgeSource{kd.ID, kd.AgentID, kd.Kind, kd.Content, kd.Embedding, kd.CreatedAt}
}

func (d *DB) persistKnowledgeLocked(k KnowledgeSource) error {
	d.knowledge[k.ID] = k
	return atomicWriteJSON(d.dir(dirKnowledge, k.ID+".json"), toKnowledgeDisk(k))
}

func (d *DB) loadKnowledge() error {
	disks, err := loadJSONDir[knowledgeDisk](d.dir(dirKnowledge))
	if err != nil {
		return err
	}
	for _, kd := range disks {
		d.knowledge[kd.ID] = kd.toModel()
	}
	return nil
}

// CreateKnowledge inserts a memory and returns it.
func (d *DB) CreateKnowledge(ctx context.Context, k KnowledgeSource) (KnowledgeSource, error) {
	k.ID = newID()
	k.CreatedAt = now()
	if k.Kind == "" {
		k.Kind = MemoryDocument
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return k, d.persistKnowledgeLocked(k)
}

// GetKnowledge loads a memory by id.
func (d *DB) GetKnowledge(ctx context.Context, id string) (KnowledgeSource, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	k, ok := d.knowledge[id]
	if !ok {
		return KnowledgeSource{}, ErrNotFound
	}
	return k, nil
}

// ListKnowledge returns an agent's memories, newest first. If kinds is
// non-empty, only those kinds are returned.
func (d *DB) ListKnowledge(ctx context.Context, agentID string, kinds ...string) ([]KnowledgeSource, error) {
	allow := map[string]bool{}
	for _, k := range kinds {
		allow[k] = true
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]KnowledgeSource, 0)
	for _, k := range d.knowledge {
		if k.AgentID != agentID {
			continue
		}
		if len(allow) > 0 && !allow[k.Kind] {
			continue
		}
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// DeleteKnowledge removes a memory.
func (d *DB) DeleteKnowledge(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.knowledge[id]; !ok {
		return ErrNotFound
	}
	delete(d.knowledge, id)
	return removeFile(d.dir(dirKnowledge, id+".json"))
}
