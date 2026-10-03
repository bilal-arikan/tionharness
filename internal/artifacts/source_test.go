package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestSinkMediaSourceOwnershipAndFailures(t *testing.T) {
	database := openSinkDB(t)
	ctx := context.Background()
	workingDir := t.TempDir()
	path := filepath.Join(workingDir, "report.bin")
	if err := os.WriteFile(path, []byte("first revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := database.CreateSession(ctx, db.Session{WorkingDir: workingDir, AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	var notifications []events.Event
	sink := NewSink(database, session.ID, "agent", func(event events.Event) {
		notifications = append(notifications, event)
	})
	ref, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{Title: "Report", Kind: "file", SourcePath: "report.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertCopy := func(want string) {
		t.Helper()
		row, err := database.GetArtifact(ctx, ref.ID)
		if err != nil {
			t.Fatal(err)
		}
		if row.SourcePath == "" || filepath.IsAbs(row.SourcePath) || row.SessionID != session.ID || row.AgentID != "agent" {
			t.Fatalf("source row = %+v", row)
		}
		body, err := os.ReadFile(filepath.Join(filepath.Dir(database.Root()), "workspace", filepath.FromSlash(row.SourcePath)))
		if err != nil || string(body) != want {
			t.Fatalf("source contents = %q, %v; want %q", body, err, want)
		}
	}
	assertCopy("first revision")
	if updated, err := sink.UpdateArtifactSource(ctx, ref.ID, "report.bin"); err != nil || updated != ref {
		t.Fatalf("source update = %+v, %v", updated, err)
	}
	assertCopy("second revision")
	if len(notifications) != 2 || notifications[1].Title != "Artifact updated: Report" {
		t.Fatalf("source notifications = %+v", notifications)
	}
	if _, err := sink.UpdateArtifactSource(ctx, ref.ID, "missing.bin"); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{SourcePath: "missing.bin"}); err == nil {
		t.Fatal("missing create source accepted")
	}
	foreignSink := NewSink(database, "other-session", "other-agent", func(events.Event) { t.Fatal("foreign source update notified") })
	if _, err := foreignSink.UpdateArtifactSource(ctx, ref.ID, path); err == nil {
		t.Fatal("source update accepted for another session's artifact")
	}
	assertCopy("second revision")
	rows, err := database.ListArtifacts(ctx, session.ID)
	if err != nil || len(rows) != 1 || len(notifications) != 2 {
		t.Fatalf("failed source writes changed artifacts/events: %v, %d rows, %d events", err, len(rows), len(notifications))
	}
}
