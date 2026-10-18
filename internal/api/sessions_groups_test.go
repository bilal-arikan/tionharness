package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestSessionGroupClassification(t *testing.T) {
	cases := []struct {
		name string
		s    db.Session
		want string
	}{
		{"plain user chat", db.Session{Kind: "chat"}, sessionGroupMain},
		{"legacy empty kind", db.Session{}, sessionGroupMain},
		{"spawn_session child", db.Session{Kind: "chat", Category: db.CategoryChat, ExecutionType: db.ExecutionInteractive,
			Origin: &db.SessionOrigin{Kind: db.OriginSpawn, TriggerSessionID: "SES1"}}, sessionGroupMain},
		{"legacy spawned kind", db.Session{Kind: "spawned"}, sessionGroupMain},
		{"handoff continuation", db.Session{Kind: "spawned", ParentSessionID: "SES1"}, sessionGroupMain},
		{"subagent", db.Session{Kind: "subagent", Category: db.CategorySubagent, ExecutionType: db.ExecutionSubagent,
			Visibility: db.VisibilityInternal}, sessionGroupOther},
		{"task", db.Session{Kind: "task"}, sessionGroupOther},
		{"schedule thread", db.Session{Kind: "schedule"}, sessionGroupOther},
		{"automation run", db.Session{Kind: "automation-run"}, sessionGroupOther},
		{"insight scan", db.Session{Kind: "insight"}, sessionGroupOther},
		{"system work", db.Session{Kind: "chat", Category: db.CategorySystem}, sessionGroupOther},
		{"system agent call", db.Session{Kind: "chat", Tags: []string{"system:titler"}}, sessionGroupOther},
		{"task stamped chat category", db.Session{Kind: "task", Category: db.CategoryChat, ExecutionType: db.ExecutionInteractive}, sessionGroupOther},
		{"automation-origin chat", db.Session{Kind: "chat",
			Origin: &db.SessionOrigin{Kind: db.OriginAutomation}}, sessionGroupOther},
	}
	for _, c := range cases {
		if got := sessionGroup(c.s); got != c.want {
			t.Errorf("%s: group = %q, want %q", c.name, got, c.want)
		}
	}
}

// The tab narrows the list BEFORE paging (like the chips) and the response
// carries both tabs' totals under the current chips.
func TestListSessionsGroupFilter(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	for _, s := range []db.Session{
		{Title: "chat a"},
		{Title: "chat b"},
		{Title: "spawned chat", Kind: "chat", Origin: &db.SessionOrigin{Kind: db.OriginSpawn, TriggerSessionID: "SES1"}},
		{Title: "subagent", Kind: "subagent", Category: db.CategorySubagent, ExecutionType: db.ExecutionSubagent},
		{Title: "task", Kind: "task"},
	} {
		if _, err := wsp.DB.CreateSession(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	all := "chips=chat,task,spawned,subagent,automation,insight,other,running"

	main := listSessionsPage(t, server, wsp.DB, all+"&group=main&limit=50")
	if main.Total != 3 {
		t.Fatalf("group=main total = %d, want 3", main.Total)
	}
	if main.GroupCounts["main"] != 3 || main.GroupCounts["other"] != 2 {
		t.Fatalf("groupCounts = %v, want main 3 / other 2", main.GroupCounts)
	}
	if got := main.ChipCounts[chipSubagent]; got != 0 {
		t.Fatalf("main tab chipCounts[subagent] = %d, want 0 (chip badges describe the tab)", got)
	}

	other := listSessionsPage(t, server, wsp.DB, all+"&group=other&limit=50")
	if other.Total != 2 {
		t.Fatalf("group=other total = %d, want 2", other.Total)
	}

	// Unticking a chip shrinks the tab badge too.
	noSubagent := listSessionsPage(t, server, wsp.DB, "chips=chat,task,spawned&group=main&limit=50")
	if noSubagent.GroupCounts["other"] != 1 {
		t.Fatalf("groupCounts[other] without subagent chip = %d, want 1", noSubagent.GroupCounts["other"])
	}

	if got := listSessionsPage(t, server, wsp.DB, "limit=50").GroupCounts; got != nil {
		t.Fatalf("groupCounts without group = %v, want absent", got)
	}
}
