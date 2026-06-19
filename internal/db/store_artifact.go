package db

import (
	"context"
	"sort"
)

// ---- Artifacts ----

func (d *DB) persistArtifactLocked(a *Artifact) error {
	// A text artifact's body lives in a real file under workspace/artifacts/, so
	// the JSON only references it. Two cases:
	//   - content supplied (tool/manual create) → write a new content file.
	//   - no content but an already-uploaded text file (chat attachment / manual
	//     upload) → use that file directly as the content file (no duplicate).
	// Media kinds keep their SourcePath binary as-is.
	if isTextArtifact(a.Kind) && a.ContentFile == "" {
		switch {
		case a.Content != "":
			if err := d.writeArtifactContent(a); err != nil {
				return err
			}
		case a.SourcePath != "":
			a.ContentFile = a.SourcePath
			d.readArtifactContent(a)
		}
	}
	d.artifacts[a.ID] = *a // in-memory keeps the full content
	// The on-disk JSON omits the body when it lives in a content file.
	stored := *a
	if stored.ContentFile != "" {
		stored.Content = ""
	}
	return atomicWriteJSON(d.dir(dirArtifacts, a.ID+".json"), stored)
}

// CreateArtifact inserts a new artifact and returns the stored row.
func (d *DB) CreateArtifact(ctx context.Context, a Artifact) (Artifact, error) {
	a.ID = d.nextID(idArtifact)
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.Kind == "" {
		a.Kind = ArtifactText
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return a, d.persistArtifactLocked(&a)
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
	return a, d.persistArtifactLocked(&a)
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
	return a, d.persistArtifactLocked(&a)
}

// SaveFileArtifact upserts an artifact mirroring a file the agent wrote: if one
// already exists for the same session + source path it is overwritten in place,
// otherwise a new one is created. This dedups repeated writes of the same file
// within a session so the Artifacts screen shows the latest content once.
func (d *DB) SaveFileArtifact(ctx context.Context, sessionID, agentID, sourcePath, title, kind, language, content, origin string) (Artifact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, a := range d.artifacts {
		if a.SessionID == sessionID && a.SourcePath != "" && a.SourcePath == sourcePath {
			a.Content = content
			a.Title = title
			a.Kind = kind
			a.Language = language
			if origin != "" {
				a.Origin = origin
			}
			a.UpdatedAt = now()
			return a, d.persistArtifactLocked(&a)
		}
	}
	a := Artifact{
		ID:         d.nextID(idArtifact),
		SessionID:  sessionID,
		AgentID:    agentID,
		Title:      title,
		Kind:       kind,
		Language:   language,
		Content:    content,
		SourcePath: sourcePath,
		Origin:     origin,
		CreatedAt:  now(),
		UpdatedAt:  now(),
	}
	return a, d.persistArtifactLocked(&a)
}

// UpsertAttachmentArtifact records a chat attachment as an artifact so every file
// added to a session appears in the Artifacts screen, tagged with its origin
// session. The uploaded file IS the artifact's file: media kinds reference it via
// SourcePath; text kinds also use it as the ContentFile (no duplicate written).
// Deduped by session + relPath — re-sending the same file is a no-op.
func (d *DB) UpsertAttachmentArtifact(ctx context.Context, sessionID, agentID, relPath, name, kind string) (Artifact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.upsertAttachmentArtifactLocked(sessionID, agentID, relPath, name, kind)
}

// upsertAttachmentArtifactLocked is the lock-free core of UpsertAttachmentArtifact
// (caller holds d.mu, or runs single-threaded during Open).
func (d *DB) upsertAttachmentArtifactLocked(sessionID, agentID, relPath, name, kind string) (Artifact, error) {
	for _, a := range d.artifacts {
		if a.SessionID == sessionID && a.SourcePath != "" && a.SourcePath == relPath {
			return a, nil // already captured
		}
	}
	a := Artifact{
		ID:         d.nextID(idArtifact),
		SessionID:  sessionID,
		AgentID:    agentID,
		Title:      name,
		Kind:       kind,
		SourcePath: relPath,
		Origin:     "chat",
		CreatedAt:  now(),
		UpdatedAt:  now(),
	}
	// persistArtifactLocked uses the uploaded file (SourcePath) as the content
	// file for text kinds and renders media from SourcePath — no duplicate write.
	return a, d.persistArtifactLocked(&a)
}

// DeleteArtifact removes an artifact.
func (d *DB) DeleteArtifact(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.artifacts[id]; !ok {
		return ErrNotFound
	}
	a := d.artifacts[id]
	delete(d.artifacts, id)
	d.removeArtifactContent(a) // drop the externalised body file (text kinds)
	return removeFile(d.dir(dirArtifacts, id+".json"))
}
