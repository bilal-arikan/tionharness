package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestChatArtifactSinkRetainsRollingPlanCapability(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	var notifications []events.Event
	var generic tools.ArtifactSink = newArtifactSink(database, "session", "agent", func(event events.Event) {
		if _, err := database.GetArtifact(ctx, event.Target["artifactId"]); err != nil {
			t.Fatalf("plan notification preceded persistence: %v", err)
		}
		notifications = append(notifications, event)
	})
	appender, ok := generic.(interface {
		AppendPlanArtifact(context.Context, string) (tools.ArtifactRef, error)
	})
	if !ok {
		t.Fatal("chat sink lost its plan-approval capability")
	}
	first, err := appender.AppendPlanArtifact(ctx, "First plan")
	if err != nil {
		t.Fatal(err)
	}
	second, err := appender.AppendPlanArtifact(ctx, "Second plan")
	if err != nil || first != second {
		t.Fatalf("rolling plan reference = %+v, %v; first = %+v", second, err, first)
	}
	row, err := database.GetArtifact(ctx, first.ID)
	if err != nil || row.SessionID != "session" || row.AgentID != "agent" || row.Origin != "plan" || !strings.Contains(row.Content, "First plan") || !strings.Contains(row.Content, "Second plan") {
		t.Fatalf("rolling plan = %+v, %v", row, err)
	}
	if len(notifications) != 2 || notifications[0].Target["artifactId"] != first.ID || notifications[1].Title != "Artifact güncellendi: "+row.Title {
		t.Fatalf("plan notifications = %+v", notifications)
	}
}
