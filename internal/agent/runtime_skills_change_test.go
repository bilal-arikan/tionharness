package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

func TestSkillCatalogChangePublishesControlEvent(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	bus := events.NewBus()
	rt.bus = bus
	rt.wsID = "WS1"
	rt.wsName = "Test Workspace"
	id, ch := bus.Subscribe()
	defer bus.Unsubscribe(id)

	if _, err := rt.Skills().Create("event-skill", skills.SkillInput{
		Name: "Event Skill",
		Body: "# Event Skill",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got := drainEvents(ch)
	if len(got) != 1 {
		t.Fatalf("want exactly one skills event, got %d: %+v", len(got), got)
	}
	if got[0].Type != events.TypeSkills || got[0].WorkspaceID != "WS1" || got[0].Target["view"] != "skills" {
		t.Fatalf("bad skills event: %+v", got[0])
	}
	if events.IsNotifyKind(got[0].Type) {
		t.Fatalf("skills event must remain a non-toast control event")
	}

	rt.Skills().Reload()
	if again := drainEvents(ch); len(again) != 0 {
		t.Fatalf("unchanged reload published %d events: %+v", len(again), again)
	}
}
