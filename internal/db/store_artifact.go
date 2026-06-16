package db

import (
	"context"
	"sort"
)

// ---- Artifacts ----

func (d *DB) persistArtifactLocked(a Artifact) error {
	d.artifacts[a.ID] = a
	return atomicWriteJSON(d.dir(dirArtifacts, a.ID+".json"), a)
}

// CreateArtifact inserts a new artifact at version 1 and returns the stored row.
func (d *DB) CreateArtifact(ctx context.Context, a Artifact) (Artifact, error) {
	a.ID = newID()
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	a.Version = 1
	if a.Kind == "" {
		a.Kind = ArtifactText
	}
	if a.Revisions == nil {
		a.Revisions = []ArtifactRevision{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return a, d.persistArtifactLocked(a)
}

// GetArtifact loads an artifact by id.
func (d *DB) GetArtifact(ctx context.Context, id string) (Artifact, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	a, ok := d.artifacts[id]
	if !ok {
		return Artifact{}, ErrNotFound
	}
	return a, nil
}

// ListArtifacts returns artifacts (optionally filtered to a session), newest
// updated first.
func (d *DB) ListArtifacts(ctx context.Context, sessionID string) ([]Artifact, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Artifact, 0, len(d.artifacts))
	for _, a := range d.artifacts {
		if sessionID == "" || a.SessionID == sessionID {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// UpdateArtifactContent replaces the current content, archiving the previous
// version into Revisions and bumping the version number. note is an optional
// change summary stored on the new revision boundary.
func (d *DB) UpdateArtifactContent(ctx context.Context, id, content, note string) (Artifact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.artifacts[id]
	if !ok {
		return Artifact{}, ErrNotFound
	}
	// Archive the outgoing version before overwriting.
	a.Revisions = append(a.Revisions, ArtifactRevision{
		Version:   a.Version,
		Content:   a.Content,
		Note:      note,
		CreatedAt: a.UpdatedAt,
	})
	a.Content = content
	a.Version++
	a.UpdatedAt = now()
	return a, d.persistArtifactLocked(a)
}

// UpdateArtifactMeta edits an artifact's title/kind/language without creating a
// new content revision. Empty fields are left unchanged.
func (d *DB) UpdateArtifactMeta(ctx context.Context, id, title, kind, language string) (Artifact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.artifacts[id]
	if !ok {
		return Artifact{}, ErrNotFound
	}
	if title != "" {
		a.Title = title
	}
	if kind != "" {
		a.Kind = kind
	}
	if language != "" {
		a.Language = language
	}
	a.UpdatedAt = now()
	return a, d.persistArtifactLocked(a)
}

// DeleteArtifact removes an artifact.
func (d *DB) DeleteArtifact(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.artifacts[id]; !ok {
		return ErrNotFound
	}
	delete(d.artifacts, id)
	return removeFile(d.dir(dirArtifacts, id+".json"))
}
