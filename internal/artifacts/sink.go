// Package artifacts provides the persistence shared by chat and autonomous
// artifact sinks without depending on their runtime adapters.
package artifacts

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Sink binds artifact writes and notifications to one session and agent.
type Sink struct {
	db        *db.DB
	sessionID string
	agentID   string
	emit      func(events.Event)
}

// NewSink creates a DB-backed sink. A nil emitter disables notifications.
func NewSink(database *db.DB, sessionID, agentID string, emit func(events.Event)) *Sink {
	return &Sink{db: database, sessionID: sessionID, agentID: agentID, emit: emit}
}

func (s *Sink) CreateArtifact(ctx context.Context, spec tools.CreateArtifactSpec) (tools.ArtifactRef, error) {
	row := db.Artifact{
		SessionID: s.sessionID,
		AgentID:   s.agentID,
		Title:     spec.Title,
		Kind:      spec.Kind,
		Language:  spec.Language,
		Content:   spec.Content,
		Origin:    "tool",
	}
	if spec.SourcePath != "" {
		rel, err := s.db.ImportMediaSource(s.sessionID, s.sourcePath(ctx, spec.SourcePath))
		if err != nil {
			return tools.ArtifactRef{}, err
		}
		row.SourcePath = rel
	}
	a, err := s.db.CreateArtifact(ctx, row)
	return s.result(a, "oluşturuldu", err)
}

func (s *Sink) UpdateArtifact(ctx context.Context, id, content string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactContent(ctx, id, content)
	return s.result(a, "güncellendi", err)
}

func (s *Sink) UpdateArtifactSource(ctx context.Context, id, path string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactFromSource(ctx, id, s.sessionID, path)
	return s.result(a, "updated", err)
}

func (s *Sink) result(a db.Artifact, verb string, err error) (tools.ArtifactRef, error) {
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	if s.emit != nil {
		s.emit(events.Event{
			Type:   events.TypeArtifact,
			Level:  "info",
			Title:  "Artifact " + verb + ": " + a.Title,
			Body:   a.Kind,
			Target: map[string]string{"view": "artifacts", "artifactId": a.ID, "sessionId": s.sessionID},
		})
	}
	return tools.ArtifactRef{ID: a.ID, Title: a.Title, Kind: a.Kind}, nil
}
