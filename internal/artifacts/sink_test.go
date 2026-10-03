package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func openSinkDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSinkPersistsBeforeNotifying(t *testing.T) {
	database := openSinkDB(t)
	ctx := context.Background()
	var notifications []events.Event
	var contents []string
	sink := NewSink(database, "child-session", "child-agent", func(event events.Event) {
		row, err := database.GetArtifact(ctx, event.Target["artifactId"])
		if err != nil {
			t.Fatalf("notification preceded persistence: %v", err)
		}
		if row.SessionID != "child-session" || row.AgentID != "child-agent" || row.Origin != "tool" {
			t.Fatalf("artifact provenance = %+v", row)
		}
		notifications = append(notifications, event)
		contents = append(contents, row.Content)
	})
	ref, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{
		Title: "Report", Kind: "markdown", Language: "markdown", Content: "first revision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID == "" || ref.Title != "Report" || ref.Kind != "markdown" {
		t.Fatalf("reference = %+v", ref)
	}
	updated, err := sink.UpdateArtifact(ctx, ref.ID, "second revision")
	if err != nil || updated != ref {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if !reflect.DeepEqual(contents, []string{"first revision", "second revision"}) {
		t.Fatalf("contents observed during notifications = %v", contents)
	}
	for i, verb := range []string{"oluşturuldu", "güncellendi"} {
		event := notifications[i]
		if event.Type != events.TypeArtifact || event.Level != "info" || event.Title != "Artifact "+verb+": Report" || event.Body != "markdown" {
			t.Fatalf("notification = %+v", event)
		}
		want := map[string]string{"view": "artifacts", "artifactId": ref.ID, "sessionId": "child-session"}
		if !reflect.DeepEqual(event.Target, want) {
			t.Fatalf("notification target = %v", event.Target)
		}
	}
	if _, err := sink.UpdateArtifact(ctx, "missing", "body"); err == nil || len(notifications) != 2 {
		t.Fatalf("failed update must not notify: %v, %d notifications", err, len(notifications))
	}
}

func TestSinkWriteFailureDoesNotNotify(t *testing.T) {
	database := openSinkDB(t)
	storeArtifacts := filepath.Join(database.Root(), "artifacts")
	if err := os.RemoveAll(storeArtifacts); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storeArtifacts, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink := NewSink(database, "session", "agent", func(events.Event) {
		t.Fatal("a failed database write emitted a notification")
	})
	ref, err := sink.CreateArtifact(context.Background(), tools.CreateArtifactSpec{Title: "Image", Kind: "image"})
	if err == nil || ref != (tools.ArtifactRef{}) {
		t.Fatalf("failed create = %+v, %v", ref, err)
	}
}
