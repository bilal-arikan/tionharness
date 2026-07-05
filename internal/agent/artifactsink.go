package agent

import (
	"context"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// artifactSink is a DB-backed tools.ArtifactSink that also publishes an artifact
// change event, so artifacts created on AUTONOMOUS turns (scheduler / spawn /
// flow — which carry no SSE chat client) still persist AND notify the UI. It
// mirrors the chat-path sink in the api package; both stamp origin session +
// agent and emit the same "artifact" event.
type artifactSink struct {
	publish   func(events.Event)
	db        *db.DB
	sessionID string
	agentID   string
}

// NewArtifactSink builds an artifact sink bound to a session + agent for this
// workspace. Used to make artifacts available on autonomous turns (the native
// loop installs it as a fallback; the CLI Interaction bridge sets it on the run).
func (r *Runtime) NewArtifactSink(sessionID, agentID string) tools.ArtifactSink {
	return &artifactSink{publish: r.publish, db: r.db, sessionID: sessionID, agentID: agentID}
}

func (s *artifactSink) notify(a db.Artifact, verb string) {
	if s.publish == nil {
		return
	}
	s.publish(events.Event{
		Type:   "artifact",
		Level:  "info",
		Title:  "Artifact " + verb + ": " + a.Title,
		Body:   a.Kind,
		Target: map[string]string{"view": "artifacts", "artifactId": a.ID, "sessionId": s.sessionID},
	})
}

func (s *artifactSink) CreateArtifact(ctx context.Context, spec tools.CreateArtifactSpec) (tools.ArtifactRef, error) {
	row := db.Artifact{
		SessionID: s.sessionID,
		AgentID:   s.agentID,
		Title:     spec.Title,
		Kind:      spec.Kind,
		Language:  spec.Language,
		Content:   spec.Content,
		Origin:    "tool",
	}
	// Media/file kinds carry a file path, not inline bytes: resolve to a
	// workspace-relative path (copying the file in if it lives outside).
	if spec.SourcePath != "" {
		rel, err := s.db.ImportMediaSource(s.sessionID, spec.SourcePath)
		if err != nil {
			return tools.ArtifactRef{}, err
		}
		row.SourcePath = rel
	}
	a, err := s.db.CreateArtifact(ctx, row)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	s.notify(a, "oluşturuldu")
	return tools.ArtifactRef{ID: a.ID, Title: a.Title, Kind: a.Kind}, nil
}

func (s *artifactSink) UpdateArtifact(ctx context.Context, id, content string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactContent(ctx, id, content)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	s.notify(a, "güncellendi")
	return tools.ArtifactRef{ID: a.ID, Title: a.Title, Kind: a.Kind}, nil
}
