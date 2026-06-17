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

// CreateArtifact inserts a new artifact and returns the stored row.
func (d *DB) CreateArtifact(ctx context.Context, a Artifact) (Artifact, error) {
	a.ID = newID()
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.Kind == "" {
		a.Kind = ArtifactText
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

// UpdateArtifactContent overwrites an artifact's content in place (no versioning).
func (d *DB) UpdateArtifactContent(ctx context.Context, id, content string) (Artifact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.artifacts[id]
	if !ok {
		return Artifact{}, ErrNotFound
	}
	a.Content = content
	a.UpdatedAt = now()
	return a, d.persistArtifactLocked(a)
}

// UpdateArtifactMeta edits an artifact's title/kind/language. Empty fields are
// left unchanged.
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

// SaveFileArtifact upserts an artifact mirroring a file the agent wrote: if one
// already exists for the same session + source path it is overwritten in place,
// otherwise a new one is created. This dedups repeated writes of the same file
// within a session so the Artifacts screen shows the latest content once.
func (d *DB) SaveFileArtifact(ctx context.Context, sessionID, agentID, sourcePath, title, kind, language, content string) (Artifact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, a := range d.artifacts {
		if a.SessionID == sessionID && a.SourcePath != "" && a.SourcePath == sourcePath {
			a.Content = content
			a.Title = title
			a.Kind = kind
			a.Language = language
			a.UpdatedAt = now()
			return a, d.persistArtifactLocked(a)
		}
	}
	a := Artifact{
		ID:         newID(),
		SessionID:  sessionID,
		AgentID:    agentID,
		Title:      title,
		Kind:       kind,
		Language:   language,
		Content:    content,
		SourcePath: sourcePath,
		CreatedAt:  now(),
		UpdatedAt:  now(),
	}
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
